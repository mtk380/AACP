#!/bin/bash
set -euo pipefail

CHAIN_ID="aacp-testnet-1"
VALIDATORS=3
HOME_BASE="./testnet"

mkdir -p "${HOME_BASE}"

echo "=== Initialize ${VALIDATORS} local validator folders for ${CHAIN_ID} ==="
for i in $(seq 0 $((VALIDATORS - 1))); do
  HOME_DIR="${HOME_BASE}/validator-${i}"
  mkdir -p "${HOME_DIR}/config"
  cp config/node.toml "${HOME_DIR}/config/node.toml"
  echo "validator-${i}" > "${HOME_DIR}/MONIKER"
done

echo "=== Seed basic genesis config (placeholder) ==="
cat > "${HOME_BASE}/genesis.json" <<JSON
{
  "chain_id": "${CHAIN_ID}",
  "genesis_time": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "initial_height": 1
}
JSON

echo "done"
