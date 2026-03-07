package rep

import (
	"encoding/hex"
	"encoding/json"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type ReputationScore struct {
	Entity           []byte `json:"entity"`
	EntityType       string `json:"entity_type"`
	QualityScore     uint32 `json:"quality_score"`
	TimelinessScore  uint32 `json:"timeliness_score"`
	ReliabilityScore uint32 `json:"reliability_score"`
	DisputeScore     uint32 `json:"dispute_score"`
	NetworkScore     uint32 `json:"network_score"`
	TotalScore       uint32 `json:"total_score"`
	TotalTasks       uint64 `json:"total_tasks"`
	SuccessTasks     uint64 `json:"success_tasks"`
	FailedTasks      uint64 `json:"failed_tasks"`
	DisputedTasks    uint64 `json:"disputed_tasks"`
	LastUpdated      int64  `json:"last_updated"`
	UpdateEpoch      uint64 `json:"update_epoch"`
}

type updatePayload struct {
	Entity       []byte `json:"entity"`
	EntityType   string `json:"entity_type"`
	Result       string `json:"result"`
	CompMS       uint32 `json:"comp_ms"`
	SLAMS        uint32 `json:"sla_ms"`
	Rating       uint32 `json:"rating"`
	VerifyOK     bool   `json:"verify_ok"`
	HBMiss       uint32 `json:"hb_miss"`
	Timestamp    int64  `json:"timestamp"`
}

type decayPayload struct {
	CurrentEpoch uint64 `json:"current_epoch"`
}

type overridePayload struct {
	Entity []byte `json:"entity"`
	Score  uint32 `json:"score"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "rep",
		Handlers: map[string]common.ActionHandler{
			"update_reputation": m.updateReputation,
			"apply_decay":       m.applyDecay,
			"override_score":    m.overrideScore,
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

func (m *Module) updateReputation(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[updatePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeREPBase+1, "decode update_reputation: %v", err)
	}
	score, _ := loadScore(ctx.Store, p.Entity)
	if score == nil {
		score = defaultScore(p.Entity, p.EntityType)
	}
	dq, dt, dr, dd := computeDeltas(p)
	score.QualityScore = clampU32(int64(score.QualityScore)+int64(dq), 0, 1000)
	score.TimelinessScore = clampU32(int64(score.TimelinessScore)+int64(dt), 0, 1000)
	score.ReliabilityScore = clampU32(int64(score.ReliabilityScore)+int64(dr), 0, 1000)
	score.DisputeScore = clampU32(int64(score.DisputeScore)+int64(dd), 0, 1000)
	score.TotalScore = computeTotal(score)
	score.TotalTasks++
	switch p.Result {
	case "SUCCESS":
		score.SuccessTasks++
	case "FAILED":
		score.FailedTasks++
	case "DISPUTED_LOST", "DISPUTED_WON":
		score.DisputedTasks++
	}
	if p.Timestamp == 0 {
		p.Timestamp = ctx.BlockTime.Unix()
	}
	score.LastUpdated = p.Timestamp
	if err := saveScore(ctx.Store, score); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) applyDecay(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[decayPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeREPBase+1, "decode apply_decay: %v", err)
	}
	if p.CurrentEpoch == 0 {
		p.CurrentEpoch = uint64(ctx.Height / 1000)
	}
	entries := ctx.Store.IteratePrefix([]byte(state.PrefixRepScore))
	for _, kv := range entries {
		var score ReputationScore
		if err := json.Unmarshal(kv.Value, &score); err != nil {
			continue
		}
		if p.CurrentEpoch-score.UpdateEpoch < 4 {
			continue
		}
		score.QualityScore = decayScore(score.QualityScore)
		score.TimelinessScore = decayScore(score.TimelinessScore)
		score.ReliabilityScore = decayScore(score.ReliabilityScore)
		score.DisputeScore = decayScore(score.DisputeScore)
		score.NetworkScore = decayScore(score.NetworkScore)
		score.TotalScore = computeTotal(&score)
		score.UpdateEpoch = p.CurrentEpoch
		_ = saveScore(ctx.Store, &score)
	}
	return types.SuccessResult(500 * uint64(len(entries))), nil
}

func (m *Module) overrideScore(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[overridePayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeREPBase+1, "decode override_score: %v", err)
	}
	score, _ := loadScore(ctx.Store, p.Entity)
	if score == nil {
		score = defaultScore(p.Entity, "provider")
	}
	score.TotalScore = p.Score
	if p.Score > 1000 {
		score.TotalScore = 1000
	}
	if err := saveScore(ctx.Store, score); err != nil {
		return nil, err
	}
	return types.SuccessResult(5000), nil
}

func loadScore(store *state.Store, entity []byte) (*ReputationScore, bool) {
	k := state.RepKey(hex.EncodeToString(entity))
	raw, ok := store.Get(k)
	if !ok {
		return nil, false
	}
	var score ReputationScore
	if err := json.Unmarshal(raw, &score); err != nil {
		return nil, false
	}
	return &score, true
}

func saveScore(store *state.Store, score *ReputationScore) error {
	k := state.RepKey(hex.EncodeToString(score.Entity))
	b, err := json.Marshal(score)
	if err != nil {
		return types.NewAppError(types.CodeDecodeFail, "marshal score: %v", err)
	}
	store.Set(k, b)
	return nil
}

func defaultScore(entity []byte, entityType string) *ReputationScore {
	base := uint32(500)
	return &ReputationScore{
		Entity:           append([]byte{}, entity...),
		EntityType:       entityType,
		QualityScore:     base,
		TimelinessScore:  base,
		ReliabilityScore: base,
		DisputeScore:     base,
		NetworkScore:     base,
		TotalScore:       base,
	}
}

func computeDeltas(p *updatePayload) (int32, int32, int32, int32) {
	var dq, dt, dr, dd int32
	switch p.Result {
	case "SUCCESS":
		if p.VerifyOK {
			rating := p.Rating
			if rating < 1 {
				rating = 1
			}
			if rating > 5 {
				rating = 5
			}
			dq = int32(10 * float32(rating) / 5.0)
		}
		timeRatio := ratio(p.CompMS, p.SLAMS)
		switch {
		case timeRatio <= 0.5:
			dt = 8
		case timeRatio <= 1.0:
			dt = 4
		case timeRatio <= 1.5:
			dt = -10
		default:
			dt = -25
		}
		dr = reliabilityDelta(p.HBMiss)
		dd = 3
	case "FAILED":
		dq = -30
		dt = -25
		dr = reliabilityDelta(p.HBMiss) - 15
	case "DISPUTED_LOST":
		dq = -50
		dt = -25
		dr = reliabilityDelta(p.HBMiss)
		dd = -60
	case "DISPUTED_WON":
		dq = 5
		dt = -25
		dr = reliabilityDelta(p.HBMiss)
		dd = 0
	default:
	}
	return dq, dt, dr, dd
}

func reliabilityDelta(missed uint32) int32 {
	switch {
	case missed == 0:
		return 5
	case missed <= 2:
		return 2
	case missed <= 5:
		return -10
	default:
		return -30
	}
}

func ratio(a, b uint32) float64 {
	if b == 0 {
		return 2.0
	}
	return float64(a) / float64(b)
}

func computeTotal(score *ReputationScore) uint32 {
	total := float64(score.QualityScore)*0.30 +
		float64(score.TimelinessScore)*0.25 +
		float64(score.ReliabilityScore)*0.20 +
		float64(score.DisputeScore)*0.15 +
		float64(score.NetworkScore)*0.10
	return clampU32(int64(total+0.5), 0, 1000)
}

func decayScore(v uint32) uint32 {
	d := int64(v) - int64(v)/50
	if d < 100 {
		d = 100
	}
	return uint32(d)
}

func clampU32(v, lo, hi int64) uint32 {
	if v < lo {
		return uint32(lo)
	}
	if v > hi {
		return uint32(hi)
	}
	return uint32(v)
}

func Level(total uint32) string {
	switch {
	case total >= 900:
		return "DIAMOND"
	case total >= 700:
		return "PLATINUM"
	case total >= 500:
		return "GOLD"
	case total >= 300:
		return "SILVER"
	case total >= 100:
		return "BRONZE"
	default:
		return "RESTRICTED"
	}
}

var _ router.Module = (*Module)(nil)
