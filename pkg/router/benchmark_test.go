package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// createTestRouter builds a router with static and parameterized routes
func createTestRouter() *Router {
	router := NewRouter()

	// Static routes
	router.AddRoute("/about", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("about"))
	})
	router.AddRoute("/contact", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		w.Write([]byte("contact"))
	})

	// Parameterized routes
	router.AddRoute("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte("Hello " + params["name"]))
	})
	router.AddRoute("/users/:id/books/:bookId", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.Write([]byte("User " + params["id"] + " Book " + params["bookId"]))
	})

	return router
}

// BenchmarkStaticRoute tests a static route (/about)
func BenchmarkStaticRoute(b *testing.B) {
	router := createTestRouter()
	req := httptest.NewRequest("GET", "/about", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkParamRoute tests a route with one parameter (/hello/:name)
func BenchmarkParamRoute(b *testing.B) {
	router := createTestRouter()
	req := httptest.NewRequest("GET", "/hello/Alice", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkNestedParamRoute tests a route with multiple parameters (/users/:id/books/:bookId)
func BenchmarkNestedParamRoute(b *testing.B) {
	router := createTestRouter()
	req := httptest.NewRequest("GET", "/users/123/books/456", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
