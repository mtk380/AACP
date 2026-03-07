package e2e

import (
	"testing"
	"time"

	"aacp/internal/abci"
	"aacp/internal/crypto"
	"aacp/tests/testutil"
)

func TestHappyPathCreateListingToTaskSettlement(t *testing.T) {
	app := abci.NewApp()
	_, _ = app.InitChain(&abci.InitChainRequest{})

	provider, _ := crypto.GenerateKeyPair()
	consumer, _ := crypto.GenerateKeyPair()

	createListing := map[string]any{
		"title": "Agent Provider",
		"description": "NLP service",
		"capabilities": []map[string]any{{"cap_type": "text_generation"}},
		"pricing": map[string]any{"currency": "CNY", "base_price": "10"},
		"sla": map[string]any{"max_latency_ms": 500, "timeout_sec": 120},
	}
	matchReq := map[string]any{
		"required_caps": []string{"text_generation"},
		"budget": map[string]any{"currency": "CNY", "amount": "20"},
		"max_latency_ms": 1000,
		"deadline": time.Now().Add(time.Hour).Unix(),
	}

	txs := [][]byte{
		testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 0, "amx", "create_listing", createListing),
		testutil.MustSignedTx(consumer.PubKey, consumer.PrivKey, 0, "amx", "submit_request", matchReq),
	}
	resp, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: txs})
	if err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	for i, r := range resp.TxResults {
		if r.Code != 0 {
			t.Fatalf("tx %d failed code=%d log=%s", i, r.Code, r.Log)
		}
	}
}
