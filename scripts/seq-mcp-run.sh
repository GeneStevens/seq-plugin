#!/bin/sh
# Starts seq-mcp for Claude Code, Codex CLI and Gemini CLI. Builds it first if
# this pinned version is not built yet. Session logs (D64, D65) are on by
# default. SEQ_LOGGING=off (or false, 0) turns them off in every client. SEQ_LOG_DIR,
# if set, names the directory (the old value "off" still turns logging off). Otherwise
# they go under ${CLAUDE_PLUGIN_DATA}/logs (Claude Code) or, where a client gives
# no data directory, ${XDG_DATA_HOME:-~/.local/share}/seq/logs/<client>, with
# <client> from SEQ_CLIENT (codex, gemini; anything else is "other"). Stdout is
# the MCP stream, so nothing but seq-mcp writes to it.
#
# The credential file is SEQ_ACCOUNT_FILE or its older name SEQ_CREDENTIAL_FILE
# (either works in every client). Gemini CLI drops any environment variable whose
# name contains CREDENTIAL, KEY, TOKEN, AUTH, CERT and the like before it starts a
# server, so SEQ_ACCOUNT_FILE is the name to use there. If the server URL or the
# credential is missing, seq-mcp is not started: a stand-in server answers the first
# tool call with what is missing and how to set it for the client in use.
# SEQ_LAUNCH_REPORT=<file>, if set, gets the SEQ_ settings that arrived (names and
# values; never the credential itself): `scripts/check-clients.sh` reads it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
# An option the user left empty is not set.
for v in SEQ_SERVER_URL SEQ_ACCOUNT_FILE SEQ_CREDENTIAL_FILE SEQ_ACCOUNT SEQ_CA_FILE; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || unset "$v"
done
if [ -z "${SEQ_CREDENTIAL_FILE:-}" ] && [ -n "${SEQ_ACCOUNT_FILE:-}" ]; then
  SEQ_CREDENTIAL_FILE=$SEQ_ACCOUNT_FILE
  export SEQ_CREDENTIAL_FILE
fi
unset SEQ_ACCOUNT_FILE
if [ -n "${SEQ_LAUNCH_REPORT:-}" ]; then
  {
    for v in SEQ_SERVER_URL SEQ_CREDENTIAL_FILE SEQ_ACCOUNT SEQ_CA_FILE; do
      eval "val=\${$v:-}"
      [ -z "$val" ] || echo "$v=$val"
    done
  } > "$SEQ_LAUNCH_REPORT"
fi

# What is missing, said for the client that started us.
missing=
[ -n "${SEQ_SERVER_URL:-}" ] || missing="the server URL (SEQ_SERVER_URL)"
if [ -z "${SEQ_CREDENTIAL_FILE:-}" ] && [ -z "${SEQ_AGENT_CREDENTIAL:-}" ]; then
  missing="${missing:+$missing and }the credential file (SEQ_ACCOUNT_FILE)"
elif [ -n "${SEQ_CREDENTIAL_FILE:-}" ] && [ ! -r "$SEQ_CREDENTIAL_FILE" ]; then
  missing="${missing:+$missing and }a readable credential file (the file $SEQ_CREDENTIAL_FILE is not there or cannot be read)"
fi
if [ -n "$missing" ]; then
  case "${SEQ_CLIENT:-}" in
    codex) how="Set SEQ_SERVER_URL and SEQ_CREDENTIAL_FILE in your shell and run scripts/codex-install.sh again (see the README, Codex CLI)." ;;
    gemini) how="Run: gemini extensions config seq   (it asks for each setting). Or export SEQ_SERVER_URL and SEQ_ACCOUNT_FILE where you start gemini; Gemini CLI hides any variable whose name contains CREDENTIAL, so SEQ_CREDENTIAL_FILE does not reach SEQ there." ;;
    *) how="Run /plugin configure seq@seq in Claude Code and set Server URL and Credential file (see the README)." ;;
  esac
  msg="SEQ is not set up: it is missing $missing. $how"
  echo "seq: $msg" >&2
  exec "$here/seq-mcp-unconfigured.sh" "$msg"
fi

bin=$("$here/install-seq-mcp.sh")
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
unset SEQ_LOGGING SEQ_CLIENT SEQ_LAUNCH_REPORT
exec "$bin" "$@"
