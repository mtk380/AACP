package determinism

import (
	"testing"
	"time"

	"aacp/internal/abci"
	"aacp/internal/crypto"
	"aacp/tests/testutil"
)

func TestAMXMatchDeterministicAcrossApps(t *testing.T) {
	appA := abci.NewApp()
	appB := abci.NewApp()
	_, _ = appA.InitChain(&abci.InitChainRequest{})
	_, _ = appB.InitChain(&abci.InitChainRequest{})

	provider, _ := crypto.GenerateKeyPair()
	consumer, _ := crypto.GenerateKeyPair()

	listings := []map[string]any{
		{"title": "A", "description": "x", "capabilities": []map[string]any{{"cap_type": "text_generation"}}, "pricing": map[string]any{"currency": "CNY", "base_price": "10"}, "sla": map[string]any{"max_latency_ms": 100, "timeout_sec": 30}},
		{"title": "B", "description": "x", "capabilities": []map[string]any{{"cap_type": "text_generation"}}, "pricing": map[string]any{"currency": "CNY", "base_price": "8"}, "sla": map[string]any{"max_latency_ms": 120, "timeout_sec": 30}},
	}

	txs := make([][]byte, 0)
	for i, p := range listings {
		txs = append(txs, testutil.MustSignedTx(provider.PubKey, provider.PrivKey, uint64(i), "amx", "create_listing", p))
	}
	request := map[string]any{
		"required_caps": []string{"text_generation"},
		"budget": map[string]any{"currency": "CNY", "amount": "20"},
		"max_latency_ms": 500,
		"deadline": time.Now().Add(5*time.Minute).Unix(),
	}
	txs = append(txs, testutil.MustSignedTx(consumer.PubKey, consumer.PrivKey, 0, "amx", "submit_request", request))

	respA, _ := appA.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: txs})
	respB, _ := appB.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: txs})
	if string(respA.AppHash) != string(respB.AppHash) {
		t.Fatalf("app hash mismatch A=%x B=%x", respA.AppHash, respB.AppHash)
	}
}
