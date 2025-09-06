package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestResource_RouteRegistration(t *testing.T) {
	r := NewRouter()
	r.Resource("/users", &mockController{})

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
			r.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Errorf("Route not registered: %s %s", tc.method, tc.path)
			}
		})
	}
}

func TestResource_ControllerInvocation(t *testing.T) {
	r := NewRouter()
	ctrl := &mockController{}
	r.Resource("/posts", ctrl)

	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
		wantCall *int
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
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("Expected status code %d, got %d", tt.wantCode, rec.Code)
			}

			if *tt.wantCall != 1 {
				t.Errorf("Handler was not called, call count %d", *tt.wantCall)
			}

			if tt.wantID != "" && ctrl.lastParams["id"] != tt.wantID {
				t.Errorf("Expected param id=%s, got %s", tt.wantID, ctrl.lastParams["id"])
			}
		})
	}
}

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
				t.Errorf("%s method expected 501, got %d", tt.name, rec.Code)
			}
		})
	}
}

func TestResource_PathTrimSuffix(t *testing.T) {
	r := NewRouter()
	ctrl := &mockController{}
	r.Resource("/products/", ctrl)

	req := httptest.NewRequest("GET", "/products", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if ctrl.indexCalled != 1 {
		t.Error("Path with trailing slash was not trimmed correctly, Index handler was not called")
	}
}
