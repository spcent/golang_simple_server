package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spcent/golang_simple_server/pkg/health"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func TestHealthAndReady(t *testing.T) {
	r := router.NewRouter()
	health.SetNotReady("initializing")
	(&HealthHandler{}).Register(r)

	// Liveness should always return 200.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", resp.Code)
	}

	// Readiness should fail before ready.
	readyReq := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readyResp := httptest.NewRecorder()
	r.ServeHTTP(readyResp, readyReq)
	if readyResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected readiness 503 before ready, got %d", readyResp.Code)
	}

	// Mark ready and expect 200.
	health.SetReady()
	readyResp = httptest.NewRecorder()
	r.ServeHTTP(readyResp, readyReq)
	if readyResp.Code != http.StatusOK {
		t.Fatalf("expected readiness 200 after ready, got %d", readyResp.Code)
	}
}
