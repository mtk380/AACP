package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var allowedModules = map[string]struct{}{
	"amx": {}, "aap": {}, "weave": {}, "caputxo": {}, "rep": {}, "arb": {}, "afd": {}, "fiat": {}, "gov": {}, "node": {},
}

type TxEnvelope struct {
	Sender        []byte          `json:"sender"`
	Nonce         uint64          `json:"nonce"`
	GasLimit      uint64          `json:"gas_limit"`
	GasPrice      uint64          `json:"gas_price"`
	Module        string          `json:"module"`
	Action        string          `json:"action"`
	Payload       json.RawMessage `json:"payload"`
	Signature     []byte          `json:"signature"`
	Memo          string          `json:"memo,omitempty"`
	TimeoutHeight int64           `json:"timeout_height,omitempty"`
}

type signableTx struct {
	Sender   []byte          `json:"sender"`
	Nonce    uint64          `json:"nonce"`
	GasLimit uint64          `json:"gas_limit"`
	GasPrice uint64          `json:"gas_price"`
	Module   string          `json:"module"`
	Action   string          `json:"action"`
	Payload  json.RawMessage `json:"payload"`
}

func (t *TxEnvelope) ValidateBasic() error {
	if len(t.Sender) != 32 {
		return errors.New("sender must be 32 bytes")
	}
	if t.GasLimit == 0 {
		return errors.New("gas_limit must be > 0")
	}
	if t.Module == "" || t.Action == "" {
		return errors.New("module and action are required")
	}
	if _, ok := allowedModules[t.Module]; !ok {
		return fmt.Errorf("module %q is not allowed", t.Module)
	}
	if len(t.Signature) != 64 {
		return errors.New("signature must be 64 bytes")
	}
	if len(t.Memo) > 256 {
		return errors.New("memo must be <= 256 chars")
	}
	return nil
}

func (t *TxEnvelope) SignBytes() ([]byte, error) {
	s := signableTx{
		Sender:   t.Sender,
		Nonce:    t.Nonce,
		GasLimit: t.GasLimit,
		GasPrice: t.GasPrice,
		Module:   t.Module,
		Action:   t.Action,
		Payload:  t.Payload,
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func EncodeTx(tx *TxEnvelope) ([]byte, error) {
	return json.Marshal(tx)
}

func DecodeTx(raw []byte) (*TxEnvelope, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var tx TxEnvelope
	if err := dec.Decode(&tx); err != nil {
		return nil, err
	}
	return &tx, nil
}
