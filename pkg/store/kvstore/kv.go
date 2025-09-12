package kvstore

import (
	"bufio"
	"container/list"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	ErrKeyExists    = errors.New("key already exists")
	ErrKeyNotFound  = errors.New("key not found")
	ErrKeyExpired   = errors.New("key expired")
	ErrStoreClosed  = errors.New("store is closed")
	ErrInvalidEntry = errors.New("invalid WAL entry")
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

	magicNumber uint32 = 0x4B565354 // "KVST"
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

// Stats provides runtime statistics about the KV store
type Stats struct {
	Entries      int64     // Number of entries currently in the store
	Hits         int64     // Number of successful read operations
	Misses       int64     // Number of failed read operations (key not found or expired)
	Evictions    int64     // Number of entries evicted due to LRU or memory limits
	TTLCleanups  int64     // Number of entries cleaned up due to TTL expiration
	WALSize      int64     // Current size of the WAL file in bytes
	MemoryUsage  int64     // Current memory usage in bytes
	LastSnapshot time.Time // Timestamp of the last snapshot operation
	LastFlush    time.Time // Timestamp of the last WAL flush operation
}

type valueWithTTL struct {
	Key      string
	Value    []byte
	ExpireAt time.Time
	Size     int64 // for memory usage tracking
}

type walEntry struct {
	Op       byte
	ExpireAt int64
	Key      []byte
	Value    []byte
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
}

// KVStore is a high-performance, persistent key-value store with LRU eviction and TTL support
type KVStore struct {
	// Core data structures
	mu      sync.RWMutex             // Protects data and lruList
	data    map[string]*list.Element // Map from key to list element
	lruList *list.List               // LRU list for eviction policy

	// WAL components
	walFile   *os.File     // Write-Ahead Log file handle
	walMmap   []byte       // Memory-mapped WAL file
	walOffset int64        // Current write offset in WAL
	walSize   int64        // Total size of WAL file
	walMutex  sync.RWMutex // Protects WAL operations

	// Background workers
	logChan chan walEntry      // Channel for batching WAL writes
	ctx     context.Context    // Context for coordinating shutdown
	cancel  context.CancelFunc // Cancel function for shutdown
	wg      sync.WaitGroup     // WaitGroup for background goroutines

	// Configuration
	opts Options

	// Metrics (using atomic operations for lock-free access)
	stats       atomic.Value // *Stats - current statistics
	hitCount    int64        // Atomic counter for cache hits
	missCount   int64        // Atomic counter for cache misses
	evictions   int64        // Atomic counter for evictions
	ttlCleanups int64        // Atomic counter for TTL cleanups

	// State
	closed int32 // Atomic flag indicating if store is closed
}

// NewKVStore creates a new KV store with improved error handling and validation
func NewKVStore(opts Options) (*KVStore, error) {
	if err := validateOptions(&opts); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	setDefaultOptions(&opts)

	kv := &KVStore{
		data:    make(map[string]*list.Element),
		lruList: list.New(),
		logChan: make(chan walEntry, opts.LogBufferSize),
		opts:    opts,
	}

	kv.ctx, kv.cancel = context.WithCancel(context.Background())
	kv.stats.Store(&Stats{})

	// Initialize WAL
	if !opts.ReadOnly {
		if err := kv.initWAL(); err != nil {
			return nil, fmt.Errorf("failed to initialize WAL: %w", err)
		}

		// Start background workers
		kv.wg.Add(2)
		go kv.asyncLogWriter()
		go kv.ttlCleaner()
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
	if opts.LogBufferSize <= 0 {
		return errors.New("log buffer size must be positive")
	}
	if opts.MaxEntries < 0 {
		return errors.New("max entries cannot be negative")
	}
	if opts.MaxMemoryBytes < 0 {
		return errors.New("max memory bytes cannot be negative")
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
	if opts.SnapshotPath == "" {
		opts.SnapshotPath = opts.WALPath + ".snapshot"
	}
}

// -------------------- WAL Management --------------------

func (kv *KVStore) initWAL() error {
	flag := os.O_CREATE | os.O_RDWR
	if kv.opts.ReadOnly {
		flag = os.O_RDONLY
	}

	file, err := os.OpenFile(kv.opts.WALPath, flag, 0644)
	if err != nil {
		return err
	}
	kv.walFile = file

	info, err := file.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	if size == 0 {
		size = defaultWALSize
		if !kv.opts.ReadOnly {
			if err = file.Truncate(size); err != nil {
				return err
			}
		}
	}

	prot := syscall.PROT_READ
	if !kv.opts.ReadOnly {
		prot |= syscall.PROT_WRITE
	}

	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(size), prot, syscall.MAP_SHARED)
	if err != nil {
		return err
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
		return err
	}

	// Expand file
	if err := kv.walFile.Truncate(newSize); err != nil {
		return err
	}

	// Create new mapping
	mmap, err := syscall.Mmap(int(kv.walFile.Fd()), 0, int(newSize),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	kv.walMmap = mmap
	kv.walSize = newSize
	return nil
}

// -------------------- In-Memory Operations --------------------

func (kv *KVStore) insertInMemory(key string, value []byte, expireAt time.Time) {
	size := int64(len(key) + len(value) + int(unsafe.Sizeof(valueWithTTL{})))

	if elem, ok := kv.data[key]; ok {
		// Update existing
		oldVal := elem.Value.(valueWithTTL)
		elem.Value = valueWithTTL{
			Key:      key,
			Value:    append([]byte(nil), value...), // defensive copy
			ExpireAt: expireAt,
			Size:     size,
		}
		kv.lruList.MoveToFront(elem)
		kv.updateStats(func(s *Stats) { s.MemoryUsage += size - oldVal.Size })
		return
	}

	// Check limits before insertion
	if kv.opts.MaxEntries > 0 && kv.lruList.Len() >= kv.opts.MaxEntries {
		kv.evictLRU()
	}

	if kv.opts.MaxMemoryBytes > 0 {
		for kv.getMemoryUsage()+size > kv.opts.MaxMemoryBytes && kv.lruList.Len() > 0 {
			kv.evictLRU()
		}
	}

	// Insert new element
	val := valueWithTTL{
		Key:      key,
		Value:    append([]byte(nil), value...), // defensive copy
		ExpireAt: expireAt,
		Size:     size,
	}
	elem := kv.lruList.PushFront(val)
	kv.data[key] = elem

	kv.updateStats(func(s *Stats) {
		s.Entries++
		s.MemoryUsage += size
	})
}

func (kv *KVStore) deleteInMemory(key string) bool {
	elem, ok := kv.data[key]
	if !ok {
		return false
	}

	val := elem.Value.(valueWithTTL)
	kv.lruList.Remove(elem)
	delete(kv.data, key)

	kv.updateStats(func(s *Stats) {
		s.Entries--
		s.MemoryUsage -= val.Size
	})

	return true
}

func (kv *KVStore) evictLRU() {
	back := kv.lruList.Back()
	if back == nil {
		return
	}

	val := back.Value.(valueWithTTL)
	kv.lruList.Remove(back)
	delete(kv.data, val.Key)

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
		// Non-blocking fallback
		go func() {
			select {
			case kv.logChan <- e:
			case <-kv.ctx.Done():
			}
		}()
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
					// Log error but continue
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
	switch kv.opts.Durability {
	case DurabilityPerWrite:
		if err := kv.syncWAL(); err != nil {
			return err
		}
	case DurabilityEveryFlush:
		if err := kv.syncWAL(); err != nil {
			return err
		}
	}

	kv.updateStats(func(s *Stats) {
		s.WALSize = kv.walOffset
		s.LastFlush = time.Now()
	})

	return nil
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
	var expired []string

	kv.mu.RLock()
	for key, elem := range kv.data {
		val := elem.Value.(valueWithTTL)
		if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
			expired = append(expired, key)
		}
	}
	kv.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	kv.mu.Lock()
	for _, key := range expired {
		if elem, ok := kv.data[key]; ok {
			val := elem.Value.(valueWithTTL)
			if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
				kv.deleteInMemory(key)
				if !kv.opts.ReadOnly {
					kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
				}
			}
		}
	}
	kv.mu.Unlock()

	atomic.AddInt64(&kv.ttlCleanups, int64(len(expired)))
	kv.updateStats(func(s *Stats) { s.TTLCleanups += int64(len(expired)) })
}

// -------------------- Public CRUD Operations --------------------

func (kv *KVStore) Create(key string, value []byte, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	if _, ok := kv.data[key]; ok {
		return ErrKeyExists
	}

	var expireAt time.Time
	if ttlSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	kv.insertInMemory(key, value, expireAt)

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opCreate,
			ExpireAt: expireAt.UnixNano(),
			Key:      []byte(key),
			Value:    value,
		})
	}

	return nil
}

func (kv *KVStore) Read(key string) ([]byte, error) {
	if kv.isClosed() {
		return nil, ErrStoreClosed
	}

	kv.mu.RLock()
	elem, ok := kv.data[key]
	kv.mu.RUnlock()

	if !ok {
		atomic.AddInt64(&kv.missCount, 1)
		kv.updateStats(func(s *Stats) { s.Misses++ })
		return nil, ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)

	// Check expiration
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		// Clean up expired key
		kv.mu.Lock()
		kv.deleteInMemory(key)
		if !kv.opts.ReadOnly {
			kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
		}
		kv.mu.Unlock()

		atomic.AddInt64(&kv.missCount, 1)
		kv.updateStats(func(s *Stats) { s.Misses++ })
		return nil, ErrKeyExpired
	}

	// Update LRU position
	kv.mu.Lock()
	kv.lruList.MoveToFront(elem)
	kv.mu.Unlock()

	atomic.AddInt64(&kv.hitCount, 1)
	kv.updateStats(func(s *Stats) { s.Hits++ })

	// Return defensive copy
	return append([]byte(nil), val.Value...), nil
}

func (kv *KVStore) Update(key string, value []byte, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	elem, ok := kv.data[key]
	if !ok {
		return ErrKeyNotFound
	}

	var expireAt time.Time
	if ttlSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	// Check if key is expired
	val := elem.Value.(valueWithTTL)
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		kv.deleteInMemory(key)
		return ErrKeyExpired
	}

	kv.insertInMemory(key, value, expireAt)

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opUpdate,
			ExpireAt: expireAt.UnixNano(),
			Key:      []byte(key),
			Value:    value,
		})
	}

	return nil
}

func (kv *KVStore) Delete(key string) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	if !kv.deleteInMemory(key) {
		return ErrKeyNotFound
	}

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
	}

	return nil
}

// -------------------- Snapshot and Recovery --------------------

func (kv *KVStore) Snapshot() error {
	if kv.isClosed() || kv.opts.ReadOnly {
		return ErrStoreClosed
	}

	tempPath := kv.opts.SnapshotPath + ".tmp"
	f, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)

	// Write magic number and version
	binary.Write(writer, binary.LittleEndian, magicNumber)
	binary.Write(writer, binary.LittleEndian, uint32(1)) // version

	kv.mu.RLock()
	count := len(kv.data)
	binary.Write(writer, binary.LittleEndian, uint32(count))

	for key, elem := range kv.data {
		val := elem.Value.(valueWithTTL)
		entry := walEntry{
			Op:       opCreate,
			ExpireAt: val.ExpireAt.UnixNano(),
			Key:      []byte(key),
			Value:    val.Value,
		}
		entryBytes := kv.encodeEntry(entry)
		if _, err = writer.Write(entryBytes); err != nil {
			kv.mu.RUnlock()
			return err
		}
	}
	kv.mu.RUnlock()

	if err = writer.Flush(); err != nil {
		return err
	}

	if err = f.Sync(); err != nil {
		return err
	}

	// Atomic replace
	if err = os.Rename(tempPath, kv.opts.SnapshotPath); err != nil {
		return err
	}

	// Reset WAL
	kv.walMutex.Lock()
	defer kv.walMutex.Unlock()

	if err = syscall.Munmap(kv.walMmap); err != nil {
		return err
	}

	if err = kv.walFile.Truncate(defaultWALSize); err != nil {
		return err
	}

	mmap, err := syscall.Mmap(int(kv.walFile.Fd()), 0, defaultWALSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	kv.walMmap = mmap
	kv.walSize = defaultWALSize
	kv.walOffset = 0

	kv.updateStats(func(s *Stats) {
		s.LastSnapshot = time.Now()
		s.WALSize = 0
	})

	return nil
}

func (kv *KVStore) loadFromSnapshot() error {
	f, err := os.Open(kv.opts.SnapshotPath)
	if os.IsNotExist(err) {
		return nil // No snapshot exists
	}
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	// Check magic number
	var magic, version, count uint32
	if err := binary.Read(reader, binary.LittleEndian, &magic); err != nil {
		return err
	}
	if magic != magicNumber {
		return errors.New("invalid snapshot file")
	}

	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil {
		return err
	}
	if version != 1 {
		return errors.New("unsupported snapshot version")
	}

	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return err
	}

	for i := uint32(0); i < count; i++ {
		entry, err := kv.decodeEntry(reader)
		if err != nil {
			return err
		}

		if entry.Op == opCreate {
			var expireAt time.Time
			if entry.ExpireAt > 0 {
				expireAt = time.Unix(0, entry.ExpireAt)
			}
			kv.insertInMemory(string(entry.Key), entry.Value, expireAt)
		}
	}

	return nil
}

func (kv *KVStore) replayWAL() error {
	if kv.walOffset == 0 {
		return nil
	}

	reader := &mmapReader{data: kv.walMmap[:kv.walOffset]}

	for reader.offset < len(reader.data) {
		entry, err := kv.decodeEntry(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		key := string(entry.Key)
		var expireAt time.Time
		if entry.ExpireAt > 0 {
			expireAt = time.Unix(0, entry.ExpireAt)
		}

		switch entry.Op {
		case opCreate, opUpdate:
			kv.insertInMemory(key, entry.Value, expireAt)
		case opDelete:
			kv.deleteInMemory(key)
		}
	}

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
	return newStats
}

func (kv *KVStore) updateStats(fn func(*Stats)) {
	if !kv.opts.EnableMetrics {
		return
	}

	for {
		old := kv.stats.Load().(*Stats)
		newStats := *old
		fn(&newStats)
		if kv.stats.CompareAndSwap(old, &newStats) {
			break
		}
	}
}

func (kv *KVStore) getMemoryUsage() int64 {
	stats := kv.stats.Load().(*Stats)
	return stats.MemoryUsage
}

func (kv *KVStore) isClosed() bool {
	return atomic.LoadInt32(&kv.closed) != 0
}

// -------------------- Cleanup --------------------

func (kv *KVStore) Close() error {
	if !atomic.CompareAndSwapInt32(&kv.closed, 0, 1) {
		return ErrStoreClosed
	}

	kv.cancel()
	kv.wg.Wait()

	if kv.walMmap != nil {
		syscall.Munmap(kv.walMmap)
	}

	if kv.walFile != nil {
		return kv.walFile.Close()
	}

	return nil
}

// -------------------- Encoding/Decoding --------------------

func (kv *KVStore) encodeEntry(e walEntry) []byte {
	keyLen := uint32(len(e.Key))
	valLen := uint32(len(e.Value))

	buf := make([]byte, entryHeaderSize+len(e.Key)+len(e.Value))

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

	copy(buf[offset:], e.Key)
	offset += len(e.Key)

	copy(buf[offset:], e.Value)

	// Calculate and set CRC32
	crc := crc32.ChecksumIEEE(buf[4:])
	binary.LittleEndian.PutUint32(buf[0:], crc)

	return buf
}

func (kv *KVStore) decodeEntry(r io.Reader) (walEntry, error) {
	header := make([]byte, entryHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return walEntry{}, err
	}

	crc := binary.LittleEndian.Uint32(header[0:])
	op := header[4]
	expireAt := int64(binary.LittleEndian.Uint64(header[5:]))
	keyLen := binary.LittleEndian.Uint32(header[13:])
	valLen := binary.LittleEndian.Uint32(header[17:])

	// Read key and value
	data := make([]byte, keyLen+valLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return walEntry{}, err
	}

	// Verify CRC
	checkData := make([]byte, entryHeaderSize-4+keyLen+valLen)
	copy(checkData, header[4:])
	copy(checkData[entryHeaderSize-4:], data)

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
	}, nil
}

// mmapReader implements io.Reader for memory-mapped data
type mmapReader struct {
	data   []byte
	offset int
}

func (r *mmapReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

// -------------------- Additional Public Methods --------------------

// Exists checks if a key exists and is not expired
func (kv *KVStore) Exists(key string) bool {
	if kv.isClosed() {
		return false
	}

	kv.mu.RLock()
	elem, ok := kv.data[key]
	kv.mu.RUnlock()

	if !ok {
		return false
	}

	val := elem.Value.(valueWithTTL)
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		// Clean up expired key asynchronously
		go func() {
			kv.mu.Lock()
			kv.deleteInMemory(key)
			if !kv.opts.ReadOnly {
				kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
			}
			kv.mu.Unlock()
		}()
		return false
	}

	return true
}

// Keys returns all non-expired keys
func (kv *KVStore) Keys() []string {
	if kv.isClosed() {
		return nil
	}

	kv.mu.RLock()
	defer kv.mu.RUnlock()

	now := time.Now()
	keys := make([]string, 0, len(kv.data))

	for key, elem := range kv.data {
		val := elem.Value.(valueWithTTL)
		if val.ExpireAt.IsZero() || now.Before(val.ExpireAt) {
			keys = append(keys, key)
		}
	}

	return keys
}

// Size returns the number of non-expired entries
func (kv *KVStore) Size() int {
	if kv.isClosed() {
		return 0
	}

	kv.mu.RLock()
	defer kv.mu.RUnlock()

	now := time.Now()
	count := 0

	for _, elem := range kv.data {
		val := elem.Value.(valueWithTTL)
		if val.ExpireAt.IsZero() || now.Before(val.ExpireAt) {
			count++
		}
	}

	return count
}

// TTL returns the time-to-live for a key
func (kv *KVStore) TTL(key string) (time.Duration, error) {
	if kv.isClosed() {
		return 0, ErrStoreClosed
	}

	kv.mu.RLock()
	elem, ok := kv.data[key]
	kv.mu.RUnlock()

	if !ok {
		return 0, ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)
	if val.ExpireAt.IsZero() {
		return -1, nil // No expiration
	}

	ttl := time.Until(val.ExpireAt)
	if ttl <= 0 {
		// Key is expired
		go func() {
			kv.mu.Lock()
			kv.deleteInMemory(key)
			if !kv.opts.ReadOnly {
				kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
			}
			kv.mu.Unlock()
		}()
		return 0, ErrKeyExpired
	}

	return ttl, nil
}

// SetTTL updates the TTL for an existing key
func (kv *KVStore) SetTTL(key string, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	elem, ok := kv.data[key]
	if !ok {
		return ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)

	// Check if key is already expired
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		kv.deleteInMemory(key)
		return ErrKeyExpired
	}

	// Update expiration
	var newExpireAt time.Time
	if ttlSeconds > 0 {
		newExpireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	val.ExpireAt = newExpireAt
	elem.Value = val

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opUpdate,
			ExpireAt: newExpireAt.UnixNano(),
			Key:      []byte(key),
			Value:    val.Value,
		})
	}

	return nil
}

// Clear removes all entries
func (kv *KVStore) Clear() error {
	if kv.isClosed() || kv.opts.ReadOnly {
		return ErrStoreClosed
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	// Get all keys for logging
	keys := make([]string, 0, len(kv.data))
	for key := range kv.data {
		keys = append(keys, key)
	}

	// Clear in-memory data
	kv.data = make(map[string]*list.Element)
	kv.lruList.Init()

	kv.updateStats(func(s *Stats) {
		s.Entries = 0
		s.MemoryUsage = 0
	})

	// Log deletions
	for _, key := range keys {
		kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
	}

	return nil
}

// Compact forces a snapshot and WAL reset
func (kv *KVStore) Compact() error {
	if kv.isClosed() || kv.opts.ReadOnly {
		return ErrStoreClosed
	}

	return kv.Snapshot()
}

// -------------------- Iterator Support --------------------

// Iterator provides a way to iterate over all entries
type Iterator struct {
	kv      *KVStore
	keys    []string
	current int
}

// NewIterator creates a new iterator
func (kv *KVStore) NewIterator() *Iterator {
	keys := kv.Keys()
	return &Iterator{
		kv:   kv,
		keys: keys,
	}
}

// Next advances the iterator to the next entry
func (it *Iterator) Next() bool {
	it.current++
	return it.current < len(it.keys)
}

// Key returns the current key
func (it *Iterator) Key() string {
	if it.current >= len(it.keys) {
		return ""
	}
	return it.keys[it.current]
}

// Value returns the current value
func (it *Iterator) Value() ([]byte, error) {
	if it.current >= len(it.keys) {
		return nil, io.EOF
	}
	return it.kv.Read(it.keys[it.current])
}

// Reset resets the iterator
func (it *Iterator) Reset() {
	it.current = -1
}

// -------------------- Batch Operations --------------------

// Batch represents a batch of operations
type Batch struct {
	kv   *KVStore
	ops  []batchOp
	size int64
}

type batchOp struct {
	op    byte
	key   string
	value []byte
	ttl   int64
}

// NewBatch creates a new batch
func (kv *KVStore) NewBatch() *Batch {
	return &Batch{kv: kv}
}

// Put adds a put operation to the batch
func (b *Batch) Put(key string, value []byte, ttlSeconds int64) {
	b.ops = append(b.ops, batchOp{
		op:    opUpdate,
		key:   key,
		value: append([]byte(nil), value...), // defensive copy
		ttl:   ttlSeconds,
	})
	b.size += int64(len(key) + len(value))
}

// Delete adds a delete operation to the batch
func (b *Batch) Delete(key string) {
	b.ops = append(b.ops, batchOp{
		op:  opDelete,
		key: key,
	})
	b.size += int64(len(key))
}

// Size returns the number of operations in the batch
func (b *Batch) Size() int {
	return len(b.ops)
}

// EstimatedSize returns the estimated size of the batch
func (b *Batch) EstimatedSize() int64 {
	return b.size
}

// Reset clears the batch
func (b *Batch) Reset() {
	b.ops = b.ops[:0]
	b.size = 0
}

// Commit executes all operations in the batch
func (b *Batch) Commit() error {
	if b.kv.isClosed() {
		return ErrStoreClosed
	}

	if len(b.ops) == 0 {
		return nil
	}

	b.kv.mu.Lock()
	defer b.kv.mu.Unlock()

	// Execute all operations
	for _, op := range b.ops {
		var expireAt time.Time
		if op.ttl > 0 {
			expireAt = time.Now().Add(time.Duration(op.ttl) * time.Second)
		}

		switch op.op {
		case opUpdate:
			b.kv.insertInMemory(op.key, op.value, expireAt)
			if !b.kv.opts.ReadOnly {
				b.kv.enqueueLog(walEntry{
					Op:       opUpdate,
					ExpireAt: expireAt.UnixNano(),
					Key:      []byte(op.key),
					Value:    op.value,
				})
			}
		case opDelete:
			b.kv.deleteInMemory(op.key)
			if !b.kv.opts.ReadOnly {
				b.kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(op.key)})
			}
		}
	}

	return nil
}

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

	health.Errors = errors
	return health
}
