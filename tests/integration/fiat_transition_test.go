package integration

import (
	"encoding/json"
	"testing"
	"time"

	"aacp/internal/abci"
	"aacp/internal/crypto"
	"aacp/internal/state"
	"aacp/tests/testutil"
)

func TestFiatEscrowTransitions(t *testing.T) {
	app := abci.NewApp()
	_, _ = app.InitChain(&abci.InitChainRequest{})

	provider, _ := crypto.GenerateKeyPair()
	consumer, _ := crypto.GenerateKeyPair()

	escrowID := "escrow-1"
	txCreate := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 0, "fiat", "create_escrow", map[string]any{
		"escrow_id": escrowID,
		"order_id": "order-1",
		"consumer": consumer.PubKey,
		"provider": provider.PubKey,
		"service_fee": map[string]any{"currency": "CNY", "amount": "100"},
		"commission_bps": 1200,
	})
	txFund := testutil.MustSignedTx(consumer.PubKey, consumer.PrivKey, 0, "fiat", "fund_escrow", map[string]any{"escrow_id": escrowID})
	txLock := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 1, "fiat", "lock_escrow", map[string]any{"escrow_id": escrowID})
	txRefund := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 2, "fiat", "refund_escrow", map[string]any{"escrow_id": escrowID})

	resp, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txCreate, txFund, txLock, txRefund}})
	if err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	for i, r := range resp.TxResults {
		if r.Code != 0 {
			t.Fatalf("tx %d failed code=%d log=%s", i, r.Code, r.Log)
		}
	}

	raw, ok := app.Store().Get([]byte(state.PrefixFiatEscrow + escrowID))
	if !ok {
		t.Fatalf("escrow not found")
	}
	var esc struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &esc); err != nil {
		t.Fatalf("decode escrow failed: %v", err)
	}
	if esc.Status != "ESCROW_REFUNDED" {
		t.Fatalf("expected ESCROW_REFUNDED got %s", esc.Status)
	}
}
