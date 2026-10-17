#!/bin/sh
# Installs SEQ into Codex CLI: the how-to-play skill (as a plugin from this
# checkout) and the seq MCP server. Reads the same four settings as the other
# clients from the environment: SEQ_SERVER_URL and SEQ_CREDENTIAL_FILE (required),
# SEQ_ACCOUNT and SEQ_CA_FILE (optional). Only paths and the URL go into Codex's
# config.toml, never the credential. Installs under CODEX_HOME (default ~/.codex).
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
: "${SEQ_SERVER_URL:?set SEQ_SERVER_URL, like wss://host:port/seq/v1}"
: "${SEQ_CREDENTIAL_FILE:?set SEQ_CREDENTIAL_FILE to a file holding your seq_ag_... credential}"
codex plugin marketplace add "$root"
codex plugin add seq@seq
codex mcp remove seq >/dev/null 2>&1 || true
set -- --env "SEQ_SERVER_URL=$SEQ_SERVER_URL" --env "SEQ_CREDENTIAL_FILE=$SEQ_CREDENTIAL_FILE"
[ -z "${SEQ_ACCOUNT:-}" ] || set -- "$@" --env "SEQ_ACCOUNT=$SEQ_ACCOUNT"
[ -z "${SEQ_CA_FILE:-}" ] || set -- "$@" --env "SEQ_CA_FILE=$SEQ_CA_FILE"
codex mcp add seq "$@" -- "$root/scripts/seq-mcp-run.sh"
