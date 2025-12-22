package main

import (
	"fmt"
	"net/http"

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
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})

	// Example 3: Use router with custom Handler (dynamic route with params)
	app.Router().Get("/user/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		userID := params["id"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user_id":"%s"}`, userID)))
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
