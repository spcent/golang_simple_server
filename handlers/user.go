package handlers

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/foundation"
)

type UserHandler struct{}

func (u *UserHandler) Register(r foundation.RouteRegister) {
	r.Get("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, "User List")
	})
}
