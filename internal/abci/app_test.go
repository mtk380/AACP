package abci

import (
	"testing"
	"time"

	"aacp/internal/crypto"
	"aacp/tests/testutil"
)

func TestFinalizeBlockNonceAndExecution(t *testing.T) {
	app := NewApp()
	if _, err := app.InitChain(&InitChainRequest{}); err != nil {
		t.Fatalf("init chain failed: %v", err)
	}
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	createListing := map[string]any{
		"title": "provider-a",
		"description": "desc",
		"capabilities": []map[string]any{{"cap_type": "text_generation"}},
		"pricing": map[string]any{"currency": "CNY", "base_price": "10"},
		"sla": map[string]any{"max_latency_ms": 500, "timeout_sec": 60},
	}
	tx1 := testutil.MustSignedTx(kp.PubKey, kp.PrivKey, 0, "amx", "create_listing", createListing)
	resp, err := app.FinalizeBlock(&FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{tx1}})
	if err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	if len(resp.TxResults) != 1 || resp.TxResults[0].Code != 0 {
		t.Fatalf("unexpected tx result: %+v", resp.TxResults)
	}

	// Reuse nonce should fail.
	tx2 := testutil.MustSignedTx(kp.PubKey, kp.PrivKey, 0, "amx", "create_listing", createListing)
	resp2, err := app.FinalizeBlock(&FinalizeBlockRequest{Height: 2, Time: time.Now(), Txs: [][]byte{tx2}})
	if err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	if len(resp2.TxResults) != 1 || resp2.TxResults[0].Code == 0 {
		t.Fatalf("expected bad nonce failure")
	}
}
