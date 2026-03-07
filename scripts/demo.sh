#!/bin/bash
set -euo pipefail

HOST="${AACP_DEMO_HOST:-127.0.0.1}"
PORT="${AACP_DEMO_PORT:-8888}"
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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aacp-demo.XXXXXX")"
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

cat > "${TMP_DIR}/gen_demo_tx.go" <<'EOF'
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

func main() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	payload, err := json.Marshal(map[string]any{
		"holder":      pub,
		"cap_type":    "text_generation",
		"cap_version": "1.0.0",
		"scopes":      []string{"read", "write"},
		"delegatable": false,
	})
	if err != nil {
		panic(err)
	}

	env := txEnvelope{
		Sender:   pub,
		Nonce:    0,
		GasLimit: 1_000_000,
		GasPrice: 1,
		Module:   "caputxo",
		Action:   "mint_capability",
		Payload:  payload,
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
		panic(err)
	}
	hash := sha256.Sum256(signBytes)
	env.Signature = ed25519.Sign(priv, hash[:])

	raw, err := json.Marshal(env)
	if err != nil {
		panic(err)
	}
	fmt.Print(hex.EncodeToString(raw))
}
EOF

TX_HEX="$(go run "${TMP_DIR}/gen_demo_tx.go")"
if [[ -z "${TX_HEX}" ]]; then
  echo "failed to generate demo tx" >&2
  exit 1
fi

echo "== health before =="
cat "${TMP_DIR}/health-before.json"
echo
echo "== send caputxo mint tx =="
go run ./cmd/aacp-cli --node="${NODE_URL}" --cmd=send --tx="${TX_HEX}"
echo "== health after =="
curl -fsS "${NODE_URL}/api/health"
echo
echo "demo done"
