package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spcent/golang_simple_server/pkg/core"
	ws "github.com/spcent/golang_simple_server/pkg/net/websocket"
)

func TestIntegrationTLSWebSocketAndGracefulShutdown(t *testing.T) {
	addr := findFreeAddr(t)
	tempDir := t.TempDir()

	secret := "integration-secret"
	envPath := writeEnvFile(t, tempDir, secret)
	certPath, keyPath := writeSelfSignedCert(t, tempDir)

	app := core.New(
		core.WithAddr(addr),
		core.WithEnvPath(envPath),
		core.WithTLS(certPath, keyPath),
		core.WithShutdownTimeout(2*time.Second),
	)

	app.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("pong"))
	})

	if _, err := app.ConfigureWebSocket(); err != nil {
		t.Fatalf("configure websocket: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Boot()
	}()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
		},
	}
	baseURL := "https://" + addr
	waitForResponse(t, client, baseURL+"/ping", "pong")

	wsURL := "wss://" + addr + "/ws"
	wsClient1 := newTLSWSClient(t, wsURL)
	defer wsClient1.close()
	wsClient2 := newTLSWSClient(t, wsURL)
	defer wsClient2.close()

	// Allow both WebSocket connections to finish registering with the hub
	time.Sleep(50 * time.Millisecond)

	message := []byte("hello-room")
	if err := wsClient1.sendFrame(ws.OpcodeText, true, message); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	received := readNextDataFrame(t, wsClient2, 5*time.Second)
	if string(received) != string(message) {
		t.Fatalf("unexpected payload: %s", string(received))
	}

	// Trigger graceful shutdown path and ensure Boot returns.
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("boot error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for graceful shutdown")
	}
}

func findFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func writeEnvFile(t *testing.T, dir, secret string) string {
	t.Helper()
	envPath := dir + "/integration.env"
	if err := os.WriteFile(envPath, []byte("WS_SECRET="+secret+"\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	return envPath
}

func writeSelfSignedCert(t *testing.T, dir string) (string, string) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certOut := dir + "/cert.pem"
	if err := os.WriteFile(certOut, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyOut := dir + "/key.pem"
	if err := os.WriteFile(keyOut, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certOut, keyOut
}

func waitForResponse(t *testing.T, client *http.Client, url, expected string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lastErr error

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && string(body) == expected {
				return
			}
			lastErr = fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body))
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			t.Fatalf("server not ready: %v", lastErr)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type testWSClient struct {
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
}

const (
	opcodePing = 0x9
	opcodePong = 0xA
)

func newTLSWSClient(t *testing.T, rawURL string) *testWSClient {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	addr := parsed.Host
	if parsed.Port() == "" {
		addr = parsed.Host + ":443"
	}

	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // nolint:gosec
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	keyBytes := make([]byte, 16)
	if _, err = rand.Read(keyBytes); err != nil {
		t.Fatalf("key: %v", err)
	}

	key := base64.StdEncoding.EncodeToString(keyBytes)
	req := "GET " + parsed.RequestURI() + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"\r\n"

	bw := bufio.NewWriter(conn)
	if _, err := bw.WriteString(req); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("handshake status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("handshake failed: %s", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("handshake header: %v", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	return &testWSClient{conn: conn, br: br, bw: bufio.NewWriter(conn)}
}

func (c *testWSClient) sendFrame(op byte, fin bool, payload []byte) error {
	maskKey := make([]byte, 4)
	if _, err := rand.Read(maskKey); err != nil {
		return err
	}

	var header [14]byte
	hlen := 0
	b0 := byte(0)
	if fin {
		b0 |= 0x80
	}
	b0 |= op & 0x0F
	header[0] = b0
	hlen = 1
	n := len(payload)
	switch {
	case n <= 125:
		header[hlen] = byte(n) | 0x80
		hlen++
	case n < 65536:
		header[hlen] = 126 | 0x80
		hlen++
		header[hlen] = byte(n >> 8)
		header[hlen+1] = byte(n)
		hlen += 2
	default:
		header[hlen] = 127 | 0x80
		hlen++
		for i := 0; i < 8; i++ {
			header[hlen+i] = byte(n >> (56 - 8*i))
		}
		hlen += 8
	}
	copy(header[hlen:], maskKey)
	hlen += 4

	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}

	if _, err := c.bw.Write(header[:hlen]); err != nil {
		return err
	}
	if _, err := c.bw.Write(masked); err != nil {
		return err
	}
	return c.bw.Flush()
}

func (c *testWSClient) readFrame() (byte, []byte, error) {
	var h [2]byte
	if _, err := c.br.Read(h[:]); err != nil {
		return 0, nil, err
	}
	fin := h[0]&0x80 != 0
	op := h[0] & 0x0F
	_ = fin
	masked := h[1]&0x80 != 0
	payloadLen := int(h[1] & 0x7F)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := c.br.Read(ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int(ext[0])<<8 | int(ext[1])
	case 127:
		var ext [8]byte
		if _, err := c.br.Read(ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int(ext[7]) | int(ext[6])<<8 | int(ext[5])<<16 | int(ext[4])<<24
	}

	var maskKey [4]byte
	if masked {
		if _, err := c.br.Read(maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := c.br.Read(payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}
	return op, payload, nil
}

func (c *testWSClient) close() {
	_ = c.conn.Close()
}

func readNextDataFrame(t *testing.T, client *testWSClient, timeout time.Duration) []byte {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for data frame")
		}

		op, payload, err := client.readFrame()
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}

		switch op {
		case ws.OpcodeText, ws.OpcodeBinary:
			return payload
		case opcodePing, opcodePong:
			continue
		default:
			t.Fatalf("unexpected opcode: %d", op)
		}
	}
}
