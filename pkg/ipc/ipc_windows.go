//go:build windows
// +build windows

package ipc

import (
	"errors"
	"net"
	"time"
)

// winServer implements Server for Windows (TCP fallback).
type winServer struct {
	listener net.Listener
	addr     string
}

// winClient implements Client for Windows.
type winClient struct {
	conn net.Conn
}

// NewServer creates a server using TCP loopback on Windows.
func NewServer(addr string) (Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &winServer{listener: ln, addr: ln.Addr().String()}, nil
}

func (s *winServer) Accept() (Client, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, err
	}
	return &winClient{conn: conn}, nil
}

func (s *winServer) Addr() string {
	return s.addr
}

func (s *winServer) Close() error {
	return s.listener.Close()
}

// Dial connects to a server using TCP (always on Windows).
func Dial(addr string, _ bool) (Client, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &winClient{conn: conn}, nil
}

func (c *winClient) Write(data []byte) (int, error) {
	if c.conn == nil {
		return 0, errors.New("connection is nil")
	}
	return c.conn.Write(data)
}

func (c *winClient) Read(buf []byte) (int, error) {
	if c.conn == nil {
		return 0, errors.New("connection is nil")
	}
	return c.conn.Read(buf)
}

func (c *winClient) Close() error {
	if c.conn == nil {
		return errors.New("connection is nil")
	}
	return c.conn.Close()
}
