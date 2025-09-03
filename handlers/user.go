package handlers

import (
	"fmt"
	"net/http"
)

type UserHandler struct{}

func (u *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "User List")
	})
}
