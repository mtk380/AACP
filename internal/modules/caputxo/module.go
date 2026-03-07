package caputxo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type CapabilityUTXO struct {
	UTXOID      string   `json:"utxo_id"`
	Issuer      []byte   `json:"issuer"`
	Holder      []byte   `json:"holder"`
	CapType     string   `json:"cap_type"`
	CapVersion  string   `json:"cap_version"`
	Scopes      []string `json:"scopes"`
	IssuedAt    int64    `json:"issued_at"`
	ExpiresAt   int64    `json:"expires_at"`
	MaxUses     uint32   `json:"max_uses"`
	UsedCount   uint32   `json:"used_count"`
	Delegatable bool     `json:"delegatable"`
	MaxDepth    uint32   `json:"max_depth"`
	CurDepth    uint32   `json:"cur_depth"`
	ParentUTXO  string   `json:"parent_utxo"`
	Status      string   `json:"status"`
	HolderSig   []byte   `json:"holder_sig"`
}

type MintPayload struct {
	Holder      []byte   `json:"holder"`
	CapType     string   `json:"cap_type"`
	CapVersion  string   `json:"cap_version"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   int64    `json:"expires_at"`
	MaxUses     uint32   `json:"max_uses"`
	Delegatable bool     `json:"delegatable"`
	MaxDepth    uint32   `json:"max_depth"`
}

type SpendPayload struct {
	UTXOID     string `json:"utxo_id"`
	TaskID     string `json:"task_id"`
	ProofOfUse string `json:"proof_of_use"`
}

type DelegatePayload struct {
	ParentUTXOID string   `json:"parent_utxo_id"`
	NewHolder    []byte   `json:"new_holder"`
	Scopes       []string `json:"scopes"`
	ExpiresAt    int64    `json:"expires_at"`
	MaxUses      uint32   `json:"max_uses"`
}

type RevokePayload struct {
	UTXOID string `json:"utxo_id"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "caputxo",
		Handlers: map[string]common.ActionHandler{
			"mint_capability":     m.mint,
			"spend_capability":    m.spend,
			"delegate_capability": m.delegate,
			"revoke_capability":   m.revoke,
			"batch_expire":        m.batchExpire,
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
	return m.expireAtTime(ctx, ctx.BlockTime.Unix())
}

func (m *Module) mint(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[MintPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeCapUTXOBase+1, "decode mint: %v", err)
	}
	if len(p.Holder) == 0 || p.CapType == "" || len(p.Scopes) == 0 {
		return nil, types.NewAppError(types.CodeCapUTXOBase+1, "missing required fields")
	}
	id := utxoID(env.Sender, p.CapType, env.Nonce)
	utxo := &CapabilityUTXO{
		UTXOID:      id,
		Issuer:      append([]byte{}, env.Sender...),
		Holder:      append([]byte{}, p.Holder...),
		CapType:     p.CapType,
		CapVersion:  p.CapVersion,
		Scopes:      append([]string{}, p.Scopes...),
		IssuedAt:    ctx.BlockTime.Unix(),
		ExpiresAt:   p.ExpiresAt,
		MaxUses:     p.MaxUses,
		Delegatable: p.Delegatable,
		MaxDepth:    p.MaxDepth,
		CurDepth:    0,
		Status:      "UTXO_UNSPENT",
	}
	if utxo.Delegatable && utxo.MaxDepth == 0 {
		return nil, types.NewAppError(types.CodeCapUTXOBase+2, "max_depth must be >=1 for delegatable utxo")
	}
	if err := saveUTXO(ctx.Store, utxo); err != nil {
		return nil, err
	}
	ctx.Store.Set([]byte(fmt.Sprintf("%s%x/%s", state.PrefixCapHolder, utxo.Holder, utxo.UTXOID)), []byte{1})
	ctx.Store.Set([]byte(fmt.Sprintf("%s%s/%s", state.PrefixCapType, utxo.CapType, utxo.UTXOID)), []byte{1})
	return types.SuccessResult(3000), nil
}

func (m *Module) spend(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[SpendPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeCapUTXOBase+1, "decode spend: %v", err)
	}
	utxo, err := loadUTXO(ctx.Store, p.UTXOID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(utxo.Holder, env.Sender) {
		return nil, types.NewAppError(types.CodeCapUTXOBase+2, "holder mismatch")
	}
	if utxo.Status != "UTXO_UNSPENT" {
		return nil, types.NewAppError(types.CodeCapUTXOBase+41, "utxo is not unspent")
	}
	if utxo.ExpiresAt > 0 && ctx.BlockTime.Unix() >= utxo.ExpiresAt {
		utxo.Status = "UTXO_EXPIRED"
		_ = saveUTXO(ctx.Store, utxo)
		return nil, types.NewAppError(types.CodeCapUTXOBase+43, "utxo expired")
	}
	utxo.UsedCount++
	if utxo.MaxUses > 0 && utxo.UsedCount >= utxo.MaxUses {
		utxo.Status = "UTXO_SPENT"
	}
	if err := saveUTXO(ctx.Store, utxo); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) delegate(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[DelegatePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeCapUTXOBase+1, "decode delegate: %v", err)
	}
	parent, err := loadUTXO(ctx.Store, p.ParentUTXOID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(parent.Holder, env.Sender) {
		return nil, types.NewAppError(types.CodeCapUTXOBase+2, "holder mismatch")
	}
	if !parent.Delegatable {
		return nil, types.NewAppError(types.CodeCapUTXOBase+42, "utxo is not delegatable")
	}
	childDepth := parent.CurDepth + 1
	if parent.MaxDepth > 0 && childDepth > parent.MaxDepth {
		return nil, types.NewAppError(types.CodeCapUTXOBase+42, "exceeds max_depth")
	}
	if !isSubset(p.Scopes, parent.Scopes) {
		return nil, types.NewAppError(types.CodeCapUTXOBase+42, "delegated scopes must be subset")
	}
	if parent.ExpiresAt > 0 && p.ExpiresAt > parent.ExpiresAt {
		return nil, types.NewAppError(types.CodeCapUTXOBase+42, "delegated expiry exceeds parent")
	}
	if parent.MaxUses > 0 && p.MaxUses > parent.MaxUses {
		return nil, types.NewAppError(types.CodeCapUTXOBase+42, "delegated max_uses exceeds parent")
	}
	id := delegatedUTXOID(parent.UTXOID, env.Sender, env.Nonce)
	child := &CapabilityUTXO{
		UTXOID:      id,
		Issuer:      append([]byte{}, parent.Issuer...),
		Holder:      append([]byte{}, p.NewHolder...),
		CapType:     parent.CapType,
		CapVersion:  parent.CapVersion,
		Scopes:      append([]string{}, p.Scopes...),
		IssuedAt:    ctx.BlockTime.Unix(),
		ExpiresAt:   p.ExpiresAt,
		MaxUses:     p.MaxUses,
		Delegatable: parent.Delegatable,
		MaxDepth:    parent.MaxDepth,
		CurDepth:    childDepth,
		ParentUTXO:  parent.UTXOID,
		Status:      "UTXO_UNSPENT",
	}
	if child.ExpiresAt == 0 {
		child.ExpiresAt = parent.ExpiresAt
	}
	if child.MaxUses == 0 {
		child.MaxUses = parent.MaxUses
	}
	if err := saveUTXO(ctx.Store, child); err != nil {
		return nil, err
	}
	ctx.Store.Set([]byte(fmt.Sprintf("%s%s/%s", state.PrefixCapDerive, parent.UTXOID, child.UTXOID)), []byte{1})
	return types.SuccessResult(3000), nil
}

func (m *Module) revoke(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[RevokePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeCapUTXOBase+1, "decode revoke: %v", err)
	}
	utxo, err := loadUTXO(ctx.Store, p.UTXOID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(utxo.Issuer, env.Sender) {
		return nil, types.NewAppError(types.CodeCapUTXOBase+2, "issuer mismatch")
	}
	utxo.Status = "UTXO_REVOKED"
	if err := saveUTXO(ctx.Store, utxo); err != nil {
		return nil, err
	}
	if _, err := m.cascade(ctx.Store, utxo.UTXOID, "UTXO_REVOKED"); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) batchExpire(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	events, err := m.expireAtTime(ctx, ctx.BlockTime.Unix())
	if err != nil {
		return nil, err
	}
	return types.SuccessResult(100*uint64(len(events)), events...), nil
}

func (m *Module) expireAtTime(ctx *router.Context, now int64) ([]types.Event, error) {
	candidates := ctx.Store.IteratePrefix([]byte(state.PrefixCapUTXO))
	events := make([]types.Event, 0)
	for _, kv := range candidates {
		var utxo CapabilityUTXO
		if err := json.Unmarshal(kv.Value, &utxo); err != nil {
			continue
		}
		if utxo.Status != "UTXO_UNSPENT" || utxo.ExpiresAt == 0 || utxo.ExpiresAt > now {
			continue
		}
		utxo.Status = "UTXO_EXPIRED"
		if err := saveUTXO(ctx.Store, &utxo); err != nil {
			return nil, err
		}
		if _, err := m.cascade(ctx.Store, utxo.UTXOID, "UTXO_EXPIRED"); err != nil {
			return nil, err
		}
		events = append(events, types.Event{Type: "CapabilityExpired", Attributes: map[string]string{"utxo_id": utxo.UTXOID}})
	}
	return events, nil
}

func (m *Module) cascade(store *state.Store, parentID, status string) ([]string, error) {
	affected := make([]string, 0)
	prefix := []byte(state.PrefixCapDerive + parentID + "/")
	children := store.IteratePrefix(prefix)
	for _, child := range children {
		key := string(child.Key)
		parts := splitPath(key)
		if len(parts) == 0 {
			continue
		}
		childID := parts[len(parts)-1]
		u, err := loadUTXO(store, childID)
		if err != nil {
			return nil, err
		}
		if u.Status == "UTXO_UNSPENT" {
			u.Status = status
			if err := saveUTXO(store, u); err != nil {
				return nil, err
			}
			affected = append(affected, childID)
			nested, err := m.cascade(store, childID, status)
			if err != nil {
				return nil, err
			}
			affected = append(affected, nested...)
		}
	}
	return affected, nil
}

func saveUTXO(store *state.Store, utxo *CapabilityUTXO) error {
	return saveJSON(store, state.UTXOKey(utxo.UTXOID), utxo)
}

func loadUTXO(store *state.Store, id string) (*CapabilityUTXO, error) {
	raw, ok := store.Get(state.UTXOKey(id))
	if !ok {
		return nil, types.NewAppError(types.CodeCapUTXOBase, "utxo not found")
	}
	var utxo CapabilityUTXO
	if err := json.Unmarshal(raw, &utxo); err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "decode utxo: %v", err)
	}
	return &utxo, nil
}

func saveJSON(store *state.Store, key []byte, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return types.NewAppError(types.CodeDecodeFail, "marshal state: %v", err)
	}
	store.Set(key, b)
	return nil
}

func utxoID(issuer []byte, capType string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%x|%s|%d", issuer, capType, nonce)))
	return hex.EncodeToString(h[:8])
}

func delegatedUTXOID(parentUTXOID string, delegator []byte, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%x|%d", parentUTXOID, delegator, nonce)))
	return hex.EncodeToString(h[:8])
}

func isSubset(subset, superset []string) bool {
	if len(subset) == 0 {
		return false
	}
	m := make(map[string]struct{}, len(superset))
	for _, s := range superset {
		m[s] = struct{}{}
	}
	for _, s := range subset {
		if _, ok := m[s]; !ok {
			return false
		}
	}
	return true
}

func splitPath(k string) []string {
	parts := make([]string, 0)
	curr := ""
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			parts = append(parts, curr)
			curr = ""
			continue
		}
		curr += string(k[i])
	}
	if curr != "" {
		parts = append(parts, curr)
	}
	return parts
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
