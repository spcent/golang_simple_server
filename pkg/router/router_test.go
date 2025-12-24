package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spcent/golang_simple_server/pkg/middleware"
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

func TestParamsInjectedIntoContext(t *testing.T) {
	r := NewRouter()

	r.Get("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		ctxParams := ParamsFromContext(r.Context())
		if ctxParams == nil {
			t.Fatalf("expected params in context")
		}
		if ctxParams["name"] != params["name"] {
			t.Fatalf("context params mismatch: got %s want %s", ctxParams["name"], params["name"])
		}
		w.Write([]byte(ctxParams["name"]))
	})

	req := httptest.NewRequest(http.MethodGet, "/hello/Alice", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if got := strings.TrimSpace(w.Body.String()); got != "Alice" {
		t.Fatalf("expected context value to be written, got %q", got)
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

func TestMethodNotAllowed(t *testing.T) {
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

func TestRouteGroup(t *testing.T) {
	r := NewRouter()

	api := r.Group("/api")
	v1 := api.Group("/v1")
	v2 := api.Group("/v2")

	v1.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte("User " + params["id"]))
	})
	v1.Post("/users", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("Create User"))
	})
	v2.Get("/users", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("Users v2"))
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

		resp := w.Body.String()
		if resp != tt.expected {
			t.Errorf("[%s %s] expected %q, got %q", tt.method, tt.path, tt.expected, resp)
		}
	}
}

func TestRouteGroupMiddlewares(t *testing.T) {
	r := NewRouter()

	api := r.Group("/api")
	api.Use(func(next middleware.Handler) middleware.Handler {
		return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Group", "api")
			next.ServeHTTP(w, r)
		}))
	})

	api.Get("/ping", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("pong"))
	})

	req := httptest.NewRequest("GET", "/api/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("X-Group") != "api" {
		t.Errorf("expected middleware to set X-Group header")
	}
	if w.Body.String() != "pong" {
		t.Errorf("expected response body 'pong', got %q", w.Body.String())
	}
}

func TestRouterFreeze(t *testing.T) {
	r := NewRouter()

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {})
	r.Freeze()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when adding route after freeze")
		}
	}()

	r.Get("/panic", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {})
}
