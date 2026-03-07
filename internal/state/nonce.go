package state

import (
	"encoding/binary"
	"encoding/hex"
)

func (s *Store) GetNonce(pub []byte) uint64 {
	key := NonceKey(hex.EncodeToString(pub))
	raw, ok := s.Get(key)
	if !ok || len(raw) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(raw)
}

func (s *Store) SetNonce(pub []byte, n uint64) {
	key := NonceKey(hex.EncodeToString(pub))
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	s.Set(key, buf)
}
