package node

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type NodeRegistration struct {
	PubKey       []byte       `json:"pubkey"`
	Tier         string       `json:"tier"`
	Moniker      string       `json:"moniker"`
	Endpoint     string       `json:"endpoint"`
	Region       string       `json:"region"`
	Hardware     HardwareSpec `json:"hardware"`
	DepositTxID  string       `json:"deposit_tx_id"`
	RegisteredAt int64        `json:"registered_at"`
	Signature    []byte       `json:"signature"`
	Status       string       `json:"status"`
	Uptime30d    float64      `json:"uptime_30d"`
	Reputation   uint32       `json:"reputation"`
	Deposit      string       `json:"deposit"`
}

type HardwareSpec struct {
	CPUCores      uint32 `json:"cpu_cores"`
	RAMMB         uint64 `json:"ram_mb"`
	StorageGB     uint64 `json:"storage_gb"`
	BandwidthMbps uint32 `json:"bandwidth_mbps"`
	Arch          string `json:"arch"`
	GPU           string `json:"gpu"`
}

type RegisterPayload struct {
	Tier        string       `json:"tier"`
	Moniker     string       `json:"moniker"`
	Endpoint    string       `json:"endpoint"`
	Region      string       `json:"region"`
	Hardware    HardwareSpec `json:"hardware"`
	DepositTxID string       `json:"deposit_tx_id"`
	Signature   []byte       `json:"signature"`
	Deposit     string       `json:"deposit"`
}

type RotatePayload struct {
	OldPubKey []byte `json:"old_pubkey"`
	NewPubKey []byte `json:"new_pubkey"`
	Signature []byte `json:"signature"`
}

type EpochPayload struct {
	MaxValidators uint32 `json:"max_validators"`
}

type banPayload struct {
	PubKey []byte `json:"pubkey"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "node",
		Handlers: map[string]common.ActionHandler{
			"register_node":   m.registerNode,
			"rotate_key":      m.rotateKey,
			"elect_validators": m.electValidators,
			"ban_node":        m.banNode,
		},
	}
	return m
}

func (m *Module) Name() string                                              { return m.base.Name() }
func (m *Module) InitGenesis(ctx *router.Context, raw []byte) error         { return m.base.InitGenesis(ctx, raw) }
func (m *Module) Query(ctx *router.Context, path string, data []byte) ([]byte, error) {
	return m.base.Query(ctx, path, data)
}
func (m *Module) EndBlock(ctx *router.Context) ([]types.Event, error) {
	return nil, nil
}
func (m *Module) ExecuteTx(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.base.ExecuteTx(ctx, env)
}

func (m *Module) registerNode(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[RegisterPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeNodeBase+1, "decode register_node: %v", err)
	}
	if err := validateHardware(p.Tier, p.Hardware); err != nil {
		return nil, types.NewAppError(types.CodeNodeBase+81, err.Error())
	}
	n := &NodeRegistration{
		PubKey:       append([]byte{}, env.Sender...),
		Tier:         p.Tier,
		Moniker:      p.Moniker,
		Endpoint:     p.Endpoint,
		Region:       p.Region,
		Hardware:     p.Hardware,
		DepositTxID:  p.DepositTxID,
		RegisteredAt: ctx.BlockTime.Unix(),
		Signature:    append([]byte{}, p.Signature...),
		Status:       "REGISTERED",
		Uptime30d:    1.0,
		Reputation:   500,
		Deposit:      p.Deposit,
	}
	if n.Deposit == "" {
		n.Deposit = "0"
	}
	if err := saveNode(ctx.Store, n); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) rotateKey(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[RotatePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeNodeBase+1, "decode rotate_key: %v", err)
	}
	old, err := loadNode(ctx.Store, p.OldPubKey)
	if err != nil {
		return nil, err
	}
	if !equalBytes(old.PubKey, env.Sender) {
		return nil, types.NewAppError(types.CodeNodeBase+1, "sender is not old key")
	}
	old.PubKey = append([]byte{}, p.NewPubKey...)
	if err := saveNode(ctx.Store, old); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) electValidators(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[EpochPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeNodeBase+1, "decode elect_validators: %v", err)
	}
	if p.MaxValidators == 0 {
		p.MaxValidators = 21
	}
	all := ctx.Store.IteratePrefix([]byte(state.PrefixNodeReg))
	type candidate struct {
		Node  *NodeRegistration
		Score float64
	}
	maxDep := 0.0
	cands := make([]candidate, 0)
	for _, kv := range all {
		var n NodeRegistration
		if err := json.Unmarshal(kv.Value, &n); err != nil {
			continue
		}
		if n.Reputation < 500 {
			continue
		}
		if n.Tier != "T0_VALIDATOR" && n.Tier != "T1_FULL" {
			continue
		}
		dep := parseFloat(n.Deposit)
		if dep > maxDep {
			maxDep = dep
		}
		cands = append(cands, candidate{Node: &n})
	}
	if maxDep == 0 {
		maxDep = 1
	}
	for i := range cands {
		n := cands[i].Node
		dep := parseFloat(n.Deposit)
		score := (dep/maxDep)*0.5 + (float64(n.Reputation)/1000.0)*0.3 + n.Uptime30d*0.2
		cands[i].Score = score
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Score == cands[j].Score {
			return hex.EncodeToString(cands[i].Node.PubKey) < hex.EncodeToString(cands[j].Node.PubKey)
		}
		return cands[i].Score > cands[j].Score
	})
	if len(cands) > int(p.MaxValidators) {
		cands = cands[:p.MaxValidators]
	}
	for i, c := range cands {
		c.Node.Status = "ACTIVE_VALIDATOR"
		ctx.Store.Set([]byte(fmt.Sprintf("%s%s", state.PrefixNodeVal, hex.EncodeToString(c.Node.PubKey))), []byte(fmt.Sprintf("%d", int(c.Score*100))))
		_ = saveNode(ctx.Store, c.Node)
		_ = i
	}
	return types.SuccessResult(5000), nil
}

func (m *Module) banNode(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[banPayload](env)
	if err != nil {
		return nil, err
	}
	n, err := loadNode(ctx.Store, p.PubKey)
	if err != nil {
		return nil, err
	}
	n.Status = "BANNED"
	if err := saveNode(ctx.Store, n); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func saveNode(store *state.Store, n *NodeRegistration) error {
	k := []byte(state.PrefixNodeReg + hex.EncodeToString(n.PubKey))
	b, err := json.Marshal(n)
	if err != nil {
		return err
	}
	store.Set(k, b)
	return nil
}

func loadNode(store *state.Store, pubkey []byte) (*NodeRegistration, error) {
	k := []byte(state.PrefixNodeReg + hex.EncodeToString(pubkey))
	raw, ok := store.Get(k)
	if !ok {
		return nil, types.NewAppError(types.CodeNodeBase, "node not found")
	}
	var n NodeRegistration
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func validateHardware(tier string, hw HardwareSpec) error {
	req := minimumHardware(tier)
	if hw.CPUCores < req.CPUCores || hw.RAMMB < req.RAMMB || hw.StorageGB < req.StorageGB || hw.BandwidthMbps < req.BandwidthMbps {
		return fmt.Errorf("hardware requirement not met for tier %s", tier)
	}
	return nil
}

func minimumHardware(tier string) HardwareSpec {
	switch tier {
	case "T0_VALIDATOR":
		return HardwareSpec{CPUCores: 16, RAMMB: 64 * 1024, StorageGB: 2 * 1024, BandwidthMbps: 1000}
	case "T1_FULL":
		return HardwareSpec{CPUCores: 8, RAMMB: 32 * 1024, StorageGB: 1024, BandwidthMbps: 500}
	case "T2_ARCHIVE":
		return HardwareSpec{CPUCores: 8, RAMMB: 64 * 1024, StorageGB: 10 * 1024, BandwidthMbps: 200}
	case "T3_RELAY":
		return HardwareSpec{CPUCores: 4, RAMMB: 8 * 1024, StorageGB: 500, BandwidthMbps: 500}
	case "T4_EDGE":
		return HardwareSpec{CPUCores: 2, RAMMB: 4 * 1024, StorageGB: 100, BandwidthMbps: 100}
	default:
		return HardwareSpec{CPUCores: 1, RAMMB: 1024, StorageGB: 32, BandwidthMbps: 10}
	}
}

func parseFloat(s string) float64 {
	var intPart int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		intPart = intPart*10 + int64(s[i]-'0')
	}
	return float64(intPart)
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

func DeriveNodeAddress(pub []byte) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:20])
}

var _ router.Module = (*Module)(nil)
