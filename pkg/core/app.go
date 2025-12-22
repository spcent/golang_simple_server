package core

import (
	"context"
	"flag"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spcent/golang_simple_server/pkg/config"
	glog "github.com/spcent/golang_simple_server/pkg/log"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	ws "github.com/spcent/golang_simple_server/pkg/net/websocket"
	"github.com/spcent/golang_simple_server/pkg/router"
)

var (
	addr      = flag.String("addr", "", "Server address to listen on")
	env       = flag.String("env", "", "Path to .env file")
	tlsCert   = flag.String("tls-cert", "", "Path to TLS certificate file")
	tlsKey    = flag.String("tls-key", "", "Path to TLS private key file")
	enableTLS = flag.Bool("tls", false, "Enable HTTPS support")
)

type App struct {
	addr      string         // bind address
	envFile   string         // path to .env file
	mux       *http.ServeMux // http serve mux for router
	router    *router.Router // router for http request
	wsHub     *ws.Hub        // WebSocket hub
	started   bool           // Whether the app has started
	tlsCert   string         // Path to TLS certificate file
	tlsKey    string         // Path to TLS private key file
	enableTLS bool           // Whether to enable TLS
}

// Option defines a function type for configuring the App
// It follows the functional options pattern

type Option func(*App)

// WithMux sets the http.ServeMux for the App
func WithMux(mux *http.ServeMux) Option {
	return func(a *App) {
		a.mux = mux
	}
}

// WithRouter sets the router for the App
func WithRouter(router *router.Router) Option {
	return func(a *App) {
		a.router = router
	}
}

// WithAddr sets the server address
func WithAddr(address string) Option {
	return func(a *App) {
		a.addr = address
	}
}

// WithEnvPath sets the path to the .env file
func WithEnvPath(path string) Option {
	return func(a *App) {
		a.envFile = path
	}
}

// WithTLS enables HTTPS support with the specified certificate and key files
func WithTLS(certFile, keyFile string) Option {
	return func(a *App) {
		a.enableTLS = true
		a.tlsCert = certFile
		a.tlsKey = keyFile
	}
}

// EnableTLS enables HTTPS support
func EnableTLS(a *App) {
	a.enableTLS = true
}

// WithTLSCert sets the path to the TLS certificate file
func WithTLSCert(certFile string) Option {
	return func(a *App) {
		a.tlsCert = certFile
		a.enableTLS = true
	}
}

// WithTLSKey sets the path to the TLS private key file
func WithTLSKey(keyFile string) Option {
	return func(a *App) {
		a.tlsKey = keyFile
		a.enableTLS = true
	}
}

// New creates a new App instance with the provided options
// Defaults are applied if no options are provided
func New(options ...Option) *App {
	app := &App{
		// Set default values if needed
		mux:     http.NewServeMux(),
		router:  router.NewRouter(),
		addr:    ":8080",
		envFile: ".env",
	}

	// Apply all provided options
	for _, opt := range options {
		opt(app)
	}

	return app
}

// HandleFunc registers a handler function for the given path
// It's a wrapper around http.ServeMux.HandleFunc
func (a *App) HandleFunc(pattern string, handler http.HandlerFunc) {
	a.mux.HandleFunc(pattern, handler)
}

// Handle registers a handler for the given path
// It's a wrapper around http.ServeMux.Handle
func (a *App) Handle(pattern string, handler http.Handler) {
	a.mux.Handle(pattern, handler)
}

// Get registers a GET route with the given handler
func (a *App) Get(path string, handler http.HandlerFunc) {
	a.Router().GetFunc(path, handler)
}

// Post registers a POST route with the given handler
func (a *App) Post(path string, handler http.HandlerFunc) {
	a.Router().PostFunc(path, handler)
}

// Put registers a PUT route with the given handler
func (a *App) Put(path string, handler http.HandlerFunc) {
	a.Router().PutFunc(path, handler)
}

// Delete registers a DELETE route with the given handler
func (a *App) Delete(path string, handler http.HandlerFunc) {
	a.Router().DeleteFunc(path, handler)
}

// Patch registers a PATCH route with the given handler
func (a *App) Patch(path string, handler http.HandlerFunc) {
	a.Router().PatchFunc(path, handler)
}

// Any registers a route for any HTTP method with the given handler
func (a *App) Any(path string, handler http.HandlerFunc) {
	a.Router().AnyFunc(path, handler)
}

// GetHandler registers a GET route with the router's Handler type
func (a *App) GetHandler(path string, handler router.Handler) {
	a.Router().Get(path, handler)
}

// PostHandler registers a POST route with the router's Handler type
func (a *App) PostHandler(path string, handler router.Handler) {
	a.Router().Post(path, handler)
}

// PutHandler registers a PUT route with the router's Handler type
func (a *App) PutHandler(path string, handler router.Handler) {
	a.Router().Put(path, handler)
}

// DeleteHandler registers a DELETE route with the router's Handler type
func (a *App) DeleteHandler(path string, handler router.Handler) {
	a.Router().Delete(path, handler)
}

// PatchHandler registers a PATCH route with the router's Handler type
func (a *App) PatchHandler(path string, handler router.Handler) {
	a.Router().Patch(path, handler)
}

// AnyHandler registers a route for any HTTP method with the router's Handler type
func (a *App) AnyHandler(path string, handler router.Handler) {
	a.Router().Any(path, handler)
}

// Use applies middleware to all routes
// It accepts both Middleware and FuncMiddleware types for flexibility
func (a *App) Use(middlewares ...any) {
	// Convert all middlewares to Middleware type
	typedMiddlewares := make([]middleware.Middleware, 0, len(middlewares))

	for _, mw := range middlewares {
		switch v := mw.(type) {
		case middleware.Middleware:
			typedMiddlewares = append(typedMiddlewares, v)
		case func(http.Handler) http.Handler:
			// Convert http.Handler middleware to Middleware using wrapper
			typedMiddlewares = append(typedMiddlewares, func(h middleware.Handler) middleware.Handler {
				// Convert middleware.Handler to http.Handler
				return middleware.Handler(v(http.Handler(h)))
			})
		case func(http.HandlerFunc) http.HandlerFunc:
			// Convert old FuncMiddleware to new Middleware
			typedMiddlewares = append(typedMiddlewares, middleware.FromFuncMiddleware(v))
		default:
			panic("invalid middleware type")
		}
	}

	// Create a middleware chain
	chain := middleware.NewChain(typedMiddlewares...)

	// Create a handler that tries the router first, then the mux
	combinedHandler := func(w http.ResponseWriter, r *http.Request) {
		// Create a response recorder to check if the router handled the request
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusNotFound}

		// Try the router first
		a.router.ServeHTTP(rr, r)

		// If the router didn't handle the request (status 404), try the mux
		if rr.statusCode == http.StatusNotFound {
			// Use the mux's handlers directly
			a.mux.ServeHTTP(w, r)
		}
	}

	// Apply middleware chain to the combined handler
	wrappedHandler := chain.ApplyFunc(combinedHandler)

	// Register the wrapped handler to the root
	// This ensures all requests go through our middleware chain
	a.mux.HandleFunc("/", wrappedHandler)
}

// responseRecorder is a wrapper around http.ResponseWriter that records the status code
// It's used to check if the router handled the request

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// Router returns the underlying router for advanced configuration
func (a *App) Router() *router.Router {
	return a.router
}

// Boot initializes and starts the application
// It sets up the router with the mux and starts the HTTP server
func (a *App) Boot() {
	// Setup router with mux
	glog.Init()
	defer glog.Flush()
	defer glog.Close()

	// Load.env file if it exists
	if env != nil && *env != "" {
		a.envFile = *env
	}
	if _, err := os.Stat(a.envFile); err == nil {
		glog.Infof("Load .env file: %s", a.envFile)
		err := config.LoadEnv(a.envFile, true)
		if err != nil {
			glog.Fatalf("Load .env failed: %v", err)
		}
	}

	// Middleware is applied through the Use() method
	if os.Getenv("APP_DEBUG") == "true" {
		a.router.Print(os.Stdout)
	}

	if addr != nil && *addr != "" {
		a.addr = *addr
	}

	// Apply TLS flags if provided
	if enableTLS != nil && *enableTLS {
		a.enableTLS = true
	}
	if tlsCert != nil && *tlsCert != "" {
		a.tlsCert = *tlsCert
		a.enableTLS = true
	}
	if tlsKey != nil && *tlsKey != "" {
		a.tlsKey = *tlsKey
		a.enableTLS = true
	}

	server := &http.Server{
		Addr:    a.addr,
		Handler: a.mux,
	}

	// Mark app as started
	a.started = true

	// Shutdown the server gracefully when SIGTERM is received
	idleConnsClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		glog.Info("SIGTERM received, shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			glog.Errorf("Server shutdown error: %v", err)
		}
		close(idleConnsClosed)
	}()

	glog.Infof("Server running on %s", a.addr)
	var err error
	if a.enableTLS {
		if a.tlsCert == "" || a.tlsKey == "" {
			glog.Fatal("TLS enabled but certificate or key file not provided")
			return
		}
		glog.Infof("HTTPS enabled, using certificate: %s", a.tlsCert)
		err = server.ListenAndServeTLS(a.tlsCert, a.tlsKey)
	} else {
		err = server.ListenAndServe()
	}
	if err != http.ErrServerClosed {
		glog.Errorf("Server error: %v", err)
	}

	<-idleConnsClosed

	// Stop WebSocket hub if it was created
	if a.wsHub != nil {
		glog.Info("Stopping WebSocket hub...")
		a.wsHub.Stop()
		a.wsHub = nil
	}

	glog.Info("Server stopped gracefully")
}

// WebSocketConfig defines the configuration for WebSocket
type WebSocketConfig struct {
	WorkerCount   int             // Number of worker goroutines
	JobQueueSize  int             // Size of the job queue
	SendQueueSize int             // Size of the send queue per connection
	SendTimeout   time.Duration   // Timeout for sending messages
	SendBehavior  ws.SendBehavior // Behavior when queue is full or timeout occurs
	Secret        []byte          // Secret key for JWT authentication
	WSRoutePath   string          // Path for WebSocket connection
	BroadcastPath string          // Path for broadcasting messages
}

// DefaultWebSocketConfig returns default WebSocket configuration
func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		WorkerCount:   16,
		JobQueueSize:  4096,
		SendQueueSize: 256,
		SendTimeout:   200 * time.Millisecond,
		SendBehavior:  ws.SendBlock,
		Secret:        []byte("change-this-secret"),
		WSRoutePath:   "/ws",
		BroadcastPath: "/_admin/broadcast",
	}
}

// ConfigureWebSocket configures WebSocket support for the app
// It returns the Hub for advanced usage
func (a *App) ConfigureWebSocket() *ws.Hub {
	return a.ConfigureWebSocketWithOptions(DefaultWebSocketConfig())
}

// ConfigureWebSocketWithOptions configures WebSocket support with custom options
func (a *App) ConfigureWebSocketWithOptions(config WebSocketConfig) *ws.Hub {
	// Create hub and auth
	hub := ws.NewHub(config.WorkerCount, config.JobQueueSize)
	a.wsHub = hub
	wsAuth := ws.NewSimpleRoomAuth(config.Secret)

	// Register WebSocket handler
	a.HandleFunc(config.WSRoutePath, func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWSWithAuth(w, r, hub, wsAuth, config.SendQueueSize,
			config.SendTimeout, config.SendBehavior)
	})

	// Register broadcast endpoint
	a.HandleFunc(config.BroadcastPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}

		hub.BroadcastAll(ws.OpcodeText, b)
		w.WriteHeader(http.StatusNoContent)
	})

	return hub
}

// EnableLogging enables the logging middleware
func (a *App) EnableLogging() {
	a.Use(middleware.FromFuncMiddleware(middleware.Logging))
}

// EnableAuth enables the auth middleware
func (a *App) EnableAuth() {
	a.Use(middleware.FromFuncMiddleware(middleware.Auth))
}

// EnableRateLimit enables the rate limiting middleware with the given configuration
// rate: requests per second
// capacity: maximum burst size
func (a *App) EnableRateLimit(rate float64, capacity int) {
	a.Use(middleware.RateLimit(rate, capacity, time.Minute, 5*time.Minute))
}

// EnableCORS enables the CORS middleware
func (a *App) EnableCORS() {
	a.Use(middleware.CORS)
}

// EnableRecovery enables the recovery middleware
func (a *App) EnableRecovery() {
	a.Use(middleware.RecoveryMiddleware)
}
