AI Agent (Codex) – Implementation Notes

Quickstart
- Build: `cd codex && npm install && npm run build`
- Run: `node dist/cli.js <providers> <models> <mcp-servers> <system> <user> [--config ../.ai-agent.json] [--accounting path] [--dry-run] [--verbose|--quiet]`
- Example: `node dist/cli.js openai gpt-4o-mini file-operations "You are helpful" "List files" --config ../.ai-agent.json`
- Stdin/file prompts: use `@file.txt` or `-` (stdin). Do not use `-` for both prompts.

Highlights
- Model-first fallback, then providers per model; identical request across attempts.
- Real-time streaming to stdout; partial streams are not persisted to history.
- MCP bootstrap (stdio/http/websocket/sse); initialization is non-fatal — failures are logged and LLM may proceed (`--trace-mcp` for details). `--dry-run` skips MCP spawn and LLM calls.
- Tool instructions appended once to system prompt:
  - `## TOOLS' INSTRUCTIONS`
  - `## TOOL {name} INSTRUCTIONS`
- Tool execution is orchestrated by the AI SDK/provider; the app imposes no tool count limits. Parallel tool calls can be toggled for OpenAI‑compatible providers via `--parallel-tool-calls/--no-parallel-tool-calls`. Per-call timeout via `--tool-timeout`.
- Library performs no I/O; emits via callbacks. CLI writes accounting JSONL if configured.

Providers
- `openai`, `anthropic`, `google`/`vertex`, `openrouter` (OpenAI-compatible at `https://openrouter.ai/api/v1`), and `ollama` (OpenAI-compatible at `http://localhost:11434/v1`). Configure API keys/base URLs in `.ai-agent.json`.

Tracing
- `--trace-llm`: Pretty request/response logging with Authorization redacted; raw SSE after stream completes.
- `--trace-mcp`: Connect, tools/prompts list, server stderr, callTool requests/results in a single `[mcp]` sink.

MCP Notes
- http uses the HTTP client transport; sse is distinct and uses SSE transport.
- Only tool instructions are appended to the system prompt; schemas are sent as tool definitions.

Accounting
- Logs only metadata (no content):
  - LLM: provider, model, tokens (input/output/total/cached if available), latency, status.
  - Tool: server, command, chars in/out, latency, status.
- CLI writes JSONL when `--accounting` or config `accounting.file` is set.

Example Config
- See `codex/example.ai-agent.json` for a starting point (OpenAI + Ollama + stdio file-operations MCP).
