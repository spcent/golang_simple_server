package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/spcent/golang_simple_server/pkg/health"
	"github.com/spcent/golang_simple_server/pkg/router"
)

type HealthHandler struct{}

func (h *HealthHandler) Register(r *router.Router) {
	r.GetFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resp := map[string]any{
			"status": "ok",
			"build":  health.GetBuildInfo(),
		}

		json.NewEncoder(w).Encode(resp)
	})

	r.GetFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		readiness := health.GetReadiness()
		statusCode := http.StatusOK
		status := "ready"
		if !readiness.Ready {
			statusCode = http.StatusServiceUnavailable
			status = "not_ready"
		}

		w.WriteHeader(statusCode)
		resp := map[string]any{
			"status":    status,
			"build":     health.GetBuildInfo(),
			"readiness": readiness,
		}

		json.NewEncoder(w).Encode(resp)
	})
}
