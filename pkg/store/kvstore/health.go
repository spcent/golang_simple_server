package kvstore

import (
	"fmt"
	"time"
)

// -------------------- Health Check --------------------

// Health represents the health status of the store
type Health struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Stats     Stats     `json:"stats"`
	Errors    []string  `json:"errors,omitempty"`
}

// HealthCheck performs a comprehensive health check
func (kv *KVStore) HealthCheck() Health {
	health := Health{
		Status:    "healthy",
		Timestamp: time.Now(),
		Stats:     kv.GetStats(),
	}

	var errors []string

	// Check if store is closed
	if kv.isClosed() {
		health.Status = "unhealthy"
		errors = append(errors, "store is closed")
	}

	// Check WAL file accessibility
	if kv.walFile != nil {
		if _, err := kv.walFile.Stat(); err != nil {
			health.Status = "degraded"
			errors = append(errors, fmt.Sprintf("WAL file error: %v", err))
		}
	}

	// Check memory usage
	if kv.opts.MaxMemoryBytes > 0 {
		usage := kv.getMemoryUsage()
		if float64(usage)/float64(kv.opts.MaxMemoryBytes) > 0.9 {
			health.Status = "degraded"
			errors = append(errors, "memory usage over 90%")
		}
	}

	// Check for high error rates
	stats := kv.GetStats()
	if stats.Hits+stats.Misses > 100 && stats.HitRatio < 0.5 {
		health.Status = "degraded"
		errors = append(errors, "low cache hit ratio")
	}

	health.Errors = errors
	return health
}
