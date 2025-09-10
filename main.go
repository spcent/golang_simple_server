package main

import (
	"io"
	"net/http"
	"time"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/foundation"
	ws "github.com/spcent/golang_simple_server/pkg/websocket"
)

func main() {
	// Create a new app with default configuration
	app := foundation.New()

	// Register routes directly on the app
	app.HandleFunc("/ping", pingHandler)
	app.HandleFunc("/hello", helloHandler)

	// Basic config
	workerCount := 16
	jobQueueSize := 4096
	sendQueueSize := 256
	sendTimeout := 200 * time.Millisecond
	sendBehavior := ws.SendBlock // could be configured

	hub := ws.NewHub(workerCount, jobQueueSize)
	defer hub.Stop()
	secret := []byte("change-this-secret")
	auth := ws.NewSimpleRoomAuth(secret)
	// demo: set a password on "vip" room
	auth.SetRoomPassword("vip", "letmein")

	app.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWSWithAuth(w, r, hub, auth, sendQueueSize, sendTimeout, sendBehavior)
	})
	app.HandleFunc("/admin/broadcast", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		b, _ := io.ReadAll(r.Body)
		hub.BroadcastAll(ws.OpcodeText, b)
		w.WriteHeader(http.StatusNoContent)
	})

	// Get the router and register routes
	handlers.RegisterRoutes(app.Router())

	// Apply middleware
	app.Use(app.Logging(), app.Auth())

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
