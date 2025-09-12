package foundation

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
	"github.com/spcent/golang_simple_server/pkg/glog"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
	ws "github.com/spcent/golang_simple_server/pkg/websocket"
)

var (
	addr = flag.String("addr", "", "Server address to listen on")
	env  = flag.String("env", "", "Path to .env file")
)

type App struct {
	addr    string         // bind address
	envFile string         // path to .env file
	mux     *http.ServeMux // http serve mux for router
	router  *router.Router // router for http request
	wsHub   *ws.Hub        // WebSocket hub
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

// Use applies middleware to the router
func (a *App) Use(middlewares ...middleware.Middleware) {
	// Apply middleware to the router's ServeHTTP method
	handler := a.router.ServeHTTP
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middleware.Apply(handler, middlewares[i])
	}
	a.mux.HandleFunc("/", handler)
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
	server := &http.Server{
		Addr:    a.addr,
		Handler: a.mux,
	}

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
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		glog.Errorf("Server error: %v", err)
	}

	<-idleConnsClosed
	glog.Info("Server stopped gracefully")
}

// WebSocketConfig defines the configuration for WebSocket
// WebSocket配置选项
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
// 默认WebSocket配置
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
// 配置WebSocket支持，返回Hub用于高级操作
func (a *App) ConfigureWebSocket() *ws.Hub {
	return a.ConfigureWebSocketWithOptions(DefaultWebSocketConfig())
}

// ConfigureWebSocketWithOptions configures WebSocket support with custom options
// 配置WebSocket支持（自定义选项）
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

// Logging returns the logging middleware
func (a *App) Logging() middleware.Middleware {
	return middleware.Logging
}

func (a *App) Auth() middleware.Middleware {
	return middleware.Auth
}
