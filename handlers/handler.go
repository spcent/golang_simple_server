package handlers

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/contract"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func RegisterRoutes(r *router.Router) {
	r.GetFunc("/hello/:name", func(w http.ResponseWriter, r *http.Request) {
		name, _ := contract.Param(r, "name")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})

	r.GetFunc("/users/:id/posts/:postID", func(w http.ResponseWriter, r *http.Request) {
		id, _ := contract.Param(r, "id")
		postID, _ := contract.Param(r, "postID")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user": "%s", "post": "%s"}`, id, postID)))
	})

	r.Register(&HealthHandler{}, &UserHandler{})
}
