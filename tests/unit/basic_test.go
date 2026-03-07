package unit

import (
	"testing"

	"aacp/internal/types"
)

func TestEnvelopeValidation(t *testing.T) {
	tx := &types.TxEnvelope{
		Sender:    make([]byte, 32),
		Nonce:     0,
		GasLimit:  1,
		GasPrice:  1,
		Module:    "amx",
		Action:    "create_listing",
		Payload:   []byte(`{}`),
		Signature: make([]byte, 64),
	}
	if err := tx.ValidateBasic(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}
