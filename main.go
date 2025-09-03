package main

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/bootstrap"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func main() {
	router.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})
	router.Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		id := params["id"]
		postID := params["postID"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user": "%s", "post": "%s"}`, id, postID)))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/hello", middleware.Apply(helloHandler, middleware.Logging))

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
