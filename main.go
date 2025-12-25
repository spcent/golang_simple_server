package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spcent/golang_simple_server/handlers"
	"github.com/spcent/golang_simple_server/pkg/core"
	"github.com/spcent/golang_simple_server/pkg/frontend"
	log "github.com/spcent/golang_simple_server/pkg/log"
	"github.com/spcent/golang_simple_server/pkg/net/webhookout"
	"github.com/spcent/golang_simple_server/pkg/pubsub"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	envFile := flag.String("env", ".env", "Path to .env file")
	tlsEnabled := flag.Bool("tls", false, "Enable TLS support")
	tlsCertFile := flag.String("tls-cert", "./cert.pem", "Path to TLS certificate file")
	tlsKeyFile := flag.String("tls-key", "./key.pem", "Path to TLS private key file")
	debug := flag.Bool("debug", false, "Enable debug mode")
	frontendDir := flag.String("frontend-dir", "", "Path to built frontend assets (Next.js export, etc.)")
	gracefulTimeout := flag.Duration("graceful-timeout", 5*time.Second, "Graceful shutdown timeout")
	flag.Parse()

	// Shared in-proc pubsub for webhook receivers and debug endpoint.
	pub := pubsub.New()

	webhookCfg := webhookout.ConfigFromEnv()
	webhookSvc := webhookout.NewService(webhookout.NewMemStore(), webhookCfg)
	if webhookCfg.Enabled {
		webhookSvc.Start(context.Background())
		defer webhookSvc.Stop()
	}

	// Create a new app with configuration from flags
	opts := []core.Option{
		core.WithAddr(*addr),
		core.WithEnvPath(*envFile),
		core.WithShutdownTimeout(*gracefulTimeout),
		core.WithPubSub(pub),
		core.WithPubSubDebug(core.PubSubDebugConfig{Enabled: true}),
		core.WithWebhookOut(core.WebhookOutConfig{Enabled: webhookCfg.Enabled, Service: webhookSvc, IncludeStats: true}),
		core.WithWebhookIn(core.WebhookInConfig{Enabled: true}),
	}

	if *tlsEnabled {
		opts = append(opts, core.WithTLS(*tlsCertFile, *tlsKeyFile))
	}

	if *debug {
		opts = append(opts, core.WithDebug())
	}

	app := core.New(opts...)
	l := app.Logger()

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
		l.Error("Failed to configure WebSocket", log.Fields{"error": err})
	}

	// Mount a built Node/Next.js frontend when provided via flag or env.
	frontendDirValue := strings.TrimSpace(*frontendDir)
	if frontendDirValue == "" {
		frontendDirValue = strings.TrimSpace(os.Getenv("FRONTEND_DIR"))
	}

	if frontendDirValue != "" {
		if err := frontend.RegisterFromDir(app.Router(), frontendDirValue, frontend.WithCacheControl("public, max-age=31536000")); err != nil {
			l.Error("Failed to mount frontend", log.Fields{"error": err, "dir": frontendDirValue})
		} else {
			l.Info("Frontend mounted", log.Fields{"dir": frontendDirValue})
		}
	} else if frontend.HasEmbedded() {
		if err := frontend.RegisterEmbedded(app.Router(), frontend.WithCacheControl("public, max-age=31536000")); err != nil {
			l.Error("Failed to mount embedded frontend", log.Fields{"error": err})
		} else {
			l.Info("Embedded frontend mounted", nil)
		}
	}

	// Register routes via handlers package
	handlers.RegisterRoutes(app.Router())
	app.ConfigurePubSubDebug()
	app.ConfigureWebhookOut()
	app.ConfigureWebhookIn()

	// Apply middleware using more elegant methods
	app.EnableLogging()
	// Add rate limiting: 10 requests per second with burst up to 20
	app.EnableRateLimit(10, 20)

	// Boot the application
	// HTTPS can also be enabled via command line flags:
	// ./simple -tls -tls-cert ./cert.pem -tls-key ./key.pem
	if err := app.Boot(); err != nil {
		l.Error("Failed to boot application", log.Fields{"error": err})
	}
}
