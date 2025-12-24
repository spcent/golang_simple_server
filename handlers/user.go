package handlers

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/router"
)

type UserHandler struct{}

func (u *UserHandler) Register(r *router.Router) {
	r.GetFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, "User List")
	})
}
