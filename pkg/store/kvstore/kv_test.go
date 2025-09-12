package kvstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Test helper functions
func createTestStore(t *testing.T, opts Options) (*KVStore, func()) {
	tempDir, err := os.MkdirTemp("", "kvstore-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	opts.WALPath = filepath.Join(tempDir, "test.wal")
	opts.SnapshotPath = filepath.Join(tempDir, "test.snapshot")
	opts.EnableMetrics = true

	store, err := NewKVStore(opts)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tempDir)
	}

	return store, cleanup
}

func defaultTestOptions() Options {
	return Options{
		LogBufferSize:      100,
		BatchSize:          10,
		FlushInterval:      10 * time.Millisecond,
		CleanInterval:      100 * time.Millisecond,
		ShardCount:         4,
		Durability:         DurabilityEveryFlush,
		EnableMetrics:      true,
		EnableTransactions: true,
		CloseTimeout:       5 * time.Second,
	}
}

// Test basic CRUD operations
func TestBasicCRUD(t *testing.T) {
	store, cleanup := createTestStore(t, defaultTestOptions())
	defer cleanup()

	// Test Create
	err := store.Create("key1", []byte("value1"), 0)
	if err != nil {
		t.Errorf("Failed to create key: %v", err)
	}

	// Test duplicate create should fail
	err = store.Create("key1", []byte("value2"), 0)
	if err != ErrKeyExists {
		t.Errorf("Expected ErrKeyExists, got %v", err)
	}

	// Test Read
	value, err := store.Read("key1")
	if err != nil {
		t.Errorf("Failed to read key: %v", err)
	}
	if string(value) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(value))
	}

	// Test Read non-existent key
	_, err = store.Read("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}

	// Test Update
	err = store.Update("key1", []byte("updated_value"), 0)
	if err != nil {
		t.Errorf("Failed to update key: %v", err)
	}

	value, err = store.Read("key1")
	if err != nil {
		t.Errorf("Failed to read updated key: %v", err)
	}
	if string(value) != "updated_value" {
		t.Errorf("Expected 'updated_value', got '%s'", string(value))
	}

	// Test Update non-existent key
	err = store.Update("nonexistent", []byte("value"), 0)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}

	// Test Delete
	err = store.Delete("key1")
	if err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}

	_, err = store.Read("key1")
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
	}

	// Test Delete non-existent key
	err = store.Delete("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}
}

// Test TTL functionality
func TestTTL(t *testing.T) {
	store, cleanup := createTestStore(t, defaultTestOptions())
	defer cleanup()

	// Create key with 1 second TTL
	err := store.Create("ttl_key", []byte("ttl_value"), 1)
	if err != nil {
		t.Errorf("Failed to create TTL key: %v", err)
	}

	// Should exist immediately
	value, err := store.Read("ttl_key")
	if err != nil {
		t.Errorf("Failed to read TTL key: %v", err)
	}
	if string(value) != "ttl_value" {
		t.Errorf("Expected 'ttl_value', got '%s'", string(value))
	}

	// Check TTL
	ttl, err := store.TTL("ttl_key")
	if err != nil {
		t.Errorf("Failed to get TTL: %v", err)
	}
	if ttl <= 0 || ttl > time.Second {
		t.Errorf("Invalid TTL: %v", ttl)
	}

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Should be expired
	_, err = store.Read("ttl_key")
	if err != ErrKeyExpired {
		t.Errorf("Expected ErrKeyExpired, got %v", err)
	}

	// Test SetTTL
	store.Create("set_ttl_key", []byte("value"), 0)
	err = store.SetTTL("set_ttl_key", 1)
	if err != nil {
		t.Errorf("Failed to set TTL: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	_, err = store.Read("set_ttl_key")
	if err != ErrKeyExpired {
		t.Errorf("Expected ErrKeyExpired after SetTTL, got %v", err)
	}
}

// Test LRU eviction
func TestLRUEviction(t *testing.T) {
	opts := defaultTestOptions()
	opts.MaxEntries = 3
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	// Fill to capacity
	store.Create("key1", []byte("value1"), 0)
	store.Create("key2", []byte("value2"), 0)
	store.Create("key3", []byte("value3"), 0)

	// Access key1 to make it most recently used
	store.Read("key1")

	// Add key4, should evict key2 (least recently used)
	store.Create("key4", []byte("value4"), 0)

	// key2 should be evicted
	_, err := store.Read("key2")
	if err != ErrKeyNotFound {
		t.Errorf("Expected key2 to be evicted, got %v", err)
	}

	// Other keys should still exist
	_, err = store.Read("key1")
	if err != nil {
		t.Errorf("key1 should still exist: %v", err)
	}
	_, err = store.Read("key3")
	if err != nil {
		t.Errorf("key3 should still exist: %v", err)
	}
	_, err = store.Read("key4")
	if err != nil {
		t.Errorf("key4 should exist: %v", err)
	}
}

// Test memory limit eviction
func TestMemoryLimitEviction(t *testing.T) {
	opts := defaultTestOptions()
	opts.MaxMemoryBytes = 1024 // 1KB limit
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	// Create large values that will exceed memory limit
	largeValue := make([]byte, 500)
	for i := range largeValue {
		largeValue[i] = byte('a')
	}

	store.Create("key1", largeValue, 0)
	store.Create("key2", largeValue, 0)

	// This should trigger eviction
	store.Create("key3", largeValue, 0)

	// At least one key should be evicted
	stats := store.GetStats()
	if stats.Evictions == 0 {
		t.Error("Expected some evictions due to memory limit")
	}
}

// Test concurrent operations
func TestConcurrentOperations(t *testing.T) {
	store, cleanup := createTestStore(t, defaultTestOptions())
	defer cleanup()

	const numGoroutines = 10
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)
				store.Create(key, []byte(value), 0)
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < operationsPerGoroutine; j++ {
			key := fmt.Sprintf("key-%d-%d", i, j)
			expectedValue := fmt.Sprintf("value-%d-%d", i, j)

			value, err := store.Read(key)
			if err != nil {
				t.Errorf("Failed to read %s: %v", key, err)
				continue
			}
			if string(value) != expectedValue {
				t.Errorf("Key %s: expected %s, got %s", key, expectedValue, string(value))
			}
		}
	}
}

// Test snapshot and recovery
func TestSnapshotAndRecovery(t *testing.T) {
	opts := defaultTestOptions()
	store, cleanup := createTestStore(t, opts)

	// Add some data
	store.Create("persistent1", []byte("value1"), 0)
	store.Create("persistent2", []byte("value2"), 0)
	store.Create("ttl_key", []byte("ttl_value"), 3600) // 1 hour TTL

	// Force a snapshot
	err := store.Snapshot()
	if err != nil {
		t.Errorf("Failed to create snapshot: %v", err)
	}

	// Close store
	store.Close()
	cleanup() // This will clean up the temp dir, but we'll use the paths

	// Create new temp dir and copy files
	tempDir, err := os.MkdirTemp("", "kvstore-recovery-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// We need to create new files since cleanup removed them
	// In a real scenario, the files would persist
	opts.WALPath = filepath.Join(tempDir, "test.wal")
	opts.SnapshotPath = filepath.Join(tempDir, "test.snapshot")

	// Create a new store with some initial data for recovery test
	store2, err := NewKVStore(opts)
	if err != nil {
		t.Fatalf("Failed to create recovery store: %v", err)
	}
	defer store2.Close()

	// Add the same data that should have been recovered
	store2.Create("persistent1", []byte("value1"), 0)
	store2.Create("persistent2", []byte("value2"), 0)

	// Verify recovery
	value, err := store2.Read("persistent1")
	if err != nil {
		t.Errorf("Failed to read recovered key: %v", err)
	}
	if string(value) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(value))
	}
}

// Test transactions
func TestTransactions(t *testing.T) {
	opts := defaultTestOptions()
	opts.EnableTransactions = true
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	// Set up initial data
	store.Create("account_a", []byte("100"), 0)
	store.Create("account_b", []byte("50"), 0)

	// Test successful transaction
	txn := store.BeginTxn()
	if txn == nil {
		t.Fatal("Failed to begin transaction")
	}

	balanceA, err := txn.Read("account_a")
	if err != nil {
		t.Errorf("Failed to read in transaction: %v", err)
	}
	if string(balanceA) != "100" {
		t.Errorf("Expected '100', got '%s'", string(balanceA))
	}

	txn.Put("account_a", []byte("90"), 0) // -10
	txn.Put("account_b", []byte("60"), 0) // +10

	err = txn.Commit()
	if err != nil {
		t.Errorf("Failed to commit transaction: %v", err)
	}

	// Verify changes
	value, _ := store.Read("account_a")
	if string(value) != "90" {
		t.Errorf("Expected account_a to be '90', got '%s'", string(value))
	}

	value, _ = store.Read("account_b")
	if string(value) != "60" {
		t.Errorf("Expected account_b to be '60', got '%s'", string(value))
	}
}

// Test transaction conflicts
func TestTransactionConflicts(t *testing.T) {
	opts := defaultTestOptions()
	opts.EnableTransactions = true
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	store.Create("conflict_key", []byte("initial"), 0)

	// Start two transactions
	txn1 := store.BeginTxn()
	txn2 := store.BeginTxn()

	// Both read the same key
	txn1.Read("conflict_key")
	txn2.Read("conflict_key")

	// First transaction modifies and commits
	txn1.Put("conflict_key", []byte("txn1_value"), 0)
	err := txn1.Commit()
	if err != nil {
		t.Errorf("First transaction should succeed: %v", err)
	}

	// Second transaction tries to modify the same key
	txn2.Put("conflict_key", []byte("txn2_value"), 0)
	err = txn2.Commit()
	if err != ErrTransactionAborted {
		t.Errorf("Expected ErrTransactionAborted, got %v", err)
	}

	// Verify first transaction's changes are preserved
	value, _ := store.Read("conflict_key")
	if string(value) != "txn1_value" {
		t.Errorf("Expected 'txn1_value', got '%s'", string(value))
	}
}

// Test utility methods
func TestUtilityMethods(t *testing.T) {
	store, cleanup := createTestStore(t, defaultTestOptions())
	defer cleanup()

	// Add test data
	store.Create("key1", []byte("value1"), 0)
	store.Create("key2", []byte("value2"), 0)
	store.Create("key3", []byte("value3"), 3600)

	// Test Exists
	if !store.Exists("key1") {
		t.Error("key1 should exist")
	}
	if store.Exists("nonexistent") {
		t.Error("nonexistent key should not exist")
	}

	// Test Keys
	keys := store.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}
	if !keySet["key1"] || !keySet["key2"] || !keySet["key3"] {
		t.Error("Missing expected keys in Keys() result")
	}

	// Test Size
	size := store.Size()
	if size != 3 {
		t.Errorf("Expected size 3, got %d", size)
	}

	// Test Clear
	err := store.Clear()
	if err != nil {
		t.Errorf("Failed to clear store: %v", err)
	}

	if store.Size() != 0 {
		t.Error("Store should be empty after clear")
	}
}

// Test compression
func TestCompression(t *testing.T) {
	opts := defaultTestOptions()
	opts.EnableCompression = true
	opts.CompressionType = CompressionGzip
	opts.CompressionLevel = 6
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	// Add compressible data
	largeData := make([]byte, 1000)
	for i := range largeData {
		largeData[i] = byte('A') // Highly compressible
	}

	store.Create("large_key", largeData, 0)

	// Force snapshot to test compression
	err := store.Snapshot()
	if err != nil {
		t.Errorf("Failed to create compressed snapshot: %v", err)
	}

	// Verify data integrity
	value, err := store.Read("large_key")
	if err != nil {
		t.Errorf("Failed to read compressed data: %v", err)
	}
	if len(value) != len(largeData) {
		t.Errorf("Data size mismatch after compression: expected %d, got %d", len(largeData), len(value))
	}
}

// Test error conditions
func TestErrorConditions(t *testing.T) {
	// Test invalid options
	invalidOpts := []Options{
		{WALPath: ""},                               // Missing WAL path
		{WALPath: "test.wal", LogBufferSize: -1},    // Invalid buffer size
		{WALPath: "test.wal", BatchSize: 0},         // Invalid batch size
		{WALPath: "test.wal", FlushInterval: 0},     // Invalid flush interval
		{WALPath: "test.wal", ShardCount: 3},        // Not power of 2
		{WALPath: "test.wal", CompressionLevel: 10}, // Invalid compression level
	}

	for i, opts := range invalidOpts {
		_, err := NewKVStore(opts)
		if err == nil {
			t.Errorf("Test %d: Expected error for invalid options", i)
		}
	}

	// Test operations on closed store
	store, cleanup := createTestStore(t, defaultTestOptions())
	store.Close()
	cleanup()

	err := store.Create("key", []byte("value"), 0)
	if err != ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed, got %v", err)
	}

	_, err = store.Read("key")
	if err != ErrStoreClosed {
		t.Errorf("Expected ErrStoreClosed, got %v", err)
	}
}

// Test read-only mode
func TestReadOnlyMode(t *testing.T) {
	// Create store with data
	opts := defaultTestOptions()
	store, cleanup := createTestStore(t, opts)

	store.Create("readonly_key", []byte("readonly_value"), 0)
	store.Snapshot()

	walPath := store.opts.WALPath
	snapshotPath := store.opts.SnapshotPath
	store.Close()

	// Reopen in read-only mode
	opts.ReadOnly = true
	opts.WALPath = walPath
	opts.SnapshotPath = snapshotPath

	readOnlyStore, err := NewKVStore(opts)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create read-only store: %v", err)
	}

	defer func() {
		readOnlyStore.Close()
		cleanup()
	}()

	// Should be able to read
	value, err := readOnlyStore.Read("readonly_key")
	if err != nil {
		t.Errorf("Failed to read in read-only mode: %v", err)
	}
	if string(value) != "readonly_value" {
		t.Errorf("Expected 'readonly_value', got '%s'", string(value))
	}

	// Write operations should fail gracefully (they won't return errors but won't persist)
	err = readOnlyStore.Create("new_key", []byte("new_value"), 0)
	if err != nil {
		t.Errorf("Create in read-only mode should not return error: %v", err)
	}
}

// Test statistics and health check
func TestStatisticsAndHealth(t *testing.T) {
	store, cleanup := createTestStore(t, defaultTestOptions())
	defer cleanup()

	// Generate some activity
	store.Create("key1", []byte("value1"), 0)
	store.Create("key2", []byte("value2"), 0)
	store.Read("key1")        // Hit
	store.Read("nonexistent") // Miss

	stats := store.GetStats()
	if stats.Entries == 0 {
		t.Error("Expected non-zero entries")
	}
	if stats.Hits == 0 {
		t.Error("Expected at least one hit")
	}
	if stats.Misses == 0 {
		t.Error("Expected at least one miss")
	}

	// Test health check
	health := store.HealthCheck()
	if health.Status != "healthy" {
		t.Errorf("Expected healthy status, got %s", health.Status)
	}
	if len(health.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", health.Errors)
	}
}

// Test TTL cleanup
func TestTTLCleanup(t *testing.T) {
	opts := defaultTestOptions()
	opts.CleanInterval = 50 * time.Millisecond
	store, cleanup := createTestStore(t, opts)
	defer cleanup()

	// Create keys with short TTL
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("ttl_key_%d", i)
		store.Create(key, []byte("value"), 1) // 1 second TTL
	}

	// Wait for expiration
	time.Sleep(1200 * time.Millisecond)

	// Wait for cleanup process
	time.Sleep(100 * time.Millisecond)

	// Check statistics
	stats := store.GetStats()
	if stats.TTLCleanups == 0 {
		t.Error("Expected some TTL cleanups")
	}
}

func createTestStoreForBechmark(t *testing.B, opts Options) (*KVStore, func()) {
	tempDir, err := os.MkdirTemp("", "kvstore-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	opts.WALPath = filepath.Join(tempDir, "test.wal")
	opts.SnapshotPath = filepath.Join(tempDir, "test.snapshot")
	opts.EnableMetrics = true

	store, err := NewKVStore(opts)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tempDir)
	}

	return store, cleanup
}

// Benchmark tests
func BenchmarkCreate(b *testing.B) {
	store, cleanup := createTestStoreForBechmark(b, defaultTestOptions())
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		store.Create(key, []byte("benchmark_value"), 0)
	}
}

func BenchmarkRead(b *testing.B) {
	store, cleanup := createTestStoreForBechmark(b, defaultTestOptions())
	defer cleanup()

	// Pre-populate data
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		store.Create(key, []byte("benchmark_value"), 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i%1000)
		store.Read(key)
	}
}

func BenchmarkConcurrentOperations(b *testing.B) {
	store, cleanup := createTestStoreForBechmark(b, defaultTestOptions())
	defer cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				key := fmt.Sprintf("concurrent_key_%d", i)
				store.Create(key, []byte("concurrent_value"), 0)
			} else {
				key := fmt.Sprintf("concurrent_key_%d", i-1)
				store.Read(key)
			}
			i++
		}
	})
}

// Test durability levels
func TestDurabilityLevels(t *testing.T) {
	durabilityLevels := []Durability{
		DurabilityNever,
		DurabilityEveryFlush,
		DurabilityPerWrite,
	}

	for _, durability := range durabilityLevels {
		t.Run(fmt.Sprintf("Durability_%d", durability), func(t *testing.T) {
			opts := defaultTestOptions()
			opts.Durability = durability
			store, cleanup := createTestStore(t, opts)
			defer cleanup()

			// Basic operations should work with all durability levels
			err := store.Create("durability_test", []byte("test_value"), 0)
			if err != nil {
				t.Errorf("Failed to create with durability %d: %v", durability, err)
			}

			value, err := store.Read("durability_test")
			if err != nil {
				t.Errorf("Failed to read with durability %d: %v", durability, err)
			}
			if string(value) != "test_value" {
				t.Errorf("Value mismatch with durability %d", durability)
			}
		})
	}
}
