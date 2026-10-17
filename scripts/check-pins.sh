#!/bin/sh
# The seq commit in pin.env and the seq version in go.mod (which the test's
# server is built from) must be the same commit.
set -eu
cd "$(dirname "$0")/.."
. scripts/pin.env
want=$(printf %s "$SEQ_COMMIT" | cut -c1-12)
have=$(sed -n 's|^.*github.com/GeneStevens/seq v[0-9.]*-[0-9]*-\([0-9a-f]*\)$|\1|p' go.mod | head -1)
[ "$want" = "$have" ] || { echo "pins: pin.env has seq $want, go.mod has $have" >&2; exit 1; }
echo "pins: ok"
