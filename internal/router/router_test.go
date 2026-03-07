package router

import (
	"testing"
	"time"

	"aacp/internal/eventbus"
	"aacp/internal/state"
	"aacp/internal/types"
)

type mockModule struct{}

func (m mockModule) Name() string { return "mock" }
func (m mockModule) InitGenesis(ctx *Context, raw []byte) error { return nil }
func (m mockModule) EndBlock(ctx *Context) ([]types.Event, error) { return nil, nil }
func (m mockModule) Query(ctx *Context, path string, data []byte) ([]byte, error) { return []byte("ok"), nil }
func (m mockModule) ExecuteTx(ctx *Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	return types.SuccessResult(1), nil
}

func TestRouterRegisterAndExecute(t *testing.T) {
	r := New()
	if err := r.Register(mockModule{}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	ctx := &Context{Store: state.NewStore(), EventBus: eventbus.New(), Height: 1, BlockTime: time.Now()}
	res, err := r.Execute(ctx, &types.TxEnvelope{Module: "mock", Action: "x"})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.Code != types.CodeOK {
		t.Fatalf("unexpected code %d", res.Code)
	}
}
