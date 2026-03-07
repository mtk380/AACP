package arb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type Dispute struct {
	DisputeID         string       `json:"dispute_id"`
	OrderID           string       `json:"order_id"`
	TaskID            string       `json:"task_id"`
	Plaintiff         []byte       `json:"plaintiff"`
	Defendant         []byte       `json:"defendant"`
	ReasonCode        string       `json:"reason_code"`
	Description       string       `json:"description"`
	PlaintiffEvidence []Evidence   `json:"plaintiff_evidence"`
	DefendantEvidence []Evidence   `json:"defendant_evidence"`
	Phase             string       `json:"phase"`
	Status            string       `json:"status"`
	Verdict           Verdict      `json:"verdict"`
	CommitteeMembers  [][]byte     `json:"committee_members"`
	CommitteeVotes    []VoteRecord `json:"committee_votes"`
	FullVotes         []VoteRecord `json:"full_votes"`
	FiledAt           int64        `json:"filed_at"`
	ResolvedAt        int64        `json:"resolved_at"`
}

type Evidence struct {
	Submitter   []byte `json:"submitter"`
	EvidenceType string `json:"evidence_type"`
	IPFSCID     string `json:"ipfs_cid"`
	InlineData  []byte `json:"inline_data"`
	ContentHash string `json:"content_hash"`
	SubmittedAt int64  `json:"submitted_at"`
	Signature   []byte `json:"signature"`
}

type Verdict struct {
	Winner           string      `json:"winner"`
	SplitRatio       string      `json:"split_ratio"`
	RefundAmount     types.Money `json:"refund_amount"`
	SlashAmount      types.Money `json:"slash_amount"`
	PlaintiffRepDelta int32      `json:"plaintiff_rep_delta"`
	DefendantRepDelta int32      `json:"defendant_rep_delta"`
	BanDefendant     bool        `json:"ban_defendant"`
	Reasoning        string      `json:"reasoning"`
}

type VoteRecord struct {
	Validator []byte `json:"validator"`
	Vote      string `json:"vote"`
	Reasoning string `json:"reasoning"`
	VotedAt   int64  `json:"voted_at"`
	Signature []byte `json:"signature"`
}

type filePayload struct {
	OrderID     string `json:"order_id"`
	TaskID      string `json:"task_id"`
	Defendant   []byte `json:"defendant"`
	ReasonCode  string `json:"reason_code"`
	Description string `json:"description"`
}

type evidencePayload struct {
	DisputeID string   `json:"dispute_id"`
	Evidence  Evidence `json:"evidence"`
}

type resolvePayload struct {
	DisputeID string `json:"dispute_id"`
}

type votePayload struct {
	DisputeID string `json:"dispute_id"`
	Vote      string `json:"vote"`
	Reasoning string `json:"reasoning"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "arb",
		Handlers: map[string]common.ActionHandler{
			"file_dispute":    m.fileDispute,
			"submit_evidence": m.submitEvidence,
			"auto_resolve":    m.autoResolve,
			"committee_vote":  m.committeeVote,
			"appeal":          m.appeal,
			"full_vote":       m.fullVote,
			"execute_verdict": m.executeVerdict,
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

func (m *Module) fileDispute(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[filePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode file_dispute: %v", err)
	}
	id := disputeID(p.OrderID, p.TaskID, env.Nonce)
	d := &Dispute{
		DisputeID:    id,
		OrderID:      p.OrderID,
		TaskID:       p.TaskID,
		Plaintiff:    append([]byte{}, env.Sender...),
		Defendant:    append([]byte{}, p.Defendant...),
		ReasonCode:   p.ReasonCode,
		Description:  p.Description,
		Phase:        "PHASE_AUTO",
		Status:       "DISPUTE_FILED",
		FiledAt:      ctx.BlockTime.Unix(),
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) submitEvidence(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[evidencePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode submit_evidence: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	if equalBytes(d.Plaintiff, env.Sender) {
		d.PlaintiffEvidence = append(d.PlaintiffEvidence, p.Evidence)
	} else if equalBytes(d.Defendant, env.Sender) {
		d.DefendantEvidence = append(d.DefendantEvidence, p.Evidence)
	} else {
		return nil, types.NewAppError(types.CodeARBBase+61, "only plaintiff/defendant can submit evidence")
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) autoResolve(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[resolvePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode auto_resolve: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	if d.Status == "DISPUTE_FINAL_RESOLVED" {
		return types.SuccessResult(5000), nil
	}
	result := m.applyRules(d)
	if result.Resolved {
		d.Verdict = result.Verdict
		d.Status = "DISPUTE_AUTO_RESOLVED"
	} else {
		d.Phase = "PHASE_COMMITTEE"
		d.Status = "DISPUTE_COMM_REVIEW"
		d.CommitteeMembers = deterministicCommittee(d.DisputeID)
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(5000), nil
}

func (m *Module) committeeVote(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[votePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode committee_vote: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	if d.Phase != "PHASE_COMMITTEE" {
		return nil, types.NewAppError(types.CodeARBBase+62, "dispute phase %s cannot committee vote", d.Phase)
	}
	if !containsValidator(d.CommitteeMembers, env.Sender) {
		return nil, types.NewAppError(types.CodeARBBase+61, "sender is not committee member")
	}
	d.CommitteeVotes = append(d.CommitteeVotes, VoteRecord{Validator: append([]byte{}, env.Sender...), Vote: p.Vote, Reasoning: p.Reasoning, VotedAt: ctx.BlockTime.Unix()})
	if len(d.CommitteeVotes) >= 3 {
		winner := majorityVote(d.CommitteeVotes)
		if winner == "tie" {
			d.Phase = "PHASE_FULL"
			d.Status = "DISPUTE_FULL_REVIEW"
		} else {
			d.Status = "DISPUTE_COMM_RESOLVED"
			d.Verdict = simpleVerdict(winner)
		}
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(1500), nil
}

func (m *Module) fullVote(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[votePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode full_vote: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	if d.Phase != "PHASE_FULL" {
		return nil, types.NewAppError(types.CodeARBBase+62, "dispute phase %s cannot full vote", d.Phase)
	}
	d.FullVotes = append(d.FullVotes, VoteRecord{Validator: append([]byte{}, env.Sender...), Vote: p.Vote, Reasoning: p.Reasoning, VotedAt: ctx.BlockTime.Unix()})
	if len(d.FullVotes) >= 5 {
		winner := majorityVote(d.FullVotes)
		if winner == "tie" {
			winner = "consumer"
		}
		d.Verdict = simpleVerdict(winner)
		d.Status = "DISPUTE_FINAL_RESOLVED"
		d.ResolvedAt = ctx.BlockTime.Unix()
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(1500), nil
}

func (m *Module) appeal(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[resolvePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode appeal: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	switch d.Status {
	case "DISPUTE_AUTO_RESOLVED":
		d.Phase = "PHASE_COMMITTEE"
		d.Status = "DISPUTE_COMM_REVIEW"
		d.CommitteeMembers = deterministicCommittee(d.DisputeID)
	case "DISPUTE_COMM_RESOLVED":
		d.Phase = "PHASE_FULL"
		d.Status = "DISPUTE_FULL_REVIEW"
	default:
		return nil, types.NewAppError(types.CodeARBBase+62, "status %s cannot appeal", d.Status)
	}
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) executeVerdict(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[resolvePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeARBBase+1, "decode execute_verdict: %v", err)
	}
	d, err := loadDispute(ctx.Store, p.DisputeID)
	if err != nil {
		return nil, err
	}
	if d.Status != "DISPUTE_AUTO_RESOLVED" && d.Status != "DISPUTE_COMM_RESOLVED" && d.Status != "DISPUTE_FINAL_RESOLVED" {
		return nil, types.NewAppError(types.CodeARBBase+62, "dispute status %s not resolved", d.Status)
	}
	d.Status = "DISPUTE_FINAL_RESOLVED"
	d.ResolvedAt = ctx.BlockTime.Unix()
	if err := saveDispute(ctx.Store, d); err != nil {
		return nil, err
	}
	ev := types.Event{Type: "DisputeResolved", Attributes: map[string]string{"dispute_id": d.DisputeID, "winner": d.Verdict.Winner}}
	return types.SuccessResult(5000, ev), nil
}

func (m *Module) applyRules(d *Dispute) *RuleResult {
	engine := NewAutoRuleEngine()
	ctx := &DisputeContext{Dispute: d}
	return engine.Evaluate(ctx)
}

type RuleResult struct {
	Resolved bool
	Verdict  Verdict
}

type DisputeContext struct {
	Dispute *Dispute
}

type AutoRule struct {
	Name     string
	Priority int
	Eval     func(*DisputeContext) *RuleResult
}

type AutoRuleEngine struct {
	rules []AutoRule
}

func NewAutoRuleEngine() *AutoRuleEngine {
	return &AutoRuleEngine{
		rules: []AutoRule{
			{Name: "sla_timeout", Priority: 1, Eval: ruleSLATimeout},
			{Name: "heartbeat_loss", Priority: 2, Eval: ruleHeartbeatLoss},
			{Name: "verification_fail", Priority: 3, Eval: ruleVerificationFail},
			{Name: "consumer_bad_faith", Priority: 4, Eval: ruleConsumerBadFaith},
			{Name: "insufficient_evidence", Priority: 99, Eval: ruleEscalate},
		},
	}
}

func (e *AutoRuleEngine) Evaluate(ctx *DisputeContext) *RuleResult {
	rules := append([]AutoRule{}, e.rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, r := range rules {
		res := r.Eval(ctx)
		if res != nil && res.Resolved {
			return res
		}
	}
	return &RuleResult{Resolved: false}
}

func ruleSLATimeout(ctx *DisputeContext) *RuleResult {
	if strings.EqualFold(ctx.Dispute.ReasonCode, "timeout") {
		return &RuleResult{Resolved: true, Verdict: Verdict{Winner: "consumer", RefundAmount: types.Money{Currency: "CNY", Amount: "1"}, SlashAmount: types.Money{Currency: "CNY", Amount: "0.1"}, DefendantRepDelta: -30, Reasoning: "SLA timeout"}}
	}
	return nil
}

func ruleHeartbeatLoss(ctx *DisputeContext) *RuleResult {
	if strings.EqualFold(ctx.Dispute.ReasonCode, "heartbeat") {
		return &RuleResult{Resolved: true, Verdict: Verdict{Winner: "consumer", RefundAmount: types.Money{Currency: "CNY", Amount: "1"}, SlashAmount: types.Money{Currency: "CNY", Amount: "0.05"}, DefendantRepDelta: -20, Reasoning: "Heartbeat loss"}}
	}
	return nil
}

func ruleVerificationFail(ctx *DisputeContext) *RuleResult {
	if strings.EqualFold(ctx.Dispute.ReasonCode, "quality") {
		return &RuleResult{Resolved: true, Verdict: Verdict{Winner: "consumer", RefundAmount: types.Money{Currency: "CNY", Amount: "0.8"}, DefendantRepDelta: -40, Reasoning: "Verification failed"}}
	}
	return nil
}

func ruleConsumerBadFaith(ctx *DisputeContext) *RuleResult {
	if strings.EqualFold(ctx.Dispute.ReasonCode, "payment") {
		return &RuleResult{Resolved: true, Verdict: Verdict{Winner: "provider", PlaintiffRepDelta: -20, Reasoning: "Consumer bad faith"}}
	}
	return nil
}

func ruleEscalate(ctx *DisputeContext) *RuleResult {
	return &RuleResult{Resolved: false}
}

func deterministicCommittee(disputeID string) [][]byte {
	h := sha256.Sum256([]byte(disputeID))
	members := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		member := make([]byte, 32)
		for j := range member {
			member[j] = h[(i+j)%len(h)]
		}
		members[i] = member
	}
	return members
}

func containsValidator(validators [][]byte, sender []byte) bool {
	for _, v := range validators {
		if equalBytes(v, sender) {
			return true
		}
	}
	return false
}

func majorityVote(votes []VoteRecord) string {
	counts := map[string]int{}
	for _, v := range votes {
		counts[v.Vote]++
	}
	winner := ""
	best := 0
	tie := false
	for candidate, count := range counts {
		if count > best {
			winner = candidate
			best = count
			tie = false
		} else if count == best {
			tie = true
		}
	}
	if tie {
		return "tie"
	}
	return winner
}

func simpleVerdict(winner string) Verdict {
	return Verdict{Winner: winner, Reasoning: "majority vote"}
}

func saveDispute(store *state.Store, d *Dispute) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	store.Set([]byte(state.PrefixArbDispute+d.DisputeID), b)
	return nil
}

func loadDispute(store *state.Store, id string) (*Dispute, error) {
	raw, ok := store.Get([]byte(state.PrefixArbDispute + id))
	if !ok {
		return nil, types.NewAppError(types.CodeARBBase, "dispute not found")
	}
	var d Dispute
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func disputeID(orderID, taskID string, nonce uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", orderID, taskID, nonce)))
	return hex.EncodeToString(h[:8])
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
