package pkg

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
	addr = flag.String("addr", ":8080", "Server address to listen on")
	env  = flag.String("env", ".env", "Path to .env file")
)

type App struct {
	mux    *http.ServeMux
	router *router.Router
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

// New creates a new App instance with the provided options
// Defaults are applied if no options are provided
func New(options ...Option) *App {
	app := &App{
		// Set default values if needed
		mux:    http.NewServeMux(),
		router: router.NewRouter(),
	}

	// Apply all provided options
	for _, opt := range options {
		opt(app)
	}

	return app
}

// Boot initializes and starts the application
// It sets up the router with the mux and starts the HTTP server
func (a *App) Boot() {
	// Setup router with mux
	glog.Init()
	defer glog.Flush()
	defer glog.Close()

	// Load.env file if it exists
	if _, err := os.Stat(*env); err == nil {
		glog.Infof("Load .env file: %s", *env)
		err := config.LoadEnv(*env, true)
		if err != nil {
			glog.Fatalf("Load .env failed: %v", err)
		}
	}

	a.router.Init()
	a.mux.HandleFunc("/", middleware.Apply(a.router.ServeHTTP, middleware.Logging, middleware.Auth))
	if os.Getenv("APP_DEBUG") == "true" {
		a.router.Print(os.Stdout)
	}

	server := &http.Server{
		Addr:    *addr,
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

	glog.Infof("Server running on %s", *addr)
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
