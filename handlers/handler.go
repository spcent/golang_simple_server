package handlers

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/router"
)

func RegisterRoutes(r *router.Router) {
	r.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})

	r.Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		id := params["id"]
		postID := params["postID"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user": "%s", "post": "%s"}`, id, postID)))
	})

	r.Register(&HealthHandler{}, &UserHandler{})
}
