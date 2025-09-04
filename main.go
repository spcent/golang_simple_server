package main

import (
	"net/http"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/bootstrap"
	"github.com/spcent/golang_simple_server/pkg/middleware"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/hello", middleware.Apply(helloHandler, middleware.Logging))

	handlers.RegisterRoutes()
	bootstrap.Boot(mux)
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
