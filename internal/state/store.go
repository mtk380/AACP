package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"sync"
)

type KV struct {
	Key   []byte
	Value []byte
}

type engine interface {
	Get(key []byte) ([]byte, bool)
	Set(key, value []byte)
	Delete(key []byte)
	IteratePrefix(prefix []byte) []KV
	Commit() ([]byte, uint64)
	Version() uint64
}

type Store struct {
	backendName string
	engine      engine
}

func NewStore() *Store {
	store, err := NewStoreFromEnv()
	if err != nil {
		panic(err)
	}
	return store
}

func NewStoreFromEnv() (*Store, error) {
	backend := os.Getenv("AACP_STATE_BACKEND")
	if backend == "" {
		backend = "memory"
	}
	return NewStoreWithBackend(backend)
}

func NewStoreWithBackend(backend string) (*Store, error) {
	switch backend {
	case "memory":
		return &Store{backendName: "memory", engine: newMemoryEngine()}, nil
	case "iavl":
		eng, err := newIAVLEngine()
		if err != nil {
			return nil, fmt.Errorf("init iavl engine: %w", err)
		}
		return &Store{backendName: "iavl", engine: eng}, nil
	default:
		return nil, fmt.Errorf("unsupported backend %q", backend)
	}
}

func (s *Store) BackendName() string {
	return s.backendName
}

func (s *Store) Get(key []byte) ([]byte, bool) {
	return s.engine.Get(key)
}

func (s *Store) Set(key, value []byte) {
	s.engine.Set(key, value)
}

func (s *Store) Delete(key []byte) {
	s.engine.Delete(key)
}

func (s *Store) IteratePrefix(prefix []byte) []KV {
	return s.engine.IteratePrefix(prefix)
}

func (s *Store) Commit() ([]byte, uint64) {
	return s.engine.Commit()
}

func (s *Store) Version() uint64 {
	return s.engine.Version()
}

type memoryEngine struct {
	mu      sync.RWMutex
	data    map[string][]byte
	version uint64
}

func newMemoryEngine() *memoryEngine {
	return &memoryEngine{data: map[string][]byte{}}
}

func (m *memoryEngine) Get(key []byte) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[string(key)]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, true
}

func (m *memoryEngine) Set(key, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := make([]byte, len(value))
	copy(v, value)
	m.data[string(key)] = v
}

func (m *memoryEngine) Delete(key []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, string(key))
}

func (m *memoryEngine) IteratePrefix(prefix []byte) []KV {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KV, 0)
	for k, v := range m.data {
		if bytes.HasPrefix([]byte(k), prefix) {
			vk := make([]byte, len(k))
			copy(vk, []byte(k))
			vv := make([]byte, len(v))
			copy(vv, v)
			out = append(out, KV{Key: vk, Value: vv})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i].Key, out[j].Key) < 0
	})
	return out
}

func (m *memoryEngine) Commit() ([]byte, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version++
	h := sha256.New()
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(m.data[k])
	}
	var ver [8]byte
	binary.BigEndian.PutUint64(ver[:], m.version)
	h.Write(ver[:])
	return h.Sum(nil), m.version
}

func (m *memoryEngine) Version() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}
