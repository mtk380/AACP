package afd

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"aacp/internal/modules/common"
	"aacp/internal/router"
	"aacp/internal/types"
)

type FraudAlert struct {
	AlertID           string   `json:"alert_id"`
	AlertType         string   `json:"alert_type"`
	Severity          string   `json:"severity"`
	Action            string   `json:"action"`
	InvolvedEntities  [][]byte `json:"involved_entities"`
	Description       string   `json:"description"`
	Confidence        float32  `json:"confidence"`
	DetectedAt        int64    `json:"detected_at"`
	TxHash            string   `json:"tx_hash"`
}

type ReportPayload struct {
	Alert FraudAlert `json:"alert"`
}

type Module struct {
	base *common.Module
}

func New() *Module {
	m := &Module{}
	m.base = &common.Module{
		ModuleName: "afd",
		Handlers: map[string]common.ActionHandler{
			"report_alert": m.reportAlert,
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

func (m *Module) reportAlert(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	p, err := common.DecodeJSONPayload[ReportPayload](env)
	if err != nil {
		return nil, types.NewAppError(types.CodeAFDBase+1, "decode report_alert: %v", err)
	}
	if p.Alert.AlertID == "" {
		p.Alert.AlertID = fmt.Sprintf("alert-%d", ctx.BlockTime.UnixNano())
	}
	if p.Alert.DetectedAt == 0 {
		p.Alert.DetectedAt = ctx.BlockTime.Unix()
	}
	b, err := json.Marshal(&p.Alert)
	if err != nil {
		return nil, types.NewAppError(types.CodeDecodeFail, "marshal alert: %v", err)
	}
	ctx.Store.Set([]byte("afd/alert/"+p.Alert.AlertID), b)
	eventType := "FraudFlagged"
	switch p.Alert.Action {
	case "BLOCK":
		eventType = "FraudBlocked"
	case "BAN":
		eventType = "FraudBanned"
	}
	return types.SuccessResult(1000, types.Event{Type: eventType, Attributes: map[string]string{"alert_id": p.Alert.AlertID, "type": p.Alert.AlertType}}), nil
}

func DetectSelfTrade(consumer, provider []byte) *FraudAlert {
	if equalBytes(consumer, provider) {
		return &FraudAlert{
			AlertType: "self_trade",
			Severity:  "HIGH",
			Action:    "BLOCK",
			Confidence: 1.0,
			DetectedAt: time.Now().Unix(),
			Description: "consumer equals provider",
		}
	}
	return nil
}

type PriceAnomalyDetector struct {
	WindowSize      int
	SigmaThreshold  float64
}

func NewPriceAnomalyDetector() *PriceAnomalyDetector {
	return &PriceAnomalyDetector{WindowSize: 100, SigmaThreshold: 3.0}
}

func (d *PriceAnomalyDetector) Check(price float64, recent []float64) *FraudAlert {
	if len(recent) < 10 {
		return nil
	}
	mean, stddev := meanStdDev(recent)
	if stddev == 0 {
		return nil
	}
	z := math.Abs(price-mean) / stddev
	if z <= d.SigmaThreshold {
		return nil
	}
	severity := "MEDIUM"
	action := "FLAG"
	if z > 5.0 {
		severity = "HIGH"
		action = "BLOCK"
	}
	return &FraudAlert{
		AlertType:   "price_anomaly",
		Severity:    severity,
		Action:      action,
		Confidence:  float32(math.Min(z/10.0, 1.0)),
		Description: fmt.Sprintf("price %.2f deviates %.1fσ from mean %.2f", price, z, mean),
		DetectedAt:  time.Now().Unix(),
	}
}

func meanStdDev(data []float64) (float64, float64) {
	n := float64(len(data))
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / n
	variance := 0.0
	for _, v := range data {
		d := v - mean
		variance += d * d
	}
	return mean, math.Sqrt(variance / n)
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
