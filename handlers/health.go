package handlers

import (
	"fmt"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/router"
)

type HealthHandler struct{}

func (h *HealthHandler) Register(r *router.Router) {
	r.Get("/health", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
}
