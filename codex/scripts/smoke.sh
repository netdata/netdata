#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"

if [[ ! -f "$DIR/dist/cli.js" ]]; then
  echo "Build first: (cd codex && npm install && npm run build)" >&2
  exit 1
fi

CONFIG="${1:-$DIR/example.ai-agent.json}"

node "$DIR/dist/cli.js" openai gpt-4o-mini file-operations "You are a helpful assistant" "List files" --config "$CONFIG"

