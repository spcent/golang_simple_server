package main

import (
	"flag"
	"net/http"
	"time"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/core"
	log "github.com/spcent/golang_simple_server/pkg/log"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	envFile := flag.String("env", ".env", "Path to .env file")
	tlsEnabled := flag.Bool("tls", false, "Enable TLS support")
	tlsCertFile := flag.String("tls-cert", "./cert.pem", "Path to TLS certificate file")
	tlsKeyFile := flag.String("tls-key", "./key.pem", "Path to TLS private key file")
	debug := flag.Bool("debug", false, "Enable debug mode")
	gracefulTimeout := flag.Duration("graceful-timeout", 5*time.Second, "Graceful shutdown timeout")
	flag.Parse()

	// Create a new app with configuration from flags
	opts := []core.Option{
		core.WithAddr(*addr),
		core.WithEnvPath(*envFile),
		core.WithShutdownTimeout(*gracefulTimeout),
	}

	if *tlsEnabled {
		opts = append(opts, core.WithTLS(*tlsCertFile, *tlsKeyFile))
	}

	if *debug {
		opts = append(opts, core.WithDebug())
	}

	app := core.New(opts...)

	// Register routes directly on the app using the new API
	app.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"pong"}`))
	})

	app.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Hello, World!"}`))
	})

	// Configure WebSocket with custom secret for JWT authentication
	if _, err := app.ConfigureWebSocket(); err != nil {
		app.Logger().Error("Failed to configure WebSocket", log.Fields{"error": err})
	}

	// Register routes via handlers package
	handlers.RegisterRoutes(app.Router())

	// Apply middleware using more elegant methods
	app.EnableLogging()
	// Add rate limiting: 10 requests per second with burst up to 20
	app.EnableRateLimit(10, 20)

	// Boot the application
	// HTTPS can also be enabled via command line flags:
	// ./simple -tls -tls-cert ./cert.pem -tls-key ./key.pem
	if err := app.Boot(); err != nil {
		app.Logger().Error("Failed to boot application", log.Fields{"error": err})
	}
}
