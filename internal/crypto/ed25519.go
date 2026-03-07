package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
)

const (
	PubKeySize    = ed25519.PublicKeySize
	PrivKeySize   = ed25519.PrivateKeySize
	SignatureSize = ed25519.SignatureSize
	AddressSize   = 20
)

type KeyPair struct {
	PrivKey ed25519.PrivateKey
	PubKey  ed25519.PublicKey
}

func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{PrivKey: priv, PubKey: pub}, nil
}

func SignMessage(priv ed25519.PrivateKey, message []byte) []byte {
	h := sha256.Sum256(message)
	return ed25519.Sign(priv, h[:])
}

func VerifySignature(pub ed25519.PublicKey, message, signature []byte) bool {
	h := sha256.Sum256(message)
	return ed25519.Verify(pub, h[:], signature)
}

func PubKeyToAddress(pub ed25519.PublicKey) [AddressSize]byte {
	h := sha256.Sum256(pub)
	var out [AddressSize]byte
	copy(out[:], h[:AddressSize])
	return out
}
