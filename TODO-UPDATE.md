AI-Agent Upgrade Roadmap (created 2025-10-20)
=============================================

Phase 1 – TypeScript 5.9 baseline  
- Bump TypeScript to 5.9.x, switch compiler target/module to Node 20 defaults.  
- Update tsconfig, ensure scripts build, address new inference errors.

Phase 2 – ESLint 9 migration  
- Replace current config with ESLint 9 flat setup + `typescript-eslint` 8 helpers.  
- Restore strict linting with minimal overrides, add lint ban for `import defer`.

Phase 3 – AI SDK 5 adoption  
- Upgrade `ai`, `@ai-sdk/*`, OpenRouter, Ollama providers to latest.  
- Refactor provider adapters/tool glue to native v5 APIs and streaming semantics.

Phase 4 – Headend runtime updates  
- Port Express routers/middleware to v5.1 syntax and behaviors.  
- Upgrade Slack Bolt to 4.5.x, adjust socket-mode startup and receivers.

Phase 5 – MCP stack refresh  
- Raise `@modelcontextprotocol/sdk` to ≥1.20, align transports with 2025-06-18 spec.  
- Revisit MCP warmup/init logging for new capabilities and auth defaults.

Phase 6 – Regression + docs  ✅  
- Verified 2025-10-21 stack: `typescript@5.9.3`, `eslint@9.38.0`, `ai@5.0.76`, `express@5.1.0`, `@slack/bolt@4.5.0`, `@modelcontextprotocol/sdk@1.20.1`.  
- Build/lint/tests (`npm run build`, `npm run lint`, `npm run test:phase1`) all pass.  
- Note for changelog: upgraded toolchain to latest 2025 releases; no runtime regressions observed.

Phase 7 – Feature adoption review (deferred)  
- Evaluate AI SDK `stopWhen` / agent-loop controls.  
- Review Slack Work Objects & Agents support defaults.  
- Revisit enabling `import defer` once runtime/bundler coverage exists.
