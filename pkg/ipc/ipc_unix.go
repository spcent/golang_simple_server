//go:build !windows
// +build !windows

package ipc

import (
	"net"
	"os"
)

// unixServer implements Server for Unix-like systems.
type unixServer struct {
	listener net.Listener
}

// unixClient implements Client for Unix-like systems.
type unixClient struct {
	conn net.Conn
}

// NewServer creates a Unix Domain Socket server, fallback to TCP.
func NewServer(addr string) (Server, error) {
	_ = os.Remove(addr) // remove stale socket file

	ln, err := net.Listen("unix", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
	}
	return &unixServer{listener: ln}, nil
}

func (s *unixServer) Accept() (Client, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, err
	}
	return &unixClient{conn: conn}, nil
}

func (s *unixServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *unixServer) Close() error {
	return s.listener.Close()
}

// Dial connects to a server using UDS, fallback to TCP.
func Dial(addr string, useTCP bool) (Client, error) {
	var conn net.Conn
	var err error

	if useTCP {
		conn, err = net.Dial("tcp", addr)
	} else {
		conn, err = net.Dial("unix", addr)
		if err != nil {
			conn, err = net.Dial("tcp", addr)
		}
	}
	if err != nil {
		return nil, err
	}
	return &unixClient{conn: conn}, nil
}

func (c *unixClient) Write(data []byte) (int, error) {
	return c.conn.Write(data)
}

func (c *unixClient) Read(buf []byte) (int, error) {
	return c.conn.Read(buf)
}

func (c *unixClient) Close() error {
	return c.conn.Close()
}
