package tiny

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spcent/golang_simple_server/pkg/middleware"
)

func TestMain(m *testing.M) {
	// 设置测试环境变量
	os.Setenv("AUTH_TOKEN", "secret")

	// 注册测试路由
	Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello, ` + name + `!"}`))
	})
	Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	os.Exit(m.Run())
}

func TestParamExtraction(t *testing.T) {
	params, ok := matchRoute("/hello/{name}", "/hello/Alice")
	if !ok || params["name"] != "Alice" {
		t.Fatalf("expected name=Alice, got %v", params)
	}

	params2, ok2 := matchRoute("/users/{id}/posts/{postID}", "/users/123/posts/456")
	if !ok2 || params2["id"] != "123" || params2["postID"] != "456" {
		t.Fatalf("expected id=123 postID=456, got %v", params2)
	}
}

func TestInvalidParamRoute(t *testing.T) {
	_, ok := matchRoute("/hello/{name}", "/hello/")
	if ok {
		t.Fatal("expected route not match for /hello/")
	}
	_, ok2 := matchRoute("/users/{id}/posts/{postID}", "/users/123/posts")
	if ok2 {
		t.Fatal("expected route not match for incomplete path")
	}
}

// Helper 创建带鉴权头的请求
func newAuthRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Token", "secret")
	return req
}

func TestHelloName(t *testing.T) {
	names := []string{"Alice", "Bob", "Charlie"}
	handler := middleware.Apply(Handle, middleware.Auth)
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
	handler := middleware.Apply(Handle, middleware.Auth)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDynamicRoute(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/users/123/posts/456")
	handler := middleware.Apply(Handle, middleware.Auth)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/notfound")
	routerHandlerWithMiddleware := middleware.Apply(Handle, middleware.Auth)
	routerHandlerWithMiddleware(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func BenchmarkRouterHandler(b *testing.B) {
	// 初始化测试路由
	Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})
	Get("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})

	req := newAuthRequest("GET", "/users/123/posts/456")
	w := httptest.NewRecorder()
	handler := middleware.Apply(Handle, middleware.Auth)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler(w, req)
	}
}

func BenchmarkRouteLookup(b *testing.B) {
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/api/v%d/resource/{id}", i)
		Get(path, dummyHandler)
		Post(path, dummyHandler)
		Put(path, dummyHandler)
		Delete(path, dummyHandler)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/api/v123/resource/456", nil)
		w := httptest.NewRecorder()
		Handle(w, req)
	}
}

func dummyHandler(w http.ResponseWriter, r *http.Request, params map[string]string) {}
