package abci

import (
	"time"

	"aacp/internal/types"
)

type InitChainRequest struct {
	Genesis map[string][]byte
}

type InitChainResponse struct {
	AppHash []byte
}

type PrepareProposalRequest struct {
	Txs [][]byte
}

type PrepareProposalResponse struct {
	Txs [][]byte
}

type ProcessProposalRequest struct {
	Txs [][]byte
}

type ProcessProposalResponse struct {
	Accept bool
}

type FinalizeBlockRequest struct {
	Height int64
	Time   time.Time
	Txs    [][]byte
}

type FinalizeBlockResponse struct {
	TxResults        []*types.ExecTxResult
	AppHash          []byte
	ValidatorUpdates []ValidatorUpdate
	Events           []types.Event
}

type CommitRequest struct{}

type CommitResponse struct {
	RetainHeight int64
}

type QueryRequest struct {
	Module string
	Path   string
	Data   []byte
}

type QueryResponse struct {
	Value []byte
}

type ValidatorUpdate struct {
	PubKey []byte
	Power  int64
}
