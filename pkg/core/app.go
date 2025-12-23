package core

import (
	"context"
	"flag"
	"fmt"
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

// Command line flags
var (
	// TLS flags
	tlsEnabled  = flag.Bool("tls", false, "Enable TLS support")
	tlsCertFile = flag.String("tls-cert", "./cert.pem", "Path to TLS certificate file")
	tlsKeyFile  = flag.String("tls-key", "./key.pem", "Path to TLS private key file")

	// Server address flag
	addrFlag = flag.String("addr", ":8080", "Server address")

	// Debug mode flag
	debugFlag = flag.Bool("debug", false, "Enable debug mode")
)

// TLSConfig defines TLS configuration
type TLSConfig struct {
	Enabled  bool   // Whether to enable TLS
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

// AppConfig defines application configuration
type AppConfig struct {
	Addr    string    // Server address
	EnvFile string    // Path to .env file
	TLS     TLSConfig // TLS configuration
	Debug   bool      // Debug mode
}

// App represents the main application instance
type App struct {
	config      AppConfig               // Application configuration
	mux         *http.ServeMux          // HTTP serve mux
	router      *router.Router          // HTTP router
	wsHub       *ws.Hub                 // WebSocket hub
	started     bool                    // Whether the app has started
	httpServer  *http.Server            // HTTP server instance
	middlewares []middleware.Middleware // Stored middleware for all routes
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
		a.config.Addr = address
	}
}

// WithEnvPath sets the path to the .env file
func WithEnvPath(path string) Option {
	return func(a *App) {
		a.config.EnvFile = path
	}
}

// WithTLS configures TLS for the app
func WithTLS(certFile, keyFile string) Option {
	return func(a *App) {
		a.config.TLS = TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  keyFile,
		}
	}
}

// WithTLSConfig sets the TLS configuration for the App
func WithTLSConfig(tlsConfig TLSConfig) Option {
	return func(a *App) {
		a.config.TLS = tlsConfig
	}
}

// New creates a new App instance with the provided options
// Defaults are applied if no options are provided
func New(options ...Option) *App {
	app := &App{
		config: AppConfig{
			Addr:    ":8080",
			EnvFile: ".env",
			TLS:     TLSConfig{Enabled: false},
			Debug:   false,
		},
		mux:    http.NewServeMux(),
		router: router.NewRouter(),
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

// Use adds middleware to the application's middleware chain
func (a *App) Use(middlewares ...middleware.Middleware) {
	// Collect middleware
	a.middlewares = append(a.middlewares, middlewares...)
	
	// Apply middleware immediately for testing purposes and to ensure latest middleware is used
	chain := middleware.NewChain(a.middlewares...)
	
	combinedHandler := func(w http.ResponseWriter, r *http.Request) {
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusNotFound}
		a.router.ServeHTTP(rr, r)
		
		if rr.statusCode == http.StatusNotFound {
			a.mux.ServeHTTP(w, r)
		}
	}
	
	wrappedHandler := chain.ApplyFunc(combinedHandler)
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
func (a *App) Boot() error {
	// Parse command line flags
	flag.Parse()

	// Initialize logger
	glog.Init()
	defer glog.Flush()
	defer glog.Close()

	// Load environment variables from .env file if it exists
	if err := a.loadEnv(); err != nil {
		return err
	}

	// Update configuration based on command line flags
	if *addrFlag != ":8080" {
		a.config.Addr = *addrFlag
	}
	if *tlsEnabled {
		a.config.TLS.Enabled = true
		a.config.TLS.CertFile = *tlsCertFile
		a.config.TLS.KeyFile = *tlsKeyFile
	}
	if *debugFlag {
		a.config.Debug = true
		os.Setenv("APP_DEBUG", "true")
	}

	// Setup HTTP server
	if err := a.setupServer(); err != nil {
		return err
	}

	// Start HTTP server
	if err := a.startServer(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// loadEnv loads environment variables from .env file if it exists
func (a *App) loadEnv() error {
	if _, err := os.Stat(a.config.EnvFile); err == nil {
		glog.Infof("Load .env file: %s", a.config.EnvFile)
		err := config.LoadEnv(a.config.EnvFile, true)
		if err != nil {
			glog.Errorf("Load .env failed: %v", err)
			return err
		}
	}
	return nil
}

// setupServer sets up the HTTP server with the configured settings
func (a *App) setupServer() error {
	// Print registered routes if debug mode is enabled
	if os.Getenv("APP_DEBUG") == "true" {
		a.router.Print(os.Stdout)
	}

	// Apply all accumulated middleware at once
	chain := middleware.NewChain(a.middlewares...)

	combinedHandler := func(w http.ResponseWriter, r *http.Request) {
		rr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusNotFound}
		a.router.ServeHTTP(rr, r)

		if rr.statusCode == http.StatusNotFound {
			a.mux.ServeHTTP(w, r)
		}
	}

	wrappedHandler := chain.ApplyFunc(combinedHandler)

	// Update the middleware handler regardless of whether it's been registered before
	// This ensures the latest middleware chain is always applied
	a.mux.HandleFunc("/", wrappedHandler)

	// Create HTTP server instance
	a.httpServer = &http.Server{
		Addr:    a.config.Addr,
		Handler: a.mux,
	}

	return nil
}

// startServer starts the HTTP server and handles graceful shutdown
func (a *App) startServer() error {
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

		if err := a.httpServer.Shutdown(ctx); err != nil {
			glog.Errorf("Server shutdown error: %v", err)
		}
		close(idleConnsClosed)
	}()

	glog.Infof("Server running on %s", a.config.Addr)

	var err error
	if a.config.TLS.Enabled {
		if a.config.TLS.CertFile == "" || a.config.TLS.KeyFile == "" {
			glog.Errorf("TLS enabled but certificate or key file not provided")
			return fmt.Errorf("TLS enabled but certificate or key file not provided")
		}
		glog.Infof("HTTPS enabled, using certificate: %s", a.config.TLS.CertFile)
		err = a.httpServer.ListenAndServeTLS(a.config.TLS.CertFile, a.config.TLS.KeyFile)
	} else {
		err = a.httpServer.ListenAndServe()
	}

	<-idleConnsClosed

	// Stop WebSocket hub if it was created
	if a.wsHub != nil {
		glog.Info("Stopping WebSocket hub...")
		a.wsHub.Stop()
		a.wsHub = nil
	}

	glog.Info("Server stopped gracefully")
	return err
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
	// Get secret from environment variable or use default (for development only)
	secret := []byte(os.Getenv("WS_SECRET"))
	if len(secret) == 0 {
		secret = []byte("change-this-secret")
	}

	return WebSocketConfig{
		WorkerCount:   16,
		JobQueueSize:  4096,
		SendQueueSize: 256,
		SendTimeout:   200 * time.Millisecond,
		SendBehavior:  ws.SendBlock,
		Secret:        secret,
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
	// Convert CORS middleware from func(http.Handler) http.Handler to middleware.Middleware
	a.Use(middleware.FromHTTPHandlerMiddleware(middleware.CORS))
}

// EnableRecovery enables the recovery middleware
func (a *App) EnableRecovery() {
	// Convert http.Handler middleware to Middleware type
	a.Use(middleware.FromHTTPHandlerMiddleware(middleware.RecoveryMiddleware))
}
