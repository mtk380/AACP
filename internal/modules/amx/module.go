package amx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/state"
	"aacp/internal/types"
)

const (
	statusActive    = "LISTING_ACTIVE"
	statusPaused    = "LISTING_PAUSED"
	statusDelisted  = "LISTING_DELISTED"
	orderMatching   = "ORDER_MATCHING"
	orderMatched    = "ORDER_MATCHED"
	orderEscrowed   = "ORDER_ESCROWED"
	orderActive     = "ORDER_ACTIVE"
	orderCompleted  = "ORDER_COMPLETED"
	orderDisputed   = "ORDER_DISPUTED"
	orderCanceled   = "ORDER_CANCELED"
	orderFailed     = "ORDER_FAILED"
	orderRejected   = "ORDER_REJECTED"
)

type CapabilityRef struct {
	CapType string `json:"cap_type"`
}

type PricingModel struct {
	Currency  string `json:"currency"`
	BasePrice string `json:"base_price"`
}

type SLATemplate struct {
	MaxLatencyMS uint32 `json:"max_latency_ms"`
	TimeoutSec   uint64 `json:"timeout_sec"`
}

type Listing struct {
	ListingID    string          `json:"listing_id"`
	Provider     []byte          `json:"provider"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Capabilities []CapabilityRef `json:"capabilities"`
	Pricing      PricingModel    `json:"pricing"`
	SLA          SLATemplate     `json:"sla"`
	Tags         []string        `json:"tags"`
	Status       string          `json:"status"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
}

type CreateListingPayload struct {
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Capabilities []CapabilityRef `json:"capabilities"`
	Pricing      PricingModel    `json:"pricing"`
	SLA          SLATemplate     `json:"sla"`
	Tags         []string        `json:"tags"`
}

type UpdateListingPayload struct {
	ListingID   string       `json:"listing_id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Pricing     PricingModel `json:"pricing"`
}

type ToggleListingPayload struct {
	ListingID string `json:"listing_id"`
}

type SubmitRequestPayload struct {
	RequiredCaps []string    `json:"required_caps"`
	Budget       types.Money `json:"budget"`
	MaxLatencyMS uint32      `json:"max_latency_ms"`
	Deadline     int64       `json:"deadline"`
}

type Order struct {
	OrderID           string      `json:"order_id"`
	ListingID         string      `json:"listing_id"`
	Consumer          []byte      `json:"consumer"`
	Provider          []byte      `json:"provider"`
	AgreedPrice       types.Money `json:"agreed_price"`
	CommissionBPS     uint32      `json:"commission_bps"`
	CommissionAmount  types.Money `json:"commission_amount"`
	EscrowID          string      `json:"escrow_id"`
	TaskID            string      `json:"task_id"`
	Status            string      `json:"status"`
	CreatedAt         int64       `json:"created_at"`
	UpdatedAt         int64       `json:"updated_at"`
}

type transitionPayload struct {
	OrderID string `json:"order_id"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "amx",
		Handlers: map[string]common.ActionHandler{
			"create_listing": m.createListing,
			"update_listing": m.updateListing,
			"pause_listing":  m.pauseListing,
			"delist_listing": m.delistListing,
			"submit_request": m.submitRequest,
			"confirm_match":  m.confirmMatch,
			"reject_match":   m.rejectMatch,
			"cancel_order":   m.cancelOrder,
		},
	}
	return m
}

func (m *Module) Name() string                                              { return m.base.Name() }
func (m *Module) InitGenesis(ctx *router.Context, raw []byte) error         { return m.base.InitGenesis(ctx, raw) }
func (m *Module) EndBlock(ctx *router.Context) ([]types.Event, error)       { return m.base.EndBlock(ctx) }
func (m *Module) Query(ctx *router.Context, path string, data []byte) ([]byte, error) {
	return m.base.Query(ctx, path, data)
}
func (m *Module) ExecuteTx(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.base.ExecuteTx(ctx, env)
}

func (m *Module) createListing(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[CreateListingPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+1, "decode create_listing: %v", err)
	}
	if len(p.Capabilities) == 0 {
		return nil, types.NewAppError(types.CodeAMXBase+2, "capabilities must not be empty")
	}
	if err := ptoPrice(p.Pricing.BasePrice); err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+2, "invalid base_price: %v", err)
	}
	id := shortHash(env.Sender, []byte(fmt.Sprintf("%d", env.Nonce)), []byte(p.Title))
	now := ctx.BlockTime.Unix()
	listing := Listing{
		ListingID:    id,
		Provider:     append([]byte{}, env.Sender...),
		Title:        p.Title,
		Description:  p.Description,
		Capabilities: p.Capabilities,
		Pricing:      p.Pricing,
		SLA:          p.SLA,
		Tags:         p.Tags,
		Status:       statusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := saveJSON(ctx.Store, state.ListingKey(id), &listing); err != nil {
		return nil, err
	}
	for _, capRef := range listing.Capabilities {
		idxKey := []byte(fmt.Sprintf("%s%s/%s", state.PrefixAMXIndex, capRef.CapType, listing.ListingID))
		ctx.Store.Set(idxKey, []byte{1})
	}
	ev := types.Event{Type: "OrderBookChanged", Attributes: map[string]string{"listing_id": id, "status": statusActive}}
	return types.SuccessResult(5000, ev), nil
}

func (m *Module) updateListing(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[UpdateListingPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+1, "decode update_listing: %v", err)
	}
	listing, err := loadListing(ctx.Store, p.ListingID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(listing.Provider, env.Sender) {
		return nil, types.NewAppError(types.CodeAMXBase+3, "provider mismatch")
	}
	if p.Title != "" {
		listing.Title = p.Title
	}
	if p.Description != "" {
		listing.Description = p.Description
	}
	if p.Pricing.Currency != "" {
		listing.Pricing = p.Pricing
	}
	listing.UpdatedAt = ctx.BlockTime.Unix()
	if err := saveJSON(ctx.Store, state.ListingKey(listing.ListingID), listing); err != nil {
		return nil, err
	}
	return types.SuccessResult(3000), nil
}

func (m *Module) pauseListing(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.toggleListing(ctx, env, statusPaused, 1000)
}

func (m *Module) delistListing(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.toggleListing(ctx, env, statusDelisted, 1000)
}

func (m *Module) toggleListing(ctx *router.Context, env *types.TxEnvelope, next string, gas uint64) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[ToggleListingPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+1, "decode toggle_listing: %v", err)
	}
	listing, err := loadListing(ctx.Store, p.ListingID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(listing.Provider, env.Sender) {
		return nil, types.NewAppError(types.CodeAMXBase+3, "provider mismatch")
	}
	listing.Status = next
	listing.UpdatedAt = ctx.BlockTime.Unix()
	if err := saveJSON(ctx.Store, state.ListingKey(listing.ListingID), listing); err != nil {
		return nil, err
	}
	return types.SuccessResult(gas), nil
}

func (m *Module) submitRequest(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[SubmitRequestPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+1, "decode submit_request: %v", err)
	}
	if p.Deadline > 0 && ctx.BlockTime.Unix() > p.Deadline {
		return nil, types.NewAppError(types.CodeAMXBase+12, "match timeout")
	}

	listings := ctx.Store.IteratePrefix([]byte(state.PrefixAMXListing))
	cand := make([]Listing, 0)
	for _, kv := range listings {
		var l Listing
		if err := json.Unmarshal(kv.Value, &l); err != nil {
			continue
		}
		if l.Status != statusActive {
			continue
		}
		if strings.ToUpper(l.Pricing.Currency) != strings.ToUpper(p.Budget.Currency) {
			continue
		}
		if !capsContain(l.Capabilities, p.RequiredCaps) {
			continue
		}
		if exceedsBudget(l.Pricing.BasePrice, p.Budget.Amount) {
			continue
		}
		cand = append(cand, l)
	}
	if len(cand) == 0 {
		id := shortHash(env.Sender, []byte(fmt.Sprintf("%d", env.Nonce)), []byte("failed"))
		ord := Order{OrderID: id, Status: orderFailed, Consumer: append([]byte{}, env.Sender...), CreatedAt: ctx.BlockTime.Unix(), UpdatedAt: ctx.BlockTime.Unix()}
		_ = saveJSON(ctx.Store, state.OrderKey(id), &ord)
		return types.SuccessResult(8000, types.Event{Type: "OrderFailed", Attributes: map[string]string{"order_id": id}}), nil
	}

	type scored struct {
		Listing Listing
		Score   *big.Rat
	}
	scoredList := make([]scored, 0, len(cand))
	minPrice, maxPrice := minMaxPrice(cand)
	for _, l := range cand {
		s := scoreListing(l, minPrice, maxPrice)
		scoredList = append(scoredList, scored{Listing: l, Score: s})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		cmp := scoredList[i].Score.Cmp(scoredList[j].Score)
		if cmp == 0 {
			return scoredList[i].Listing.ListingID < scoredList[j].Listing.ListingID
		}
		return cmp > 0
	})
	best := scoredList[0].Listing
	commissionBPS := uint32(1200)
	agreed := types.Money{Currency: best.Pricing.Currency, Amount: best.Pricing.BasePrice}
	commission, err := types.ApplyBPS(agreed, commissionBPS, 8)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+11, "commission calc failed: %v", err)
	}
	orderID := shortHash(env.Sender, []byte(fmt.Sprintf("%d", env.Nonce)), []byte(best.ListingID))
	order := Order{
		OrderID:          orderID,
		ListingID:        best.ListingID,
		Consumer:         append([]byte{}, env.Sender...),
		Provider:         append([]byte{}, best.Provider...),
		AgreedPrice:      agreed,
		CommissionBPS:    commissionBPS,
		CommissionAmount: commission,
		EscrowID:         shortHash([]byte(orderID), []byte("escrow")),
		Status:           orderMatched,
		CreatedAt:        ctx.BlockTime.Unix(),
		UpdatedAt:        ctx.BlockTime.Unix(),
	}
	if err := saveJSON(ctx.Store, state.OrderKey(orderID), &order); err != nil {
		return nil, err
	}
	ev := types.Event{Type: "OrderMatched", Attributes: map[string]string{"order_id": orderID, "listing_id": best.ListingID}}
	return types.SuccessResult(8000, ev), nil
}

func (m *Module) confirmMatch(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.transitionOrder(ctx, env, map[string]string{orderMatched: orderEscrowed, orderEscrowed: orderActive}, 2000)
}

func (m *Module) rejectMatch(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.transitionOrder(ctx, env, map[string]string{orderMatched: orderRejected}, 1500)
}

func (m *Module) cancelOrder(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return m.transitionOrder(ctx, env, map[string]string{orderMatched: orderCanceled, orderEscrowed: orderCanceled, orderActive: orderCanceled}, 2000)
}

func (m *Module) transitionOrder(ctx *router.Context, env *types.TxEnvelope, transitions map[string]string, gas uint64) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[transitionPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAMXBase+1, "decode order transition: %v", err)
	}
	order, err := loadOrder(ctx.Store, p.OrderID)
	if err != nil {
		return nil, err
	}
	if !equalBytes(order.Consumer, env.Sender) && !equalBytes(order.Provider, env.Sender) {
		return nil, types.NewAppError(types.CodeAMXBase+13, "sender has no permission")
	}
	next, ok := transitions[order.Status]
	if !ok {
		return nil, types.NewAppError(types.CodeAMXBase+13, "invalid order state transition: %s", order.Status)
	}
	order.Status = next
	order.UpdatedAt = ctx.BlockTime.Unix()
	if err := saveJSON(ctx.Store, state.OrderKey(order.OrderID), order); err != nil {
		return nil, err
	}
	return types.SuccessResult(gas), nil
}

func saveJSON(store *state.Store, key []byte, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return types.NewAppError(types.CodeDecodeFail, "marshal state: %v", err)
	}
	store.Set(key, b)
	return nil
}

func loadListing(store *state.Store, id string) (*Listing, error) {
	raw, ok := store.Get(state.ListingKey(id))
	if !ok {
		return nil, types.NewAppError(types.CodeAMXBase, "listing not found")
	}
	var listing Listing
	if err := json.Unmarshal(raw, &listing); err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "decode listing: %v", err)
	}
	return &listing, nil
}

func loadOrder(store *state.Store, id string) (*Order, error) {
	raw, ok := store.Get(state.OrderKey(id))
	if !ok {
		return nil, types.NewAppError(types.CodeAMXBase, "order not found")
	}
	var order Order
	if err := json.Unmarshal(raw, &order); err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "decode order: %v", err)
	}
	return &order, nil
}

func shortHash(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

func capsContain(caps []CapabilityRef, required []string) bool {
	if len(required) == 0 {
		return true
	}
	has := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		has[c.CapType] = struct{}{}
	}
	for _, r := range required {
		if _, ok := has[r]; !ok {
			return false
		}
	}
	return true
}

func ptoPrice(s string) error {
	_, err := types.ParseDecimal(s)
	return err
}

func exceedsBudget(price, budget string) bool {
	p, err := types.ParseDecimal(price)
	if err != nil {
		return true
	}
	b, err := types.ParseDecimal(budget)
	if err != nil {
		return true
	}
	return p.Cmp(b) > 0
}

func minMaxPrice(listings []Listing) (*big.Rat, *big.Rat) {
	min := new(big.Rat).SetInt64(0)
	max := new(big.Rat).SetInt64(0)
	for i, l := range listings {
		p, err := types.ParseDecimal(l.Pricing.BasePrice)
		if err != nil {
			p = new(big.Rat).SetInt64(0)
		}
		if i == 0 || p.Cmp(min) < 0 {
			min = p
		}
		if i == 0 || p.Cmp(max) > 0 {
			max = p
		}
	}
	return min, max
}

func scoreListing(l Listing, minPrice, maxPrice *big.Rat) *big.Rat {
	price, err := types.ParseDecimal(l.Pricing.BasePrice)
	if err != nil {
		price = new(big.Rat).SetInt64(0)
	}
	epsilon := new(big.Rat).SetFrac64(1, 1000000)
	rangeP := new(big.Rat).Sub(maxPrice, minPrice)
	rangeP.Add(rangeP, epsilon)
	priceNorm := new(big.Rat).Sub(price, minPrice)
	priceNorm.Quo(priceNorm, rangeP)
	one := new(big.Rat).SetInt64(1)
	priceScore := new(big.Rat).Sub(one, priceNorm)
	if priceScore.Sign() < 0 {
		priceScore = new(big.Rat).SetInt64(0)
	}
	if priceScore.Cmp(one) > 0 {
		priceScore = one
	}

	wPrice := new(big.Rat).SetFrac64(30, 100)
	wRep := new(big.Rat).SetFrac64(30, 100)
	wLat := new(big.Rat).SetFrac64(20, 100)
	wCap := new(big.Rat).SetFrac64(20, 100)
	rep := new(big.Rat).SetFrac64(700, 1000)
	lat := new(big.Rat).SetFrac64(8, 10)
	capScore := new(big.Rat).SetFrac64(int64(len(l.Capabilities)), int64(len(l.Capabilities)+1))

	total := new(big.Rat)
	total.Add(total, new(big.Rat).Mul(wPrice, priceScore))
	total.Add(total, new(big.Rat).Mul(wRep, rep))
	total.Add(total, new(big.Rat).Mul(wLat, lat))
	total.Add(total, new(big.Rat).Mul(wCap, capScore))
	return total
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
