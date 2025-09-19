package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing(t *testing.T) {
	req := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	pingHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	body := w.Body.String()
	if body != "pong" {
		t.Fatalf("expected body 'pong', got %q", body)
	}
}

func TestHello(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/hello", nil)
		w := httptest.NewRecorder()
		helloHandler(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %q", ct)
		}

		expected := `{"message":"Hello, World!"}`
		if w.Body.String() != expected {
			t.Fatalf("expected body %q, got %q", expected, w.Body.String())
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/hello/world", nil)
		w := httptest.NewRecorder()
		helloHandler(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})
}
