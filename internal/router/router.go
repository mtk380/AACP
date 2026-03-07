package router

import (
	"fmt"
	"sync"
	"time"

	"aacp/internal/eventbus"
	"aacp/internal/state"
	"aacp/internal/types"
)

type Context struct {
	Store     *state.Store
	EventBus  *eventbus.Bus
	Height    int64
	BlockTime time.Time
}

type Module interface {
	Name() string
	InitGenesis(ctx *Context, raw []byte) error
	ExecuteTx(ctx *Context, env *types.TxEnvelope) (*types.ExecTxResult, error)
	EndBlock(ctx *Context) ([]types.Event, error)
	Query(ctx *Context, path string, data []byte) ([]byte, error)
}

type Router struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func New() *Router {
	return &Router{modules: map[string]Module{}}
}

func (r *Router) Register(module Module) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := module.Name()
	if name == "" {
		return fmt.Errorf("module name cannot be empty")
	}
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %q already registered", name)
	}
	r.modules[name] = module
	return nil
}

func (r *Router) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.modules, name)
}

func (r *Router) Execute(ctx *Context, env *types.TxEnvelope) (*types.ExecTxResult, error) {
	r.mu.RLock()
	module, ok := r.modules[env.Module]
	r.mu.RUnlock()
	if !ok {
		return nil, types.NewAppError(types.CodeUnknownMod, "unknown module %q", env.Module)
	}
	return module.ExecuteTx(ctx, env)
}

func (r *Router) InitGenesis(ctx *Context, genesis map[string][]byte) error {
	r.mu.RLock()
	mods := make([]Module, 0, len(r.modules))
	for _, m := range r.modules {
		mods = append(mods, m)
	}
	r.mu.RUnlock()

	for _, m := range mods {
		if err := m.InitGenesis(ctx, genesis[m.Name()]); err != nil {
			return fmt.Errorf("module %s init genesis failed: %w", m.Name(), err)
		}
	}
	return nil
}

func (r *Router) EndBlock(ctx *Context) ([]types.Event, error) {
	r.mu.RLock()
	mods := make([]Module, 0, len(r.modules))
	for _, m := range r.modules {
		mods = append(mods, m)
	}
	r.mu.RUnlock()

	out := make([]types.Event, 0)
	for _, m := range mods {
		events, err := m.EndBlock(ctx)
		if err != nil {
			return nil, fmt.Errorf("module %s end block failed: %w", m.Name(), err)
		}
		out = append(out, events...)
	}
	return out, nil
}

func (r *Router) Query(ctx *Context, module, path string, data []byte) ([]byte, error) {
	r.mu.RLock()
	m, ok := r.modules[module]
	r.mu.RUnlock()
	if !ok {
		return nil, types.NewAppError(types.CodeUnknownMod, "unknown module %q", module)
	}
	return m.Query(ctx, path, data)
}
