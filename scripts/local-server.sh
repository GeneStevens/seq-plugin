#!/bin/sh
# Runs a local SEQ server for the plugin: seqd built at the pinned seq commit,
# serving the pinned seq-world packs, with a development agent credential.
# Everything lives in a private directory (default ~/.seq-local, or $SEQ_LOCAL_DIR):
# a certificate made once, and a credential file rewritten at every start (the
# server keeps its accounts in memory, so a restart needs a new credential).
# Prints the three values for `claude plugin install ... --config`, then runs the
# server until Ctrl+C. Needs Go, openssl (not LibreSSL's EC keys: this uses RSA)
# and read access to the private seq and seq-world repositories.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
. "$here/pin.env"
export GOPRIVATE=${GOPRIVATE:-github.com/GeneStevens/*}
addr=${SEQ_ADDR:-127.0.0.1:8443}
dir=${SEQ_LOCAL_DIR:-$HOME/.seq-local}
umask 077
mkdir -p "$dir"
if [ ! -f "$dir/cert.pem" ] || [ ! -f "$dir/key.pem" ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -keyout "$dir/key.pem" -out "$dir/cert.pem" -days 30 \
    -subj /CN=localhost -addext subjectAltName=DNS:localhost,IP:127.0.0.1 2>/dev/null
fi
echo "seq: building seqd at $SEQ_COMMIT ..." >&2
GOBIN="$dir/bin" go install "$SEQ_MODULE/cmd/seqd@$SEQ_COMMIT" >&2
# The world content at the pinned commit, from Go's module cache (read-only).
packs=$(cd "$here/.." && go mod download -json "$SEQ_WORLD_MODULE@$SEQ_WORLD_COMMIT" |
  sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')/packs
[ -d "$packs" ] || { echo "seq: could not fetch the seq-world packs" >&2; exit 1; }
log="$dir/seqd.log"
"$dir/bin/seqd" -listen "$addr" -cert "$dir/cert.pem" -key "$dir/key.pem" -packs "$packs" \
  -dev-account me -dev-agent claude >"$log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT INT TERM
cred=""
for _ in $(seq 1 100); do
  cred=$(sed -n 's/^agent credential claude for [^:]*: //p' "$log")
  [ -n "$cred" ] && break
  kill -0 "$pid" 2>/dev/null || { cat "$log" >&2; exit 1; }
  sleep 0.2
done
[ -n "$cred" ] || { echo "seq: seqd did not start; see $log" >&2; exit 1; }
printf '%s\n' "$cred" >"$dir/credential"
cat <<OUT
seqd is up (log: $log). Install the plugin with:
  --config server_url=wss://$addr/seq/v1 --config credential_file=$dir/credential --config ca_file=$dir/cert.pem
Ctrl+C stops it. A restart makes a new credential (written to the same file), so restart Claude Code's seq server too (/mcp).
OUT
wait "$pid"
