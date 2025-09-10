package store

import (
	"bufio"
	"container/list"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// KVStore represents a thread-safe in-memory KV store with async persistence and TTL.
type KVStore struct {
	data         map[string]*list.Element
	lruList      *list.List
	filePath     string
	file         *os.File
	mutex        sync.RWMutex
	logChan      chan entry
	stopChan     chan struct{}
	batchSize    int
	snapshotFreq time.Duration
	maxEntries   int
}

type valueWithTTL struct {
	Key      string
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
func NewKVStore(filePath string, logBuffer int, batchSize int, snapshotFreq time.Duration, maxEntries int) (*KVStore, error) {
	store := &KVStore{
		data:         make(map[string]*list.Element),
		lruList:      list.New(),
		filePath:     filePath,
		logChan:      make(chan entry, logBuffer),
		stopChan:     make(chan struct{}),
		batchSize:    batchSize,
		snapshotFreq: snapshotFreq,
		maxEntries:   maxEntries,
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
			elem := kv.lruList.PushFront(valueWithTTL{Key: e.Key, Value: e.Value, ExpireAt: expireAt})
			kv.data[e.Key] = elem
		case "delete":
			if elem, ok := kv.data[e.Key]; ok {
				kv.lruList.Remove(elem)
				delete(kv.data, e.Key)
			}
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
	if kv.lruList.Len() >= kv.maxEntries {
		kv.evictLRU()
	}

	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}

	elem := kv.lruList.PushFront(valueWithTTL{Key: key, Value: value, ExpireAt: expireAt})
	kv.data[key] = elem
	kv.logChan <- entry{Key: key, Value: value, Op: "create", ExpireSec: ttl}
	return nil
}

// Read returns value if key exists and is not expired.
func (kv *KVStore) Read(key string) ([]byte, bool) {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	elem, ok := kv.data[key]
	if !ok {
		return nil, false
	}

	val := elem.Value.(valueWithTTL)
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		kv.removeElement(elem)
		kv.logChan <- entry{Key: key, Op: "delete"}
		return nil, false
	}

	kv.lruList.MoveToFront(elem)
	return val.Value, true
}

// Update modifies existing key-value. Returns error if key does not exist.
func (kv *KVStore) Update(key string, value []byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()

	elem, ok := kv.data[key]
	if !ok {
		return os.ErrNotExist
	}

	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}

	elem.Value = valueWithTTL{Key: key, Value: value, ExpireAt: expireAt}
	kv.lruList.MoveToFront(elem)
	kv.logChan <- entry{Key: key, Value: value, Op: "update", ExpireSec: ttl}
	return nil
}

// Delete removes a key-value pair.
func (kv *KVStore) Delete(key string) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	elem, ok := kv.data[key]
	if ok {
		kv.removeElement(elem)
		kv.logChan <- entry{Key: key, Op: "delete"}
	}
	return nil
}

func (kv *KVStore) evictLRU() {
	if kv.lruList.Len() == 0 {
		return
	}
	elem := kv.lruList.Back()
	if elem != nil {
		kv.removeElement(elem)
		val := elem.Value.(valueWithTTL)
		kv.logChan <- entry{Key: val.Key, Op: "delete"} // log eviction
	}
}

func (kv *KVStore) removeElement(elem *list.Element) {
	val := elem.Value.(valueWithTTL)
	delete(kv.data, val.Key)
	kv.lruList.Remove(elem)
}

func (kv *KVStore) MCreate(kvs map[string][]byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for k := range kvs {
		if _, exists := kv.data[k]; exists {
			return os.ErrExist
		}
	}
	for k, v := range kvs {
		if kv.lruList.Len() >= kv.maxEntries {
			kv.evictLRU()
		}
		expireAt := time.Time{}
		if ttl > 0 {
			expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
		}
		elem := kv.lruList.PushFront(valueWithTTL{Key: k, Value: v, ExpireAt: expireAt})
		kv.data[k] = elem
		kv.logChan <- entry{Key: k, Value: v, Op: "create", ExpireSec: ttl}
	}
	return nil
}

func (kv *KVStore) MRead(keys []string) map[string][]byte {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	res := make(map[string][]byte)
	for _, k := range keys {
		if elem, ok := kv.data[k]; ok {
			val := elem.Value.(valueWithTTL)
			if val.ExpireAt.IsZero() || time.Now().Before(val.ExpireAt) {
				res[k] = val.Value
				kv.lruList.MoveToFront(elem)
			} else {
				kv.removeElement(elem)
				kv.logChan <- entry{Key: k, Op: "delete"}
			}
		}
	}
	return res
}

func (kv *KVStore) MUpdate(kvs map[string][]byte, ttl int64) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for k, v := range kvs {
		elem, ok := kv.data[k]
		if !ok {
			return os.ErrNotExist
		}
		expireAt := time.Time{}
		if ttl > 0 {
			expireAt = time.Now().Add(time.Duration(ttl) * time.Second)
		}
		elem.Value = valueWithTTL{Key: k, Value: v, ExpireAt: expireAt}
		kv.lruList.MoveToFront(elem)
		kv.logChan <- entry{Key: k, Value: v, Op: "update", ExpireSec: ttl}
	}
	return nil
}

func (kv *KVStore) MDelete(keys []string) error {
	kv.mutex.Lock()
	defer kv.mutex.Unlock()
	for _, k := range keys {
		if elem, ok := kv.data[k]; ok {
			kv.removeElement(elem)
			kv.logChan <- entry{Key: k, Op: "delete"}
		}
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
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			kv.mutex.Lock()
			for _, elem := range kv.data {
				val := elem.Value.(valueWithTTL)
				if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
					kv.removeElement(elem)
					kv.logChan <- entry{Key: val.Key, Op: "delete"}
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

	for elem := kv.lruList.Front(); elem != nil; elem = elem.Next() {
		val := elem.Value.(valueWithTTL)
		ttl := int64(0)
		if !val.ExpireAt.IsZero() {
			ttl = int64(time.Until(val.ExpireAt).Seconds())
		}
		e := entry{Key: val.Key, Value: val.Value, Op: "create", ExpireSec: ttl}
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
