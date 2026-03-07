package abci

import (
	"encoding/hex"
	"fmt"

	"aacp/internal/crypto"
	"aacp/internal/router"
	"aacp/internal/types"
)

func (a *App) InitChain(req *InitChainRequest) (*InitChainResponse, error) {
	ctx := &router.Context{Store: a.store, EventBus: a.eventBus, Height: 0}
	if req != nil {
		if err := a.router.InitGenesis(ctx, req.Genesis); err != nil {
			return nil, err
		}
	}
	hash, _ := a.store.Commit()
	a.lastAppHash = hash
	return &InitChainResponse{AppHash: hash}, nil
}

func (a *App) PrepareProposal(req *PrepareProposalRequest) (*PrepareProposalResponse, error) {
	if req == nil {
		return &PrepareProposalResponse{}, nil
	}
	// Deterministic order: keep original order and drop txs larger than 256KiB.
	filtered := make([][]byte, 0, len(req.Txs))
	for _, tx := range req.Txs {
		if len(tx) <= 262144 {
			filtered = append(filtered, tx)
		}
	}
	return &PrepareProposalResponse{Txs: filtered}, nil
}

func (a *App) ProcessProposal(req *ProcessProposalRequest) (*ProcessProposalResponse, error) {
	if req == nil {
		return &ProcessProposalResponse{Accept: true}, nil
	}
	for _, raw := range req.Txs {
		if len(raw) > 262144 {
			return &ProcessProposalResponse{Accept: false}, nil
		}
	}
	return &ProcessProposalResponse{Accept: true}, nil
}

func (a *App) FinalizeBlock(req *FinalizeBlockRequest) (*FinalizeBlockResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil finalize request")
	}
	ctx := &router.Context{Store: a.store, EventBus: a.eventBus, Height: req.Height, BlockTime: req.Time}
	results := make([]*types.ExecTxResult, 0, len(req.Txs))

	for _, raw := range req.Txs {
		env, err := types.DecodeTx(raw)
		if err != nil {
			results = append(results, types.FailResult(types.CodeDecodeFail, err.Error()))
			continue
		}
		if err := env.ValidateBasic(); err != nil {
			results = append(results, types.FailResult(types.CodeDecodeFail, err.Error()))
			continue
		}
		if env.TimeoutHeight > 0 && req.Height > env.TimeoutHeight {
			results = append(results, types.FailResult(types.CodeDecodeFail, "tx timeout height exceeded"))
			continue
		}
		signBytes, err := env.SignBytes()
		if err != nil {
			results = append(results, types.FailResult(types.CodeDecodeFail, err.Error()))
			continue
		}
		if !crypto.VerifySignature(env.Sender, signBytes, env.Signature) {
			results = append(results, types.FailResult(types.CodeBadSignature, "invalid signature"))
			continue
		}
		expectedNonce := a.store.GetNonce(env.Sender)
		if env.Nonce != expectedNonce {
			results = append(results, types.FailResult(types.CodeBadNonce, fmt.Sprintf("expected nonce %d got %d", expectedNonce, env.Nonce)))
			continue
		}

		gasCtx := NewGasContext(env.GasLimit)
		if err := gasCtx.Consume(1000, "signature_verify"); err != nil {
			results = append(results, types.FailResult(types.CodeOutOfGas, err.Error()))
			continue
		}
		if err := gasCtx.Consume(uint64((len(env.Payload)+1023)/1024*10), "payload_decode"); err != nil {
			results = append(results, types.FailResult(types.CodeOutOfGas, err.Error()))
			continue
		}

		res, err := a.router.Execute(ctx, env)
		if err != nil {
			code := types.CodeFromError(err, types.CodeUnknownMod)
			results = append(results, types.FailResult(code, err.Error()))
			continue
		}
		res.GasUsed += gasCtx.Used
		if res.Code == types.CodeOK {
			a.store.SetNonce(env.Sender, expectedNonce+1)
		}
		results = append(results, res)
	}

	endEvents, err := a.router.EndBlock(ctx)
	if err != nil {
		return nil, err
	}
	for _, ev := range endEvents {
		a.eventBus.Emit(ev)
	}
	hash, _ := a.store.Commit()
	a.lastAppHash = hash
	return &FinalizeBlockResponse{
		TxResults:        results,
		AppHash:          hash,
		ValidatorUpdates: nil,
		Events:           a.eventBus.Flush(),
	}, nil
}

func (a *App) Commit(_ *CommitRequest) (*CommitResponse, error) {
	return &CommitResponse{RetainHeight: 0}, nil
}

func (a *App) Query(req *QueryRequest) (*QueryResponse, error) {
	if req == nil {
		return &QueryResponse{}, nil
	}
	ctx := &router.Context{Store: a.store, EventBus: a.eventBus}
	if req.Module == "state" {
		val, ok := a.store.Get(req.Data)
		if !ok {
			return &QueryResponse{Value: nil}, nil
		}
		return &QueryResponse{Value: val}, nil
	}
	val, err := a.router.Query(ctx, req.Module, req.Path, req.Data)
	if err != nil {
		return nil, err
	}
	return &QueryResponse{Value: val}, nil
}

func (a *App) LastAppHashHex() string {
	return hex.EncodeToString(a.lastAppHash)
}
