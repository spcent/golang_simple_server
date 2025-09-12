package kvstore

import (
	"container/list"
	"sync/atomic"
	"time"
)

// Default returns a new KVStore with default options
func Default() (*KVStore, error) {
	opts := Options{
		WALPath:            "data/store.wal",
		SnapshotPath:       "data/store.snapshot",
		MaxEntries:         100000,
		MaxMemoryBytes:     200 * 1024 * 1024, // 200MB
		EnableCompression:  true,
		CompressionType:    CompressionGzip,
		ShardCount:         defaultShardCount,
		EnableTransactions: true,
		EnableMetrics:      true,
	}
	return NewKVStore(opts)
}

// -------------------- Public CRUD Operations --------------------

func (kv *KVStore) Create(key string, value []byte, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if _, ok := shard.data[key]; ok {
		return ErrKeyExists
	}

	var expireAt time.Time
	if ttlSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	kv.insertInMemory(shard, key, value, expireAt)

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opCreate,
			ExpireAt: expireAt.UnixNano(),
			Key:      []byte(key),
			Value:    value,
			Version:  atomic.LoadInt64(&kv.globalVersion),
		})
	}

	return nil
}

func (kv *KVStore) Read(key string) ([]byte, error) {
	if kv.isClosed() {
		return nil, ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.RLock()
	elem, ok := shard.data[key]
	if !ok {
		shard.mu.RUnlock()
		atomic.AddInt64(&kv.missCount, 1)
		kv.updateStats(func(s *Stats) {
			s.Misses++
			if s.Hits+s.Misses > 0 {
				s.HitRatio = float64(s.Hits) / float64(s.Hits+s.Misses)
			}
		})
		return nil, ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)
	shard.mu.RUnlock()

	// Check expiration
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		// Clean up expired key
		shard.mu.Lock()
		kv.deleteInMemory(shard, key)
		shard.mu.Unlock()

		if !kv.opts.ReadOnly {
			kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
		}

		atomic.AddInt64(&kv.missCount, 1)
		kv.updateStats(func(s *Stats) {
			s.Misses++
			if s.Hits+s.Misses > 0 {
				s.HitRatio = float64(s.Hits) / float64(s.Hits+s.Misses)
			}
		})
		return nil, ErrKeyExpired
	}

	// Update LRU position
	shard.mu.Lock()
	shard.lruList.MoveToFront(elem)
	shard.mu.Unlock()

	atomic.AddInt64(&kv.hitCount, 1)
	kv.updateStats(func(s *Stats) {
		s.Hits++
		if s.Hits+s.Misses > 0 {
			s.HitRatio = float64(s.Hits) / float64(s.Hits+s.Misses)
		}
	})

	// Return defensive copy
	return append([]byte(nil), val.Value...), nil
}

func (kv *KVStore) Update(key string, value []byte, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	elem, ok := shard.data[key]
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
		kv.deleteInMemory(shard, key)
		return ErrKeyExpired
	}

	kv.insertInMemory(shard, key, value, expireAt)

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opUpdate,
			ExpireAt: expireAt.UnixNano(),
			Key:      []byte(key),
			Value:    value,
			Version:  atomic.LoadInt64(&kv.globalVersion),
		})
	}

	return nil
}

func (kv *KVStore) Delete(key string) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if !kv.deleteInMemory(shard, key) {
		return ErrKeyNotFound
	}

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
	}

	return nil
}

// -------------------- Additional Public Methods --------------------

// Exists checks if a key exists and is not expired
func (kv *KVStore) Exists(key string) bool {
	if kv.isClosed() {
		return false
	}

	shard := kv.getShard(key)
	shard.mu.RLock()
	elem, ok := shard.data[key]
	if !ok {
		shard.mu.RUnlock()
		return false
	}

	val := elem.Value.(valueWithTTL)
	shard.mu.RUnlock()

	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		// Clean up expired key synchronously to avoid goroutine leaks
		shard.mu.Lock()
		kv.deleteInMemory(shard, key)
		shard.mu.Unlock()

		if !kv.opts.ReadOnly {
			kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
		}
		return false
	}

	return true
}

// Keys returns all non-expired keys
func (kv *KVStore) Keys() []string {
	if kv.isClosed() {
		return nil
	}

	now := time.Now()
	var allKeys []string

	for _, shard := range kv.shards {
		shard.mu.RLock()
		for key, elem := range shard.data {
			val := elem.Value.(valueWithTTL)
			if val.ExpireAt.IsZero() || now.Before(val.ExpireAt) {
				allKeys = append(allKeys, key)
			}
		}
		shard.mu.RUnlock()
	}

	return allKeys
}

// Size returns the number of non-expired entries
func (kv *KVStore) Size() int {
	if kv.isClosed() {
		return 0
	}

	now := time.Now()
	count := 0

	for _, shard := range kv.shards {
		shard.mu.RLock()
		for _, elem := range shard.data {
			val := elem.Value.(valueWithTTL)
			if val.ExpireAt.IsZero() || now.Before(val.ExpireAt) {
				count++
			}
		}
		shard.mu.RUnlock()
	}

	return count
}

// TTL returns the time-to-live for a key
func (kv *KVStore) TTL(key string) (time.Duration, error) {
	if kv.isClosed() {
		return 0, ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.RLock()
	elem, ok := shard.data[key]
	shard.mu.RUnlock()

	if !ok {
		return 0, ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)
	if val.ExpireAt.IsZero() {
		return -1, nil // No expiration
	}

	ttl := time.Until(val.ExpireAt)
	if ttl <= 0 {
		// Key is expired - clean up synchronously
		shard.mu.Lock()
		kv.deleteInMemory(shard, key)
		shard.mu.Unlock()

		if !kv.opts.ReadOnly {
			kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(key)})
		}
		return 0, ErrKeyExpired
	}

	return ttl, nil
}

// SetTTL updates the TTL for an existing key
func (kv *KVStore) SetTTL(key string, ttlSeconds int64) error {
	if kv.isClosed() {
		return ErrStoreClosed
	}

	shard := kv.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	elem, ok := shard.data[key]
	if !ok {
		return ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)

	// Check if key is already expired
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		kv.deleteInMemory(shard, key)
		return ErrKeyExpired
	}

	// Update expiration
	var newExpireAt time.Time
	if ttlSeconds > 0 {
		newExpireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	val.ExpireAt = newExpireAt
	val.Version = atomic.AddInt64(&kv.globalVersion, 1)
	elem.Value = val

	if !kv.opts.ReadOnly {
		kv.enqueueLog(walEntry{
			Op:       opUpdate,
			ExpireAt: newExpireAt.UnixNano(),
			Key:      []byte(key),
			Value:    val.Value,
			Version:  val.Version,
		})
	}

	return nil
}

// Clear removes all entries
func (kv *KVStore) Clear() error {
	if kv.isClosed() || kv.opts.ReadOnly {
		return ErrStoreClosed
	}

	var allKeys []string

	// Collect all keys first
	for _, shard := range kv.shards {
		shard.mu.RLock()
		for key := range shard.data {
			allKeys = append(allKeys, key)
		}
		shard.mu.RUnlock()
	}

	// Clear all shards
	for _, shard := range kv.shards {
		shard.mu.Lock()
		shard.data = make(map[string]*list.Element)
		shard.lruList.Init()
		shard.mu.Unlock()
	}

	kv.updateStats(func(s *Stats) {
		s.Entries = 0
		s.MemoryUsage = 0
	})

	// Log deletions
	for _, key := range allKeys {
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

// -------------------- Cleanup --------------------

func (kv *KVStore) Close() error {
	if !atomic.CompareAndSwapInt32(&kv.closed, 0, 1) {
		return ErrStoreClosed
	}

	// Use timeout for graceful shutdown
	done := make(chan struct{})
	go func() {
		defer close(done)
		kv.cancel()
		kv.wg.Wait()
	}()

	select {
	case <-done:
		// Normal shutdown
	case <-time.After(kv.opts.CloseTimeout):
		return ErrCloseTimeout
	}

	return kv.cleanup()
}
