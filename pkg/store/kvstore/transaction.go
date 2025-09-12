package kvstore

import (
	"sync/atomic"
	"time"
)

// -------------------- Transaction Support --------------------

// BeginTxn starts a new transaction
func (kv *KVStore) BeginTxn() *Transaction {
	if !kv.opts.EnableTransactions {
		return nil
	}

	return &Transaction{
		kv:        kv,
		readSet:   make(map[string]int64),
		startTime: time.Now(),
	}
}

func (txn *Transaction) Read(key string) ([]byte, error) {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	if txn.kv.isClosed() {
		return nil, ErrStoreClosed
	}

	shard := txn.kv.getShard(key)
	shard.mu.RLock()
	elem, ok := shard.data[key]
	if !ok {
		shard.mu.RUnlock()
		return nil, ErrKeyNotFound
	}

	val := elem.Value.(valueWithTTL)
	shard.mu.RUnlock()

	// Check expiration
	if !val.ExpireAt.IsZero() && time.Now().After(val.ExpireAt) {
		return nil, ErrKeyExpired
	}

	// Record read version
	txn.readSet[key] = val.Version

	return append([]byte(nil), val.Value...), nil
}

func (txn *Transaction) Put(key string, value []byte, ttlSeconds int64) {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	txn.operations = append(txn.operations, txnOp{
		op:    opUpdate,
		key:   key,
		value: append([]byte(nil), value...),
		ttl:   ttlSeconds,
	})
}

func (txn *Transaction) Delete(key string) {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	txn.operations = append(txn.operations, txnOp{
		op:  opDelete,
		key: key,
	})
}

func (txn *Transaction) Commit() error {
	txn.mu.Lock()
	defer txn.mu.Unlock()

	if txn.kv.isClosed() {
		return ErrStoreClosed
	}

	// Phase 1: Validation - check if read versions are still current
	for key, readVersion := range txn.readSet {
		shard := txn.kv.getShard(key)
		shard.mu.RLock()
		if elem, ok := shard.data[key]; ok {
			val := elem.Value.(valueWithTTL)
			if val.Version != readVersion {
				shard.mu.RUnlock()
				return ErrTransactionAborted
			}
		}
		shard.mu.RUnlock()
	}

	// Phase 2: Execute all operations atomically
	for _, op := range txn.operations {
		shard := txn.kv.getShard(op.key)
		shard.mu.Lock()

		var expireAt time.Time
		if op.ttl > 0 {
			expireAt = time.Now().Add(time.Duration(op.ttl) * time.Second)
		}

		switch op.op {
		case opUpdate:
			txn.kv.insertInMemory(shard, op.key, op.value, expireAt)
			if !txn.kv.opts.ReadOnly {
				txn.kv.enqueueLog(walEntry{
					Op:       opUpdate,
					ExpireAt: expireAt.UnixNano(),
					Key:      []byte(op.key),
					Value:    op.value,
					Version:  atomic.LoadInt64(&txn.kv.globalVersion),
				})
			}
		case opDelete:
			txn.kv.deleteInMemory(shard, op.key)
			if !txn.kv.opts.ReadOnly {
				txn.kv.enqueueLog(walEntry{Op: opDelete, Key: []byte(op.key)})
			}
		}

		shard.mu.Unlock()
	}

	return nil
}
