#!/usr/bin/env bash
set -euo pipefail

SKILL_DIR="$(cd "$(dirname "$0")/../skill" && pwd)"
LINK="$HOME/.claude/skills/teleport-dbtest"

if [[ -e "$LINK" || -L "$LINK" ]]; then
  echo "error: $LINK already exists" >&2
  exit 1
fi

mkdir -p "$HOME/.claude/skills"
ln -s "$SKILL_DIR" "$LINK"
echo "installed: $LINK -> $SKILL_DIR"
