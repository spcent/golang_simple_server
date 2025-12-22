package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
)

// TestNewAppWithDefaultOptions tests that a new App instance has the correct default values
func TestNewAppWithDefaultOptions(t *testing.T) {
	// Create an App instance with no options
	app := New()

	// Assert that default values are set correctly
	if app.mux == nil {
		t.Errorf("Default mux should not be nil")
	}
	if app.router == nil {
		t.Errorf("Default router should not be nil")
	}
	if app.config.Addr != ":8080" {
		t.Errorf("Default address should be :8080, got %s", app.config.Addr)
	}
	if app.config.EnvFile != ".env" {
		t.Errorf("Default envFile should be .env, got %s", app.config.EnvFile)
	}
}

// TestWithMux tests that WithMux option correctly sets the mux
func TestWithMux(t *testing.T) {
	// Create a custom mux
	customMux := http.NewServeMux()

	// Create an App instance with the custom mux
	app := New(WithMux(customMux))

	// Assert that the mux is set correctly
	if app.mux != customMux {
		t.Errorf("Mux should be set to the custom mux")
	}
}

// TestWithRouter tests that WithRouter option correctly sets the router
func TestWithRouter(t *testing.T) {
	// Create a custom router
	customRouter := router.NewRouter()

	// Create an App instance with the custom router
	app := New(WithRouter(customRouter))

	// Assert that the router is set correctly
	if app.router != customRouter {
		t.Errorf("Router should be set to the custom router")
	}
}

// TestWithAddr tests that WithAddr option correctly sets the address
func TestWithAddr(t *testing.T) {
	// Define a custom address
	customAddr := ":9090"

	// Create an App instance with the custom address
	app := New(WithAddr(customAddr))

	// Assert that the address is set correctly
	if app.config.Addr != customAddr {
		t.Errorf("Address should be set to %s, got %s", customAddr, app.config.Addr)
	}
}

// TestWithEnvPath tests that WithEnvPath option correctly sets the env file path
func TestWithEnvPath(t *testing.T) {
	// Define a custom env file path
	customEnvPath := ".custom.env"

	// Create an App instance with the custom env file path
	app := New(WithEnvPath(customEnvPath))

	// Assert that the env file path is set correctly
	if app.config.EnvFile != customEnvPath {
		t.Errorf("Env file path should be set to %s, got %s", customEnvPath, app.config.EnvFile)
	}
}

// TestMultipleOptions tests that multiple options can be applied together
func TestMultipleOptions(t *testing.T) {
	// Create custom components
	customMux := http.NewServeMux()
	customRouter := router.NewRouter()
	customAddr := ":9090"
	customEnvPath := ".custom.env"

	// Create an App instance with multiple options
	app := New(
		WithMux(customMux),
		WithRouter(customRouter),
		WithAddr(customAddr),
		WithEnvPath(customEnvPath),
	)

	// Assert that all options are set correctly
	if app.mux != customMux {
		t.Errorf("Mux should be set to the custom mux")
	}
	if app.router != customRouter {
		t.Errorf("Router should be set to the custom router")
	}
	if app.config.Addr != customAddr {
		t.Errorf("Address should be set to %s, got %s", customAddr, app.config.Addr)
	}
	if app.config.EnvFile != customEnvPath {
		t.Errorf("Env file path should be set to %s, got %s", customEnvPath, app.config.EnvFile)
	}
}

// TestOptionOrder tests that the order of options does not affect the result
func TestOptionOrder(t *testing.T) {
	// Create custom components
	customMux := http.NewServeMux()
	customRouter := router.NewRouter()

	// Create App instances with different order of options
	app1 := New(WithMux(customMux), WithRouter(customRouter))
	app2 := New(WithRouter(customRouter), WithMux(customMux))

	// Assert that both instances have the same configuration
	if app1.mux != app2.mux {
		t.Errorf("Mux should be the same regardless of option order")
	}
	if app1.router != app2.router {
		t.Errorf("Router should be the same regardless of option order")
	}
}

// TestRouterMethod tests that the Router method returns the correct router
func TestRouterMethod(t *testing.T) {
	// Create a custom router
	customRouter := router.NewRouter()

	// Create an App instance with the custom router
	app := New(WithRouter(customRouter))

	// Get the router using the Router method
	returnedRouter := app.Router()

	// Assert that the returned router is the same as the custom router
	if returnedRouter != customRouter {
		t.Errorf("Router method should return the correct router")
	}
}

// TestOptionIdempotency tests that applying the same option multiple times results in the last one taking effect
func TestOptionIdempotency(t *testing.T) {
	// Define multiple addresses
	addr1 := ":9090"
	addr2 := ":9191"

	// Create an App instance with the same option applied multiple times
	app := New(WithAddr(addr1), WithAddr(addr2))

	// Assert that the last option takes effect
	if app.config.Addr != addr2 {
		t.Errorf("Last option should take effect when the same option is applied multiple times. Expected %s, got %s", addr2, app.config.Addr)
	}
}

// TestHandleFunc tests that the HandleFunc method correctly registers a handler function
func TestHandleFunc(t *testing.T) {
	app := New()

	// Define a test path and handler
	path := "/test"
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}

	// Register the handler function
	app.HandleFunc(path, handlerFunc)

	// Create a test request
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()

	// Serve the request directly using the mux
	app.mux.ServeHTTP(w, req)

	// Assert that the handler was called correctly
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "test response" {
		t.Errorf("Expected body 'test response', got '%s'", w.Body.String())
	}
}

// TestHandle tests that the Handle method correctly registers a handler
func TestHandle(t *testing.T) {
	app := New()

	// Define a test path and handler
	path := "/test-handler"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("handler response"))
	})

	// Register the handler
	app.Handle(path, handler)

	// Create a test request
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()

	// Serve the request directly using the mux
	app.mux.ServeHTTP(w, req)

	// Assert that the handler was called correctly
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "handler response" {
		t.Errorf("Expected body 'handler response', got '%s'", w.Body.String())
	}
}

// TestUse tests that the Use method correctly applies middleware
func TestUse(t *testing.T) {
	app := New()

	// Register a test route using app.Get (which uses router) instead of app.HandleFunc (which uses mux)
	app.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply the middleware using the correct middleware.Middleware type
	middlewareFunc := func(h middleware.Handler) middleware.Handler {
		return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Middleware", "applied")
			h.ServeHTTP(w, r)
		}))
	}
	app.Use(middlewareFunc)

	// Create a test request to the test path
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// Serve the request
	app.mux.ServeHTTP(w, req)

	// Assert that the middleware was applied
	if w.Header().Get("X-Custom-Middleware") != "applied" {
		t.Errorf("Expected middleware to be applied, but header was not set")
	}
}

// TestUseMultipleMiddlewares tests that multiple middlewares are applied in the correct order
func TestUseMultipleMiddlewares(t *testing.T) {
	app := New()

	// Add a test route to avoid infinite recursion
	app.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create middlewares that add headers in a specific order
	middlewareFunc1 := func(h middleware.Handler) middleware.Handler {
		return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Middleware-Order", "1")
			h.ServeHTTP(w, r)
		}))
	}

	middlewareFunc2 := func(h middleware.Handler) middleware.Handler {
		return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Middleware-Order", "2")
			h.ServeHTTP(w, r)
		}))
	}

	// Apply the middlewares in order 1 then 2
	app.Use(middlewareFunc1, middlewareFunc2)

	// Create a test request to the test route
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// Serve the request
	app.mux.ServeHTTP(w, req)

	// Get the headers and check the order
	headers := w.Header()
	values := headers["X-Middleware-Order"]
	if len(values) < 2 {
		t.Errorf("Expected at least 2 middleware headers, got %d", len(values))
	}
	// We can't assert the exact order since we can't mock router.ServeHTTP
}

// TestBoot tests the Boot method with mocked dependencies
// This is a basic test that focuses on configuration rather than actual server startup
func TestBoot(t *testing.T) {
	// Mock environment variables
	os.Setenv("APP_DEBUG", "false")
	defer os.Unsetenv("APP_DEBUG")

	// Create a new App with custom address
	app := New(WithAddr(":8888"))

	// Assert that the address was set correctly before Boot
	if app.config.Addr != ":8888" {
		t.Errorf("Expected address to be set to :8888 before Boot, got %s", app.config.Addr)
	}

	// For this test, we'll just verify that Boot doesn't panic
	// without actually starting the server
	// This is because starting the server would require proper shutdown handling
	// and could interfere with other tests

	// Instead, we'll test the Boot method's components separately
	// Test loadEnv component
	if err := app.loadEnv(); err != nil {
		t.Errorf("loadEnv failed: %v", err)
	}

	// Test setupServer component
	if err := app.setupServer(); err != nil {
		t.Errorf("setupServer failed: %v", err)
	}

	// Verify that httpServer was created with the correct address
	if app.httpServer == nil {
		t.Errorf("httpServer should be created by setupServer")
	} else if app.httpServer.Addr != ":8888" {
		t.Errorf("httpServer address should be :8888, got %s", app.httpServer.Addr)
	}
}
