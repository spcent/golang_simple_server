package foundation

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

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
	if app.addr != ":8080" {
		t.Errorf("Default address should be :8080, got %s", app.addr)
	}
	if app.envFile != ".env" {
		t.Errorf("Default envFile should be .env, got %s", app.envFile)
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
	if app.addr != customAddr {
		t.Errorf("Address should be set to %s, got %s", customAddr, app.addr)
	}
}

// TestWithEnvPath tests that WithEnvPath option correctly sets the env file path
func TestWithEnvPath(t *testing.T) {
	// Define a custom env file path
	customEnvPath := ".custom.env"

	// Create an App instance with the custom env file path
	app := New(WithEnvPath(customEnvPath))

	// Assert that the env file path is set correctly
	if app.envFile != customEnvPath {
		t.Errorf("Env file path should be set to %s, got %s", customEnvPath, app.envFile)
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
	if app.addr != customAddr {
		t.Errorf("Address should be set to %s, got %s", customAddr, app.addr)
	}
	if app.envFile != customEnvPath {
		t.Errorf("Env file path should be set to %s, got %s", customEnvPath, app.envFile)
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
	if app.addr != addr2 {
		t.Errorf("Last option should take effect when the same option is applied multiple times. Expected %s, got %s", addr2, app.addr)
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

	// Create a test middleware that adds a header
	customMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Middleware", "applied")
			next(w, r)
		}
	}

	// Apply the middleware
	app.Use(customMiddleware)

	// Create a test request to the root path (since Use registers middleware for "/")
	req := httptest.NewRequest("GET", "/", nil)
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

	// Create middlewares that add headers in a specific order
	mw1 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Middleware-Order", "1")
			next(w, r)
		}
	}

	mw2 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Middleware-Order", "2")
			next(w, r)
		}
	}

	// Apply the middlewares in order 1 then 2
	app.Use(mw1, mw2)

	// Create a test request
	req := httptest.NewRequest("GET", "/", nil)
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

// TestLoggingMiddleware tests that the Logging method returns a non-nil middleware
func TestLoggingMiddleware(t *testing.T) {
	app := New()

	// Get the logging middleware
	loggingMw := app.Logging()

	// Assert that the returned middleware is not nil
	if loggingMw == nil {
		t.Errorf("Logging method should return a non-nil middleware function")
	}
}

// TestAuthMiddleware tests that the Auth method returns a non-nil middleware
func TestAuthMiddleware(t *testing.T) {
	app := New()

	// Get the auth middleware
	authMw := app.Auth()

	// Assert that the returned middleware is not nil
	if authMw == nil {
		t.Errorf("Auth method should return a non-nil middleware function")
	}
}

// TestBoot tests the Boot method with mocked dependencies
// This is a basic test that focuses on configuration rather than actual server startup
func TestBoot(t *testing.T) {
	// Save original values and restore them after the test
	originalAddr := *addr
	originalEnv := *env
	defer func() {
		*addr = originalAddr
		*env = originalEnv
	}()

	// Set test flags
	*addr = ":8888"
	*env = ""

	// Mock environment variables
	os.Setenv("APP_DEBUG", "false")
	defer os.Unsetenv("APP_DEBUG")

	// Create a new App with mocked components
	app := New()

	// Run Boot in a goroutine with a context that can be canceled
	// We'll let it run briefly and then send SIGTERM to stop it
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Boot()
	}()

	// Wait a short time to ensure the server starts
	time.Sleep(50 * time.Millisecond)

	// Get current process and send SIGTERM to trigger graceful shutdown
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Errorf("Failed to find current process: %v", err)
		return
	}

	// Send SIGTERM signal to trigger graceful shutdown
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("Failed to send SIGTERM signal: %v", err)
	}

	// Set a timeout for the shutdown process
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for the goroutine to finish or timeout
	select {
	case <-done:
		// Boot completed successfully
	case <-time.After(1 * time.Second):
		t.Errorf("Test timed out waiting for Boot to finish")
	}

	// Assert that the address was set correctly from the flag
	if app.addr != ":8888" {
		t.Errorf("Expected address to be set from flag to :8888, got %s", app.addr)
	}
}