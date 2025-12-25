package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/core"
	"github.com/spcent/golang_simple_server/pkg/middleware"
)

func main() {
	// Create a new app with default configuration
	app := core.New()

	// Example 1: Use original http.HandlerFunc (direct route)
	app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"pong"}`))
	})

	// Example 2: Use router with http.HandlerFunc (dynamic route)
	app.Router().GetFunc("/hello/:name", func(w http.ResponseWriter, r *http.Request) {
		// Get the name parameter from URL
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "Guest"
		}
		w.Header().Set("Content-Type", "application/json")

		// Safely encode the message as JSON to avoid injection
		type helloResponse struct {
			Message string `json:"message"`
		}
		resp := helloResponse{Message: fmt.Sprintf("Hello, %s!", name)}
		payload, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Write(payload)
	})

	// Example 3: Use router with custom Handler (dynamic route with params)
	app.Router().GetFunc("/user/:id", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := contract.Param(r, "id")
		w.Header().Set("Content-Type", "application/json")

		// Safely encode the user ID as JSON to avoid injection
		type userResponse struct {
			UserID string `json:"user_id"`
		}
		resp := userResponse{UserID: userID}
		payload, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Write(payload)
	})

	// Apply middleware using the correct middleware.Middleware type
	middlewareFunc := func(h middleware.Handler) middleware.Handler {
		return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Printf("[Middleware] Request: %s %s\n", r.Method, r.URL.Path)
			h.ServeHTTP(w, r)
		}))
	}
	app.Use(middlewareFunc)

	// Start the server
	fmt.Println("Server starting on :8080...")
	app.Boot()
}
