package fiat

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

type Escrow struct {
	EscrowID      string      `json:"escrow_id"`
	OrderID       string      `json:"order_id"`
	Consumer      []byte      `json:"consumer"`
	Provider      []byte      `json:"provider"`
	TotalAmount   types.Money `json:"total_amount"`
	ServiceFee    types.Money `json:"service_fee"`
	Commission    types.Money `json:"commission"`
	CommissionBPS uint32      `json:"commission_bps"`
	Status        string      `json:"status"`
	CreatedAt     int64       `json:"created_at"`
	FundedAt      int64       `json:"funded_at"`
	ReleasedAt    int64       `json:"released_at"`
	GatewayTxID   string      `json:"gateway_tx_id"`
}

type DepositAccount struct {
	Provider       []byte `json:"provider"`
	Currency       string `json:"currency"`
	TotalDeposited string `json:"total_deposited"`
	Available      string `json:"available"`
	Frozen         string `json:"frozen"`
	Slashed        string `json:"slashed"`
	LastDepositAt  int64  `json:"last_deposit_at"`
	LastWithdrawAt int64  `json:"last_withdraw_at"`
	Tier           string `json:"tier"`
}

type InsurancePool struct {
	BalanceCNY      string `json:"balance_cny"`
	BalanceUSD      string `json:"balance_usd"`
	TotalIncome     string `json:"total_income"`
	TotalPayout     string `json:"total_payout"`
	MaxSinglePayout string `json:"max_single_payout"`
	SafetyRatioBPS  uint32 `json:"safety_ratio_bps"`
}

type CreateEscrowPayload struct {
	EscrowID      string      `json:"escrow_id"`
	OrderID       string      `json:"order_id"`
	Consumer      []byte      `json:"consumer"`
	Provider      []byte      `json:"provider"`
	ServiceFee    types.Money `json:"service_fee"`
	CommissionBPS uint32      `json:"commission_bps"`
}

type EscrowRefPayload struct {
	EscrowID string `json:"escrow_id"`
}

type SlashPayload struct {
	Provider []byte      `json:"provider"`
	Amount   types.Money `json:"amount"`
	Dispute  string      `json:"dispute_id"`
}

type DepositPayload struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

const (
	escrowPending   = "ESCROW_PENDING"
	escrowFunded    = "ESCROW_FUNDED"
	escrowLocked    = "ESCROW_LOCKED"
	escrowReleasing = "ESCROW_RELEASING"
	escrowReleased  = "ESCROW_RELEASED"
	escrowRefunded  = "ESCROW_REFUNDED"
	escrowSlashed   = "ESCROW_SLASHED"
	escrowCanceled  = "ESCROW_CANCELED"
)

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "fiat",
		Handlers: map[string]common.ActionHandler{
			"create_escrow":    m.createEscrow,
			"fund_escrow":      m.fundEscrow,
			"release_escrow":   m.releaseEscrow,
			"lock_escrow":      m.lockEscrow,
			"refund_escrow":    m.refundEscrow,
			"slash_deposit":    m.slashDeposit,
			"deposit_funds":    m.depositFunds,
			"withdraw_deposit": m.withdrawDeposit,
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
	pool, _ := m.getInsurancePool(ctx.Store)
	threshold := computeSafetyThreshold(pool)
	if lessDecimal(pool.BalanceCNY, threshold) {
		return []types.Event{{Type: "InsurancePoolBelowSafety", Attributes: map[string]string{"balance": pool.BalanceCNY, "threshold": threshold}}}, nil
	}
	return nil, nil
}
func (m *Module) ExecuteTx(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.base.ExecuteTx(ctx, env)
}

func (m *Module) createEscrow(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[CreateEscrowPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode create_escrow: %v", err)
	}
	if p.CommissionBPS == 0 {
		p.CommissionBPS = 1200
	}
	commission, err := types.ApplyBPS(p.ServiceFee, p.CommissionBPS, 8)
	if err != nil {
		return nil, err
	}
	total, err := types.AddMoney(p.ServiceFee, commission, 8)
	if err != nil {
		return nil, err
	}
	escrowID := p.EscrowID
	if escrowID == "" {
		escrowID = fmt.Sprintf("esc-%x", ctx.BlockTime.UnixNano())
	}
	e := &Escrow{
		EscrowID:      escrowID,
		OrderID:       p.OrderID,
		Consumer:      append([]byte{}, p.Consumer...),
		Provider:      append([]byte{}, p.Provider...),
		TotalAmount:   total,
		ServiceFee:    p.ServiceFee,
		Commission:    commission,
		CommissionBPS: p.CommissionBPS,
		Status:        escrowPending,
		CreatedAt:     ctx.BlockTime.Unix(),
	}
	if err := saveEscrow(ctx.Store, e); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) fundEscrow(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[EscrowRefPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode fund_escrow: %v", err)
	}
	e, err := loadEscrow(ctx.Store, p.EscrowID)
	if err != nil {
		return nil, err
	}
	if e.Status != escrowPending {
		return nil, types.NewAppError(types.CodeFIATBase+1, "escrow status %s cannot fund", e.Status)
	}
	e.Status = escrowFunded
	e.FundedAt = ctx.BlockTime.Unix()
	if err := saveEscrow(ctx.Store, e); err != nil {
		return nil, err
	}
	return types.SuccessResult(1500), nil
}

func (m *Module) releaseEscrow(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[EscrowRefPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode release_escrow: %v", err)
	}
	e, err := loadEscrow(ctx.Store, p.EscrowID)
	if err != nil {
		return nil, err
	}
	if e.Status != escrowFunded && e.Status != escrowLocked {
		return nil, types.NewAppError(types.CodeFIATBase+1, "escrow status %s cannot release", e.Status)
	}
	if e.Status == escrowFunded {
		e.Status = escrowReleasing
	}
	if err := m.distributeCommission(ctx.Store, e); err != nil {
		return nil, err
	}
	e.Status = escrowReleased
	e.ReleasedAt = ctx.BlockTime.Unix()
	if err := saveEscrow(ctx.Store, e); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) lockEscrow(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[EscrowRefPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode lock_escrow: %v", err)
	}
	e, err := loadEscrow(ctx.Store, p.EscrowID)
	if err != nil {
		return nil, err
	}
	if e.Status != escrowFunded {
		return nil, types.NewAppError(types.CodeFIATBase+1, "escrow status %s cannot lock", e.Status)
	}
	e.Status = escrowLocked
	if err := saveEscrow(ctx.Store, e); err != nil {
		return nil, err
	}
	return types.SuccessResult(1000), nil
}

func (m *Module) refundEscrow(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[EscrowRefPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode refund_escrow: %v", err)
	}
	e, err := loadEscrow(ctx.Store, p.EscrowID)
	if err != nil {
		return nil, err
	}
	if e.Status != escrowLocked && e.Status != escrowFunded {
		return nil, types.NewAppError(types.CodeFIATBase+1, "escrow status %s cannot refund", e.Status)
	}
	e.Status = escrowRefunded
	if err := saveEscrow(ctx.Store, e); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) slashDeposit(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[SlashPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode slash_deposit: %v", err)
	}
	acc, err := m.getDeposit(ctx.Store, p.Provider)
	if err != nil {
		return nil, err
	}
	slashAmount := p.Amount.Amount
	if lessDecimal(acc.Available, slashAmount) {
		slashAmount = acc.Available
	}
	acc.Available = subDecimal(acc.Available, slashAmount)
	acc.Slashed = addDecimal(acc.Slashed, slashAmount)
	if err := m.setDeposit(ctx.Store, acc); err != nil {
		return nil, err
	}
	pool, _ := m.getInsurancePool(ctx.Store)
	if p.Amount.Currency == "USD" {
		pool.BalanceUSD = addDecimal(pool.BalanceUSD, slashAmount)
	} else {
		pool.BalanceCNY = addDecimal(pool.BalanceCNY, slashAmount)
	}
	_ = m.setInsurancePool(ctx.Store, pool)
	return types.SuccessResult(3000, types.Event{Type: "DepositSlashed", Attributes: map[string]string{"provider": hex.EncodeToString(p.Provider), "amount": slashAmount, "dispute_id": p.Dispute}}), nil
}

func (m *Module) depositFunds(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[DepositPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode deposit_funds: %v", err)
	}
	acc, _ := m.getDeposit(ctx.Store, env.Sender)
	if acc == nil {
		acc = &DepositAccount{Provider: append([]byte{}, env.Sender...), Currency: p.Currency, TotalDeposited: "0", Available: "0", Frozen: "0", Slashed: "0", Tier: "starter"}
	}
	acc.TotalDeposited = addDecimal(acc.TotalDeposited, p.Amount)
	acc.Available = addDecimal(acc.Available, p.Amount)
	acc.LastDepositAt = ctx.BlockTime.Unix()
	acc.Tier = tierFor(acc.Available)
	if err := m.setDeposit(ctx.Store, acc); err != nil {
		return nil, err
	}
	return types.SuccessResult(1500), nil
}

func (m *Module) withdrawDeposit(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[DepositPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeFIATBase+1, "decode withdraw_deposit: %v", err)
	}
	acc, err := m.getDeposit(ctx.Store, env.Sender)
	if err != nil {
		return nil, err
	}
	if ctx.BlockTime.Unix()-acc.LastDepositAt < 7*24*3600 {
		return nil, types.NewAppError(types.CodeFIATBase+72, "unbonding period not reached")
	}
	if !lessOrEqualDecimal(p.Amount, acc.Available) {
		return nil, types.NewAppError(types.CodeFIATBase+70, "insufficient available balance")
	}
	newAvail := subDecimal(acc.Available, p.Amount)
	minRequired := minDepositForTier(acc.Tier)
	if lessDecimal(newAvail, minRequired) {
		return nil, types.NewAppError(types.CodeFIATBase+71, "deposit floor violation")
	}
	acc.Available = newAvail
	acc.LastWithdrawAt = ctx.BlockTime.Unix()
	if err := m.setDeposit(ctx.Store, acc); err != nil {
		return nil, err
	}
	return types.SuccessResult(2000), nil
}

func (m *Module) distributeCommission(store *state.Store, e *Escrow) error {
	total, err := types.ParseDecimal(e.Commission.Amount)
	if err != nil {
		return err
	}
	if total.Sign() <= 0 {
		return types.NewAppError(types.CodeFIATBase+1, "commission must be positive")
	}
	validatorPool := mulRatio(total, 40, 100)
	relayShare := mulRatio(total, 20, 100)
	insuranceShare := mulRatio(total, 20, 100)
	opsShare := mulRatio(total, 20, 100)
	proposerShare := mulRatio(validatorPool, 50, 100)
	_ = proposerShare
	_ = relayShare
	_ = opsShare
	pool, _ := m.getInsurancePool(store)
	if e.Commission.Currency == "USD" {
		pool.BalanceUSD = addDecimal(pool.BalanceUSD, types.FormatDecimal(insuranceShare, 8))
	} else {
		pool.BalanceCNY = addDecimal(pool.BalanceCNY, types.FormatDecimal(insuranceShare, 8))
	}
	pool.TotalIncome = addDecimal(pool.TotalIncome, types.FormatDecimal(total, 8))
	return m.setInsurancePool(store, pool)
}

func (m *Module) getDeposit(store *state.Store, provider []byte) (*DepositAccount, error) {
	k := []byte(state.PrefixFiatDeposit + hex.EncodeToString(provider))
	raw, ok := store.Get(k)
	if !ok {
		return nil, types.NewAppError(types.CodeFIATBase+71, "deposit account not found")
	}
	var acc DepositAccount
	if err := json.Unmarshal(raw, &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

func (m *Module) setDeposit(store *state.Store, acc *DepositAccount) error {
	k := []byte(state.PrefixFiatDeposit + hex.EncodeToString(acc.Provider))
	b, err := json.Marshal(acc)
	if err != nil {
		return err
	}
	store.Set(k, b)
	return nil
}

func (m *Module) getInsurancePool(store *state.Store) (*InsurancePool, error) {
	raw, ok := store.Get([]byte(state.PrefixFiatPool))
	if !ok {
		return &InsurancePool{BalanceCNY: "0", BalanceUSD: "0", TotalIncome: "0", TotalPayout: "0", MaxSinglePayout: "0", SafetyRatioBPS: 20000}, nil
	}
	var p InsurancePool
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *Module) setInsurancePool(store *state.Store, pool *InsurancePool) error {
	b, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	store.Set([]byte(state.PrefixFiatPool), b)
	return nil
}

func saveEscrow(store *state.Store, e *Escrow) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	store.Set([]byte(state.PrefixFiatEscrow+e.EscrowID), b)
	return nil
}

func loadEscrow(store *state.Store, id string) (*Escrow, error) {
	raw, ok := store.Get([]byte(state.PrefixFiatEscrow + id))
	if !ok {
		return nil, types.NewAppError(types.CodeFIATBase, "escrow not found")
	}
	var e Escrow
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func addDecimal(a, b string) string {
	ra, _ := types.ParseDecimal(a)
	rb, _ := types.ParseDecimal(b)
	return types.FormatDecimal(new(big.Rat).Add(ra, rb), 8)
}

func subDecimal(a, b string) string {
	ra, _ := types.ParseDecimal(a)
	rb, _ := types.ParseDecimal(b)
	return types.FormatDecimal(new(big.Rat).Sub(ra, rb), 8)
}

func lessDecimal(a, b string) bool {
	ra, _ := types.ParseDecimal(a)
	rb, _ := types.ParseDecimal(b)
	return ra.Cmp(rb) < 0
}

func lessOrEqualDecimal(a, b string) bool {
	ra, _ := types.ParseDecimal(a)
	rb, _ := types.ParseDecimal(b)
	return ra.Cmp(rb) <= 0
}

func mulRatio(r *big.Rat, num, den int64) *big.Rat {
	return new(big.Rat).Mul(r, new(big.Rat).SetFrac64(num, den))
}

func computeSafetyThreshold(pool *InsurancePool) string {
	maxPayout, _ := types.ParseDecimal(pool.MaxSinglePayout)
	ratio := new(big.Rat).SetFrac64(int64(pool.SafetyRatioBPS), 10000)
	return types.FormatDecimal(new(big.Rat).Mul(maxPayout, ratio), 8)
}

func tierFor(available string) string {
	v, _ := types.ParseDecimal(available)
	ent, _ := types.ParseDecimal("100000")
	pro, _ := types.ParseDecimal("10000")
	s, _ := types.ParseDecimal("1000")
	switch {
	case v.Cmp(ent) >= 0:
		return "enterprise"
	case v.Cmp(pro) >= 0:
		return "pro"
	case v.Cmp(s) >= 0:
		return "starter"
	default:
		return "starter"
	}
}

func minDepositForTier(tier string) string {
	switch tier {
	case "enterprise":
		return "100000"
	case "pro":
		return "10000"
	default:
		return "1000"
	}
}

func CalculateCommissionRate(monthlyVolume float64, riskTier int, reputation uint32) float64 {
	volumeSteps := math.Min(math.Floor(monthlyVolume/10000.0), 4)
	volumeAdj := -0.01 * volumeSteps
	riskAdj := 0.02 * float64(riskTier)
	repSteps := math.Min(math.Floor(float64(reputation)/250.0), 2)
	repAdj := -0.01 * repSteps
	rate := 0.12 + volumeAdj + riskAdj + repAdj
	if rate < 0.08 {
		return 0.08
	}
	if rate > 0.15 {
		return 0.15
	}
	return rate
}

func CommissionBPS(rate float64) uint32 {
	return uint32(math.Round(rate * 10000))
}

var _ router.Module = (*Module)(nil)
