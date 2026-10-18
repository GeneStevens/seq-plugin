#!/bin/sh
# Checks that Codex CLI and Gemini CLI themselves accept this repository's
# packaging, and then that each makes a real game call. Every check runs in a
# throwaway config directory (no login, no chat; nothing of the real ~/.codex or
# ~/.gemini is touched). A client whose CLI is not installed is skipped with a
# message.
#
# 1. Packaging: Codex lists the server and the plugin; Gemini validates the
#    extension.
# 2. A real call (test/clients): for each installed client, a local SEQ server on the
#    Thornwick packs, the client's own configuration, the server started the way
#    that client starts it, and new_character, look and logout over stdio. The look
#    must answer "You are ...". For Gemini the server is started by Gemini itself, so
#    its removal of variables like SEQ_CREDENTIAL_FILE is part of the test, and the
#    old setup (that variable alone) must fail.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
here="$root/scripts"
bin=$("$here/install-seq-mcp.sh")
bindir=$(dirname "$bin")
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail() { echo "clients: $*" >&2; exit 1; }
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
  gemini extensions validate "$root" </dev/null >"$tmp/gemini.out" 2>&1 ||
    { cat "$tmp/gemini.out" >&2; fail "gemini extensions validate failed"; }
  grep -q 'successfully validated' "$tmp/gemini.out" || { cat "$tmp/gemini.out" >&2; fail "gemini did not validate the extension"; }
  echo "clients: gemini $(gemini --version) accepts the packaging"
else
  echo "clients: gemini not installed: skipping its check"
fi

# The real calls. The variables above are for the packaging checks only.
unset SEQ_SERVER_URL SEQ_CREDENTIAL_FILE SEQ_PLUGIN_BIN_DIR
if command -v codex >/dev/null 2>&1 || command -v gemini >/dev/null 2>&1; then
  cd "$root"
  SEQ_CHECK_CLIENTS=1 GOPRIVATE=${GOPRIVATE:-github.com/GeneStevens/*} go test -count=1 -v ./test/clients >"$tmp/calls.out" 2>&1 ||
    { grep -v '^[0-9]\{10,\} ' "$tmp/calls.out" >&2; fail "a client could not make a real game call"; }
  grep -E '^\s+clients_test.go:[0-9]+: (codex|gemini)' "$tmp/calls.out" || true
  echo "clients: real game calls ok"
fi
