package kvstore

import (
	"bufio"
	"compress/gzip"
	"container/list"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	ErrKeyExists          = errors.New("key already exists")
	ErrKeyNotFound        = errors.New("key not found")
	ErrKeyExpired         = errors.New("key expired")
	ErrStoreClosed        = errors.New("store is closed")
	ErrInvalidEntry       = errors.New("invalid WAL entry")
	ErrCloseTimeout       = errors.New("close operation timed out")
	ErrTransactionAborted = errors.New("transaction aborted")
	ErrInvalidPath        = errors.New("invalid file path")
	ErrPermissionDenied   = errors.New("permission denied")
)

const (
	opCreate byte = 1
	opUpdate byte = 2
	opDelete byte = 3

	// Entry format: [CRC32(4)] [Op(1)] [ExpireAt(8)] [KeyLen(4)] [ValueLen(4)] [Key] [Value]
	entryHeaderSize = 4 + 1 + 8 + 4 + 4 // 21 bytes

	defaultWALSize       = 64 * 1024 * 1024 // 64MB
	defaultBatchSize     = 128
	defaultFlushInterval = 50 * time.Millisecond
	defaultCleanInterval = 30 * time.Second
	defaultShardCount    = 16
	defaultCloseTimeout  = 30 * time.Second

	magicNumber uint32 = 0x4B565354 // "KVST"
	version     uint32 = 2

	maxStatsUpdateRetries = 100
)

// Memory pools for reducing GC pressure
var (
	entryPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}

	stringSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]string, 0, 64)
		},
	}
)

// Durability controls when WAL writes are synced to disk
type Durability int

const (
	// DurabilityNever - WAL writes are never explicitly synced (fastest, least durable)
	DurabilityNever Durability = iota
	// DurabilityEveryFlush - WAL writes are synced after each batch flush
	DurabilityEveryFlush
	// DurabilityPerWrite - WAL writes are synced after each individual write (slowest, most durable)
	DurabilityPerWrite
)

// CompressionType defines the compression algorithm
type CompressionType int

const (
	CompressionNone CompressionType = iota
	CompressionGzip
)

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

type valueWithTTL struct {
	Key      string
	Value    []byte
	ExpireAt time.Time
	Size     int64 // for memory usage tracking
	Version  int64 // for transaction support
}

type walEntry struct {
	Op       byte
	ExpireAt int64
	Key      []byte
	Value    []byte
	Version  int64
}

// Options configures the behavior of the KV store
type Options struct {
	// WALPath is the file path for the Write-Ahead Log
	WALPath string

	// SnapshotPath is the file path for snapshots (defaults to WALPath + ".snapshot")
	SnapshotPath string

	// LogBufferSize is the size of the in-memory log buffer for batching WAL writes
	LogBufferSize int

	// MaxEntries limits the maximum number of entries in the store (0 = unlimited)
	MaxEntries int

	// MaxMemoryBytes limits the maximum memory usage in bytes (0 = unlimited)
	MaxMemoryBytes int64

	// CleanInterval is how often to run the TTL cleanup process
	CleanInterval time.Duration

	// FlushInterval is how often to flush buffered WAL entries to disk
	FlushInterval time.Duration

	// BatchSize is the maximum number of entries to batch before forcing a WAL flush
	BatchSize int

	// Durability controls when WAL writes are synced to disk
	Durability Durability

	// EnableMetrics enables collection of runtime statistics
	EnableMetrics bool

	// ReadOnly opens the store in read-only mode (no WAL writes)
	ReadOnly bool

	// EnableCompression enables data compression in snapshots and WAL
	EnableCompression bool

	// CompressionType specifies the compression algorithm
	CompressionType CompressionType

	// CompressionLevel controls compression trade-off (1=fast, 9=best compression)
	CompressionLevel int

	// ShardCount enables sharding to reduce lock contention (must be power of 2)
	ShardCount int

	// CloseTimeout is the maximum time to wait for graceful shutdown
	CloseTimeout time.Duration

	// EnableTransactions enables simple transaction support
	EnableTransactions bool
}

// Shard represents a single shard of the key-value store
type Shard struct {
	mu      sync.RWMutex
	data    map[string]*list.Element
	lruList *list.List
}

// KVStore is a high-performance, persistent key-value store with LRU eviction and TTL support
type KVStore struct {
	// Core data structures (sharded for better concurrency)
	shards    []*Shard // Sharded data structures
	shardMask uint32   // For fast modulo operation

	// WAL components
	walFile   *os.File     // Write-Ahead Log file handle
	walMmap   []byte       // Memory-mapped WAL file
	walOffset int64        // Current write offset in WAL
	walSize   int64        // Total size of WAL file
	walMutex  sync.RWMutex // Protects WAL operations

	// Background workers
	logChan       chan walEntry      // Channel for batching WAL writes
	cleanupChan   chan string        // Channel for expired key cleanup
	ctx           context.Context    // Context for coordinating shutdown
	cancel        context.CancelFunc // Cancel function for shutdown
	wg            sync.WaitGroup     // WaitGroup for background goroutines
	workerPool    chan struct{}      // Worker pool for async operations
	cleanupWorker chan struct{}      // Cleanup worker semaphore

	// Configuration
	opts Options

	// Metrics (using atomic operations for lock-free access)
	stats            atomic.Value // *Stats - current statistics
	hitCount         int64        // Atomic counter for cache hits
	missCount        int64        // Atomic counter for cache misses
	evictions        int64        // Atomic counter for evictions
	ttlCleanups      int64        // Atomic counter for TTL cleanups
	flushCount       int64        // Atomic counter for WAL flushes
	compactionCount  int64        // Atomic counter for compactions
	lastFlushLatency int64        // Last flush latency in nanoseconds
	globalVersion    int64        // Global version counter for transactions

	// State
	closed int32 // Atomic flag indicating if store is closed

	// Performance tracking
	flushLatencySum int64 // Sum of flush latencies for averaging
}

// Transaction represents a simple transaction
type Transaction struct {
	kv         *KVStore
	operations []txnOp
	readSet    map[string]int64 // key -> version
	startTime  time.Time
	mu         sync.Mutex
}

type txnOp struct {
	op    byte
	key   string
	value []byte
	ttl   int64
}

// NewKVStore creates a new KV store with comprehensive error handling and validation
func NewKVStore(opts Options) (*KVStore, error) {
	if err := validateOptions(&opts); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	setDefaultOptions(&opts)

	// Initialize shards
	shardCount := opts.ShardCount
	shards := make([]*Shard, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = &Shard{
			data:    make(map[string]*list.Element),
			lruList: list.New(),
		}
	}

	kv := &KVStore{
		shards:        shards,
		shardMask:     uint32(shardCount - 1),
		logChan:       make(chan walEntry, opts.LogBufferSize),
		cleanupChan:   make(chan string, opts.LogBufferSize),
		workerPool:    make(chan struct{}, runtime.NumCPU()),
		cleanupWorker: make(chan struct{}, 1),
		opts:          opts,
	}

	kv.ctx, kv.cancel = context.WithCancel(context.Background())
	kv.stats.Store(&Stats{})

	// Initialize WAL
	if !opts.ReadOnly {
		if err := kv.initWAL(); err != nil {
			return nil, fmt.Errorf("failed to initialize WAL: %w", err)
		}

		// Start background workers
		kv.wg.Add(3)
		go kv.asyncLogWriter()
		go kv.ttlCleaner()
		go kv.asyncCleanupWorker()
	}

	// Load existing data
	if err := kv.loadFromSnapshot(); err != nil {
		kv.Close()
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}

	if err := kv.replayWAL(); err != nil {
		kv.Close()
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	return kv, nil
}

func validateOptions(opts *Options) error {
	if opts.WALPath == "" {
		return errors.New("WAL path is required")
	}

	// Check path permissions
	if dir := filepath.Dir(opts.WALPath); dir != "." {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("%w: cannot create directory %s", ErrPermissionDenied, dir)
			}
		}
	}

	if opts.LogBufferSize <= 0 {
		return errors.New("log buffer size must be positive")
	}

	if opts.BatchSize <= 0 {
		return errors.New("batch size must be positive")
	}

	if opts.FlushInterval < time.Millisecond {
		return errors.New("flush interval too small (minimum 1ms)")
	}

	if opts.MaxEntries < 0 {
		return errors.New("max entries cannot be negative")
	}

	if opts.MaxMemoryBytes < 0 {
		return errors.New("max memory bytes cannot be negative")
	}

	if opts.ShardCount <= 0 || (opts.ShardCount&(opts.ShardCount-1)) != 0 {
		return errors.New("shard count must be a power of 2")
	}

	if opts.EnableCompression && (opts.CompressionLevel < 1 || opts.CompressionLevel > 9) {
		return errors.New("compression level must be between 1 and 9")
	}

	return nil
}

func setDefaultOptions(opts *Options) {
	if opts.LogBufferSize == 0 {
		opts.LogBufferSize = 1000
	}
	if opts.CleanInterval == 0 {
		opts.CleanInterval = defaultCleanInterval
	}
	if opts.FlushInterval == 0 {
		opts.FlushInterval = defaultFlushInterval
	}
	if opts.BatchSize == 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.ShardCount == 0 {
		opts.ShardCount = defaultShardCount
	}
	if opts.SnapshotPath == "" {
		opts.SnapshotPath = opts.WALPath + ".snapshot"
	}
	if opts.CompressionLevel == 0 {
		opts.CompressionLevel = 6
	}
	if opts.CloseTimeout == 0 {
		opts.CloseTimeout = defaultCloseTimeout
	}
}

// -------------------- Sharding Utilities --------------------

func (kv *KVStore) getShard(key string) *Shard {
	hash := crc32.ChecksumIEEE([]byte(key))
	return kv.shards[hash&kv.shardMask]
}

// -------------------- WAL Management --------------------

func (kv *KVStore) initWAL() error {
	flag := os.O_CREATE | os.O_RDWR
	if kv.opts.ReadOnly {
		flag = os.O_RDONLY
	}

	file, err := os.OpenFile(kv.opts.WALPath, flag, 0644)
	if err != nil {
		return fmt.Errorf("failed to open WAL file: %w", err)
	}
	kv.walFile = file

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat WAL file: %w", err)
	}

	size := info.Size()
	if size == 0 {
		size = defaultWALSize
		if !kv.opts.ReadOnly {
			if err = file.Truncate(size); err != nil {
				return fmt.Errorf("failed to truncate WAL file: %w", err)
			}
		}
	}

	prot := syscall.PROT_READ
	if !kv.opts.ReadOnly {
		prot |= syscall.PROT_WRITE
	}

	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(size), prot, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap WAL file: %w", err)
	}

	kv.walMmap = mmap
	kv.walSize = size
	kv.walOffset = 0

	return nil
}

func (kv *KVStore) expandWAL(minSize int) error {
	if kv.opts.ReadOnly {
		return errors.New("cannot expand WAL in read-only mode")
	}

	newSize := kv.walSize*2 + int64(minSize)

	// Unmap current mapping
	if err := syscall.Munmap(kv.walMmap); err != nil {
		return fmt.Errorf("failed to unmap WAL: %w", err)
	}

	// Expand file
	if err := kv.walFile.Truncate(newSize); err != nil {
		return fmt.Errorf("failed to expand WAL file: %w", err)
	}

	// Create new mapping
	mmap, err := syscall.Mmap(int(kv.walFile.Fd()), 0, int(newSize),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to remap expanded WAL: %w", err)
	}

	kv.walMmap = mmap
	kv.walSize = newSize
	return nil
}

// -------------------- In-Memory Operations --------------------

func (kv *KVStore) insertInMemory(shard *Shard, key string, value []byte, expireAt time.Time) {
	size := int64(len(key) + len(value) + int(unsafe.Sizeof(valueWithTTL{})))
	version := atomic.AddInt64(&kv.globalVersion, 1)

	if elem, ok := shard.data[key]; ok {
		// Update existing
		oldVal := elem.Value.(valueWithTTL)
		elem.Value = valueWithTTL{
			Key:      key,
			Value:    append([]byte(nil), value...), // defensive copy
			ExpireAt: expireAt,
			Size:     size,
			Version:  version,
		}
		shard.lruList.MoveToFront(elem)
		kv.updateStats(func(s *Stats) { s.MemoryUsage += size - oldVal.Size })
		return
	}

	// Check limits before insertion
	if kv.opts.MaxEntries > 0 && kv.getTotalEntries() >= int64(kv.opts.MaxEntries) {
		kv.evictLRU(shard)
	}

	if kv.opts.MaxMemoryBytes > 0 {
		for kv.getMemoryUsage()+size > kv.opts.MaxMemoryBytes && shard.lruList.Len() > 0 {
			kv.evictLRU(shard)
		}
	}

	// Insert new element
	val := valueWithTTL{
		Key:      key,
		Value:    append([]byte(nil), value...), // defensive copy
		ExpireAt: expireAt,
		Size:     size,
		Version:  version,
	}
	elem := shard.lruList.PushFront(val)
	shard.data[key] = elem

	kv.updateStats(func(s *Stats) {
		s.Entries++
		s.MemoryUsage += size
	})
}

func (kv *KVStore) deleteInMemory(shard *Shard, key string) bool {
	elem, ok := shard.data[key]
	if !ok {
		return false
	}

	val := elem.Value.(valueWithTTL)
	shard.lruList.Remove(elem)
	delete(shard.data, key)

	kv.updateStats(func(s *Stats) {
		s.Entries--
		s.MemoryUsage -= val.Size
	})

	return true
}

func (kv *KVStore) evictLRU(shard *Shard) {
	back := shard.lruList.Back()
	if back == nil {
		return
	}

	val := back.Value.(valueWithTTL)
	shard.lruList.Remove(back)
	delete(shard.data, val.Key)

	atomic.AddInt64(&kv.evictions, 1)
	kv.updateStats(func(s *Stats) {
		s.Entries--
		s.Evictions++
		s.MemoryUsage -= val.Size
	})

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(val.Key)})
	}
}

func (kv *KVStore) enqueueLog(e walEntry) {
	if kv.isClosed() {
		return
	}

	select {
	case kv.logChan <- e:
	case <-kv.ctx.Done():
	default:
		// Use worker pool for non-blocking fallback
		select {
		case kv.workerPool <- struct{}{}:
			go func() {
				defer func() { <-kv.workerPool }()
				select {
				case kv.logChan <- e:
				case <-kv.ctx.Done():
				}
			}()
		default:
			// Drop if worker pool is full
		}
	}
}

// -------------------- Async WAL Writer --------------------

func (kv *KVStore) asyncLogWriter() {
	defer kv.wg.Done()

	batch := make([]walEntry, 0, kv.opts.BatchSize)
	ticker := time.NewTicker(kv.opts.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case e := <-kv.logChan:
			batch = append(batch, e)
			if len(batch) >= kv.opts.BatchSize {
				if err := kv.flushBatch(batch); err != nil {
					// Log error but continue - could use proper logging here
					fmt.Printf("WAL flush error: %v\n", err)
				}
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				if err := kv.flushBatch(batch); err != nil {
					fmt.Printf("WAL flush error: %v\n", err)
				}
				batch = batch[:0]
			}

		case <-kv.ctx.Done():
			if len(batch) > 0 {
				kv.flushBatch(batch)
			}
			return
		}
	}
}

func (kv *KVStore) flushBatch(batch []walEntry) error {
	if len(batch) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		latency := time.Since(start)
		atomic.StoreInt64(&kv.lastFlushLatency, latency.Nanoseconds())
		atomic.AddInt64(&kv.flushLatencySum, latency.Nanoseconds())
		atomic.AddInt64(&kv.flushCount, 1)
	}()

	kv.walMutex.Lock()
	defer kv.walMutex.Unlock()

	for _, e := range batch {
		entryBytes := kv.encodeEntry(e)

		// Check if we need to expand WAL
		if kv.walOffset+int64(len(entryBytes)) > kv.walSize {
			if err := kv.expandWAL(len(entryBytes)); err != nil {
				return fmt.Errorf("failed to expand WAL: %w", err)
			}
		}

		copy(kv.walMmap[kv.walOffset:], entryBytes)
		kv.walOffset += int64(len(entryBytes))
	}

	// Sync based on durability setting
	var syncErr error
	switch kv.opts.Durability {
	case DurabilityPerWrite:
		syncErr = kv.syncWAL()
	case DurabilityEveryFlush:
		syncErr = kv.syncWAL()
	}

	kv.updateStats(func(s *Stats) {
		s.WALSize = kv.walOffset
		s.LastFlush = time.Now()
		s.WALFlushCount++
		if flushCount := atomic.LoadInt64(&kv.flushCount); flushCount > 0 {
			s.WALFlushLatency = time.Duration(atomic.LoadInt64(&kv.flushLatencySum) / flushCount)
		}
	})

	return syncErr
}

func (kv *KVStore) syncWAL() error {
	if len(kv.walMmap) == 0 {
		return nil
	}

	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		addr := uintptr(unsafe.Pointer(&kv.walMmap[0]))
		length := uintptr(len(kv.walMmap))
		_, _, errno := syscall.Syscall(syscall.SYS_MSYNC, addr, length, syscall.MS_SYNC)
		if errno != 0 {
			return kv.walFile.Sync()
		}
	default:
		return kv.walFile.Sync()
	}

	return nil
}

// -------------------- TTL Cleaner --------------------

func (kv *KVStore) ttlCleaner() {
	defer kv.wg.Done()

	ticker := time.NewTicker(kv.opts.CleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			kv.cleanExpiredKeys()
		case <-kv.ctx.Done():
			return
		}
	}
}

func (kv *KVStore) cleanExpiredKeys() {
	now := time.Now()
	startTime := now

	for _, shard := range kv.shards {
		expired := stringSlicePool.Get().([]string)
		expired = expired[:0]

		shard.mu.RLock()
		for key, elem := range shard.data {
			val := elem.Value.(valueWithTTL)
			if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
				expired = append(expired, key)
			}
		}
		shard.mu.RUnlock()

		if len(expired) > 0 {
			shard.mu.Lock()
			for _, key := range expired {
				if elem, ok := shard.data[key]; ok {
					val := elem.Value.(valueWithTTL)
					if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
						kv.deleteInMemory(shard, key)
						if !kv.opts.ReadOnly {
							select {
							case kv.cleanupChan <- key:
							default:
								// If cleanup channel is full, enqueue directly
								kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
							}
						}
					}
				}
			}
			shard.mu.Unlock()

			atomic.AddInt64(&kv.ttlCleanups, int64(len(expired)))
		}

		stringSlicePool.Put(expired)
	}

	// Update expiration rate
	elapsed := time.Since(startTime)
	if elapsed > 0 {
		cleanupCount := atomic.LoadInt64(&kv.ttlCleanups)
		rate := float64(cleanupCount) / elapsed.Seconds()
		kv.updateStats(func(s *Stats) {
			s.TTLCleanups = cleanupCount
			s.ExpiredKeysPerSec = rate
		})
	}
}

func (kv *KVStore) asyncCleanupWorker() {
	defer kv.wg.Done()

	for {
		select {
		case key := <-kv.cleanupChan:
			if !kv.opts.ReadOnly {
				kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
			}
		case <-kv.ctx.Done():
			return
		}
	}
}

// -------------------- Snapshot and Recovery --------------------

func (kv *KVStore) Snapshot() error {
	if kv.isClosed() || kv.opts.ReadOnly {
		return ErrStoreClosed
	}

	tempPath := kv.opts.SnapshotPath + ".tmp"
	f, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer f.Close()

	var writer io.Writer = bufio.NewWriter(f)

	// Add compression if enabled
	if kv.opts.EnableCompression && kv.opts.CompressionType == CompressionGzip {
		gzWriter, gzErr := gzip.NewWriterLevel(writer, kv.opts.CompressionLevel)
		if gzErr != nil {
			return fmt.Errorf("failed to create gzip writer: %w", gzErr)
		}
		defer gzWriter.Close()
		writer = gzWriter
	}

	// Write magic number and version
	if err = binary.Write(writer, binary.LittleEndian, magicNumber); err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}
	if err = binary.Write(writer, binary.LittleEndian, version); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	// Count total entries
	totalEntries := kv.getTotalEntries()
	if err = binary.Write(writer, binary.LittleEndian, uint32(totalEntries)); err != nil {
		return fmt.Errorf("failed to write entry count: %w", err)
	}

	// Write entries from all shards
	for _, shard := range kv.shards {
		shard.mu.RLock()
		for key, elem := range shard.data {
			val := elem.Value.(valueWithTTL)
			entry := walEntry{
				Op:       opCreate,
				ExpireAt: val.ExpireAt.UnixNano(),
				Key:      []byte(key),
				Value:    val.Value,
				Version:  val.Version,
			}
			entryBytes := kv.encodeEntry(entry)
			if _, err = writer.Write(entryBytes); err != nil {
				shard.mu.RUnlock()
				return fmt.Errorf("failed to write entry: %w", err)
			}
		}
		shard.mu.RUnlock()
	}

	// Flush and sync
	if bufWriter, ok := writer.(*bufio.Writer); ok {
		if err = bufWriter.Flush(); err != nil {
			return fmt.Errorf("failed to flush snapshot: %w", err)
		}
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("failed to sync snapshot: %w", err)
	}

	// Get snapshot size
	info, _ := f.Stat()
	snapshotSize := info.Size()

	// Atomic replace
	if err = os.Rename(tempPath, kv.opts.SnapshotPath); err != nil {
		return fmt.Errorf("failed to rename snapshot: %w", err)
	}

	// Reset WAL
	kv.walMutex.Lock()
	defer kv.walMutex.Unlock()

	if err = syscall.Munmap(kv.walMmap); err != nil {
		return fmt.Errorf("failed to unmap WAL: %w", err)
	}

	if err = kv.walFile.Truncate(defaultWALSize); err != nil {
		return fmt.Errorf("failed to truncate WAL: %w", err)
	}

	mmap, err := syscall.Mmap(int(kv.walFile.Fd()), 0, defaultWALSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to remap WAL: %w", err)
	}

	kv.walMmap = mmap
	kv.walSize = defaultWALSize
	kv.walOffset = 0

	atomic.AddInt64(&kv.compactionCount, 1)
	kv.updateStats(func(s *Stats) {
		s.LastSnapshot = time.Now()
		s.WALSize = 0
		s.SnapshotSize = snapshotSize
		s.CompactionCount++
	})

	return nil
}

func (kv *KVStore) loadFromSnapshot() error {
	f, err := os.Open(kv.opts.SnapshotPath)
	if os.IsNotExist(err) {
		return nil // No snapshot exists
	}
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer f.Close()

	var reader io.Reader = bufio.NewReader(f)

	// Try to detect compression
	if kv.opts.EnableCompression {
		gzReader, err := gzip.NewReader(reader)
		if err == nil {
			defer gzReader.Close()
			reader = gzReader
		}
		// If gzip fails, continue with uncompressed reader
	}

	// Check magic number
	var magic, ver, count uint32
	if err := binary.Read(reader, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("failed to read magic number: %w", err)
	}
	if magic != magicNumber {
		return errors.New("invalid snapshot file")
	}

	if err := binary.Read(reader, binary.LittleEndian, &ver); err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if ver < 1 || ver > version {
		return fmt.Errorf("unsupported snapshot version: %d", ver)
	}

	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return fmt.Errorf("failed to read entry count: %w", err)
	}

	for i := uint32(0); i < count; i++ {
		entry, err := kv.decodeEntry(reader)
		if err != nil {
			return fmt.Errorf("failed to decode entry %d: %w", i, err)
		}

		if entry.Op == opCreate {
			var expireAt time.Time
			if entry.ExpireAt > 0 {
				expireAt = time.Unix(0, entry.ExpireAt)
			}

			shard := kv.getShard(string(entry.Key))
			shard.mu.Lock()
			kv.insertInMemory(shard, string(entry.Key), entry.Value, expireAt)
			shard.mu.Unlock()
		}
	}

	return nil
}

func (kv *KVStore) replayWAL() error {
	// Fixed: Properly scan entire WAL to find actual end offset
	reader := &mmapReader{data: kv.walMmap}
	var lastValidOffset int64

	for reader.offset < len(reader.data) {
		startOffset := reader.offset
		entry, err := kv.decodeEntry(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			// If we encounter corruption, stop at last valid entry
			if err == ErrInvalidEntry {
				break
			}
			return fmt.Errorf("WAL replay failed: %w", err)
		}

		lastValidOffset = int64(reader.offset)

		key := string(entry.Key)
		var expireAt time.Time
		if entry.ExpireAt > 0 {
			expireAt = time.Unix(0, entry.ExpireAt)
		}

		shard := kv.getShard(key)
		shard.mu.Lock()
		switch entry.Op {
		case opCreate, opUpdate:
			kv.insertInMemory(shard, key, entry.Value, expireAt)
		case opDelete:
			kv.deleteInMemory(shard, key)
		}
		shard.mu.Unlock()

		// Prevent infinite loop on corrupted data
		if reader.offset <= startOffset {
			break
		}
	}

	kv.walOffset = lastValidOffset
	return nil
}

// -------------------- Statistics and Utilities --------------------

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

func (kv *KVStore) updateStats(fn func(*Stats)) {
	if !kv.opts.EnableMetrics {
		return
	}

	for i := 0; i < maxStatsUpdateRetries; i++ {
		old := kv.stats.Load().(*Stats)
		newStats := *old
		fn(&newStats)
		if kv.stats.CompareAndSwap(old, &newStats) {
			return
		}
		if i%10 == 9 {
			runtime.Gosched() // Yield CPU periodically
		}
	}
	// Failed to update after max retries - acceptable for non-critical stats
}

func (kv *KVStore) getMemoryUsage() int64 {
	stats := kv.stats.Load().(*Stats)
	return stats.MemoryUsage
}

func (kv *KVStore) getTotalEntries() int64 {
	var total int64
	for _, shard := range kv.shards {
		shard.mu.RLock()
		total += int64(len(shard.data))
		shard.mu.RUnlock()
	}
	return total
}

func (kv *KVStore) isClosed() bool {
	return atomic.LoadInt32(&kv.closed) != 0
}

func (kv *KVStore) cleanup() error {
	var lastErr error

	if kv.walMmap != nil {
		if err := syscall.Munmap(kv.walMmap); err != nil {
			lastErr = fmt.Errorf("failed to unmap WAL: %w", err)
		}
	}

	if kv.walFile != nil {
		if err := kv.walFile.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close WAL file: %w", err)
		}
	}

	return lastErr
}

// -------------------- Encoding/Decoding --------------------

func (kv *KVStore) encodeEntry(e walEntry) []byte {
	keyLen := uint32(len(e.Key))
	valLen := uint32(len(e.Value))

	buf := entryPool.Get().([]byte)
	buf = buf[:0] // Reset length but keep capacity

	// Ensure sufficient capacity
	needed := entryHeaderSize + 8 + len(e.Key) + len(e.Value) // +8 for version
	if cap(buf) < needed {
		buf = make([]byte, 0, needed)
	}

	buf = buf[:needed]

	// Skip CRC32 for now, will calculate at end
	offset := 4

	buf[offset] = e.Op
	offset++

	binary.LittleEndian.PutUint64(buf[offset:], uint64(e.ExpireAt))
	offset += 8

	binary.LittleEndian.PutUint32(buf[offset:], keyLen)
	offset += 4

	binary.LittleEndian.PutUint32(buf[offset:], valLen)
	offset += 4

	// Version (new in v2)
	binary.LittleEndian.PutUint64(buf[offset:], uint64(e.Version))
	offset += 8

	copy(buf[offset:], e.Key)
	offset += len(e.Key)

	copy(buf[offset:], e.Value)

	// Calculate and set CRC32
	crc := crc32.ChecksumIEEE(buf[4:])
	binary.LittleEndian.PutUint32(buf[0:], crc)

	// Make a copy to return since buf will be reused
	result := make([]byte, len(buf))
	copy(result, buf)

	entryPool.Put(buf)
	return result
}

func (kv *KVStore) decodeEntry(r io.Reader) (walEntry, error) {
	header := make([]byte, entryHeaderSize+8) // +8 for version in v2
	if _, err := io.ReadFull(r, header); err != nil {
		return walEntry{}, err
	}

	crc := binary.LittleEndian.Uint32(header[0:])
	op := header[4]
	expireAt := int64(binary.LittleEndian.Uint64(header[5:]))
	keyLen := binary.LittleEndian.Uint32(header[13:])
	valLen := binary.LittleEndian.Uint32(header[17:])
	version := int64(binary.LittleEndian.Uint64(header[21:]))

	// Read key and value
	data := make([]byte, keyLen+valLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return walEntry{}, err
	}

	// Verify CRC
	checkData := make([]byte, entryHeaderSize-4+8+keyLen+valLen) // +8 for version
	copy(checkData, header[4:])
	copy(checkData[entryHeaderSize-4+8:], data)

	expectedCRC := crc32.ChecksumIEEE(checkData)
	if crc != expectedCRC {
		return walEntry{}, ErrInvalidEntry
	}

	key := data[:keyLen]
	value := data[keyLen:]

	return walEntry{
		Op:       op,
		ExpireAt: expireAt,
		Key:      key,
		Value:    value,
		Version:  version,
	}, nil
}
