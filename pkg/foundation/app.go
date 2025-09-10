package foundation

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spcent/golang_simple_server/pkg/config"
	"github.com/spcent/golang_simple_server/pkg/glog"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
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

// Handle websocket request
func (a *App) Websocket(path ...string) {
	wsPath := "/ws"
	if len(path) > 0 {
		wsPath = path[0]
	}

	hub := NewHub()
	a.mux.HandleFunc(wsPath, func(w http.ResponseWriter, r *http.Request) {
		ServeWS(w, r, func(c *Conn) {
			hub.Register(c)
			// on close, unregister
			go func() {
				<-c.closeC
				hub.Unregister(c)
			}()
			// custom read handler: broadcast text messages to hub
			go c.readPump(func(op byte, payload []byte) {
				if op == opcodeText {
					// broadcast
					hub.Broadcast(payload)
				}
			}, func() {
				// on close
			})
		})
	})
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

	a.router.Init()
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

func (a *App) Logging() middleware.Middleware {
	return middleware.Logging
}

func (a *App) Auth() middleware.Middleware {
	return middleware.Auth
}
