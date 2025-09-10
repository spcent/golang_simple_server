package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// KVStore represents a thread-safe in-memory KV store with async persistence and TTL.
type KVStore struct {
	data         map[string]valueWithTTL
	filePath     string
	file         *os.File
	mutex        sync.RWMutex
	logChan      chan entry
	stopChan     chan struct{}
	snapshotCh   chan struct{}
	batchSize    int
	snapshotFreq time.Duration
}

type valueWithTTL struct {
	Value    []byte
	ExpireAt time.Time
}

type entry struct {
	Key       string `json:"key"`
	Value     []byte `json:"value,omitempty"`
	Op        string `json:"op"` // "create", "update", "delete"
	ExpireSec int64  `json:"expire_sec,omitempty"`
}

// NewKVStore creates a KVStore with automatic snapshotting and TTL logging
func NewKVStore(filePath string, logBuffer int, batchSize int, snapshotFreq time.Duration) (*KVStore, error) {
	store := &KVStore{
		data:         make(map[string]valueWithTTL),
		filePath:     filePath,
		logChan:      make(chan entry, logBuffer),
		stopChan:     make(chan struct{}),
		snapshotCh:   make(chan struct{}, 1),
		batchSize:    batchSize,
		snapshotFreq: snapshotFreq,
	}

	// Open log file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	store.file = file

	// Load existing data
	if err := store.loadFromFile(); err != nil {
		return nil, err
	}

	// Start async log writer
	go store.asyncLogWriter()

	// Start TTL cleaner
	go store.ttlCleaner()
	go store.autoSnapshot()

	return store, nil
}

// loadFromFile reconstructs the in-memory data from log file.
func (kv *KVStore) loadFromFile() error {
	scanner := bufio.NewScanner(kv.file)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		switch e.Op {
		case "create", "update":
			expireAt := time.Time{}
			if e.ExpireSec > 0 {
				expireAt = time.Now().Add(time.Duration(e.ExpireSec) * time.Second)
			}
			kv.data[e.Key] = valueWithTTL{Value: e.Value, ExpireAt: expireAt}
		case "delete":
			delete(kv.data, e.Key)
		}
	}
	return scanner.Err()
}

// Create adds a new key-value pair with optional TTL in seconds.
func (kv *KVStore) Create(key string, value []byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	if _, exists := kv.data[key]; exists {
		return os.ErrExist
	}
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}
	kv.data[key] = valueWithTTL{Value: value, ExpireAt: expireAt}
	kv.logChan <- entry{Key: key, Value: value, Op: "create", ExpireSec: ttl}
	return nil
}

// Read returns value if key exists and is not expired.
func (kv *KVStore) Read(key string) ([]byte, bool) {
	kv.mutex.RLock()
	defer kv.mutex.RUnlock()
	val, ok := kv.data[key]
	if !ok || (!val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt)) {
		return nil, false
	}
	return val.Value, true
}

// Update modifies existing key-value. Returns error if key does not exist.
func (kv *KVStore) Update(key string, value []byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	if _, exists := kv.data[key]; !exists {
		return os.ErrNotExist
	}
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}
	kv.data[key] = valueWithTTL{Value: value, ExpireAt: expireAt}
	kv.logChan <- entry{Key: key, Value: value, Op: "update", ExpireSec: ttl}
	return nil
}

// Delete removes a key-value pair.
func (kv *KVStore) Delete(key string) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	delete(kv.data, key)
	kv.logChan <- entry{Key: key, Op: "delete"}
	return nil
}

func (kv *KVStore) MCreate(kvs map[string][]byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for k := range kvs {
		if _, exists := kv.data[k]; exists {
			return os.ErrExist
		}
	}
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}
	for k, v := range kvs {
		kv.data[k] = valueWithTTL{Value: v, ExpireAt: expireAt}
		kv.logChan <- entry{Key: k, Value: v, Op: "create", ExpireSec: ttl}
	}
	return nil
}

func (kv *KVStore) MRead(keys []string) map[string][]byte {
	kv.mutex.RLock()
	defer kv.mutex.RUnlock()
	res := make(map[string][]byte)
	for _, k := range keys {
		if val, ok := kv.data[k]; ok && (val.ExpireAt.IsZero() || time.Now().Before(val.ExpireAt)) {
			res[k] = val.Value
		}
	}
	return res
}

func (kv *KVStore) MUpdate(kvs map[string][]byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for k := range kvs {
		if _, exists := kv.data[k]; !exists {
			return os.ErrNotExist
		}
	}
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}
	for k, v := range kvs {
		kv.data[k] = valueWithTTL{Value: v, ExpireAt: expireAt}
		kv.logChan <- entry{Key: k, Value: v, Op: "update", ExpireSec: ttl}
	}
	return nil
}

func (kv *KVStore) MDelete(keys []string) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for _, k := range keys {
		delete(kv.data, k)
		kv.logChan <- entry{Key: k, Op: "delete"}
	}
	return nil
}

// asyncLogWriter writes log entries to disk asynchronously.
func (kv *KVStore) asyncLogWriter() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	buffer := make([]entry, 0, kv.batchSize)

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		for _, e := range buffer {
			b, _ := json.Marshal(e)
			kv.file.Write(append(b, '\n'))
		}
		kv.file.Sync()
		buffer = buffer[:0]
	}

	for {
		select {
		case e := <-kv.logChan:
			buffer = append(buffer, e)
			if len(buffer) >= kv.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-kv.stopChan:
			flush()
			return
		}
	}
}

// ttlCleaner periodically removes expired keys.
func (kv *KVStore) ttlCleaner() {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			kv.mutex.Lock()
			for k, v := range kv.data {
				if !v.ExpireAt.IsZero() && now.After(v.ExpireAt) {
					delete(kv.data, k)
				}
			}
			kv.mutex.Unlock()
		case <-kv.stopChan:
			return
		}
	}
}

func (kv *KVStore) autoSnapshot() {
	if kv.snapshotFreq <= 0 {
		return
	}
	ticker := time.NewTicker(kv.snapshotFreq)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			kv.Snapshot()
		case <-kv.stopChan:
			return
		}
	}
}

// Snapshot writes all in-memory data to a new file and replaces old log.
func (kv *KVStore) Snapshot() error {
	kv.mutex.RLock()
	defer kv.mutex.RUnlock()
	tmpFile := kv.filePath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for k, v := range kv.data {
		ttl := int64(0)
		if !v.ExpireAt.IsZero() {
			ttl = int64(time.Until(v.ExpireAt).Seconds())
		}
		e := entry{Key: k, Value: v.Value, Op: "create", ExpireSec: ttl}
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
	}

	kv.file.Close()
	os.Rename(tmpFile, kv.filePath)
	kv.file, _ = os.OpenFile(kv.filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	return nil
}

// Close gracefully stops background goroutines and flushes logs.
func (kv *KVStore) Close() error {
	close(kv.stopChan)
	time.Sleep(50 * time.Millisecond) // wait for async writer to flush
	return kv.file.Close()
}
