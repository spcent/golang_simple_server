package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBasicRoutes(t *testing.T) {
	// Reset global router
	r := NewRouter()

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("pong"))
	})

	r.Post("/echo", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("echo"))
	})

	tests := []struct {
		method   string
		path     string
		expected string
	}{
		{"GET", "/ping", "pong"},
		{"POST", "/echo", "echo"},
		{"GET", "/echo", "404 page not found\n"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Body.String()
		if resp != tt.expected {
			t.Errorf("[%s %s] expected %q, got %q", tt.method, tt.path, tt.expected, resp)
		}
	}
}

func TestParamRoutes(t *testing.T) {
	r := NewRouter()

	r.Get("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte("Hello " + params["name"]))
	})

	r.Get("/users/:id/books/:bookId", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte("User " + params["id"] + " Book " + params["bookId"]))
	})

	tests := []struct {
		path     string
		expected string
	}{
		{"/hello/Alice", "Hello Alice"},
		{"/hello/Bob", "Hello Bob"},
		{"/users/123/books/456", "User 123 Book 456"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := strings.TrimSpace(w.Body.String())
		if resp != tt.expected {
			t.Errorf("[%s] expected %q, got %q", tt.path, tt.expected, resp)
		}
	}
}

func TestAnyRoute(t *testing.T) {
	r := NewRouter()

	r.Any("/any", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("any"))
	})

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/any", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Body.String() != "any" {
			t.Errorf("[%s /any] expected %q, got %q", method, "any", w.Body.String())
		}
	}
}

func TestPrintRoutes(t *testing.T) {
	// Reset global r
	r := NewRouter()

	r.Get("/print1", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {})
	r.Post("/print2", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {})
	r.Any("/print3", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {})

	// Read captured output
	var outBuf bytes.Buffer
	r.Print(&outBuf)

	output := outBuf.String()
	if !strings.Contains(output, "GET    /print1") {
		t.Errorf("PrintRoutes output missing GET /print1: %s", output)
	}
	if !strings.Contains(output, "POST   /print2") {
		t.Errorf("PrintRoutes output missing POST /print2: %s", output)
	}
	if !strings.Contains(output, "/print3") {
		t.Errorf("PrintRoutes output missing /print3: %s", output)
	}
}

func TestStaticAndParamRoutes(t *testing.T) {
	r := NewRouter()

	// Static route
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		w.Write([]byte("pong"))
	})

	// Param route
	r.Get("/hello/:name", func(w http.ResponseWriter, _ *http.Request, params map[string]string) {
		w.Write([]byte("Hello " + params["name"]))
	})

	tests := []struct {
		method   string
		path     string
		expected string
	}{
		{"GET", "/ping", "pong"},
		{"GET", "/hello/Alice", "Hello Alice"},
		{"GET", "/hello/Bob", "Hello Bob"},
		{"GET", "/notfound", "404 page not found\n"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Body.String() != tt.expected {
			t.Errorf("[%s %s] expected %q, got %q", tt.method, tt.path, tt.expected, w.Body.String())
		}
	}
}

func TestGroupRoutes(t *testing.T) {
	r := NewRouter()
	api := r.Group("/api")
	v1 := api.Group("/v1")

	v1.Get("/users/:id", func(w http.ResponseWriter, _ *http.Request, params map[string]string) {
		w.Write([]byte("User " + params["id"]))
	})

	v1.Post("/users", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		w.Write([]byte("Create User"))
	})

	tests := []struct {
		method   string
		path     string
		expected string
	}{
		{"GET", "/api/v1/users/123", "User 123"},
		{"POST", "/api/v1/users", "Create User"},
		{"GET", "/api/v1/users", "404 page not found\n"},
		{"GET", "/api/v2/users/123", "404 page not found\n"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Body.String() != tt.expected {
			t.Errorf("[%s %s] expected %q, got %q", tt.method, tt.path, tt.expected, w.Body.String())
		}
	}
}

func TestAnyMethodRoute(t *testing.T) {
	r := NewRouter()
	r.Any("/ping", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		w.Write([]byte("pong"))
	})

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, m := range methods {
		req := httptest.NewRequest(m, "/ping", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Body.String() != "pong" {
			t.Errorf("[%s /ping] expected %q, got %q", m, "pong", w.Body.String())
		}
	}
}

func TestMultipleMethodsSamePath(t *testing.T) {
	r := NewRouter()
	r.Get("/resource", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		w.Write([]byte("GET resource"))
	})
	r.Post("/resource", func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
		w.Write([]byte("POST resource"))
	})

	req := httptest.NewRequest("GET", "/resource", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "GET resource" {
		t.Errorf("expected GET resource, got %q", w.Body.String())
	}

	req = httptest.NewRequest("POST", "/resource", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "POST resource" {
		t.Errorf("expected POST resource, got %q", w.Body.String())
	}
}
