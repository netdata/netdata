#!/bin/bash

# AI Agent Usage Examples
# Make sure you have built the project: npm run build

echo "=== AI Agent Examples ==="
echo ""

# Set up configuration
CONFIG_FILE="config.example.json"

echo "1. Basic OpenAI chat (requires OPENAI_API_KEY):"
echo "ai-agent openai gpt-4o-mini \"\" \"You are a helpful assistant.\" \"What is TypeScript?\" --config $CONFIG_FILE"
echo ""

echo "2. Ollama local inference:"
echo "ai-agent ollama llama3.2:3b \"\" \"You are a coding assistant.\" \"Write Hello World in Python\" --config $CONFIG_FILE"
echo ""

echo "3. Multiple providers with fallback:"
echo "ai-agent \"openai,ollama\" \"gpt-4o-mini,llama3.2:3b\" \"\" \"System prompt\" \"User message\" --config $CONFIG_FILE"
echo ""

echo "4. File-based prompts:"
echo "ai-agent openai gpt-4o-mini \"\" \"@system-prompt.txt\" \"@user-prompt.txt\" --config $CONFIG_FILE"
echo ""

echo "5. Save and load conversations:"
echo "ai-agent openai gpt-4o-mini \"\" \"System prompt\" \"Hello\" --save chat.json --config $CONFIG_FILE"
echo "ai-agent openai gpt-4o-mini \"\" \"System prompt\" \"Continue\" --load chat.json --save chat.json --config $CONFIG_FILE"
echo ""

echo "6. With MCP tools (filesystem example):"
echo "ai-agent openai gpt-4o-mini filesystem \"You can read/write files.\" \"List the files in current directory\" --config $CONFIG_FILE"
echo ""

echo "7. Custom parameters:"
echo "ai-agent openai gpt-4o-mini \"\" \"System\" \"User\" --temperature 0.9 --top-p 0.8 --max-parallel-tools 2 --config $CONFIG_FILE"
echo ""

echo "To run any of these examples, copy the command and replace API keys in your config file."