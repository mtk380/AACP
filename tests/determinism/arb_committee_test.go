package determinism

import (
	"encoding/json"
	"testing"
	"time"

	"aacp/internal/abci"
	"aacp/internal/crypto"
	"aacp/internal/state"
	"aacp/tests/testutil"
)

func TestARBCommitteeSelectionDeterministic(t *testing.T) {
	appA := abci.NewApp()
	appB := abci.NewApp()
	_, _ = appA.InitChain(&abci.InitChainRequest{})
	_, _ = appB.InitChain(&abci.InitChainRequest{})

	plaintiff, _ := crypto.GenerateKeyPair()
	defendant, _ := crypto.GenerateKeyPair()

	file := map[string]any{
		"order_id": "order-1",
		"task_id": "task-1",
		"defendant": defendant.PubKey,
		"reason_code": "other",
		"description": "need arbitration",
	}
	txFileA := testutil.MustSignedTx(plaintiff.PubKey, plaintiff.PrivKey, 0, "arb", "file_dispute", file)
	txFileB := testutil.MustSignedTx(plaintiff.PubKey, plaintiff.PrivKey, 0, "arb", "file_dispute", file)

	_, _ = appA.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txFileA}})
	_, _ = appB.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txFileB}})

	dA := appA.Store().IteratePrefix([]byte(state.PrefixArbDispute))
	dB := appB.Store().IteratePrefix([]byte(state.PrefixArbDispute))
	if len(dA) != 1 || len(dB) != 1 {
		t.Fatalf("dispute not created")
	}
	var disputeA struct{ DisputeID string `json:"dispute_id"` }
	var disputeB struct{ DisputeID string `json:"dispute_id"` }
	_ = json.Unmarshal(dA[0].Value, &disputeA)
	_ = json.Unmarshal(dB[0].Value, &disputeB)

	resolveA := testutil.MustSignedTx(plaintiff.PubKey, plaintiff.PrivKey, 1, "arb", "auto_resolve", map[string]any{"dispute_id": disputeA.DisputeID})
	resolveB := testutil.MustSignedTx(plaintiff.PubKey, plaintiff.PrivKey, 1, "arb", "auto_resolve", map[string]any{"dispute_id": disputeB.DisputeID})
	_, _ = appA.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now(), Txs: [][]byte{resolveA}})
	_, _ = appB.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now(), Txs: [][]byte{resolveB}})

	vA := appA.Store().IteratePrefix([]byte(state.PrefixArbDispute))[0].Value
	vB := appB.Store().IteratePrefix([]byte(state.PrefixArbDispute))[0].Value
	if string(vA) != string(vB) {
		t.Fatalf("committee selection is not deterministic")
	}
}
