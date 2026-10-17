#!/bin/sh
# Starts seq-mcp for Claude Code, Codex CLI and Gemini CLI. Builds it first if
# this pinned version is not built yet. Session logs (D64, D65) are on by
# default. SEQ_LOGGING=off (or false, 0) turns them off in every client. SEQ_LOG_DIR,
# if set, names the directory (the old value "off" still turns logging off). Otherwise
# they go under ${CLAUDE_PLUGIN_DATA}/logs (Claude Code) or, where a client gives
# no data directory, ${XDG_DATA_HOME:-~/.local/share}/seq/logs/<client>, with
# <client> from SEQ_CLIENT (codex, gemini; anything else is "other"). Stdout is
# the MCP stream, so nothing but seq-mcp writes to it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bin=$("$here/install-seq-mcp.sh")
# An option the user left empty is not set.
for v in SEQ_SERVER_URL SEQ_CREDENTIAL_FILE SEQ_ACCOUNT SEQ_CA_FILE; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || unset "$v"
done
logging=on
case "${SEQ_LOGGING:-}" in
  off | OFF | Off | false | FALSE | False | 0) logging=off ;;
esac
case "${SEQ_LOG_DIR:-}" in
  off | OFF | Off | false | none) logging=off ;; # legacy: remove after one release
esac
if [ "$logging" = off ]; then
  unset SEQ_LOG_DIR
elif [ -z "${SEQ_LOG_DIR:-}" ]; then
  case "${SEQ_CLIENT:-}" in
    codex | gemini) client=$SEQ_CLIENT ;;
    "" | claude) client= ;;
    *) client=other ;;
  esac
  if [ -z "$client" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ]; then
    SEQ_LOG_DIR=$CLAUDE_PLUGIN_DATA/logs
  else
    SEQ_LOG_DIR=${XDG_DATA_HOME:-$HOME/.local/share}/seq/logs/${client:-other}
  fi
  export SEQ_LOG_DIR
fi
unset SEQ_LOGGING SEQ_CLIENT
exec "$bin" "$@"
