package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// 设置测试环境变量
	os.Setenv("AUTH_TOKEN", "secret")

	// 清空路由
	routes = []Route{}

	// 注册测试路由
	AddRoute("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello, ` + name + `!"}`))
	})
	AddRoute("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
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
	handler := ApplyMiddleware(routerHandler, AuthMiddleware)
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
	handler := ApplyMiddleware(routerHandler, AuthMiddleware)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDynamicRoute(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/users/123/posts/456")
	handler := ApplyMiddleware(routerHandler, AuthMiddleware)
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := newAuthRequest("GET", "/notfound")
	routerHandlerWithMiddleware := ApplyMiddleware(routerHandler, AuthMiddleware)
	routerHandlerWithMiddleware(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
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

func TestLoadEnv(t *testing.T) {
	// 创建临时文件模拟 .env
	content := `
# 注释行
DB_HOST=127.0.0.1
DB_USER=root
DB_PASS="secret"
EMPTY_KEY=
QUOTED_KEY='quoted_value'
`
	tmpFile := "test.env"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 清理环境变量，避免影响测试
	os.Clearenv()

	// 设置一个已有变量，确保不会被覆盖
	os.Setenv("DB_USER", "existing_user")

	// 调用 LoadEnv
	err = LoadEnv(tmpFile, false)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}

	// 校验结果
	tests := []struct {
		key      string
		expected string
	}{
		{"DB_HOST", "127.0.0.1"},
		{"DB_USER", "existing_user"}, // 不覆盖已有值
		{"DB_PASS", "secret"},
		{"EMPTY_KEY", ""},
		{"QUOTED_KEY", "quoted_value"},
	}

	for _, tt := range tests {
		got := os.Getenv(tt.key)
		if got != tt.expected {
			t.Errorf("环境变量 %s = %q, 期望 %q", tt.key, got, tt.expected)
		}
	}
}

func TestLoadEnvWithOverwrite(t *testing.T) {
	content := `
DB_HOST=127.0.0.1
DB_USER=root
`
	tmpFile := "test_overwrite.env"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 情况1: 不覆盖已有值
	os.Clearenv()
	os.Setenv("DB_USER", "existing_user")
	err = LoadEnv(tmpFile, false)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}
	if got := os.Getenv("DB_USER"); got != "existing_user" {
		t.Errorf("DB_USER 应该保持为 existing_user, 实际是 %q", got)
	}

	// 情况2: 覆盖已有值
	os.Clearenv()
	os.Setenv("DB_USER", "existing_user")
	err = LoadEnv(tmpFile, true)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}
	if got := os.Getenv("DB_USER"); got != "root" {
		t.Errorf("DB_USER 应该被覆盖为 root, 实际是 %q", got)
	}
}

func TestLoadEnvFileNotFound(t *testing.T) {
	os.Clearenv()
	err := LoadEnv("nonexistent.env", false)
	if err == nil {
		t.Fatal("期望 LoadEnv 返回错误，但没有返回")
	}
}

func BenchmarkRouterHandler(b *testing.B) {
	// 初始化测试路由
	AddRoute("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})
	AddRoute("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {})

	req := newAuthRequest("GET", "/users/123/posts/456")
	w := httptest.NewRecorder()
	handler := ApplyMiddleware(routerHandler, AuthMiddleware)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler(w, req)
	}
}
