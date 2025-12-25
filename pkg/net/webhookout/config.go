package webhookout

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type DropPolicy string

const (
	DropNewest     DropPolicy = "drop_newest"
	BlockWithLimit DropPolicy = "block_timeout"
	FailFast       DropPolicy = "fail_fast"
)

type Config struct {
	Enabled bool

	QueueSize  int
	Workers    int
	DrainMax   time.Duration
	DropPolicy DropPolicy
	BlockWait  time.Duration

	DefaultTimeout    time.Duration
	DefaultMaxRetries int
	BackoffBase       time.Duration
	BackoffMax        time.Duration
	RetryOn429        bool

	// For safety (recommended)
	AllowPrivateNetwork bool
}

func ConfigFromEnv() Config {
	cfg := Config{
		Enabled:    envBool("WEBHOOK_ENABLED", true),
		QueueSize:  envInt("WEBHOOK_QUEUE_SIZE", 2048),
		Workers:    envInt("WEBHOOK_WORKERS", 8),
		DrainMax:   envDurationMs("WEBHOOK_DRAIN_MAX_MS", 5000),
		DropPolicy: DropPolicy(envString("WEBHOOK_DROP_POLICY", string(BlockWithLimit))),
		BlockWait:  envDurationMs("WEBHOOK_BLOCK_WAIT_MS", 50),

		DefaultTimeout:    envDurationMs("WEBHOOK_DEFAULT_TIMEOUT_MS", 5000),
		DefaultMaxRetries: envInt("WEBHOOK_DEFAULT_MAX_RETRIES", 6),
		BackoffBase:       envDurationMs("WEBHOOK_BACKOFF_BASE_MS", 500),
		BackoffMax:        envDurationMs("WEBHOOK_BACKOFF_MAX_MS", 30000),
		RetryOn429:        envBool("WEBHOOK_RETRY_ON_429", true),

		AllowPrivateNetwork: envBool("WEBHOOK_ALLOW_PRIVATE_NET", false),
	}

	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.BlockWait < 0 {
		cfg.BlockWait = 0
	}
	switch cfg.DropPolicy {
	case DropNewest, BlockWithLimit, FailFast:
	default:
		cfg.DropPolicy = BlockWithLimit
	}
	return cfg
}

func envString(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func envDurationMs(key string, defMs int) time.Duration {
	return time.Duration(envInt(key, defMs)) * time.Millisecond
}
