package gov

import (
	"encoding/json"
	"fmt"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type Proposal struct {
	ProposalID string         `json:"proposal_id"`
	Type       string         `json:"type"`
	Status     string         `json:"status"`
	Changes    []ParamChange  `json:"changes"`
	Votes      map[string]int `json:"votes"`
	CreatedAt  int64          `json:"created_at"`
	ClosedAt   int64          `json:"closed_at"`
}

type ParamChange struct {
	Module string `json:"module"`
	Key    string `json:"key"`
	Value  any    `json:"value"`
}

type CreateProposalPayload struct {
	ProposalID string       `json:"proposal_id"`
	Type       string       `json:"type"`
	Changes    []ParamChange `json:"changes"`
}

type VotePayload struct {
	ProposalID string `json:"proposal_id"`
	Vote       string `json:"vote"`
}

type ExecutePayload struct {
	ProposalID string `json:"proposal_id"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "gov",
		Handlers: map[string]common.ActionHandler{
			"create_proposal": m.createProposal,
			"vote_proposal":   m.voteProposal,
			"execute_proposal": m.executeProposal,
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

func (m *Module) createProposal(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[CreateProposalPayload](env)
	if err != nil {
		return nil, err
	}
	if p.ProposalID == "" {
		p.ProposalID = fmt.Sprintf("p-%d", ctx.BlockTime.UnixNano())
	}
	prop := &Proposal{
		ProposalID: p.ProposalID,
		Type:       p.Type,
		Status:     "OPEN",
		Changes:    p.Changes,
		Votes:      map[string]int{"yes": 0, "no": 0, "abstain": 0},
		CreatedAt:  ctx.BlockTime.Unix(),
	}
	if err := saveProposal(ctx.Store, prop); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) voteProposal(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[VotePayload](env)
	if err != nil {
		return nil, err
	}
	prop, err := loadProposal(ctx.Store, p.ProposalID)
	if err != nil {
		return nil, err
	}
	if prop.Status != "OPEN" {
		return nil, types.NewAppError(types.CodeGovBase+1, "proposal is not open")
	}
	if _, ok := prop.Votes[p.Vote]; !ok {
		return nil, types.NewAppError(types.CodeGovBase+1, "invalid vote")
	}
	prop.Votes[p.Vote]++
	if err := saveProposal(ctx.Store, prop); err != nil {
		return nil, err
	}
	ctx.Store.Set([]byte(fmt.Sprintf("%s%s/%x", state.PrefixGovVote, prop.ProposalID, env.Sender)), []byte(p.Vote))
	return types.SuccessResult(1500), nil
}

func (m *Module) executeProposal(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[ExecutePayload](env)
	if err != nil {
		return nil, err
	}
	prop, err := loadProposal(ctx.Store, p.ProposalID)
	if err != nil {
		return nil, err
	}
	if prop.Status != "OPEN" {
		return nil, types.NewAppError(types.CodeGovBase+1, "proposal is not open")
	}
	if prop.Votes["yes"] <= prop.Votes["no"] {
		return nil, types.NewAppError(types.CodeGovBase+1, "proposal not approved")
	}
	for _, c := range prop.Changes {
		if err := ctx.Store.SetParam(c.Module, c.Key, c.Value); err != nil {
			return nil, err
		}
	}
	prop.Status = "EXECUTED"
	prop.ClosedAt = ctx.BlockTime.Unix()
	if err := saveProposal(ctx.Store, prop); err != nil {
		return nil, err
	}
	return types.SuccessResult(2500), nil
}

func saveProposal(store *state.Store, p *Proposal) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	store.Set([]byte(state.PrefixGovProp+p.ProposalID), b)
	return nil
}

func loadProposal(store *state.Store, id string) (*Proposal, error) {
	raw, ok := store.Get([]byte(state.PrefixGovProp + id))
	if !ok {
		return nil, types.NewAppError(types.CodeGovBase+90, "proposal not found")
	}
	var p Proposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

var _ router.Module = (*Module)(nil)
