import os
import textwrap
from datetime import date

OUT_PDF = "output/pdf/aacp_app_summary_onepager.pdf"
PAGE_W, PAGE_H = 612, 792  # Letter
MARGIN_L = 46
MARGIN_R = 46
TOP = 760
BOTTOM = 38


def esc(text: str) -> str:
    return text.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")


def make_layout(scale: float):
    title_size = 19 * scale
    subtitle_size = 10.3 * scale
    heading_size = 12.6 * scale
    body_size = 11 * scale

    title_gap = 25 * scale
    subtitle_gap = 16 * scale
    heading_gap = 17 * scale
    body_gap = 13.5 * scale
    section_gap = 8 * scale

    y = TOP
    cmds = []

    def line(text, font="F1", size=body_size, x=MARGIN_L):
        nonlocal y
        cmds.append(f"BT /{font} {size:.2f} Tf 1 0 0 1 {x:.2f} {y:.2f} Tm ({esc(text)}) Tj ET")

    def wrapped(text, width, font="F1", size=body_size, x=MARGIN_L, gap=body_gap):
        nonlocal y
        for part in textwrap.wrap(text, width=width, break_long_words=False, break_on_hyphens=False):
            line(part, font=font, size=size, x=x)
            y -= gap

    def heading(text):
        nonlocal y
        line(text, font="F2", size=heading_size, x=MARGIN_L)
        y -= heading_gap

    def bullet(text):
        nonlocal y
        parts = textwrap.wrap(text, width=78, break_long_words=False, break_on_hyphens=False)
        if not parts:
            return
        line("- " + parts[0], font="F1", size=body_size, x=MARGIN_L + 8)
        y -= body_gap
        for part in parts[1:]:
            line("  " + part, font="F1", size=body_size, x=MARGIN_L + 8)
            y -= body_gap

    # Title
    line("AACP App Summary (Repo-Based)", font="F2", size=title_size, x=MARGIN_L)
    y -= title_gap
    line(f"One-page brief generated from repository evidence only - {date.today().isoformat()}", font="F1", size=subtitle_size, x=MARGIN_L)
    y -= subtitle_gap

    # What it is
    heading("What it is")
    wrapped("AACP v0.9.0 is a Go implementation scaffold for the AACP technical specification.", width=86)
    wrapped("It provides a testnet-grade execution baseline with module routing, deterministic state commits, and ABCI-style lifecycle hooks.", width=86)
    y -= section_gap

    # Who it's for
    heading("Who it's for")
    wrapped("Primary user/persona: Not found in repo.", width=86)
    wrapped("Closest evidence suggests protocol and blockchain engineers building or testing AACP modules in Go.", width=86)
    y -= section_gap

    # What it does
    heading("What it does")
    bullet("Runs an HTTP daemon (`cmd/aacpd`) with `/api/health`, `/api/finalize-empty`, and `/api/finalize`.")
    bullet("Validates transactions end-to-end: decode, basic checks, timeout height, signature, nonce, and gas accounting.")
    bullet("Dispatches module actions through a router to AMX, AAP, WEAVE, Cap-UTXO, REP, ARB, AFD, FIAT, NODE, and GOV.")
    bullet("Commits state each block and returns `app_hash`; supports `memory` (default) or `iavl` backends via `AACP_STATE_BACKEND`.")
    bullet("Collects and flushes block/module events through an internal event bus.")
    bullet("Ships supporting executables: CLI (`cmd/aacp-cli`), relay heartbeat process (`cmd/aacp-relay`), and testnet docker compose.")
    y -= section_gap

    # How it works
    heading("How it works (architecture overview)")
    bullet("Client flow: `aacp-cli` or curl sends tx bytes to `aacpd` HTTP handlers.")
    bullet("`aacpd` initializes `abci.App`, tracks height, and calls `FinalizeBlock` per request.")
    bullet("Finalize pipeline: `DecodeTx -> ValidateBasic -> VerifySignature -> Nonce/Gas checks -> router.Execute`.")
    bullet("Router invokes module handlers; modules persist domain state in `internal/state.Store` and emit events.")
    bullet("EndBlock aggregates module events, `Commit()` computes new hash/version, and responses return tx results plus events.")
    bullet("Optional CometBFT bridge exists behind build tag in `internal/abci/comet_adapter.go`.")
    y -= section_gap

    # How to run
    heading("How to run (minimal getting started)")
    bullet("Install prerequisites: Go >= 1.22; `buf` or `protoc` for proto generation.")
    bullet("On macOS, optional helper: `./scripts/bootstrap_dev_tools.sh`.")
    bullet("From repo root: `make test` then `make build`.")
    bullet("Start daemon: `go run ./cmd/aacpd --port=8888`.")
    bullet("Verify: `curl -sS http://127.0.0.1:8888/api/health`.")

    return cmds, y


def write_pdf(content_stream: str, out_path: str):
    objs = []
    objs.append("<< /Type /Catalog /Pages 2 0 R >>")
    objs.append("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
    objs.append("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> /Contents 4 0 R >>")
    stream_bytes = content_stream.encode("utf-8")
    objs.append(f"<< /Length {len(stream_bytes)} >>\nstream\n{content_stream}\nendstream")
    objs.append("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
    objs.append("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")

    out = bytearray()
    out.extend(b"%PDF-1.4\n")
    out.extend(b"%\xe2\xe3\xcf\xd3\n")
    offsets = [0]

    for i, obj in enumerate(objs, start=1):
        offsets.append(len(out))
        out.extend(f"{i} 0 obj\n".encode("ascii"))
        out.extend(obj.encode("utf-8"))
        out.extend(b"\nendobj\n")

    xref_pos = len(out)
    out.extend(f"xref\n0 {len(objs) + 1}\n".encode("ascii"))
    out.extend(b"0000000000 65535 f \n")
    for off in offsets[1:]:
        out.extend(f"{off:010d} 00000 n \n".encode("ascii"))

    out.extend(f"trailer\n<< /Size {len(objs)+1} /Root 1 0 R >>\nstartxref\n{xref_pos}\n%%EOF\n".encode("ascii"))

    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "wb") as f:
        f.write(out)


if __name__ == "__main__":
    for scale in (1.0, 0.96, 0.92, 0.88):
        cmds, y_end = make_layout(scale)
        if y_end >= BOTTOM:
            selected = (cmds, y_end, scale)
            break
    else:
        selected = (cmds, y_end, 0.84)

    cmds, y_end, scale = selected
    content = "\n".join(cmds)
    write_pdf(content, OUT_PDF)
    print(f"written={OUT_PDF}")
    print(f"scale={scale:.2f}")
    print(f"y_end={y_end:.2f}")
