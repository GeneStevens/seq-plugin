#!/bin/sh
# Starts seq-mcp for Claude Code, Codex CLI and Gemini CLI. Builds it first if
# this pinned version is not built yet. Session logs (D64) are on by default:
# SEQ_LOG_DIR, if set, names the directory; "off" turns logging off; otherwise
# they go under ${CLAUDE_PLUGIN_DATA}/logs (Claude Code) or, where a client gives
# no data directory (Codex, Gemini), ~/.local/share/seq/logs. Stdout is the MCP stream, so nothing but
# seq-mcp writes to it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bin=$("$here/install-seq-mcp.sh")
# An option the user left empty is not set.
for v in SEQ_SERVER_URL SEQ_CREDENTIAL_FILE SEQ_ACCOUNT SEQ_CA_FILE; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || unset "$v"
done
case "${SEQ_LOG_DIR:-}" in
  off | OFF | Off | false | none) unset SEQ_LOG_DIR ;;
  "")
    if [ -n "${CLAUDE_PLUGIN_DATA:-}" ]; then
      SEQ_LOG_DIR=$CLAUDE_PLUGIN_DATA/logs
    else
      SEQ_LOG_DIR=${XDG_DATA_HOME:-$HOME/.local/share}/seq/logs
    fi
    export SEQ_LOG_DIR
    ;;
esac
exec "$bin" "$@"
