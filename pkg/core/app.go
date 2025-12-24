package core

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spcent/golang_simple_server/pkg/config"
	"github.com/spcent/golang_simple_server/pkg/health"
	log "github.com/spcent/golang_simple_server/pkg/log"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	ws "github.com/spcent/golang_simple_server/pkg/net/websocket"
	"github.com/spcent/golang_simple_server/pkg/router"
)

// TLSConfig defines TLS configuration
type TLSConfig struct {
	Enabled  bool   // Whether to enable TLS
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

// AppConfig defines application configuration
type AppConfig struct {
	Addr            string        // Server address
	EnvFile         string        // Path to .env file
	TLS             TLSConfig     // TLS configuration
	Debug           bool          // Debug mode
	ShutdownTimeout time.Duration // Graceful shutdown timeout
	// HTTP server hardening
	ReadTimeout       time.Duration // Maximum duration for reading the entire request, including the body
	ReadHeaderTimeout time.Duration // Maximum duration for reading the request headers (slowloris protection)
	WriteTimeout      time.Duration // Maximum duration before timing out writes of the response
	IdleTimeout       time.Duration // Maximum time to wait for the next request when keep-alives are enabled
	MaxHeaderBytes    int           // Maximum size of request headers
	EnableHTTP2       bool          // Whether to keep HTTP/2 support enabled
	DrainInterval     time.Duration // How often to log in-flight connection counts while draining
	MaxBodyBytes      int64         // Per-request body limit
	MaxConcurrency    int           // Maximum number of concurrent requests being served
	QueueDepth        int           // Maximum number of requests allowed to queue while waiting for a worker
	QueueTimeout      time.Duration // Maximum time a request can wait in the queue
}

// App represents the main application instance
type App struct {
	config        AppConfig               // Application configuration
	router        *router.Router          // HTTP router
	wsHub         *ws.Hub                 // WebSocket hub
	started       bool                    // Whether the app has started
	envLoaded     bool                    // Whether environment variables have been loaded
	httpServer    *http.Server            // HTTP server instance
	middlewares   []middleware.Middleware // Stored middleware for all routes
	handler       http.Handler            // Combined handler with middleware applied
	connTracker   *connectionTracker
	guardsApplied bool

	logger           log.StructuredLogger
	metricsCollector middleware.MetricsCollector
	tracer           middleware.Tracer
}

// Option defines a function type for configuring the App
// It follows the functional options pattern
type Option func(*App)

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

// WithShutdownTimeout sets graceful shutdown timeout
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(a *App) {
		a.config.ShutdownTimeout = timeout
	}
}

// WithServerTimeouts configures HTTP server timeouts for read, write and idle connections.
func WithServerTimeouts(read, readHeader, write, idle time.Duration) Option {
	return func(a *App) {
		a.config.ReadTimeout = read
		a.config.ReadHeaderTimeout = readHeader
		a.config.WriteTimeout = write
		a.config.IdleTimeout = idle
	}
}

// WithMaxHeaderBytes sets the maximum header size accepted by the server.
func WithMaxHeaderBytes(bytes int) Option {
	return func(a *App) {
		a.config.MaxHeaderBytes = bytes
	}
}

// WithMaxBodyBytes sets the default request body limit.
func WithMaxBodyBytes(bytes int64) Option {
	return func(a *App) {
		a.config.MaxBodyBytes = bytes
	}
}

// WithConcurrencyLimits sets the maximum concurrent requests and queueing strategy.
func WithConcurrencyLimits(maxConcurrent, queueDepth int, queueTimeout time.Duration) Option {
	return func(a *App) {
		a.config.MaxConcurrency = maxConcurrent
		a.config.QueueDepth = queueDepth
		a.config.QueueTimeout = queueTimeout
	}
}

// WithHTTP2 enables or disables HTTP/2 support when TLS is configured.
func WithHTTP2(enabled bool) Option {
	return func(a *App) {
		a.config.EnableHTTP2 = enabled
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

// WithDebug enables debug mode for the app
func WithDebug() Option {
	return func(a *App) {
		a.config.Debug = true
	}
}

// WithLogger sets a custom logger for the App.
func WithLogger(logger log.StructuredLogger) Option {
	return func(a *App) {
		if logger != nil {
			a.logger = logger
		}
	}
}

// WithMetricsCollector sets a metrics collector hook for observability middleware.
func WithMetricsCollector(collector middleware.MetricsCollector) Option {
	return func(a *App) {
		a.metricsCollector = collector
	}
}

// WithTracer sets a tracer hook for observability middleware.
func WithTracer(tracer middleware.Tracer) Option {
	return func(a *App) {
		a.tracer = tracer
	}
}

// New creates a new App instance with the provided options
// Defaults are applied if no options are provided
func New(options ...Option) *App {
	app := &App{
		config: AppConfig{
			Addr:              ":8080",
			EnvFile:           ".env",
			TLS:               TLSConfig{Enabled: false},
			Debug:             false,
			ShutdownTimeout:   5 * time.Second,
			ReadTimeout:       30 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1 MiB
			EnableHTTP2:       true,
			DrainInterval:     500 * time.Millisecond,
			MaxBodyBytes:      10 << 20, // 10 MiB
			MaxConcurrency:    256,
			QueueDepth:        512,
			QueueTimeout:      250 * time.Millisecond,
		},
		router: router.NewRouter(),
		logger: log.NewGLogger(),
	}

	// Apply all provided options
	for _, opt := range options {
		opt(app)
	}

	if app.router != nil {
		app.router.SetLogger(app.logger)
	}

	return app
}

// HandleFunc registers a handler function for the given path
func (a *App) HandleFunc(pattern string, handler http.HandlerFunc) {
	a.router.HandleFunc(router.ANY, pattern, handler)
}

// Handle registers a handler for the given path
func (a *App) Handle(pattern string, handler http.Handler) {
	a.router.Handle(router.ANY, pattern, handler)
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

// Context-aware registration helpers keep compatibility with net/http while exposing the unified Ctx.
func (a *App) GetCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().GetCtx(path, handler)
}

func (a *App) PostCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().PostCtx(path, handler)
}

func (a *App) PutCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().PutCtx(path, handler)
}

func (a *App) DeleteCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().DeleteCtx(path, handler)
}

func (a *App) PatchCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().PatchCtx(path, handler)
}

func (a *App) AnyCtx(path string, handler router.CtxHandlerFunc) {
	a.Router().AnyCtx(path, handler)
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
	if a.started {
		panic("cannot add middleware after app has started")
	}

	// Collect middleware for later handler construction during setupServer
	a.middlewares = append(a.middlewares, middlewares...)
}

func (a *App) applyGuardrails() {
	if a.guardsApplied {
		return
	}

	var guards []middleware.Middleware

	if a.config.MaxBodyBytes > 0 {
		guards = append(guards, middleware.BodyLimit(a.config.MaxBodyBytes, a.logger))
	}

	if a.config.MaxConcurrency > 0 {
		guards = append(guards, middleware.ConcurrencyLimit(a.config.MaxConcurrency, a.config.QueueDepth, a.config.QueueTimeout, a.logger))
	}

	if len(guards) > 0 {
		// Hardening middleware should execute before user-specified middleware.
		a.middlewares = append(guards, a.middlewares...)
	}

	a.guardsApplied = true
}

// buildHandler builds the combined handler with current middleware stack
func (a *App) buildHandler() {
	chain := middleware.NewChain(a.middlewares...)
	a.handler = chain.Apply(a.router)
}

// Router returns the underlying router for advanced configuration
func (a *App) Router() *router.Router {
	return a.router
}

// Logger returns the configured application logger.
func (a *App) Logger() log.StructuredLogger {
	return a.logger
}

// Boot initializes and starts the application
// It sets up the router with the mux and starts the HTTP server
func (a *App) Boot() error {
	if lifecycle, ok := a.logger.(log.Lifecycle); ok {
		if err := lifecycle.Start(context.Background()); err != nil {
			return err
		}
		defer lifecycle.Stop(context.Background())
	}

	// Load environment variables from .env file if it exists
	if err := a.loadEnv(); err != nil {
		return err
	}

	if a.config.Debug {
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
	if a.envLoaded || a.config.EnvFile == "" {
		return nil
	}

	if _, err := os.Stat(a.config.EnvFile); err == nil {
		a.logger.Info("Load .env file", log.Fields{"path": a.config.EnvFile})
		err := config.LoadEnv(a.config.EnvFile, true)
		if err != nil {
			a.logger.Error("Load .env failed", log.Fields{"error": err})
			return err
		}
	}

	a.envLoaded = true
	return nil
}

// setupServer sets up the HTTP server with the configured settings
func (a *App) setupServer() error {
	// Print registered routes if debug mode is enabled
	if os.Getenv("APP_DEBUG") == "true" {
		a.router.Print(os.Stdout)
	}

	a.applyGuardrails()
	a.buildHandler()

	// Create HTTP server instance
	a.httpServer = &http.Server{
		Addr:              a.config.Addr,
		Handler:           a.handler,
		ReadTimeout:       a.config.ReadTimeout,
		ReadHeaderTimeout: a.config.ReadHeaderTimeout,
		WriteTimeout:      a.config.WriteTimeout,
		IdleTimeout:       a.config.IdleTimeout,
		MaxHeaderBytes:    a.config.MaxHeaderBytes,
	}

	a.connTracker = newConnectionTracker(a.logger, a.config.DrainInterval)
	a.httpServer.ConnState = a.connTracker.track

	if !a.config.EnableHTTP2 {
		a.httpServer.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	return nil
}

// startServer starts the HTTP server and handles graceful shutdown
func (a *App) startServer() error {
	a.started = true

	// Mark the service as ready once the server is configured.
	health.SetReady()

	// Shutdown the server gracefully when SIGTERM is received
	idleConnsClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		health.SetNotReady("draining connections")
		a.logger.Info("SIGTERM received, shutting down", nil)

		timeout := a.config.ShutdownTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if a.connTracker != nil {
			go a.connTracker.drain(ctx)
		}

		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.logger.Error("Server shutdown error", log.Fields{"error": err})
		}
		close(idleConnsClosed)
	}()

	a.logger.Info("Server running", log.Fields{"addr": a.config.Addr})

	var err error
	if a.config.TLS.Enabled {
		if a.config.TLS.CertFile == "" || a.config.TLS.KeyFile == "" {
			a.logger.Error("TLS enabled but certificate or key file not provided", nil)
			return fmt.Errorf("TLS enabled but certificate or key file not provided")
		}
		a.logger.Info("HTTPS enabled", log.Fields{"cert": a.config.TLS.CertFile})
		err = a.httpServer.ListenAndServeTLS(a.config.TLS.CertFile, a.config.TLS.KeyFile)
	} else {
		err = a.httpServer.ListenAndServe()
	}

	// The server is shutting down; mark it as not ready for new traffic.
	health.SetNotReady("shutting down")

	<-idleConnsClosed

	// Stop WebSocket hub if it was created
	if a.wsHub != nil {
		a.logger.Info("Stopping WebSocket hub", nil)
		a.wsHub.Stop()
		a.wsHub = nil
	}

	a.logger.Info("Server stopped gracefully", nil)
	return err
}

type connectionTracker struct {
	active   atomic.Int64
	logger   log.StructuredLogger
	interval time.Duration
}

func newConnectionTracker(logger log.StructuredLogger, interval time.Duration) *connectionTracker {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &connectionTracker{logger: logger, interval: interval}
}

func (t *connectionTracker) track(_ net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		t.active.Add(1)
	case http.StateHijacked, http.StateClosed:
		t.active.Add(-1)
	}
}

func (t *connectionTracker) drain(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		if t.active.Load() <= 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if t.logger != nil {
				t.logger.Info("draining active connections", log.Fields{"active_connections": t.active.Load()})
			}
		}
	}
}

// WebSocketConfig defines the configuration for WebSocket
type WebSocketConfig struct {
	WorkerCount      int             // Number of worker goroutines
	JobQueueSize     int             // Size of the job queue
	SendQueueSize    int             // Size of the send queue per connection
	SendTimeout      time.Duration   // Timeout for sending messages
	SendBehavior     ws.SendBehavior // Behavior when queue is full or timeout occurs
	Secret           []byte          // Secret key for JWT authentication
	WSRoutePath      string          // Path for WebSocket connection
	BroadcastPath    string          // Path for broadcasting messages
	BroadcastEnabled bool            // Enable broadcast endpoint when true
}

// DefaultWebSocketConfig returns default WebSocket configuration
func DefaultWebSocketConfig() WebSocketConfig {
	// Get secret from environment variable. Must be provided explicitly.
	secret := []byte(os.Getenv("WS_SECRET"))

	return WebSocketConfig{
		WorkerCount:      16,
		JobQueueSize:     4096,
		SendQueueSize:    256,
		SendTimeout:      200 * time.Millisecond,
		SendBehavior:     ws.SendBlock,
		Secret:           secret,
		WSRoutePath:      "/ws",
		BroadcastPath:    "/_admin/broadcast",
		BroadcastEnabled: true,
	}
}

// ConfigureWebSocket configures WebSocket support for the app
// It returns the Hub for advanced usage
func (a *App) ConfigureWebSocket() (*ws.Hub, error) {
	if err := a.loadEnv(); err != nil {
		return nil, err
	}

	return a.ConfigureWebSocketWithOptions(DefaultWebSocketConfig())
}

// ConfigureWebSocketWithOptions configures WebSocket support with custom options
func (a *App) ConfigureWebSocketWithOptions(config WebSocketConfig) (*ws.Hub, error) {
	if len(config.Secret) == 0 {
		return nil, fmt.Errorf("websocket secret is required")
	}

	// Create hub and auth
	hub := ws.NewHub(config.WorkerCount, config.JobQueueSize)
	a.wsHub = hub
	wsAuth := ws.NewSimpleRoomAuth(config.Secret)

	// Register WebSocket handler
	a.Router().GetFunc(config.WSRoutePath, func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWSWithAuth(w, r, hub, wsAuth, config.SendQueueSize,
			config.SendTimeout, config.SendBehavior)
	})

	// Register broadcast endpoint
	if config.BroadcastEnabled && config.BroadcastPath != "" {
		a.Router().PostFunc(config.BroadcastPath, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			if !a.config.Debug {
				const bearerPrefix = "bearer "
				rawAuth := r.Header.Get("Authorization")
				var provided []byte
				if strings.HasPrefix(strings.ToLower(rawAuth), bearerPrefix) {
					provided = []byte(strings.TrimSpace(rawAuth[len("Bearer "):]))
				} else if q := r.URL.Query().Get("secret"); q != "" {
					provided = []byte(q)
				}

				if len(provided) == 0 || subtle.ConstantTimeCompare(provided, config.Secret) != 1 {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}

			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}

			hub.BroadcastAll(ws.OpcodeText, b)
			w.WriteHeader(http.StatusNoContent)
		})
	}

	return hub, nil
}

// EnableLogging enables the logging middleware
func (a *App) EnableLogging() {
	a.Use(middleware.Logging(a.logger, a.metricsCollector, a.tracer))
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
