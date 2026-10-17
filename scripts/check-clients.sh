#!/bin/sh
# Checks that Codex CLI and Gemini CLI themselves accept this repository's
# packaging, each against a throwaway config directory (no login, no chat, nothing
# of the real ~/.codex or ~/.gemini is touched). A client whose CLI is not installed
# is skipped with a message. seq-mcp is built (or found) first so that Gemini can
# report the server connected.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
here="$root/scripts"
bin=$("$here/install-seq-mcp.sh")
bindir=$(dirname "$bin")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail() { echo "clients: $*" >&2; exit 1; }
gem() { if command -v timeout >/dev/null 2>&1; then env HOME=$home GEMINI_CLI_HOME=$home timeout 120 "$@"; else env HOME=$home GEMINI_CLI_HOME=$home "$@"; fi; }
export SEQ_PLUGIN_BIN_DIR=$bindir
export SEQ_SERVER_URL=wss://127.0.0.1:1/seq/v1
export SEQ_CREDENTIAL_FILE=$tmp/credential
: > "$SEQ_CREDENTIAL_FILE"

if command -v codex >/dev/null 2>&1; then
  export CODEX_HOME=$tmp/codex
  mkdir -p "$CODEX_HOME"
  "$here/codex-install.sh" >"$tmp/codex.out" 2>&1 || { cat "$tmp/codex.out" >&2; fail "codex-install.sh failed"; }
  codex mcp list | tee "$tmp/codex.list"
  grep -q '^seq ' "$tmp/codex.list" || fail "codex mcp list does not show seq"
  codex mcp get seq --json | tee "$tmp/codex.get"
  grep -q "$root/scripts/seq-mcp-run.sh" "$tmp/codex.get" || fail "codex: seq does not run the launcher"
  grep -q '"SEQ_CREDENTIAL_FILE"' "$tmp/codex.get" || fail "codex: seq has no credential file setting"
  codex plugin list | tee "$tmp/codex.plugins"
  grep -q 'seq@seq.*installed, enabled' "$tmp/codex.plugins" || fail "codex: plugin seq@seq is not installed"
  [ -f "$CODEX_HOME"/plugins/cache/seq/seq/*/skills/play-seq/SKILL.md ] || fail "codex: the play-seq skill is not in the installed plugin"
  echo "clients: codex $(codex --version) accepts the packaging"
  unset CODEX_HOME
else
  echo "clients: codex not installed: skipping its check"
fi

if command -v gemini >/dev/null 2>&1; then
  home=$tmp/gemini
  mkdir -p "$home/.gemini"
  real=$(cd "$root" && pwd -P)
  printf '{"%s":"TRUST_FOLDER"}\n' "$real" > "$home/.gemini/trustedFolders.json"
  # Gemini passes its own environment on to the extension's server, so the settings
  # are plain environment variables here too. HOME moves the whole ~/.gemini.
  gem gemini extensions link --consent "$root" </dev/null >"$tmp/gemini.out" 2>&1 ||
    { cat "$tmp/gemini.out" >&2; fail "gemini extensions link failed"; }
  gem gemini extensions list </dev/null >"$tmp/gemini.ext" 2>&1; cat "$tmp/gemini.ext"
  grep -q 'seq.*0\.1\.0' "$tmp/gemini.ext" || fail "gemini extensions list does not show seq"
  grep -q 'GEMINI.md' "$tmp/gemini.ext" || fail "gemini: the context file is not loaded"
  gem gemini mcp list </dev/null >"$tmp/gemini.mcp" 2>&1; cat "$tmp/gemini.mcp"
  grep -q 'seq.*from.*seq' "$tmp/gemini.mcp" || fail "gemini mcp list does not show seq"
  grep -q 'Connected' "$tmp/gemini.mcp" || fail "gemini: seq is not connected"
  echo "clients: gemini $(gemini --version) accepts the packaging"
else
  echo "clients: gemini not installed: skipping its check"
fi
