#!/usr/bin/env bash

set -e

cd codex && npm install && npm run build
cd -

# Source .env exporting everything globally
set -a
source .env
set +a

exec node codex/dist/cli.js openrouter openai/gpt-oss-120b brave-search '@prompts/web-researcher.md' 'What is netdata and does it support metrics, logs and traces?' --config .ai-agent.json # --verbose --trace-llm --trace-mcp
