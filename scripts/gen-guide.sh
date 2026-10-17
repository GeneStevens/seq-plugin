#!/bin/sh
# Writes each client's form of the play guide from the one source, guide/PLAY.md:
#   skills/play-seq/SKILL.md   Claude Code skill (Codex reads the same file as a skill)
#   GEMINI.md                  the Gemini extension's context file
# With --check it writes nothing and fails if a copy is out of date.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
{
  cat <<'H'
---
name: play-seq
description: How to play SEQ, a multiplayer fantasy RPG, through the seq tools (look, go_to, take_contract, attack, loot, turn_in, release). Use when the player asks to play SEQ, make a character, take a contract, fight, or check what is going on in the game.
---

H
  cat guide/PLAY.md
} > "$tmp/SKILL.md"
{
  echo "<!-- Generated from guide/PLAY.md by scripts/gen-guide.sh. Do not edit. -->"
  echo
  cat guide/PLAY.md
} > "$tmp/GEMINI.md"
if [ "${1:-}" = "--check" ]; then
  bad=0
  cmp -s "$tmp/SKILL.md" skills/play-seq/SKILL.md || { echo "guide: skills/play-seq/SKILL.md is out of date; run scripts/gen-guide.sh" >&2; bad=1; }
  cmp -s "$tmp/GEMINI.md" GEMINI.md || { echo "guide: GEMINI.md is out of date; run scripts/gen-guide.sh" >&2; bad=1; }
  [ "$bad" = 0 ] || exit 1
  echo "guide: ok"
else
  cp "$tmp/SKILL.md" skills/play-seq/SKILL.md
  cp "$tmp/GEMINI.md" GEMINI.md
fi
