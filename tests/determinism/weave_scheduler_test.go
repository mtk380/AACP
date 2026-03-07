package determinism

import (
	"testing"
	"time"

	"aacp/internal/abci"
	"aacp/internal/crypto"
	"aacp/tests/testutil"
)

func TestWEAVESchedulerDeterministic(t *testing.T) {
	appA := abci.NewApp()
	appB := abci.NewApp()
	_, _ = appA.InitChain(&abci.InitChainRequest{})
	_, _ = appB.InitChain(&abci.InitChainRequest{})

	orch, _ := crypto.GenerateKeyPair()
	dag := map[string]any{
		"dag_id": "dag-1",
		"root_task": "task-root",
		"global_timeout_sec": 100,
		"nodes": []map[string]any{
			{"node_id": "n1", "type": "NODE_TASK", "max_retries": 1},
			{"node_id": "n2", "type": "NODE_JOIN", "max_retries": 1},
		},
		"edges": []map[string]any{{"from_node": "n1", "to_node": "n2"}},
	}

	txCreateA := testutil.MustSignedTx(orch.PubKey, orch.PrivKey, 0, "weave", "create_dag", dag)
	txCreateB := testutil.MustSignedTx(orch.PubKey, orch.PrivKey, 0, "weave", "create_dag", dag)
	txStartA := testutil.MustSignedTx(orch.PubKey, orch.PrivKey, 1, "weave", "start_dag", map[string]any{"dag_id": "dag-1"})
	txStartB := testutil.MustSignedTx(orch.PubKey, orch.PrivKey, 1, "weave", "start_dag", map[string]any{"dag_id": "dag-1"})

	_, _ = appA.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txCreateA, txStartA}})
	_, _ = appB.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txCreateB, txStartB}})
	_, _ = appA.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now().Add(1 * time.Second), Txs: nil})
	_, _ = appB.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now().Add(1 * time.Second), Txs: nil})

	if appA.LastAppHashHex() != appB.LastAppHashHex() {
		t.Fatalf("weave scheduling diverged: A=%s B=%s", appA.LastAppHashHex(), appB.LastAppHashHex())
	}
}
