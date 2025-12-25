package core

import (
	"time"

	"github.com/spcent/golang_simple_server/pkg/net/webhookin"
	webhookout "github.com/spcent/golang_simple_server/pkg/net/webhookout"
	"github.com/spcent/golang_simple_server/pkg/pubsub"
)

// TLSConfig defines TLS configuration.
type TLSConfig struct {
	Enabled  bool   // Whether to enable TLS
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

// PubSubDebugConfig configures the optional pubsub snapshot endpoint.
type PubSubDebugConfig struct {
	Enabled bool
	Path    string
	Pub     pubsub.PubSub
}

// WebhookOutConfig configures the outbound webhook management endpoints.
type WebhookOutConfig struct {
	Enabled bool
	Service *webhookout.Service

	// TriggerToken protects event triggering. When empty and AllowEmptyToken is false, triggers are forbidden.
	TriggerToken     string
	AllowEmptyToken  bool
	BasePath         string
	IncludeStats     bool
	DefaultPageLimit int
}

// WebhookInConfig configures inbound webhook receivers.
type WebhookInConfig struct {
	Enabled bool
	Pub     pubsub.PubSub

	GitHubSecret      string
	StripeSecret      string
	MaxBodyBytes      int64
	StripeTolerance   time.Duration
	DedupTTL          time.Duration
	Deduper           *webhookin.Deduper
	GitHubPath        string
	StripePath        string
	TopicPrefixGitHub string
	TopicPrefixStripe string
}

// AppConfig defines application configuration.
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

	PubSubDebug PubSubDebugConfig
	WebhookOut  WebhookOutConfig
	WebhookIn   WebhookInConfig
}
