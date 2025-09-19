package main

import (
	"net/http"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/foundation"
)

func main() {
	// Create a new app with default configuration
	app := foundation.New()

	// Register routes directly on the app
	app.HandleFunc("/ping", pingHandler)
	app.HandleFunc("/hello", helloHandler)

	hub := app.ConfigureWebSocket()
	defer hub.Stop()

	// Register application routes via the foundation app API
	handlers.RegisterRoutes(app)

	// Apply middleware
	app.Use(app.Logging())

	// Boot the application
	app.Boot()
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`pong`))
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/hello" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Hello, World!"}`))
}
