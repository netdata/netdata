#!/usr/bin/env bash

set -e

cd codex && npm install && npm run build
cd -

# Source .env exporting everything globally
set -a
source .env
set +a

#TOOLS="brave-search"
TOOLS="jina-search"

PROVIDERS="ollama"
MODELS="gpt-oss:20b"

#PROVIDERS="openrouter"
#MODELS="openai/gpt-oss-120b"

exec node codex/dist/cli.js "${PROVIDERS}" "${MODELS}" "${TOOLS}" \
	'@prompts/web-researcher.md' \
	'I need a comprehensive analysis of what netdata is and what it can do for me.' \
	--config .ai-agent.json --verbose # --trace-llm --trace-mcp
