// ipc.go
// Cross-platform IPC with address discovery.
// - Linux/macOS: Unix Domain Socket in /tmp/{appName}.sock
// - Windows: TCP loopback with random free port, written to registry file.
// Unified API for ListenApp and DialApp.

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

// Listener is a wrapper for net.Listener
type Listener struct {
	net.Listener
	Addr    string // final resolved address
	AppName string
}

// Remove cleans up resources (only needed for Unix socket file).
func (l *Listener) Remove() error {
	_ = os.Remove(addrFile(l.AppName))
	if runtime.GOOS != "windows" {
		return os.Remove(l.Addr)
	}
	return nil
}

// Conn is just an alias for net.Conn
type Conn = net.Conn

// addrFile returns the path of registry file for this appName
func addrFile(appName string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s.ipc.addr", appName))
}

// Listen starts a cross-platform IPC listener.
// - Linux/macOS: use Unix socket in /tmp/{appName}.sock
// - Windows: use TCP loopback 127.0.0.1:0 (random free port)
func Listen(appName string) (*Listener, error) {
	if runtime.GOOS == "windows" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		addr := ln.Addr().String()
		err = os.WriteFile(addrFile(appName), []byte(addr), 0600)
		if err != nil {
			return nil, fmt.Errorf("cannot write addr file: %w", err)
		}
		return &Listener{Listener: ln, Addr: addr, AppName: appName}, nil
	}

	// Unix-like
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.sock", appName))
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(addrFile(appName), []byte(sockPath), 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot write addr file: %w", err)
	}

	return &Listener{Listener: ln, Addr: sockPath, AppName: appName}, nil
}

// Dial connects to the IPC server.
// - On Linux/macOS: connect to Unix socket file.
// - On Windows: connect via TCP loopback.
func Dial(appName string) (Conn, error) {
	data, err := os.ReadFile(addrFile(appName))
	if err != nil {
		return nil, fmt.Errorf("cannot read addr file: %w", err)
	}

	addr := string(data)
	if runtime.GOOS == "windows" {
		return net.Dial("tcp", addr)
	}

	return net.Dial("unix", addr)
}
