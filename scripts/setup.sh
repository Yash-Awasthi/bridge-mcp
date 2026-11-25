#!/usr/bin/env bash
# Build the board, put it on PATH, and wire every detected MCP client.
# Override the destination with VB_BIN_DIR; extra arguments go to `vb install`.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin="${VB_BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$bin"
cd "$root"
go build -o "$bin/vb" ./cmd/open-board
go build -o "$bin/mcp-bridge" ./cmd/mcp-bridge
echo "built $bin/vb and $bin/mcp-bridge"
case ":$PATH:" in
  *":$bin:"*) ;;
  *) echo "note: add $bin to PATH with:  export PATH=\"$bin:\$PATH\"" ;;
esac
exec "$bin/vb" install "$@"
