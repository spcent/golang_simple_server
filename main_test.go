package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
)

func TestMain(m *testing.M) {
	// 设置测试环境变量
	os.Setenv("AUTH_TOKEN", "secret")

	// 注册测试路由
	router.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello, ` + name + `!"}`))
	})
	router.Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	os.Exit(m.Run())
}

// Helper 创建带鉴权头的请求
func newAuthRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Token", "secret")
	return req
}

func TestPing(t *testing.T) {
	req := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	pingHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHello(t *testing.T) {
	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	helloHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHelloName(t *testing.T) {
	names := []string{"Alice", "Bob", "Charlie"}
	handler := middleware.Apply(router.Handle, middleware.Auth)
	for _, name := range names {
		w := httptest.NewRecorder()
		req := newAuthRequest("GET", "/hello/"+name)
		handler(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", name, w.Result().StatusCode)
		}
	}
}

func TestHelloNameUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/hello/Alice", nil) // 不带鉴权头
	handler := middleware.Apply(router.Handle, middleware.Auth)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDynamicRoute(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/users/123/posts/456")
	handler := middleware.Apply(router.Handle, middleware.Auth)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/notfound")
	routerHandlerWithMiddleware := middleware.Apply(router.Handle, middleware.Auth)
	routerHandlerWithMiddleware(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func BenchmarkRouterHandler(b *testing.B) {
	// 初始化测试路由
	router.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})
	router.Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})

	req := newAuthRequest("GET", "/users/123/posts/456")
	w := httptest.NewRecorder()
	handler := middleware.Apply(router.Handle, middleware.Auth)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler(w, req)
	}
}
