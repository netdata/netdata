#!/usr/bin/env bash

[ -z "${1}" ] && echo >&2 "Usage: $0 <query>" && exit 1

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

#TOOLS="brave"
#TOOLS="jina"
TOOLS="fetcher,brave"

#PROVIDERS="ollama"
#MODELS="gpt-oss:20b"

#PROVIDERS="vllm"
#MODELS="gpt-oss-20b"

PROVIDERS="openrouter"
MODELS="openai/gpt-oss-120b,openai/gpt-oss-20b"
#MODELS="anthropic/claude-sonnet-4"
#MODELS="google/gemini-2.0-flash-001"

# export DEBUG=true
exec node claude/dist/cli.js "${PROVIDERS}" "${MODELS}" "${TOOLS}" \
	'@prompts/contact-intelligence-researcher.md' \
	"$1" \
	--config .ai-agent.json --verbose # --trace-llm --trace-mcp
