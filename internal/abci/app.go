package abci

import (
	"aacp/internal/eventbus"
	"aacp/internal/modules/aap"
	"aacp/internal/modules/afd"
	"aacp/internal/modules/amx"
	"aacp/internal/modules/arb"
	"aacp/internal/modules/caputxo"
	"aacp/internal/modules/fiat"
	"aacp/internal/modules/gov"
	"aacp/internal/modules/node"
	"aacp/internal/modules/rep"
	"aacp/internal/modules/weave"
	"aacp/internal/router"
	"aacp/internal/state"
)

type App struct {
	store       *state.Store
	router      *router.Router
	eventBus    *eventbus.Bus
	lastAppHash []byte
}

func NewApp() *App {
	st := state.NewStore()
	eb := eventbus.New()
	r := router.New()
	mods := []router.Module{
		amx.New(),
		aap.New(),
		weave.New(),
		caputxo.New(),
		rep.New(),
		arb.New(),
		afd.New(),
		fiat.New(),
		node.New(),
		gov.New(),
	}
	for _, m := range mods {
		if err := r.Register(m); err != nil {
			panic(err)
		}
	}
	return &App{store: st, router: r, eventBus: eb}
}

func (a *App) Store() *state.Store {
	return a.store
}
