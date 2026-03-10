# AACP v0.9.0 (Implementation Scaffold)

This repository contains a staged implementation for the AACP technical spec.

```bash
在肉眼可见的未来，defi 在主权国家应该是很难推动的，但是如果是建立在法币体系和 agent 能力上的 deli（我自己造的，li 为 lives）我倒是觉得值得探讨一下。在我看来，人们的使用场景是极其分散且很难统一数据权限的，也就是说诸如豆包、千问这种中心化大一统入口不太能真的“一统江山”，以后的 agent 体系应该是去中心化的散点状态（openclaw 可能算开个先河吧）。基座模型厂商相当于电信运营商提供“流量”，agent 不再是简单的工具，而是 agent 经济市场里面的雇工，通过交易市场与责任网络在联盟链里面进行“可交易、可协作、可问责、可结算”的市场经济。基于此，浅浅构思了下 web4.0 时代的体系，并 vibe 了一下我的构想：
白皮书： https://mtk380.github.io/AACP.github.io/
```

## Quick start

For GitHub visitors, the fastest way is:

```bash
git clone <your-repo-url>
cd web4.0
make demo-ui
```

This starts the server and keeps it running. Press `Ctrl+C` to stop.

Prerequisites for local run:

- Go >= 1.22
- `curl`

On macOS with Homebrew:

```bash
./scripts/bootstrap_dev_tools.sh
```

You can also run without `make`:

```bash
./scripts/demo-ui.sh
```

Optional env vars:

- `AACP_UI_DEMO_HOST` (default `127.0.0.1`)
- `AACP_UI_DEMO_PORT` (default `8888`)
- `AACP_UI_DEMO_OPEN` (`1` auto-open browser, `0` disable)

Manual start (if you want full control):

```bash
make test
make build
go run ./cmd/aacpd --port=8888
```

Open demo page:

```bash
open http://127.0.0.1:8888/
```

Demo page includes:

- Everyday story demo (poster design order: match → escrow → dispute → refund)
- Per-step drilldown in story demo (implementation file/function + input/output + state before/after)
- One-click run + reset for the full lifecycle
- API response output panel
- Live node health badge

## UI demo screenshots

### 1) Initial overview

![Story overview](docs/screenshots/story-overview.png)

### 2) After one-click run

![Story after run](docs/screenshots/story-after-run.png)

### 3) Step detail drilldown

![Story step detail](docs/screenshots/story-step-detail.png)

To recapture screenshots after UI changes:

```bash
# terminal A: keep node running
AACP_UI_DEMO_OPEN=0 AACP_UI_DEMO_PORT=8899 ./scripts/demo-ui.sh

# terminal B: install playwright once (temp dir), then capture
mkdir -p /tmp/pwshot && cd /tmp/pwshot && npm init -y && npm install playwright
cd /path/to/web4.0  # replace with your local repo path
NODE_PATH=/tmp/pwshot/node_modules AACP_UI_SCREENSHOT_URL=http://127.0.0.1:8899 node ./scripts/capture-ui-screenshots.cjs
```

Health check:

```bash
curl -sS http://127.0.0.1:8888/api/health
```

One-command demo (start node, send one signed tx, print before/after health):

```bash
./scripts/demo.sh
```

Optional env vars:

- `AACP_DEMO_HOST` (default `127.0.0.1`)
- `AACP_DEMO_PORT` (default `8888`)

CapUTXO full-flow demo (`mint -> delegate -> revoke`):

```bash
./scripts/demo-caputxo.sh
```

Optional env vars:

- `AACP_CAPUTXO_DEMO_HOST` (default `127.0.0.1`)
- `AACP_CAPUTXO_DEMO_PORT` (default `8888`)

## Current delivery

- Step 1-6 implemented as executable foundation:
  - Go project skeleton + tooling.
  - Full proto contract files.
  - Core types, signature, decimal money helpers.
  - Deterministic in-memory state tree with prefix index and commit hash.
  - ABCI-like lifecycle with tx validation/signature/nonce/gas/module routing.
  - Module router + event bus.
- Protocol modules included with functional MVP logic:
  - AMX/AAP/WEAVE/Cap-UTXO/FIAT/REP/ARB/AFD/NODE/GOV.
- ABCI bridge scaffolding for CometBFT and selectable state backend:
  - `AACP_STATE_BACKEND=memory` (default)
  - `AACP_STATE_BACKEND=iavl` (real IAVL mutable tree backend)
  - `go test -tags iavl ./...` enables the IAVL backend build.
  - `go test -tags cometbft ./...` enables CometBFT ABCI adapter build.
- Deployment/config seeds and basic tests for unit/integration/e2e/determinism.

## Notes

- This code is designed as a testnet-grade baseline and intentionally keeps external integrations as stubs.
- Proto generation is wired through `buf` or `protoc` if available locally.

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request.

## License

This project is licensed under the [MIT License](./LICENSE).
