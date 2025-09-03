package bootstrap

import (
	"bufio"
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spcent/golang_simple_server/pkg/glog"
	"github.com/spcent/golang_simple_server/pkg/middleware"
	"github.com/spcent/golang_simple_server/pkg/router"
)

var (
	addr = flag.String("addr", ":8080", "Server address to listen on")
	env  = flag.String("env", ".env", "Path to .env file")
)

func Boot(mux *http.ServeMux) {
	glog.Init()
	defer glog.Flush()
	defer glog.Close()

	// Load.env file if it exists
	if _, err := os.Stat(*env); err == nil {
		glog.Infof("Load .env file: %s", *env)
		err := LoadEnv(*env, true)
		if err != nil {
			glog.Fatalf("Load .env failed: %v", err)
		}
	}

	mux.HandleFunc("/", middleware.Apply(router.Handle, middleware.Logging, middleware.Auth))
	if os.Getenv("APP_DEBUG") == "true" {
		router.PrintRoutes()
	}

	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
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

// If overwrite=true, existing environment variables will be overwritten.
// If overwrite=false, existing environment variables will not be overwritten.
func LoadEnv(filepath string, overwrite bool) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if overwrite || os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
