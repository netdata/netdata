#!/usr/bin/env bash

set -e

# Get the directory where this script resides
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Build the program
./build.sh

# Source .env exporting everything globally
set -a
source .env
set +a

#TOOLS="brave-search"
TOOLS="jina-search"

#PROVIDERS="ollama"
#MODELS="gpt-oss:20b"

#PROVIDERS="vllm"
#MODELS="gpt-oss-20b"

PROVIDERS="openrouter"
MODELS="openai/gpt-oss-120b"

exec node claude/dist/cli.js "${PROVIDERS}" "${MODELS}" "${TOOLS}" \
	'@prompts/web-researcher.md' \
	'I need a comprehensive analysis of what netdata is and what it can do for me.' \
	--config .ai-agent.json --verbose # --trace-llm --trace-mcp
