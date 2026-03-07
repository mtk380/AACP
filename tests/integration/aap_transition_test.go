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

func TestAAPTaskTransitionsToCompleted(t *testing.T) {
	app := abci.NewApp()
	_, _ = app.InitChain(&abci.InitChainRequest{})

	provider, _ := crypto.GenerateKeyPair()
	consumer, _ := crypto.GenerateKeyPair()

	txCreate := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 0, "aap", "create_task", map[string]any{
		"order_id": "order-1",
		"consumer": consumer.PubKey,
		"provider": provider.PubKey,
		"service_fee": map[string]any{"currency": "CNY", "amount": "10"},
		"timeout_sec": 120,
	})
	resp, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 1, Time: time.Now(), Txs: [][]byte{txCreate}})
	if err != nil || resp.TxResults[0].Code != 0 {
		t.Fatalf("create task failed err=%v code=%d", err, resp.TxResults[0].Code)
	}
	tasks := app.Store().IteratePrefix([]byte(state.PrefixAAPTask))
	if len(tasks) != 1 {
		t.Fatalf("expected one task")
	}
	var task struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(tasks[0].Value, &task); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}

	txSign := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 1, "aap", "sign_sla", map[string]any{"task_id": task.TaskID, "consumer_sig": []byte("c"), "provider_sig": []byte("p")})
	txHB := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 2, "aap", "submit_heartbeat", map[string]any{"task_id": task.TaskID, "seq": 1, "progress": 10, "timestamp": time.Now().Unix()})
	txResult := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 3, "aap", "submit_result", map[string]any{"task_id": task.TaskID, "content_hash": "abc"})
	txVerify := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 4, "aap", "verify_result", map[string]any{"task_id": task.TaskID, "passed": true})
	txSettle := testutil.MustSignedTx(provider.PubKey, provider.PrivKey, 5, "aap", "settle_task", map[string]any{"task_id": task.TaskID})

	resp2, err := app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: 2, Time: time.Now(), Txs: [][]byte{txSign, txHB, txResult, txVerify, txSettle}})
	if err != nil {
		t.Fatalf("finalize transitions failed: %v", err)
	}
	for i, r := range resp2.TxResults {
		if r.Code != 0 {
			t.Fatalf("tx %d failed code=%d log=%s", i, r.Code, r.Log)
		}
	}

	latest := app.Store().IteratePrefix([]byte(state.PrefixAAPTask))
	var final struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(latest[0].Value, &final); err != nil {
		t.Fatalf("decode final task failed: %v", err)
	}
	if final.Status != "TASK_COMPLETED" {
		t.Fatalf("expected TASK_COMPLETED got %s", final.Status)
	}
}
