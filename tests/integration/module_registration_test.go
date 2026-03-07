package integration

import (
	"testing"

	"aacp/internal/abci"
)

func TestAppBootstrapsModules(t *testing.T) {
	app := abci.NewApp()
	if _, err := app.InitChain(&abci.InitChainRequest{}); err != nil {
		t.Fatalf("init chain failed: %v", err)
	}
	if app.Store() == nil {
		t.Fatalf("store should be initialized")
	}
}
