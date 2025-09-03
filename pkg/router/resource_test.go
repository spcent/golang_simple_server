package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 测试路由注册和控制器方法调用

// mockController 用于验证方法调用和参数传递
type mockController struct {
	indexCalled  int
	showCalled   int
	createCalled int
	updateCalled int
	deleteCalled int
	patchCalled  int
	lastParams   map[string]string
}

func (m *mockController) Index(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.indexCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusOK)
}

func (m *mockController) Show(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.showCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusOK)
}

func (m *mockController) Create(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.createCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusCreated)
}

func (m *mockController) Update(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.updateCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusOK)
}

func (m *mockController) Delete(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.deleteCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusNoContent)
}

func (m *mockController) Patch(w http.ResponseWriter, r *http.Request, params map[string]string) {
	m.patchCalled++
	m.lastParams = params
	w.WriteHeader(http.StatusOK)
}

// 测试Resource函数注册的路由是否正确
func TestResource_RouteRegistration(t *testing.T) {
	// 注册资源路由
	Resource("/users", &mockController{})

	// 验证基础路径路由
	testCases := []struct {
		method string
		path   string
	}{
		{"GET", "/users"},
		{"POST", "/users"},
		{"GET", "/users/123"},
		{"PUT", "/users/123"},
		{"DELETE", "/users/123"},
		{"PATCH", "/users/123"},
	}

	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			Handle(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Errorf("路由未注册: %s %s", tc.method, tc.path)
			}
		})
	}
}

// 测试控制器方法调用和参数传递
func TestResource_ControllerInvocation(t *testing.T) {
	ctrl := &mockController{}
	Resource("/posts", ctrl)

	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		wantCall *int // 指向mockController的调用计数器
		wantID   string
	}{
		{"Index", "GET", "/posts", http.StatusOK, &ctrl.indexCalled, ""},
		{"Create", "POST", "/posts", http.StatusCreated, &ctrl.createCalled, ""},
		{"Show", "GET", "/posts/456", http.StatusOK, &ctrl.showCalled, "456"},
		{"Update", "PUT", "/posts/789", http.StatusOK, &ctrl.updateCalled, "789"},
		{"Delete", "DELETE", "/posts/000", http.StatusNoContent, &ctrl.deleteCalled, "000"},
		{"Patch", "PATCH", "/posts/111", http.StatusOK, &ctrl.patchCalled, "111"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			Handle(rec, req)

			// 验证状态码
			if rec.Code != tt.wantCode {
				t.Errorf("期望状态码 %d，实际 %d", tt.wantCode, rec.Code)
			}

			// 验证方法被调用
			if *tt.wantCall != 1 {
				t.Errorf("方法未被调用，调用次数 %d", *tt.wantCall)
			}

			// 验证参数传递
			if tt.wantID != "" && ctrl.lastParams["id"] != tt.wantID {
				t.Errorf("期望参数id=%s，实际 %s", tt.wantID, ctrl.lastParams["id"])
			}
		})
	}
}

// 测试BaseResourceController默认返回501
func TestBaseResourceController_DefaultImplementation(t *testing.T) {
	ctrl := &BaseResourceController{}
	tests := []struct {
		name   string
		method string
		path   string
		call   func(w http.ResponseWriter, r *http.Request)
	}{
		{"Index", "GET", "/posts", func(w http.ResponseWriter, r *http.Request) { ctrl.Index(w, r, nil) }},
		{"Show", "GET", "/posts/456", func(w http.ResponseWriter, r *http.Request) { ctrl.Show(w, r, nil) }},
		{"Create", "POST", "/posts", func(w http.ResponseWriter, r *http.Request) { ctrl.Create(w, r, nil) }},
		{"Update", "PUT", "/posts/789", func(w http.ResponseWriter, r *http.Request) { ctrl.Update(w, r, nil) }},
		{"Delete", "DELETE", "/posts/000", func(w http.ResponseWriter, r *http.Request) { ctrl.Delete(w, r, nil) }},
		{"Patch", "PATCH", "/posts/111", func(w http.ResponseWriter, r *http.Request) { ctrl.Patch(w, r, nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.call(rec, req)
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("%s 方法期望返回501，实际返回 %d", tt.name, rec.Code)
			}
		})
	}
}

// 测试路径末尾斜杠处理
func TestResource_PathTrimSuffix(t *testing.T) {
	ctrl := &mockController{}

	// 测试带斜杠的路径
	Resource("/products/", ctrl)

	req := httptest.NewRequest("GET", "/products", nil)
	rec := httptest.NewRecorder()
	Handle(rec, req)

	if ctrl.indexCalled != 1 {
		t.Error("带斜杠的路径未正确修剪，Index方法未被调用")
	}
}
