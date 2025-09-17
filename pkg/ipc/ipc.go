package ipc

// Server defines the IPC server interface.
type Server interface {
	Accept() (Client, error) // Wait for client connection
	Addr() string            // Return server address
	Close() error            // Close the server
}

// Client defines the IPC client interface.
type Client interface {
	Write([]byte) (int, error) // Send data
	Read([]byte) (int, error)  // Receive data
	Close() error              // Close connection
}

// NewServer creates a new IPC server on given address.
// Implementation differs by platform (see ipc_unix.go / ipc_windows.go).
// func NewServer(addr string) (Server, error)

// Dial connects to a server at given address.
// Implementation differs by platform (see ipc_unix.go / ipc_windows.go).
// func Dial(addr string, useTCP bool) (Client, error)
