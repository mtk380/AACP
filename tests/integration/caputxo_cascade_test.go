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

func TestCapUTXOCascadeRevoke(t *testing.T) {
	app := abci.NewApp()
	_, _ = app.InitChain(&abci.InitChainRequest{})

	issuer, _ := crypto.GenerateKeyPair()
	holder, _ := crypto.GenerateKeyPair()
	delegate, _ := crypto.GenerateKeyPair()

	mint := map[string]any{
		"holder": holder.PubKey,
		"cap_type": "text_generation",
		"cap_version": "1.0.0",
		"scopes": []string{"read", "write"},
		"delegatable": true,
		"max_depth": 3,
	}

	txMint := testutil.MustSignedTx(issuer.PubKey, issuer.PrivKey, 0, "caputxo", "mint_capability", mint)
	resp, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txMint}})
	if err != nil || resp.TxResults[0].Code != 0 {
		t.Fatalf("mint failed err=%v code=%d", err, resp.TxResults[0].Code)
	}

	entries := app.Store().IteratePrefix([]byte(state.PrefixCapUTXO))
	if len(entries) != 1 {
		t.Fatalf("expected one utxo")
	}
	var parent struct {
		UTXOID string `json:"utxo_id"`
	}
	if err := json.Unmarshal(entries[0].Value, &parent); err != nil {
		t.Fatalf("decode parent failed: %v", err)
	}

	delegatePayload := map[string]any{
		"parent_utxo_id": parent.UTXOID,
		"new_holder": delegate.PubKey,
		"scopes": []string{"read"},
	}
	txDelegate := testutil.MustSignedTx(holder.PubKey, holder.PrivKey, 0, "caputxo", "delegate_capability", delegatePayload)
	resp2, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now(), Txs: [][]byte{txDelegate}})
	if err != nil || resp2.TxResults[0].Code != 0 {
		t.Fatalf("delegate failed err=%v code=%d", err, resp2.TxResults[0].Code)
	}

	txRevoke := testutil.MustSignedTx(issuer.PubKey, issuer.PrivKey, 1, "caputxo", "revoke_capability", map[string]any{"utxo_id": parent.UTXOID})
	resp3, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 3, Time: time.Now(), Txs: [][]byte{txRevoke}})
	if err != nil || resp3.TxResults[0].Code != 0 {
		t.Fatalf("revoke failed err=%v code=%d", err, resp3.TxResults[0].Code)
	}

	all := app.Store().IteratePrefix([]byte(state.PrefixCapUTXO))
	if len(all) < 2 {
		t.Fatalf("expected parent and child utxos")
	}
	for _, kv := range all {
		var item struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(kv.Value, &item); err != nil {
			t.Fatalf("decode utxo failed: %v", err)
		}
		if item.Status != "UTXO_REVOKED" {
			t.Fatalf("expected revoked status got %s", item.Status)
		}
	}
}
