#!/usr/bin/env bash
set -euo pipefail

LINK="$HOME/.claude/skills/teleport-dbtest"

if [[ ! -L "$LINK" ]]; then
  echo "error: $LINK is not a symlink (nothing to remove)" >&2
  exit 1
fi

rm "$LINK"
echo "removed: $LINK"
