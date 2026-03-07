package rest

import (
	"encoding/json"
	"net/http"

	"aacp/internal/abci"
)

func NewHandler(app *abci.App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/node/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chain_id":     "aacp-devnet-1",
			"latest_height": app.Store().Version(),
			"latest_hash":  app.LastAppHashHex(),
		})
	})
	return mux
}
