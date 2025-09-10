package foundation

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	guid                    = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	opcodeText        byte  = 0x1
	opcodeBinary      byte  = 0x2
	opcodeClose       byte  = 0x8
	opcodePing        byte  = 0x9
	opcodePong        byte  = 0xA
	finBit            byte  = 0x80
	maxControlPayload int64 = 125
	defaultBufSize    int   = 4096
	defaultPingPeriod       = 20 * time.Second
	defaultPongWait         = 30 * time.Second
)

// Conn represents a single websocket connection
type Conn struct {
	conn       net.Conn
	br         *bufio.Reader
	bw         *bufio.Writer
	sendCh     chan []byte
	closeOnce  sync.Once
	closed     int32
	closeC     chan struct{}
	readLimit  int64
	lastPong   int64 // unix nanos
	pingPeriod time.Duration
	pongWait   time.Duration
	pool       *sync.Pool
}

// NewConn wraps a net.Conn (after handshake) into our Conn
func NewConn(c net.Conn) *Conn {
	pool := &sync.Pool{
		New: func() any { b := make([]byte, 0, defaultBufSize); return &b },
	}
	return &Conn{
		conn:       c,
		br:         bufio.NewReaderSize(c, 8192),
		bw:         bufio.NewWriterSize(c, 8192),
		sendCh:     make(chan []byte, 256),
		closeC:     make(chan struct{}),
		readLimit:  1 << 20, // 1 MiB default per message (tunable)
		pingPeriod: defaultPingPeriod,
		pongWait:   defaultPongWait,
		pool:       pool,
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

// SetReadLimit sets max payload that will be accepted per message
func (c *Conn) SetReadLimit(n int64) { c.readLimit = n }

// SendText sends a text message (non-blocking attempt; if buffer full, it will drop)
func (c *Conn) SendText(b []byte) {
	if c.IsClosed() {
		return
	}
	// copy to avoid races
	cp := make([]byte, len(b))
	copy(cp, b)
	select {
	case c.sendCh <- cp:
	default:
		// drop if buffer full (backpressure strategy). You may block instead.
	}
}

// internal: write a frame to writer (server->client must not mask)
func (c *Conn) writeFrame(op byte, payload []byte) error {
	// single-frame write
	var header [14]byte
	hlen := 0
	header[0] = finBit | (op & 0x0F)
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

// readFrame reads one full frame from client and returns opcode and payload (unmasked)
func (c *Conn) readFrame() (byte, []byte, error) {
	// header: 2 bytes
	h := make([]byte, 2)
	if _, err := io.ReadFull(c.br, h); err != nil {
		return 0, nil, err
	}
	fin := h[0]&finBit != 0
	op := h[0] & 0x0F
	mask := h[1]&0x80 != 0
	len7 := int64(h[1] & 0x7F)

	if !mask {
		// per RFC, client->server frames must be masked
		// Close connection on protocol error
		return 0, nil, errors.New("unmasked client frame")
	}

	var payloadLen int64
	switch len7 {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(c.br, ext); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(uint16(ext[0])<<8 | uint16(ext[1]))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(c.br, ext); err != nil {
			return 0, nil, err
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
		return 0, nil, errors.New("payload too large")
	}

	// mask key
	maskKey := make([]byte, 4)
	if _, err := io.ReadFull(c.br, maskKey); err != nil {
		return 0, nil, err
	}

	// read payload
	b := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.br, b); err != nil {
			return 0, nil, err
		}
		// unmask
		for i := int64(0); i < payloadLen; i++ {
			b[i] ^= maskKey[i%4]
		}
	}
	if !fin {
		// for simplicity, we do not currently support fragmented messages in this example.
		// In production you should assemble fragments.
		return 0, nil, errors.New("fragmented frames not supported in this example")
	}
	return op, b, nil
}

// readPump reads frames and dispatches them
func (c *Conn) readPump(onMessage func(op byte, payload []byte), onClose func()) {
	defer func() {
		onClose()
		c.Close()
	}()
	// set initial last pong
	atomic.StoreInt64(&c.lastPong, time.Now().UnixNano())
	// start a goroutine to enforce pong timeout
	go func() {
		ticker := time.NewTicker(c.pingPeriod / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if c.IsClosed() {
					return
				}
				last := time.Unix(0, atomic.LoadInt64(&c.lastPong))
				if time.Since(last) > c.pongWait {
					// timeout
					c.Close()
					return
				}
			case <-c.closeC:
				return
			}
		}
	}()

	for {
		op, payload, err := c.readFrame()
		if err != nil {
			if !c.IsClosed() {
				// log but swallow EOF-like errors
				if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
					log.Println("readFrame error:", err)
				}
			}
			return
		}
		switch op {
		case opcodeText, opcodeBinary:
			onMessage(op, payload)
		case opcodePing:
			// respond with Pong carrying the same payload
			_ = c.writeFrame(opcodePong, payload)
		case opcodePong:
			atomic.StoreInt64(&c.lastPong, time.Now().UnixNano())
		case opcodeClose:
			// echo close and return
			_ = c.writeFrame(opcodeClose, payload)
			return
		default:
			// ignore other opcodes
		}
	}
}

// writePump consumes sendCh and writes frames; also sends periodic pings
func (c *Conn) writePump() {
	ticker := time.NewTicker(c.pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case <-c.closeC:
			return
		case msg, ok := <-c.sendCh:
			if !ok {
				// channel closed
				return
			}
			if err := c.writeFrame(opcodeText, msg); err != nil {
				return
			}
		case <-ticker.C:
			// send ping
			if err := c.writeFrame(opcodePing, []byte("ping")); err != nil {
				return
			}
		}
	}
}

// ServeWS performs handshake and registers conn to the hub callback
func ServeWS(w http.ResponseWriter, r *http.Request, onConn func(*Conn)) {
	// Only accept GET
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// basic header checks
	if !headerContains(r.Header, "Connection", "Upgrade") || !headerContains(r.Header, "Upgrade", "websocket") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// handshake response
	accept := computeAcceptKey(key)
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return
	}
	// write response headers manually
	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	// Build HTTP response
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
	// replace the buffered reader/writer to use the hijacked conn's buffers
	c.br = bufio.NewReaderSize(conn, 8192)
	c.bw = bufio.NewWriterSize(conn, 8192)

	// run pumps
	go c.writePump()
	go c.readPump(func(op byte, payload []byte) {
		// default behavior: echo text
		if op == opcodeText {
			// echo
			c.SendText(payload)
		}
	}, func() {
		// onClose - nothing here; hub might handle registration
	})
	onConn(c)
}

// computeAcceptKey RFC6455
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
	// split by comma and trim
	parts := strings.Split(v, ",")
	for _, p := range parts {
		if strings.EqualFold(strings.TrimSpace(p), val) {
			return true
		}
	}
	return false
}

// Hub is a simple broadcast hub for multiple clients
type Hub struct {
	clients   map[*Conn]struct{}
	register  chan *Conn
	unreg     chan *Conn
	broadcast chan []byte
	mu        sync.RWMutex
}

func NewHub() *Hub {
	h := &Hub{
		clients:   make(map[*Conn]struct{}),
		register:  make(chan *Conn),
		unreg:     make(chan *Conn),
		broadcast: make(chan []byte, 1024),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
		case c := <-h.unreg:
			h.mu.Lock()
			delete(h.clients, c)
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.RLock()
			for cl := range h.clients {
				cl.SendText(msg)
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Register(c *Conn)   { h.register <- c }
func (h *Hub) Unregister(c *Conn) { h.unreg <- c }
func (h *Hub) Broadcast(b []byte) { h.broadcast <- b }
