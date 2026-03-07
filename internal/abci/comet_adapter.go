//go:build cometbft

package abci

import (
	"context"
	"time"

	cometabci "github.com/cometbft/cometbft/abci/types"

	"aacp/internal/crypto"
	"aacp/internal/types"
)

// CometAdapter bridges the internal AACP execution engine to CometBFT ABCI.
type CometAdapter struct {
	app *App
}

func NewCometAdapter(app *App) *CometAdapter {
	return &CometAdapter{app: app}
}

func (a *CometAdapter) Info(ctx context.Context, req *cometabci.RequestInfo) (*cometabci.ResponseInfo, error) {
	_ = ctx
	_ = req
	return &cometabci.ResponseInfo{
		Data:             "aacp",
		Version:          "0.9.0",
		AppVersion:       1,
		LastBlockHeight:  int64(a.app.Store().Version()),
		LastBlockAppHash: []byte(a.app.LastAppHashHex()),
	}, nil
}

func (a *CometAdapter) InitChain(ctx context.Context, req *cometabci.RequestInitChain) (*cometabci.ResponseInitChain, error) {
	_ = ctx
	genesis := map[string][]byte{"app_state": req.AppStateBytes}
	resp, err := a.app.InitChain(&InitChainRequest{Genesis: genesis})
	if err != nil {
		return nil, err
	}
	return &cometabci.ResponseInitChain{AppHash: resp.AppHash}, nil
}

func (a *CometAdapter) CheckTx(ctx context.Context, req *cometabci.RequestCheckTx) (*cometabci.ResponseCheckTx, error) {
	_ = ctx
	env, err := types.DecodeTx(req.Tx)
	if err != nil {
		return &cometabci.ResponseCheckTx{Code: types.CodeDecodeFail, Log: err.Error()}, nil
	}
	if err := env.ValidateBasic(); err != nil {
		return &cometabci.ResponseCheckTx{Code: types.CodeDecodeFail, Log: err.Error()}, nil
	}
	signBytes, err := env.SignBytes()
	if err != nil {
		return &cometabci.ResponseCheckTx{Code: types.CodeDecodeFail, Log: err.Error()}, nil
	}
	if !crypto.VerifySignature(env.Sender, signBytes, env.Signature) {
		return &cometabci.ResponseCheckTx{Code: types.CodeBadSignature, Log: "invalid signature"}, nil
	}
	return &cometabci.ResponseCheckTx{Code: types.CodeOK, GasWanted: int64(env.GasLimit)}, nil
}

func (a *CometAdapter) PrepareProposal(ctx context.Context, req *cometabci.RequestPrepareProposal) (*cometabci.ResponsePrepareProposal, error) {
	_ = ctx
	resp, err := a.app.PrepareProposal(&PrepareProposalRequest{Txs: req.Txs})
	if err != nil {
		return nil, err
	}
	return &cometabci.ResponsePrepareProposal{Txs: resp.Txs}, nil
}

func (a *CometAdapter) ProcessProposal(ctx context.Context, req *cometabci.RequestProcessProposal) (*cometabci.ResponseProcessProposal, error) {
	_ = ctx
	resp, err := a.app.ProcessProposal(&ProcessProposalRequest{Txs: req.Txs})
	if err != nil {
		return nil, err
	}
	status := cometabci.ResponseProcessProposal_ACCEPT
	if !resp.Accept {
		status = cometabci.ResponseProcessProposal_REJECT
	}
	return &cometabci.ResponseProcessProposal{Status: status}, nil
}

func (a *CometAdapter) FinalizeBlock(ctx context.Context, req *cometabci.RequestFinalizeBlock) (*cometabci.ResponseFinalizeBlock, error) {
	_ = ctx
	when := req.Time
	if when.IsZero() {
		when = time.Now()
	}
	resp, err := a.app.FinalizeBlock(&FinalizeBlockRequest{Height: req.Height, Time: when, Txs: req.Txs})
	if err != nil {
		return nil, err
	}
	results := make([]*cometabci.ExecTxResult, 0, len(resp.TxResults))
	for _, r := range resp.TxResults {
		results = append(results, &cometabci.ExecTxResult{
			Code:    r.Code,
			Log:     r.Log,
			GasUsed: int64(r.GasUsed),
			Events:  toCometEvents(r.Events),
			Data:    r.Data,
		})
	}
	return &cometabci.ResponseFinalizeBlock{
		TxResults: results,
		AppHash:   resp.AppHash,
		Events:    toCometEvents(resp.Events),
	}, nil
}

func (a *CometAdapter) Commit(ctx context.Context, req *cometabci.RequestCommit) (*cometabci.ResponseCommit, error) {
	_ = ctx
	_ = req
	resp, err := a.app.Commit(&CommitRequest{})
	if err != nil {
		return nil, err
	}
	return &cometabci.ResponseCommit{RetainHeight: resp.RetainHeight}, nil
}

func (a *CometAdapter) Query(ctx context.Context, req *cometabci.RequestQuery) (*cometabci.ResponseQuery, error) {
	_ = ctx
	module := req.Path
	path := req.Path
	if module == "" {
		module = "state"
	}
	resp, err := a.app.Query(&QueryRequest{Module: module, Path: path, Data: req.Data})
	if err != nil {
		return nil, err
	}
	return &cometabci.ResponseQuery{Code: types.CodeOK, Value: resp.Value, Height: int64(a.app.Store().Version())}, nil
}

func (a *CometAdapter) ExtendVote(ctx context.Context, req *cometabci.RequestExtendVote) (*cometabci.ResponseExtendVote, error) {
	_ = ctx
	_ = req
	return &cometabci.ResponseExtendVote{}, nil
}

func (a *CometAdapter) VerifyVoteExtension(ctx context.Context, req *cometabci.RequestVerifyVoteExtension) (*cometabci.ResponseVerifyVoteExtension, error) {
	_ = ctx
	_ = req
	return &cometabci.ResponseVerifyVoteExtension{Status: cometabci.ResponseVerifyVoteExtension_ACCEPT}, nil
}

func toCometEvents(events []types.Event) []cometabci.Event {
	out := make([]cometabci.Event, 0, len(events))
	for _, ev := range events {
		attrs := make([]cometabci.EventAttribute, 0, len(ev.Attributes))
		for k, v := range ev.Attributes {
			attrs = append(attrs, cometabci.EventAttribute{Key: k, Value: v, Index: true})
		}
		out = append(out, cometabci.Event{Type: ev.Type, Attributes: attrs})
	}
	return out
}
