package p2p

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"aacp/internal/crypto"
)

type Message struct {
	Protocol  string
	MsgType   string
	Sender    []byte
	Seq       uint64
	Timestamp int64
	Payload   []byte
	Signature []byte
}

func (m *Message) SignBytes() []byte {
	preimage := fmt.Sprintf("%s|%s|%x|%d|%d|%x", m.Protocol, m.MsgType, m.Sender, m.Seq, m.Timestamp, m.Payload)
	h := sha256.Sum256([]byte(preimage))
	return h[:]
}

func (m *Message) ID() string {
	h := sha256.Sum256(append(append([]byte{}, m.Sender...), byte(m.Seq)))
	return hex.EncodeToString(h[:8])
}

func (m *Message) Verify() error {
	now := time.Now().Unix()
	if m.Timestamp < now-30 || m.Timestamp > now+30 {
		return fmt.Errorf("timestamp tolerance exceeded")
	}
	if len(m.Sender) != crypto.PubKeySize {
		return fmt.Errorf("sender pubkey must be %d bytes", crypto.PubKeySize)
	}
	if len(m.Signature) != crypto.SignatureSize {
		return fmt.Errorf("signature must be %d bytes", crypto.SignatureSize)
	}
	if !crypto.VerifySignature(m.Sender, m.SignBytes(), m.Signature) {
		return fmt.Errorf("bad signature")
	}
	return nil
}
