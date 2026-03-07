package grpc

import (
	"context"

	"aacp/internal/abci"
)

type Server struct {
	app *abci.App
}

func NewServer(app *abci.App) *Server {
	return &Server{app: app}
}

func (s *Server) BroadcastTx(ctx context.Context, tx []byte) (*abci.FinalizeBlockResponse, error) {
	_ = ctx
	return s.app.FinalizeBlock(&abci.FinalizeBlockRequest{Height: int64(s.app.Store().Version()) + 1, Txs: [][]byte{tx}})
}
