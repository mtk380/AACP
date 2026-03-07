#!/bin/bash
set -euo pipefail

HOST="${AACP_CAPUTXO_DEMO_HOST:-127.0.0.1}"
PORT="${AACP_CAPUTXO_DEMO_PORT:-8888}"
NODE_URL="http://${HOST}:${PORT}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

for cmd in go curl; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "missing required command: ${cmd}" >&2
    exit 1
  fi
done

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aacp-caputxo-demo.XXXXXX")"
PID=""

cleanup() {
  if [[ -n "${PID}" ]]; then
    kill "${PID}" >/dev/null 2>&1 || true
    wait "${PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

echo "starting aacpd on ${NODE_URL}"
go run ./cmd/aacpd --host="${HOST}" --port="${PORT}" >"${TMP_DIR}/aacpd.log" 2>&1 &
PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "${NODE_URL}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

if ! curl -fsS "${NODE_URL}/api/health" >"${TMP_DIR}/health-before.json"; then
  echo "aacpd did not become ready; log follows:" >&2
  cat "${TMP_DIR}/aacpd.log" >&2
  exit 1
fi

cat > "${TMP_DIR}/gen_caputxo_flow_txs.go" <<'EOF'
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type txEnvelope struct {
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

func signTx(senderPub, senderPriv []byte, nonce uint64, module, action string, payload any) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	env := txEnvelope{
		Sender:   senderPub,
		Nonce:    nonce,
		GasLimit: 1_000_000,
		GasPrice: 1,
		Module:   module,
		Action:   action,
		Payload:  payloadBytes,
	}
	signable := signableTx{
		Sender:   env.Sender,
		Nonce:    env.Nonce,
		GasLimit: env.GasLimit,
		GasPrice: env.GasPrice,
		Module:   env.Module,
		Action:   env.Action,
		Payload:  env.Payload,
	}
	signBytes, err := json.Marshal(signable)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(signBytes)
	env.Signature = ed25519.Sign(senderPriv, hash[:])
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func utxoID(issuer []byte, capType string, nonce uint64) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%x|%s|%d", issuer, capType, nonce)))
	return hex.EncodeToString(hash[:8])
}

func main() {
	issuerPub, issuerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	holderPub, holderPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	delegatePub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	parentUTXO := utxoID(issuerPub, "text_generation", 0)

	mintTx, err := signTx(issuerPub, issuerPriv, 0, "caputxo", "mint_capability", map[string]any{
		"holder":      holderPub,
		"cap_type":    "text_generation",
		"cap_version": "1.0.0",
		"scopes":      []string{"read", "write"},
		"delegatable": true,
		"max_depth":   3,
	})
	if err != nil {
		panic(err)
	}

	delegateTx, err := signTx(holderPub, holderPriv, 0, "caputxo", "delegate_capability", map[string]any{
		"parent_utxo_id": parentUTXO,
		"new_holder":     delegatePub,
		"scopes":         []string{"read"},
	})
	if err != nil {
		panic(err)
	}

	revokeTx, err := signTx(issuerPub, issuerPriv, 1, "caputxo", "revoke_capability", map[string]any{
		"utxo_id": parentUTXO,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("PARENT_UTXO=%s\n", parentUTXO)
	fmt.Printf("MINT=%s\n", mintTx)
	fmt.Printf("DELEGATE=%s\n", delegateTx)
	fmt.Printf("REVOKE=%s\n", revokeTx)
}
EOF

META="$(go run "${TMP_DIR}/gen_caputxo_flow_txs.go")"
PARENT_UTXO="$(printf '%s\n' "${META}" | sed -n 's/^PARENT_UTXO=//p')"
MINT_TX_HEX="$(printf '%s\n' "${META}" | sed -n 's/^MINT=//p')"
DELEGATE_TX_HEX="$(printf '%s\n' "${META}" | sed -n 's/^DELEGATE=//p')"
REVOKE_TX_HEX="$(printf '%s\n' "${META}" | sed -n 's/^REVOKE=//p')"

for var_name in PARENT_UTXO MINT_TX_HEX DELEGATE_TX_HEX REVOKE_TX_HEX; do
  if [[ -z "${!var_name}" ]]; then
    echo "failed to generate ${var_name}" >&2
    exit 1
  fi
done

echo "== health before =="
cat "${TMP_DIR}/health-before.json"
echo

echo "== parent utxo id =="
echo "${PARENT_UTXO}"

echo "== tx1 mint_capability =="
MINT_RESP="$(go run ./cmd/aacp-cli --node="${NODE_URL}" --cmd=send --tx="${MINT_TX_HEX}")"
echo "${MINT_RESP}"
if [[ "${MINT_RESP}" != *"\"code\":0"* ]]; then
  echo "mint_capability failed" >&2
  exit 1
fi

echo "== tx2 delegate_capability =="
DELEGATE_RESP="$(go run ./cmd/aacp-cli --node="${NODE_URL}" --cmd=send --tx="${DELEGATE_TX_HEX}")"
echo "${DELEGATE_RESP}"
if [[ "${DELEGATE_RESP}" != *"\"code\":0"* ]]; then
  echo "delegate_capability failed" >&2
  exit 1
fi

echo "== tx3 revoke_capability (cascade) =="
REVOKE_RESP="$(go run ./cmd/aacp-cli --node="${NODE_URL}" --cmd=send --tx="${REVOKE_TX_HEX}")"
echo "${REVOKE_RESP}"
if [[ "${REVOKE_RESP}" != *"\"code\":0"* ]]; then
  echo "revoke_capability failed" >&2
  exit 1
fi

echo "== health after =="
curl -fsS "${NODE_URL}/api/health"
echo

echo "caputxo full-flow demo done"
