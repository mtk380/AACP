//go:build iavl

package state

import (
	"bytes"
	"fmt"
	"sync"

	dbm "github.com/cosmos/cosmos-db"
	iavl "github.com/cosmos/iavl"
)

type iavlEngine struct {
	mu      sync.RWMutex
	tree    *iavl.MutableTree
	version uint64
}

func newIAVLEngine() (*iavlEngine, error) {
	db := dbm.NewMemDB()
	tree := iavl.NewMutableTree(db, 500000, false, iavl.NewNopLogger())
	if _, err := tree.Load(); err != nil {
		return nil, fmt.Errorf("load iavl tree: %w", err)
	}
	return &iavlEngine{tree: tree, version: uint64(tree.Version())}, nil
}

func (e *iavlEngine) Get(key []byte) ([]byte, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v, err := e.tree.Get(key)
	if err != nil || v == nil {
		return nil, false
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true
}

func (e *iavlEngine) Set(key, value []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := make([]byte, len(value))
	copy(v, value)
	if _, err := e.tree.Set(key, v); err != nil {
		panic(err)
	}
}

func (e *iavlEngine) Delete(key []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, _, err := e.tree.Remove(key); err != nil {
		panic(err)
	}
}

func (e *iavlEngine) IteratePrefix(prefix []byte) []KV {
	e.mu.RLock()
	defer e.mu.RUnlock()
	start := append([]byte{}, prefix...)
	end := prefixRangeEnd(prefix)
	iter, err := e.tree.Iterator(start, end, true)
	if err != nil {
		panic(err)
	}
	defer iter.Close()

	out := make([]KV, 0)
	for ; iter.Valid(); iter.Next() {
		k := iter.Key()
		v := iter.Value()
		if !bytes.HasPrefix(k, prefix) {
			break
		}
		kk := make([]byte, len(k))
		copy(kk, k)
		vv := make([]byte, len(v))
		copy(vv, v)
		out = append(out, KV{Key: kk, Value: vv})
	}
	return out
}

func (e *iavlEngine) Commit() ([]byte, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	hash, version, err := e.tree.SaveVersion()
	if err != nil {
		panic(err)
	}
	e.version = uint64(version)
	return hash, e.version
}

func (e *iavlEngine) Version() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.version
}

func prefixRangeEnd(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	end := append([]byte{}, prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xFF {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}
