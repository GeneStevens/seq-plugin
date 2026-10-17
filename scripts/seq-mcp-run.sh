#!/bin/sh
# Starts seq-mcp for Claude Code (the command in mcp/seq.json). Builds it first if
# this pinned version is not built yet. Stdout is the MCP stream, so nothing but
# seq-mcp writes to it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bin=$("$here/install-seq-mcp.sh")
# An option the user left empty is not set.
for v in SEQ_SERVER_URL SEQ_CREDENTIAL_FILE SEQ_ACCOUNT SEQ_CA_FILE; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || unset "$v"
done
exec "$bin" "$@"
