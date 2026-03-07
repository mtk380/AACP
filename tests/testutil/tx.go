package testutil

import (
	"encoding/json"

	"aacp/internal/crypto"
	"aacp/internal/types"
)

func MustSignedTx(senderPub, senderPriv []byte, nonce uint64, module, action string, payload any) []byte {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	env := &types.TxEnvelope{
		Sender:   senderPub,
		Nonce:    nonce,
		GasLimit: 1_000_000,
		GasPrice: 1,
		Module:   module,
		Action:   action,
		Payload:  payloadBytes,
	}
	signBytes, err := env.SignBytes()
	if err != nil {
		panic(err)
	}
	env.Signature = crypto.SignMessage(senderPriv, signBytes)
	raw, err := types.EncodeTx(env)
	if err != nil {
		panic(err)
	}
	return raw
}
