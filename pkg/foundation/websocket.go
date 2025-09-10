package foundation

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	guid                     = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	opcodeContinuation byte  = 0x0
	opcodeText         byte  = 0x1
	opcodeBinary       byte  = 0x2
	opcodeClose        byte  = 0x8
	opcodePing         byte  = 0x9
	opcodePong         byte  = 0xA
	finBit             byte  = 0x80
	maxControlPayload  int64 = 125
	defaultBufSize     int   = 4096
	defaultPingPeriod        = 20 * time.Second
	defaultPongWait          = 30 * time.Second
	// Fragmentation settings
	maxFragmentSize = 64 * 1024 // 64KB fragments for sending
)

// Message represents a websocket message
type Message struct {
	Op   byte   // opcodeText or opcodeBinary
	Data []byte // payload
}

// Conn is a single websocket connection wrapper with ReadMessage / WriteMessage APIs
type Conn struct {
	conn      net.Conn
	br        *bufio.Reader
	bw        *bufio.Writer
	sendCh    chan []byte // serialized text frames (we'll use for WriteMessage after framing)
	closeOnce sync.Once
	closed    int32
	closeC    chan struct{}

	readLimit  int64
	pingPeriod time.Duration
	pongWait   time.Duration

	// optional per-connection locks
	writeMu sync.Mutex
}

// NewConn wraps a net.Conn after handshake
func NewConn(c net.Conn) *Conn {
	return &Conn{
		conn:       c,
		br:         bufio.NewReaderSize(c, 8192),
		bw:         bufio.NewWriterSize(c, 8192),
		sendCh:     make(chan []byte, 512),
		closeC:     make(chan struct{}),
		readLimit:  4 << 20, // 4 MiB default per message
		pingPeriod: defaultPingPeriod,
		pongWait:   defaultPongWait,
	}
}

func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)
		close(c.closeC)
		err = c.conn.Close()
	})
	return err
}

func (c *Conn) IsClosed() bool { return atomic.LoadInt32(&c.closed) == 1 }

func (c *Conn) SetReadLimit(n int64) { c.readLimit = n }

// --------- Low level frame read/write (handles masking, lengths) ----------

// readFrame reads a single frame (one WebSocket frame, might be control or data fragment).
// It returns: opcode, fin (bool), payload bytes, error.
func (c *Conn) readFrame() (byte, bool, []byte, error) {
	// read 2 first bytes
	var h [2]byte
	if _, err := io.ReadFull(c.br, h[:]); err != nil {
		return 0, false, nil, err
	}
	fin := h[0]&finBit != 0
	op := h[0] & 0x0F
	mask := h[1]&0x80 != 0
	len7 := int64(h[1] & 0x7F)

	// per RFC, client->server frames MUST be masked
	if !mask {
		return 0, false, nil, errors.New("protocol error: unmasked client frame")
	}

	var payloadLen int64
	switch len7 {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, false, nil, err
		}
		payloadLen = int64(uint16(ext[0])<<8 | uint16(ext[1]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, false, nil, err
		}
		payloadLen = int64(uint64(ext[0])<<56 |
			uint64(ext[1])<<48 |
			uint64(ext[2])<<40 |
			uint64(ext[3])<<32 |
			uint64(ext[4])<<24 |
			uint64(ext[5])<<16 |
			uint64(ext[6])<<8 |
			uint64(ext[7]))
	default:
		payloadLen = len7
	}

	if payloadLen > c.readLimit {
		return 0, false, nil, errors.New("payload too large")
	}

	var maskKey [4]byte
	if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
		return 0, false, nil, err
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return 0, false, nil, err
		}
		// unmask
		for i := int64(0); i < payloadLen; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	// control frame rules: control frames must not be fragmented and payload <=125
	if op >= 0x8 {
		if !fin {
			return 0, false, nil, errors.New("protocol error: fragmented control frame")
		}
		if int64(len(payload)) > maxControlPayload {
			return 0, false, nil, errors.New("protocol error: control frame too large")
		}
	}

	return op, fin, payload, nil
}

// writeFrame writes a single frame to the client (server->client frames are NOT masked)
func (c *Conn) writeFrame(op byte, fin bool, payload []byte) error {
	// ensure single writer at a time
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// header
	var header [14]byte
	hlen := 0
	b0 := byte(0)
	if fin {
		b0 |= finBit
	}
	b0 |= op & 0x0F
	header[0] = b0
	hlen = 1

	n := len(payload)
	switch {
	case n <= 125:
		header[hlen] = byte(n)
		hlen++
	case n <= 0xFFFF:
		header[hlen] = 126
		hlen++
		header[hlen] = byte(n >> 8)
		header[hlen+1] = byte(n)
		hlen += 2
	default:
		header[hlen] = 127
		hlen++
		// 8 bytes big-endian
		header[hlen+0] = byte(uint64(n) >> 56)
		header[hlen+1] = byte(uint64(n) >> 48)
		header[hlen+2] = byte(uint64(n) >> 40)
		header[hlen+3] = byte(uint64(n) >> 32)
		header[hlen+4] = byte(uint64(n) >> 24)
		header[hlen+5] = byte(uint64(n) >> 16)
		header[hlen+6] = byte(uint64(n) >> 8)
		header[hlen+7] = byte(uint64(n))
		hlen += 8
	}

	if _, err := c.bw.Write(header[:hlen]); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.bw.Write(payload); err != nil {
			return err
		}
	}
	return c.bw.Flush()
}

// ----------- High level ReadMessage (handles fragmentation) ----------------

// ReadMessage blocks and returns the next complete message (text or binary).
// It handles fragmented messages by assembling continuation frames.
func (c *Conn) ReadMessage() (Message, error) {
	var msg Message
	var assembling bool
	var opcode byte
	var buf []byte

	// track lastPong for keepalive
	lastPong := time.Now()
	// launch a small goroutine to monitor timeouts if desired (skipped here for brevity)

	for {
		op, fin, payload, err := c.readFrame()
		if err != nil {
			return msg, err
		}

		switch op {
		case opcodeText, opcodeBinary:
			if assembling {
				// shouldn't get a new data opcode while assembling (protocol error)
				return msg, errors.New("protocol error: new data frame while assembling")
			}
			if fin {
				// single-frame message
				return Message{Op: op, Data: payload}, nil
			}
			// start assembling
			assembling = true
			opcode = op
			buf = append(buf[:0], payload...)
		case opcodeContinuation:
			if !assembling {
				return msg, errors.New("protocol error: continuation with no started message")
			}
			buf = append(buf, payload...)
			if fin {
				// finished
				out := make([]byte, len(buf))
				copy(out, buf)
				return Message{Op: opcode, Data: out}, nil
			}
		case opcodePing:
			// respond with pong carrying same payload
			_ = c.writeFrame(opcodePong, true, payload)
		case opcodePong:
			// update pong time - could be used externally
			lastPong = time.Now()
			_ = lastPong // placeholder if you want to expose last pong
		case opcodeClose:
			// echo close and return error/EOF
			_ = c.writeFrame(opcodeClose, true, payload)
			return msg, io.EOF
		default:
			// ignore other opcodes
		}
	}
}

// WriteMessage writes a message to the connection. It will fragment messages larger than
// maxFragmentSize into continuation frames. It blocks until written or encounter error.
func (c *Conn) WriteMessage(op byte, data []byte) error {
	if c.IsClosed() {
		return errors.New("connection closed")
	}
	// If small enough, send as single frame
	if len(data) <= maxFragmentSize {
		return c.writeFrame(op, true, data)
	}

	// fragment
	total := len(data)
	offset := 0
	first := true
	for offset < total {
		end := offset + maxFragmentSize
		if end > total {
			end = total
		}
		chunk := data[offset:end]
		fin := end == total
		var frameOp byte
		if first {
			frameOp = op // first frame uses actual opcode
			first = false
		} else {
			frameOp = opcodeContinuation
		}
		if err := c.writeFrame(frameOp, fin, chunk); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

// ----------- Hub with rooms/topics model ----------------

// Hub manages multiple rooms (topics). Each room is a set of *Conn
type Hub struct {
	rooms map[string]map[*Conn]struct{}
	mu    sync.RWMutex
	// optional backpressure / broadcasting channel can be added
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[*Conn]struct{}),
	}
}

// Join adds conn to the named room
func (h *Hub) Join(room string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rs, ok := h.rooms[room]
	if !ok {
		rs = make(map[*Conn]struct{})
		h.rooms[room] = rs
	}
	rs[c] = struct{}{}
}

// Leave removes conn from room
func (h *Hub) Leave(room string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if rs, ok := h.rooms[room]; ok {
		delete(rs, c)
		if len(rs) == 0 {
			delete(h.rooms, room)
		}
	}
}

// BroadcastRoom sends to all connections in room (non-blocking to each conn's WriteMessage).
// This method writes synchronously to each conn — consider writing asynchronously if many conns.
func (h *Hub) BroadcastRoom(room string, op byte, data []byte) {
	h.mu.RLock()
	rs, ok := h.rooms[room]
	h.mu.RUnlock()
	if !ok {
		return
	}
	// copy conns to avoid holding lock during network ops
	conns := make([]*Conn, 0, len(rs))
	for c := range rs {
		conns = append(conns, c)
	}
	for _, c := range conns {
		go func(cc *Conn) {
			// non-blocking best-effort: attempt WriteMessage; if it fails, close conn
			if err := cc.WriteMessage(op, data); err != nil {
				cc.Close()
			}
		}(c)
	}
}

// BroadcastAll broadcasts to all rooms/clients
func (h *Hub) BroadcastAll(op byte, data []byte) {
	h.mu.RLock()
	rooms := make([]map[*Conn]struct{}, 0, len(h.rooms))
	for _, rs := range h.rooms {
		rooms = append(rooms, rs)
	}
	h.mu.RUnlock()
	seen := make(map[*Conn]struct{})
	for _, rs := range rooms {
		for c := range rs {
			if _, ok := seen[c]; ok {
				continue
			}
			seen[c] = struct{}{}
			go func(cc *Conn) {
				if err := cc.WriteMessage(op, data); err != nil {
					cc.Close()
				}
			}(c)
		}
	}
}

// RemoveConn removes conn from all rooms (call on close)
func (h *Hub) RemoveConn(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for room, rs := range h.rooms {
		if _, ok := rs[c]; ok {
			delete(rs, c)
			if len(rs) == 0 {
				delete(h.rooms, room)
			}
		}
	}
}

// ---------- ServeWS: handshake + wrap Conn + integrate with Hub ------------

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func headerContains(h http.Header, key, val string) bool {
	v := h.Get(key)
	if v == "" {
		return false
	}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		if strings.EqualFold(strings.TrimSpace(p), val) {
			return true
		}
	}
	return false
}

// ServeWS performs handshake and then calls onConn with a ready *Conn.
// onConn should manage registering conn into hub/rooms and starting read loops.
// The handshake writes the 101 response via Hijack.
func ServeWS(w http.ResponseWriter, r *http.Request, onConn func(*Conn)) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !headerContains(r.Header, "Connection", "Upgrade") || !headerContains(r.Header, "Upgrade", "websocket") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	accept := computeAcceptKey(key)
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n" +
		"\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return
	}

	c := NewConn(conn)
	// Use conn's br/bw backed by the hijacked conn
	c.br = bufio.NewReaderSize(conn, 8192)
	c.bw = bufio.NewWriterSize(conn, 8192)

	onConn(c)
}
