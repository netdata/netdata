## YOU MUST READ

- docs/SPECS.md for specifications.
- docs/IMPLEMENTATION.md for AI SDK/ModelContextProtocol implementation.
- docs/DESIGN.md for the design of the application.
- docs/MULTI-AGENT.md for the recursive multi-agent design.
- docs/docs/AI-AGENT-INTERNAL-API.md current status of the ai-agent internal agent API.
- README.md for end-user documentation of the application.

Directory claude/ contains the current implementation, architected by claude and co-implemented by codex.
Directory codex/ and codex2/ contain the original and an alternative approach. Do not use them. Obsolete code, only as reference.

## TESTING MODELS

For your tests, we have ollama on host 'nova'.
Use it with model 'gpt-oss:20b'

## TESTING SCRIPT

Script ./run.sh provides a working example for using the application (codex's implementation)

## SECURITY AND PRIVACY

The file .env has provider API keys for many our MCP tools and llm providers.
YOU ARE NOT ALLOWED TO READ THIS FILE.
READING THIS FILE MEANS WE NEED TO INVALIDATE ALL KEYS AND CREATE NEW ONES.
THIS IS TREMENDOUS AMOUNT OF WORK. NEVER READ .env FILE.

## QUALITY REQUIREMENTS

After ever change you **MUST** ensure the application builds and that linter report zero warnings and zero errors, without exceptions.

This is code created by AI agent, which I will spend a tone of time testing and polishing.
You should not expect me to find issues by hand, when there are obvious issues identified by linters and build systems.
It will be weeks for me, probably an hour for you.
Fix them all. No questions. No pushback. No workarounds.
