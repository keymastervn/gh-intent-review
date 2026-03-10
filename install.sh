#!/usr/bin/env bash
set -euo pipefail

DEST="$HOME/.gh-intent-review.yml"
REPO="keymastervn/gh-intent-review"
RAW_URL="https://raw.githubusercontent.com/$REPO/main/.gh-intent-review.yml.example"

if [ -f "$DEST" ]; then
  echo "Config already exists at $DEST — skipping."
  exit 0
fi

# When run locally (not piped), prefer the copy next to this script.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
LOCAL_EXAMPLE="$SCRIPT_DIR/.gh-intent-review.yml.example"

if [ -f "$LOCAL_EXAMPLE" ]; then
  cp "$LOCAL_EXAMPLE" "$DEST"
else
  echo "Downloading default config from GitHub..."
  if command -v curl >/dev/null 2>&1; then
    curl -sSfL "$RAW_URL" -o "$DEST"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$DEST" "$RAW_URL"
  else
    echo "Error: curl or wget is required to download the config." >&2
    exit 1
  fi
fi

echo "Created default config at $DEST"
echo "Edit it to set your preferred LLM provider, model, and review options."
