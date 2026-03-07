package crypto

import "testing"

func TestSignVerify(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	msg := []byte("aacp-signature")
	sig := SignMessage(kp.PrivKey, msg)
	if !VerifySignature(kp.PubKey, msg, sig) {
		t.Fatalf("signature should verify")
	}
	if VerifySignature(kp.PubKey, []byte("bad"), sig) {
		t.Fatalf("signature should fail for modified message")
	}
}

func TestPubKeyToAddress(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	addr := PubKeyToAddress(kp.PubKey)
	if len(addr) != AddressSize {
		t.Fatalf("unexpected address size %d", len(addr))
	}
}
