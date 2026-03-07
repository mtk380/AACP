package weave

import (
	"encoding/json"
	"fmt"
	"sort"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type TaskDAG struct {
	DAGID            string    `json:"dag_id"`
	RootTask         string    `json:"root_task"`
	Orchestrator     []byte    `json:"orchestrator"`
	Nodes            []DAGNode `json:"nodes"`
	Edges            []DAGEdge `json:"edges"`
	Status           string    `json:"status"`
	CreatedAt        int64     `json:"created_at"`
	StartedAt        int64     `json:"started_at"`
	FinishedAt       int64     `json:"finished_at"`
	GlobalTimeoutSec uint64    `json:"global_timeout_sec"`
}

type DAGNode struct {
	NodeID      string `json:"node_id"`
	Type        string `json:"type"`
	SubTaskID   string `json:"sub_task_id"`
	ListingID   string `json:"listing_id"`
	Status      string `json:"status"`
	RetryCount  uint32 `json:"retry_count"`
	MaxRetries  uint32 `json:"max_retries"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
	OutputRef   string `json:"output_ref"`
}

type DAGEdge struct {
	FromNode  string `json:"from_node"`
	ToNode    string `json:"to_node"`
	Condition string `json:"condition"`
}

type createDAGPayload struct {
	DAGID            string    `json:"dag_id"`
	RootTask         string    `json:"root_task"`
	Nodes            []DAGNode `json:"nodes"`
	Edges            []DAGEdge `json:"edges"`
	GlobalTimeoutSec uint64    `json:"global_timeout_sec"`
}

type simpleDAGPayload struct {
	DAGID string `json:"dag_id"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "weave",
		Handlers: map[string]common.ActionHandler{
			"create_dag":          m.createDAG,
			"start_dag":           m.startDAG,
			"update_node_status":  m.updateNodeStatus,
			"cancel_dag":          m.cancelDAG,
			"submit_node_output":  m.submitNodeOutput,
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
	dags := ctx.Store.IteratePrefix([]byte(state.PrefixWeaveDAG))
	out := make([]types.Event, 0)
	now := ctx.BlockTime.Unix()
	for _, kv := range dags {
		var dag TaskDAG
		if err := json.Unmarshal(kv.Value, &dag); err != nil {
			continue
		}
		if dag.Status != "DAG_RUNNING" {
			continue
		}
		if dag.GlobalTimeoutSec > 0 && now-dag.StartedAt > int64(dag.GlobalTimeoutSec) {
			dag.Status = "DAG_FAILED"
			dag.FinishedAt = now
			_ = saveDAG(ctx.Store, &dag)
			out = append(out, types.Event{Type: "DAGFailed", Attributes: map[string]string{"dag_id": dag.DAGID, "reason": "global_timeout"}})
			continue
		}
		progressDag(&dag, now)
		_ = saveDAG(ctx.Store, &dag)
		if dag.Status == "DAG_COMPLETED" {
			out = append(out, types.Event{Type: "DAGCompleted", Attributes: map[string]string{"dag_id": dag.DAGID}})
		}
	}
	return out, nil
}

func (m *Module) createDAG(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[createDAGPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "decode create_dag: %v", err)
	}
	dag := &TaskDAG{
		DAGID:            p.DAGID,
		RootTask:         p.RootTask,
		Orchestrator:     append([]byte{}, env.Sender...),
		Nodes:            p.Nodes,
		Edges:            p.Edges,
		Status:           "DAG_PENDING",
		CreatedAt:        ctx.BlockTime.Unix(),
		GlobalTimeoutSec: p.GlobalTimeoutSec,
	}
	if dag.DAGID == "" {
		dag.DAGID = fmt.Sprintf("%x", ctx.BlockTime.UnixNano())[:16]
	}
	if err := ValidateDAG(dag); err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase, "invalid dag: %v", err)
	}
	if err := saveDAG(ctx.Store, dag); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000+500*uint64(len(dag.Nodes))), nil
}

func (m *Module) startDAG(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[simpleDAGPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "decode start_dag: %v", err)
	}
	dag, err := loadDAG(ctx.Store, p.DAGID)
	if err != nil {
		return nil, err
	}
	if dag.Status != "DAG_PENDING" {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "dag status %s cannot start", dag.Status)
	}
	dag.Status = "DAG_RUNNING"
	dag.StartedAt = ctx.BlockTime.Unix()
	for i := range dag.Nodes {
		dag.Nodes[i].Status = "NODE_PENDING"
	}
	if err := saveDAG(ctx.Store, dag); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) updateNodeStatus(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	type payload struct {
		DAGID  string `json:"dag_id"`
		NodeID string `json:"node_id"`
		Status string `json:"status"`
	}
	p, err := common.DecodeJSONPayload[payload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "decode update_node_status: %v", err)
	}
	dag, err := loadDAG(ctx.Store, p.DAGID)
	if err != nil {
		return nil, err
	}
	for i := range dag.Nodes {
		if dag.Nodes[i].NodeID == p.NodeID {
			dag.Nodes[i].Status = p.Status
			dag.Nodes[i].FinishedAt = ctx.BlockTime.Unix()
			break
		}
	}
	if err := saveDAG(ctx.Store, dag); err != nil {
		return nil, err
	}
	return types.SuccessResult(1000), nil
}

func (m *Module) cancelDAG(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[simpleDAGPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "decode cancel_dag: %v", err)
	}
	dag, err := loadDAG(ctx.Store, p.DAGID)
	if err != nil {
		return nil, err
	}
	dag.Status = "DAG_CANCELED"
	dag.FinishedAt = ctx.BlockTime.Unix()
	if err := saveDAG(ctx.Store, dag); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) submitNodeOutput(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	type payload struct {
		DAGID     string `json:"dag_id"`
		NodeID    string `json:"node_id"`
		OutputRef string `json:"output_ref"`
	}
	p, err := common.DecodeJSONPayload[payload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "decode submit_node_output: %v", err)
	}
	dag, err := loadDAG(ctx.Store, p.DAGID)
	if err != nil {
		return nil, err
	}
	for i := range dag.Nodes {
		if dag.Nodes[i].NodeID == p.NodeID {
			dag.Nodes[i].OutputRef = p.OutputRef
			dag.Nodes[i].Status = "NODE_COMPLETED"
			dag.Nodes[i].FinishedAt = ctx.BlockTime.Unix()
			break
		}
	}
	if err := saveDAG(ctx.Store, dag); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func ValidateDAG(dag *TaskDAG) error {
	if len(dag.Nodes) < 2 {
		return fmt.Errorf("dag must have at least 2 nodes")
	}
	nodeSet := make(map[string]struct{}, len(dag.Nodes))
	for _, n := range dag.Nodes {
		if n.NodeID == "" {
			return fmt.Errorf("node_id must not be empty")
		}
		if _, dup := nodeSet[n.NodeID]; dup {
			return fmt.Errorf("duplicate node_id: %s", n.NodeID)
		}
		nodeSet[n.NodeID] = struct{}{}
	}
	indeg := make(map[string]int, len(dag.Nodes))
	adj := make(map[string][]string)
	for _, n := range dag.Nodes {
		indeg[n.NodeID] = 0
	}
	for _, e := range dag.Edges {
		if _, ok := nodeSet[e.FromNode]; !ok {
			return fmt.Errorf("edge from unknown node: %s", e.FromNode)
		}
		if _, ok := nodeSet[e.ToNode]; !ok {
			return fmt.Errorf("edge to unknown node: %s", e.ToNode)
		}
		if e.FromNode == e.ToNode {
			return fmt.Errorf("self loop: %s", e.FromNode)
		}
		indeg[e.ToNode]++
		adj[e.FromNode] = append(adj[e.FromNode], e.ToNode)
	}
	q := make([]string, 0)
	for id, d := range indeg {
		if d == 0 {
			q = append(q, id)
		}
	}
	sort.Strings(q)
	visited := 0
	for len(q) > 0 {
		curr := q[0]
		q = q[1:]
		visited++
		nexts := append([]string{}, adj[curr]...)
		sort.Strings(nexts)
		for _, nxt := range nexts {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				q = append(q, nxt)
				sort.Strings(q)
			}
		}
	}
	if visited != len(dag.Nodes) {
		return fmt.Errorf("cycle detected")
	}
	if dag.GlobalTimeoutSec == 0 {
		return fmt.Errorf("global_timeout_sec must be > 0")
	}
	return nil
}

func progressDag(dag *TaskDAG, now int64) {
	for i := range dag.Nodes {
		n := &dag.Nodes[i]
		if n.Status == "NODE_PENDING" {
			n.Status = "NODE_READY"
		}
		if n.Status == "NODE_READY" {
			n.Status = "NODE_RUNNING"
			n.StartedAt = now
			continue
		}
		if n.Status == "NODE_RUNNING" {
			n.Status = "NODE_COMPLETED"
			n.FinishedAt = now
		}
	}
	allDone := true
	for _, n := range dag.Nodes {
		if n.Status != "NODE_COMPLETED" && n.Status != "NODE_SKIPPED" {
			allDone = false
			break
		}
	}
	if allDone {
		dag.Status = "DAG_COMPLETED"
		dag.FinishedAt = now
	}
}

func saveDAG(store *state.Store, dag *TaskDAG) error {
	return saveJSON(store, []byte(state.PrefixWeaveDAG+dag.DAGID), dag)
}

func loadDAG(store *state.Store, id string) (*TaskDAG, error) {
	raw, ok := store.Get([]byte(state.PrefixWeaveDAG + id))
	if !ok {
		return nil, types.NewAppError(types.CodeWEAVEBase+1, "dag not found")
	}
	var dag TaskDAG
	if err := json.Unmarshal(raw, &dag); err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "decode dag: %v", err)
	}
	return &dag, nil
}

func saveJSON(store *state.Store, key []byte, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return types.NewAppError(types.CodeDecodeFail, "marshal state: %v", err)
	}
	store.Set(key, b)
	return nil
}

var _ router.Module = (*Module)(nil)
