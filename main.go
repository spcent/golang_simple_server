package main

import (
	"net/http"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func main() {
	mux := http.NewServeMux()
	app := pkg.New(pkg.WithMux(mux))

	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/hello", middleware.Apply(helloHandler, middleware.Logging))

	r := router.NewRouter()
	handlers.RegisterRoutes(r)
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
