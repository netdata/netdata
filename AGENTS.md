Read ./README.md for specifications.
Read ./IMPLEMENTATION.md for AI SDK/ModelContextProtocol implementation.
Read ./TUI.md for TUI implementation.

Directory claude/ contains Claude Code's implementation.
Directory codex/ contains OpenAI Codex's implementation.

You MUST focus on your own implementation without cheating from other AI's implementation. I have the others' implementation already. I am interested for YOUR implementation! Otherwise there is no use of you.

---

For your tests, we have ollama on host 'nova'.
Use it with model 'gpt-oss:20b'

---

Script ./run.sh provides a working example for using the application (codex's implementation)

---

The directory libs/ has a checkout of the libraries we use, so you can examine their codebase

---

The file .env has provider API keys for many our MCP tools and llm providers.
YOU ARE NOT ALLOWED TO READ THIS FILE.
READING THIS FILE MEANS WE NEED TO INVALIDATE ALL KEYS AND CREATE NEW ONES.
THIS IS TREMENDOUS AMOUNT OF WORK. NEVER READ .env FILE.
- This is code you just created, which I will spend a tone of time testing, which according to eslint has 500 issues. You expect me to find these 500 issues by hand? It will be weeks for me, probably an hour for you. Fix them all. No questions. No pushback. No workarounds. Fix your code, or I will just throw it away.
- after ever change we ensure the application builds and that eslint reports zero warnings and zero errors, without exceptions.