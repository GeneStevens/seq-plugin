#!/bin/sh
# Fails on a credential-shaped string or an absolute user-home path in any
# committed file. This repo is public: nothing private goes in it.
set -eu
cd "$(dirname "$0")/.."
users="/Use""rs/"
home="/ho""me/[a-z]"
# SEQ credentials and session tokens, PEM private keys, common token shapes.
secrets='seq_(ag|st)_[A-Za-z0-9_-]{16,}|-----BEGIN [A-Z ]*PRIVATE KEY|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{24,}'
bad=0
files=$(git ls-files; git ls-files --others --exclude-standard)
for f in $files; do
  [ -f "$f" ] || continue
  if grep -nIE "$secrets" "$f"; then echo "hygiene: credential-shaped string in $f" >&2; bad=1; fi
  if grep -nIE "$users|$home" "$f"; then echo "hygiene: absolute home path in $f" >&2; bad=1; fi
done
[ "$bad" = 0 ] || exit 1
echo "hygiene: ok"
