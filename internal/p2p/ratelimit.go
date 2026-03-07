package p2p

import "sync"

type RateLimiter struct {
	mu            sync.Mutex
	MaxBytesPerSec int64
	MaxMsgPerSec   int
	PerProtocol    map[string]PeerLimit
}

type PeerLimit struct {
	BytesPerSec int64
	MsgPerSec   int
}

var DefaultLimits = map[string]PeerLimit{
	"/aacp/consensus/1.0.0": {BytesPerSec: 5 * 1024 * 1024, MsgPerSec: 100},
	"/aacp/mempool/1.0.0":   {BytesPerSec: 2 * 1024 * 1024, MsgPerSec: 500},
	"/aacp/relay/1.0.0":     {BytesPerSec: 10 * 1024 * 1024, MsgPerSec: 1000},
	"/aacp/heartbeat/1.0.0": {BytesPerSec: 100 * 1024, MsgPerSec: 50},
	"/aacp/sync/1.0.0":      {BytesPerSec: 50 * 1024 * 1024, MsgPerSec: 50},
	"/aacp/discovery/1.0.0": {BytesPerSec: 100 * 1024, MsgPerSec: 10},
}

func NewRateLimiter() *RateLimiter {
	cp := map[string]PeerLimit{}
	for k, v := range DefaultLimits {
		cp[k] = v
	}
	return &RateLimiter{PerProtocol: cp, MaxBytesPerSec: 50 * 1024 * 1024, MaxMsgPerSec: 2000}
}

func (r *RateLimiter) LimitFor(protocol string) PeerLimit {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.PerProtocol[protocol]; ok {
		return v
	}
	return PeerLimit{BytesPerSec: 128 * 1024, MsgPerSec: 20}
}
