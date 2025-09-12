package store

import (
	"container/list"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	opCreate byte = 1
	opUpdate byte = 2
	opDelete byte = 3
)

type Durability int

const (
	DurabilityNever Durability = iota
	DurabilityEveryFlush
	DurabilityPerWrite
)

type valueWithTTL struct {
	Key      string
	Value    []byte
	ExpireAt time.Time
}

type walEntry struct {
	Op       byte
	ExpireAt int64
	Key      []byte
	Value    []byte
}

type KVStore struct {
	data    map[string]*list.Element
	lruList *list.List

	walFile   *os.File
	walMmap   []byte
	walOffset int64
	walSize   int64
	walMutex  sync.Mutex

	logChan  chan walEntry
	stopChan chan struct{}
	wg       sync.WaitGroup

	durability    Durability
	maxEntries    int
	cleanInterval time.Duration
	snapshotPath  string
}

// NewKVStoreMMap creates a new KV store
func NewKVStoreMMap(walPath, snapshotPath string, logBuffer int, maxEntries int, cleanInterval time.Duration, durability Durability) (*KVStore, error) {
	if cleanInterval <= 0 {
		cleanInterval = time.Second
	}

	kv := &KVStore{
		data:          make(map[string]*list.Element),
		lruList:       list.New(),
		logChan:       make(chan walEntry, logBuffer),
		stopChan:      make(chan struct{}),
		durability:    durability,
		maxEntries:    maxEntries,
		cleanInterval: cleanInterval,
		snapshotPath:  snapshotPath,
	}

	file, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	kv.walFile = file

	info, _ := file.Stat()
	size := info.Size()
	if size < 1024*1024 {
		size = 1024 * 1024
	}
	if err := syscall.Ftruncate(int(file.Fd()), size); err != nil {
		return nil, err
	}
	mmap, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	kv.walMmap = mmap
	kv.walSize = size
	kv.walOffset = 0

	kv.wg.Add(1)
	go kv.asyncLogWriter()
	kv.wg.Add(1)
	go kv.ttlCleaner()

	return kv, nil
}

// -------------------- In-Memory Operations --------------------

func (kv *KVStore) insertInMemory(key string, value []byte, expireAt time.Time) {
	if elem, ok := kv.data[key]; ok {
		elem.Value = valueWithTTL{Key: key, Value: value, ExpireAt: expireAt}
		kv.lruList.MoveToFront(elem)
		return
	}
	if kv.maxEntries > 0 && kv.lruList.Len() >= kv.maxEntries {
		kv.evictLRU()
	}
	elem := kv.lruList.PushFront(valueWithTTL{Key: key, Value: value, ExpireAt: expireAt})
	kv.data[key] = elem
}

func (kv *KVStore) deleteInMemory(key string) {
	if elem, ok := kv.data[key]; ok {
		kv.lruList.Remove(elem)
		delete(kv.data, key)
	}
}

func (kv *KVStore) evictLRU() {
	back := kv.lruList.Back()
	if back == nil {
		return
	}
	v := back.Value.(valueWithTTL)
	kv.lruList.Remove(back)
	delete(kv.data, v.Key)
	kv.enqueueLog(walEntry{Op: opDelete, ExpireAt: 0, Key: []byte(v.Key)})
}

func (kv *KVStore) enqueueLog(e walEntry) {
	select {
	case kv.logChan <- e:
	default:
		go func() { kv.logChan <- e }()
	}
}

// -------------------- Async WAL Writer --------------------

func (kv *KVStore) asyncLogWriter() {
	defer kv.wg.Done()
	batch := []walEntry{}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case e := <-kv.logChan:
			batch = append(batch, e)
			if len(batch) >= 64 {
				kv.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				kv.flushBatch(batch)
				batch = batch[:0]
			}
		case <-kv.stopChan:
			if len(batch) > 0 {
				kv.flushBatch(batch)
			}
			return
		}
	}
}

func (kv *KVStore) flushBatch(batch []walEntry) {
	kv.walMutex.Lock()
	defer kv.walMutex.Unlock()
	for _, e := range batch {
		entryBytes := encodeEntry(e)
		if int64(len(entryBytes))+kv.walOffset > kv.walSize {
			kv.expandMMap(len(entryBytes))
		}
		copy(kv.walMmap[kv.walOffset:], entryBytes)
		kv.walOffset += int64(len(entryBytes))
		if kv.durability == DurabilityPerWrite {
			kv.flushMMap()
		}
	}
	if kv.durability == DurabilityEveryFlush {
		kv.flushMMap()
	}
}

func (kv *KVStore) flushMMap() error {
	if len(kv.walMmap) == 0 {
		return nil
	}

	// Try Unix-style msync
	addr := uintptr(unsafe.Pointer(&kv.walMmap[0]))
	length := uintptr(len(kv.walMmap))

	// Some OS (e.g. Windows) does not support SYS_MSYNC
	// so we check runtime.GOOS
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd":
		// Perform msync system call
		_, _, errno := syscall.Syscall(syscall.SYS_MSYNC, addr, length, syscall.MS_SYNC)
		if errno != 0 {
			// If msync fails, fallback to file.Sync
			if err := kv.walFile.Sync(); err != nil {
				return err
			}
		}
	default:
		// Non-Unix platforms (e.g. Windows) fallback
		if err := kv.walFile.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (kv *KVStore) expandMMap(min int) {
	newSize := kv.walSize*2 + int64(min)
	_ = syscall.Munmap(kv.walMmap)
	_ = kv.walFile.Truncate(newSize)
	mmap, _ := syscall.Mmap(int(kv.walFile.Fd()), 0, int(newSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	kv.walMmap = mmap
	kv.walSize = newSize
}

// -------------------- TTL Cleaner --------------------

func (kv *KVStore) ttlCleaner() {
	defer kv.wg.Done()
	ticker := time.NewTicker(kv.cleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			for k, elem := range kv.data {
				val := elem.Value.(valueWithTTL)
				if !val.ExpireAt.IsZero() && now.After(val.ExpireAt) {
					kv.deleteInMemory(k)
					kv.enqueueLog(walEntry{Op: opDelete, ExpireAt: 0, Key: []byte(k)})
				}
			}
		case <-kv.stopChan:
			return
		}
	}
}

// -------------------- Public CRUD --------------------

func (kv *KVStore) Create(key string, value []byte, ttlSeconds int64) error {
	if _, ok := kv.data[key]; ok {
		return errors.New("key exists")
	}
	var expireAt time.Time
	if ttlSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}
	kv.insertInMemory(key, value, expireAt)
	kv.enqueueLog(walEntry{Op: opCreate, ExpireAt: expireAt.UnixNano(), Key: []byte(key), Value: value})
	return nil
}

func (kv *KVStore) Read(key string) ([]byte, bool) {
	elem, ok := kv.data[key]
	if !ok {
		return nil, false
	}
	v := elem.Value.(valueWithTTL)
	if !v.ExpireAt.IsZero() && time.Now().After(v.ExpireAt) {
		kv.deleteInMemory(key)
		kv.enqueueLog(walEntry{Op: opDelete, ExpireAt: 0, Key: []byte(key)})
		return nil, false
	}
	return append([]byte(nil), v.Value...), true
}

func (kv *KVStore) Update(key string, value []byte, ttlSeconds int64) error {
	elem, ok := kv.data[key]
	if !ok {
		return errors.New("key not exist")
	}
	var expireAt time.Time
	if ttlSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}
	elem.Value = valueWithTTL{Key: key, Value: value, ExpireAt: expireAt}
	kv.enqueueLog(walEntry{Op: opUpdate, ExpireAt: expireAt.UnixNano(), Key: []byte(key), Value: value})
	return nil
}

func (kv *KVStore) Delete(key string) error {
	kv.deleteInMemory(key)
	kv.enqueueLog(walEntry{Op: opDelete, ExpireAt: 0, Key: []byte(key)})
	return nil
}

// -------------------- Snapshot --------------------

func (kv *KVStore) Snapshot() error {
	f, err := os.Create(kv.snapshotPath)
	if err != nil {
		return err
	}
	defer f.Close()
	for k, elem := range kv.data {
		v := elem.Value.(valueWithTTL)
		e := walEntry{Op: opCreate, ExpireAt: v.ExpireAt.UnixNano(), Key: []byte(k), Value: v.Value}
		_, _ = f.Write(encodeEntry(e))
	}
	// Reset WAL
	kv.walMutex.Lock()
	defer kv.walMutex.Unlock()
	_ = syscall.Munmap(kv.walMmap)
	_ = kv.walFile.Truncate(1024 * 1024)
	mmap, _ := syscall.Mmap(int(kv.walFile.Fd()), 0, 1024*1024, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	kv.walMmap = mmap
	kv.walSize = 1024 * 1024
	kv.walOffset = 0
	return nil
}

// -------------------- Close --------------------

func (kv *KVStore) Close() {
	close(kv.stopChan)
	kv.wg.Wait()
	_ = syscall.Munmap(kv.walMmap)
	_ = kv.walFile.Close()
}

// -------------------- Helper --------------------

func encodeEntry(e walEntry) []byte {
	keyLen := uint32(len(e.Key))
	valLen := uint32(len(e.Value))
	buf := make([]byte, 1+8+4+4+len(e.Key)+len(e.Value))
	buf[0] = e.Op
	binary.LittleEndian.PutUint64(buf[1:], uint64(e.ExpireAt))
	binary.LittleEndian.PutUint32(buf[9:], keyLen)
	binary.LittleEndian.PutUint32(buf[13:], valLen)
	copy(buf[17:], e.Key)
	copy(buf[17+len(e.Key):], e.Value)
	return buf
}
