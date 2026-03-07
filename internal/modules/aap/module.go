package aap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type Task struct {
	TaskID        string      `json:"task_id"`
	OrderID       string      `json:"order_id"`
	Consumer      []byte      `json:"consumer"`
	Provider      []byte      `json:"provider"`
	SLA           SLA         `json:"sla"`
	Status        string      `json:"status"`
	ResultHash    string      `json:"result_hash"`
	ResultIPFS    string      `json:"result_ipfs"`
	EvidenceRoot  string      `json:"evidence_root"`
	CreatedAt     int64       `json:"created_at"`
	SLASignedAt   int64       `json:"sla_signed_at"`
	StartedAt     int64       `json:"started_at"`
	CompletedAt   int64       `json:"completed_at"`
	HeartbeatSeq  uint64      `json:"heartbeat_seq"`
	LastHeartbeat int64       `json:"last_heartbeat"`
	RetryCount    uint32      `json:"retry_count"`
	MaxRetries    uint32      `json:"max_retries"`
	ServiceFee    types.Money `json:"service_fee"`
}

type SLA struct {
	MaxLatencyMS  uint32      `json:"max_latency_ms"`
	MinAccuracy   float32     `json:"min_accuracy"`
	MaxRetries    uint32      `json:"max_retries"`
	TimeoutSec    uint64      `json:"timeout_sec"`
	ServiceFee    types.Money `json:"service_fee"`
	DepositAmount types.Money `json:"deposit_amount"`
	CommissionBPS uint32      `json:"commission_bps"`
	ConsumerSig   []byte      `json:"consumer_sig"`
	ProviderSig   []byte      `json:"provider_sig"`
	SignedAt      int64       `json:"signed_at"`
	ExpiresAt     int64       `json:"expires_at"`
}

type CreateTaskPayload struct {
	OrderID     string      `json:"order_id"`
	Consumer    []byte      `json:"consumer"`
	Provider    []byte      `json:"provider"`
	MaxRetries  uint32      `json:"max_retries"`
	ServiceFee  types.Money `json:"service_fee"`
	TimeoutSec  uint64      `json:"timeout_sec"`
	MaxLatency  uint32      `json:"max_latency_ms"`
	MinAccuracy float32     `json:"min_accuracy"`
}

type SignSLAPayload struct {
	TaskID       string `json:"task_id"`
	ConsumerSig  []byte `json:"consumer_sig"`
	ProviderSig  []byte `json:"provider_sig"`
	TimeoutSec   uint64 `json:"timeout_sec"`
	CommissionBP uint32 `json:"commission_bps"`
}

type HeartbeatPayload struct {
	TaskID     string `json:"task_id"`
	Seq        uint64 `json:"seq"`
	Progress   uint32 `json:"progress"`
	StatusMsg  string `json:"status_msg"`
	Timestamp  int64  `json:"timestamp"`
	Signature  []byte `json:"signature"`
}

type SubmitResultPayload struct {
	TaskID      string `json:"task_id"`
	ResultData  []byte `json:"result_data"`
	ResultIPFS  string `json:"result_ipfs"`
	ContentHash string `json:"content_hash"`
}

type VerifyPayload struct {
	TaskID    string `json:"task_id"`
	Passed    bool   `json:"passed"`
	Reason    string `json:"reason"`
	Verifier  []byte `json:"verifier"`
}

type settlePayload struct {
	TaskID string `json:"task_id"`
}

type disputePayload struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "aap",
		Handlers: map[string]common.ActionHandler{
			"create_task":      m.createTask,
			"sign_sla":         m.signSLA,
			"submit_heartbeat": m.submitHeartbeat,
			"submit_result":    m.submitResult,
			"verify_result":    m.verifyResult,
			"settle_task":      m.settleTask,
			"file_dispute":     m.fileDispute,
		},
	}
	return m
}

func (m *Module) Name() string                                              { return m.base.Name() }
func (m *Module) InitGenesis(ctx *router.Context, raw []byte) error         { return m.base.InitGenesis(ctx, raw) }
func (m *Module) Query(ctx *router.Context, path string, data []byte) ([]byte, error) {
	return m.base.Query(ctx, path, data)
}
func (m *Module) ExecuteTx(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.base.ExecuteTx(ctx, env)
}

func (m *Module) EndBlock(ctx *router.Context) ([]types.Event, error) {
	active := ctx.Store.IteratePrefix([]byte(state.PrefixAAPTask))
	failures := make([]types.Event, 0)
	now := ctx.BlockTime.Unix()
	for _, kv := range active {
		var task Task
		if err := json.Unmarshal(kv.Value, &task); err != nil {
			continue
		}
		if task.Status != "TASK_EXECUTING" {
			continue
		}
		if now-task.LastHeartbeat > 180 {
			task.Status = "TASK_FAILED"
			_ = saveTask(ctx.Store, &task)
			failures = append(failures, types.Event{Type: "TaskFailed", Attributes: map[string]string{"task_id": task.TaskID, "reason": "heartbeat_timeout"}})
		}
	}
	return failures, nil
}

func (m *Module) createTask(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[CreateTaskPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode create_task: %v", err)
	}
	id := taskID(p.OrderID, env.Nonce)
	now := ctx.BlockTime.Unix()
	maxRetries := p.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	task := &Task{
		TaskID:      id,
		OrderID:     p.OrderID,
		Consumer:    append([]byte{}, p.Consumer...),
		Provider:    append([]byte{}, p.Provider...),
		Status:      "TASK_CREATED",
		CreatedAt:   now,
		MaxRetries:  maxRetries,
		ServiceFee:  p.ServiceFee,
		SLA: SLA{TimeoutSec: p.TimeoutSec, MaxLatencyMS: p.MaxLatency, MinAccuracy: p.MinAccuracy, MaxRetries: maxRetries, ServiceFee: p.ServiceFee, CommissionBPS: 1200},
	}
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000, types.Event{Type: "TaskCreated", Attributes: map[string]string{"task_id": id, "order_id": p.OrderID}}), nil
}

func (m *Module) signSLA(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[SignSLAPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode sign_sla: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	if task.Status != "TASK_CREATED" {
		return nil, types.NewAppError(types.CodeAAPBase+2, "task status %s cannot sign sla", task.Status)
	}
	task.SLA.ConsumerSig = append([]byte{}, p.ConsumerSig...)
	task.SLA.ProviderSig = append([]byte{}, p.ProviderSig...)
	task.SLA.SignedAt = ctx.BlockTime.Unix()
	task.SLA.ExpiresAt = ctx.BlockTime.Add(30 * time.Minute).Unix()
	if p.TimeoutSec > 0 {
		task.SLA.TimeoutSec = p.TimeoutSec
	}
	if p.CommissionBP > 0 {
		task.SLA.CommissionBPS = p.CommissionBP
	}
	task.Status = "TASK_SLA_SIGNED"
	task.SLASignedAt = task.SLA.SignedAt
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) submitHeartbeat(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[HeartbeatPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode submit_heartbeat: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(task.Provider, env.Sender) {
		return nil, types.NewAppError(types.CodeAAPBase+2, "provider mismatch")
	}
	if p.Seq <= task.HeartbeatSeq {
		return nil, types.NewAppError(types.CodeAAPBase+2, "heartbeat sequence must increase")
	}
	if task.Status == "TASK_AUTHORIZED" || task.Status == "TASK_CREATED" || task.Status == "TASK_SLA_SIGNED" {
		task.Status = "TASK_EXECUTING"
		task.StartedAt = ctx.BlockTime.Unix()
	}
	task.HeartbeatSeq = p.Seq
	task.LastHeartbeat = p.Timestamp
	if task.LastHeartbeat == 0 {
		task.LastHeartbeat = ctx.BlockTime.Unix()
	}
	hbKey := state.HeartbeatKey(task.TaskID, p.Seq)
	_ = saveJSON(ctx.Store, hbKey, p)
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	return types.SuccessResult(500), nil
}

func (m *Module) submitResult(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[SubmitResultPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode submit_result: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(task.Provider, env.Sender) {
		return nil, types.NewAppError(types.CodeAAPBase+2, "provider mismatch")
	}
	task.ResultHash = p.ContentHash
	task.ResultIPFS = p.ResultIPFS
	task.Status = "TASK_SUBMIT_RESULT"
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	return types.SuccessResult(5000), nil
}

func (m *Module) verifyResult(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[VerifyPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode verify_result: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	if task.Status != "TASK_SUBMIT_RESULT" && task.Status != "TASK_VERIFYING" {
		return nil, types.NewAppError(types.CodeAAPBase+23, "task is not ready for verification")
	}
	if p.Passed {
		task.Status = "TASK_VERIFIED"
	} else {
		task.RetryCount++
		if task.RetryCount >= task.MaxRetries {
			task.Status = "TASK_DISPUTED"
		} else {
			task.Status = "TASK_AUTHORIZED"
		}
	}
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) settleTask(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[settlePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode settle_task: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	if task.Status != "TASK_VERIFIED" {
		return nil, types.NewAppError(types.CodeAAPBase+23, "task status %s cannot settle", task.Status)
	}
	task.Status = "TASK_COMPLETED"
	task.CompletedAt = ctx.BlockTime.Unix()
	task.EvidenceRoot = computeEvidenceRoot(task)
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	ctx.Store.Set([]byte(state.PrefixAAPEvidence+task.TaskID), []byte(task.EvidenceRoot))
	return types.SuccessResult(4000, types.Event{Type: "TaskCompleted", Attributes: map[string]string{"task_id": task.TaskID}}), nil
}

func (m *Module) fileDispute(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[disputePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAAPBase+1, "decode file_dispute: %v", err)
	}
	task, err := loadTask(ctx.Store, p.TaskID)
	if err != nil {
		return nil, err
	}
	task.Status = "TASK_DISPUTED"
	if err := saveTask(ctx.Store, task); err != nil {
		return nil, err
	}
	ctx.EventBus.Emit(types.Event{Type: "DisputeFiled", Attributes: map[string]string{"task_id": task.TaskID, "reason": p.Reason}})
	return types.SuccessResult(3000), nil
}

func saveTask(store *state.Store, task *Task) error {
	return saveJSON(store, state.TaskKey(task.TaskID), task)
}

func loadTask(store *state.Store, id string) (*Task, error) {
	raw, ok := store.Get(state.TaskKey(id))
	if !ok {
		return nil, types.NewAppError(types.CodeAAPBase, "task not found")
	}
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "decode task: %v", err)
	}
	return &task, nil
}

func saveJSON(store *state.Store, key []byte, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return types.NewAppError(types.CodeDecodeFail, "marshal state: %v", err)
	}
	store.Set(key, b)
	return nil
}

func taskID(orderID string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", orderID, nonce)))
	return hex.EncodeToString(h[:8])
}

func computeEvidenceRoot(task *Task) string {
	base := fmt.Sprintf("%s|%s|%s|%s", task.TaskID, task.OrderID, task.ResultHash, task.Status)
	h := sha256.Sum256([]byte(base))
	return hex.EncodeToString(h[:])
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ router.Module = (*Module)(nil)
