#!/bin/sh
# Build seq-mcp from source at the pinned seq commit into DEST (default: the
# plugin's data directory). Needs Go and read access to the seq repository.
# Prints the path of the binary on stdout; progress goes to stderr.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
. "$here/pin.env"
dir=${SEQ_PLUGIN_BIN_DIR:-${CLAUDE_PLUGIN_DATA:-${XDG_CACHE_HOME:-$HOME/.cache}/seq-plugin}/bin}
bin="$dir/seq-mcp-$SEQ_COMMIT"
if [ ! -x "$bin" ]; then
  command -v go >/dev/null 2>&1 || { echo "seq: Go is needed to build seq-mcp (https://go.dev/dl/)" >&2; exit 1; }
  mkdir -p "$dir"
  tmp=$(mktemp -d "$dir/build.XXXXXX")
  trap 'rm -rf "$tmp"' EXIT
  echo "seq: building seq-mcp at $SEQ_COMMIT ..." >&2
  GOPRIVATE=${GOPRIVATE:-github.com/GeneStevens/*} GOBIN="$tmp" \
    go install "$SEQ_MODULE/cmd/seq-mcp@$SEQ_COMMIT" >&2 ||
    { echo "seq: the build failed; building needs read access to the seq repository (see README)" >&2; exit 1; }
  mv "$tmp/seq-mcp" "$bin"
fi
echo "$bin"
