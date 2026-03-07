#!/bin/bash
set -euo pipefail

HOST="${AACP_UI_DEMO_HOST:-127.0.0.1}"
PORT="${AACP_UI_DEMO_PORT:-8888}"
OPEN_BROWSER="${AACP_UI_DEMO_OPEN:-1}"
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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aacp-ui-demo.XXXXXX")"
PID=""

cleanup() {
  if [[ -n "${PID}" ]]; then
    kill "${PID}" >/dev/null 2>&1 || true
    wait "${PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

echo "[1/3] starting aacpd on ${NODE_URL}"
go run ./cmd/aacpd --host="${HOST}" --port="${PORT}" >"${TMP_DIR}/aacpd.log" 2>&1 &
PID=$!

echo "[2/3] waiting for health check ..."
for _ in $(seq 1 80); do
  if curl -fsS "${NODE_URL}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

if ! curl -fsS "${NODE_URL}/api/health" >"${TMP_DIR}/health.json"; then
  echo "aacpd did not become ready; log follows:" >&2
  cat "${TMP_DIR}/aacpd.log" >&2
  exit 1
fi

echo "[3/3] ready"
echo "UI Demo: ${NODE_URL}/"
echo "Health : ${NODE_URL}/api/health"
echo

if [[ "${OPEN_BROWSER}" == "1" ]]; then
  if command -v open >/dev/null 2>&1; then
    open "${NODE_URL}/" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "${NODE_URL}/" >/dev/null 2>&1 || true
  fi
fi

echo "server is running, press Ctrl+C to stop"
wait "${PID}"
