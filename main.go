package main

import (
	"net/http"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/core"
)

func main() {
	// Create a new app with default configuration
	// For HTTPS support, use WithTLS option or command line flags
	// Example: app := core.New(core.WithTLS("./cert.pem", "./key.pem"))
	app := core.New()

	// Register routes directly on the app using the new API
	app.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"pong"}`))
	})

	app.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello, World!"}`))
	})

	// Configure WebSocket with custom secret for JWT authentication
	// app.ConfigureWebSocketWithOptions(core.WebSocketConfig{
	// 	Secret: []byte("your-secure-jwt-secret"),
	// })
	app.ConfigureWebSocket()

	// Register routes via handlers package
	handlers.RegisterRoutes(app.Router())

	// Apply middleware using more elegant methods
	app.EnableLogging()
	// Add rate limiting: 10 requests per second with burst up to 20
	app.EnableRateLimit(10, 20)

	// Boot the application
	// HTTPS can also be enabled via command line flags:
	// ./simple -tls -tls-cert ./cert.pem -tls-key ./key.pem
	app.Boot()
}
