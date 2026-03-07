#!/bin/bash
set -euo pipefail

if ! command -v brew >/dev/null 2>&1; then
  echo "Homebrew is required on macOS to install prerequisites." >&2
  exit 1
fi

echo "Installing Go, buf, and protobuf ..."
brew install go buf protobuf

echo "Done. Versions:"
go version
buf --version
protoc --version
