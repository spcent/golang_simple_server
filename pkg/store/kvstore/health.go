package kvstore

import (
	"fmt"
	"sync/atomic"
	"time"
)

// -------------------- Health Check --------------------
// Stats provides comprehensive runtime statistics about the KV store
type Stats struct {
	Entries             int64         `json:"entries"`              // Number of entries currently in the store
	Hits                int64         `json:"hits"`                 // Number of successful read operations
	Misses              int64         `json:"misses"`               // Number of failed read operations (key not found or expired)
	Evictions           int64         `json:"evictions"`            // Number of entries evicted due to LRU or memory limits
	TTLCleanups         int64         `json:"ttl_cleanups"`         // Number of entries cleaned up due to TTL expiration
	WALSize             int64         `json:"wal_size"`             // Current size of the WAL file in bytes
	MemoryUsage         int64         `json:"memory_usage"`         // Current memory usage in bytes
	LastSnapshot        time.Time     `json:"last_snapshot"`        // Timestamp of the last snapshot operation
	LastFlush           time.Time     `json:"last_flush"`           // Timestamp of the last WAL flush operation
	WALFlushLatency     time.Duration `json:"wal_flush_latency"`    // Average WAL flush latency
	SnapshotSize        int64         `json:"snapshot_size"`        // Size of the last snapshot
	ExpiredKeysPerSec   float64       `json:"expired_keys_per_sec"` // Rate of key expiration
	HitRatio            float64       `json:"hit_ratio"`            // Cache hit ratio
	MemoryFragmentation float64       `json:"memory_fragmentation"` // Memory fragmentation ratio
	WALFlushCount       int64         `json:"wal_flush_count"`      // Total WAL flush operations
	CompactionCount     int64         `json:"compaction_count"`     // Total compaction operations
}

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

func (kv *KVStore) GetStats() Stats {
	stats := kv.stats.Load().(*Stats)
	newStats := *stats
	newStats.Hits = atomic.LoadInt64(&kv.hitCount)
	newStats.Misses = atomic.LoadInt64(&kv.missCount)
	newStats.Evictions = atomic.LoadInt64(&kv.evictions)
	newStats.TTLCleanups = atomic.LoadInt64(&kv.ttlCleanups)
	newStats.CompactionCount = atomic.LoadInt64(&kv.compactionCount)

	// Calculate derived metrics
	if newStats.Hits+newStats.Misses > 0 {
		newStats.HitRatio = float64(newStats.Hits) / float64(newStats.Hits+newStats.Misses)
	}

	if flushCount := atomic.LoadInt64(&kv.flushCount); flushCount > 0 {
		newStats.WALFlushLatency = time.Duration(atomic.LoadInt64(&kv.flushLatencySum) / flushCount)
	}

	return newStats
}
