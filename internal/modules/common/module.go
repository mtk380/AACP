package common

import (
	"encoding/json"
	"fmt"

	"aacp/internal/router"
	"aacp/internal/types"
)

type ActionHandler func(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error)

type Module struct {
	ModuleName string
	Handlers   map[string]ActionHandler
}

func (m *Module) Name() string {
	return m.ModuleName
}

func (m *Module) InitGenesis(ctx *router.Context, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	ctx.EventBus.Emit(types.Event{Type: "module_init", Attributes: map[string]string{"module": m.ModuleName}})
	return nil
}

func (m *Module) ExecuteTx(ctx *router.Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	h, ok := m.Handlers[env.Action]
	if !ok {
		return nil, types.NewAppError(types.CodeUnknownMod, "%s action %q is not supported", m.ModuleName, env.Action)
	}
	return h(ctx, env)
}

func (m *Module) EndBlock(ctx *router.Context) ([]types.Event, error) {
	return nil, nil
}

func (m *Module) Query(ctx *router.Context, path string, data []byte) ([]byte, error) {
	return nil, fmt.Errorf("%s query %q is not implemented", m.ModuleName, path)
}

func DecodeJSONPayload[T any](env *types.TxEnvelope) (*T, error) {
	var payload T
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
