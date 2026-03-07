package state

import (
	"bytes"
	"testing"
)

func TestStoreCRUDAndCommit(t *testing.T) {
	s := NewStore()
	s.Set([]byte("a"), []byte("1"))
	s.Set([]byte("a/b"), []byte("2"))
	s.Set([]byte("a/c"), []byte("3"))

	v, ok := s.Get([]byte("a"))
	if !ok || string(v) != "1" {
		t.Fatalf("unexpected get result")
	}

	prefix := s.IteratePrefix([]byte("a/"))
	if len(prefix) != 2 {
		t.Fatalf("expected 2 prefix keys got %d", len(prefix))
	}

	h1, ver1 := s.Commit()
	h2, ver2 := s.Commit()
	if ver2 != ver1+1 {
		t.Fatalf("version should increase")
	}
	if bytes.Equal(h1, h2) {
		t.Fatalf("hash should include version and change per commit")
	}
}

func TestNonce(t *testing.T) {
	s := NewStore()
	pub := []byte("12345678901234567890123456789012")
	if n := s.GetNonce(pub); n != 0 {
		t.Fatalf("expected zero nonce")
	}
	s.SetNonce(pub, 7)
	if n := s.GetNonce(pub); n != 7 {
		t.Fatalf("expected nonce 7 got %d", n)
	}
}
