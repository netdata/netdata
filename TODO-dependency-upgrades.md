# TL;DR
- Pin only the major versions for AI SDK + provider packages; update everything else to latest.

# Analysis
- Read required docs (SPECS/IMPLEMENTATION/DESIGN/MULTI-AGENT/TESTING/AI-AGENT-INTERNAL-API/AI-AGENT-GUIDE/README).
- `npm outdated` reports major updates for core AI SDK stack: `ai 5.0.116 -> 6.0.3`, `@ai-sdk/* 2.x -> 3.x`, `@ai-sdk/provider-utils 3.x -> 4.x`, `@ai-sdk/openai-compatible 1.x -> 2.x`.
- Minor/patch updates also available: `@typescript-eslint/* 8.50.0 -> 8.50.1`, `eslint-plugin-perfectionist 5.0.0 -> 5.1.0`, `knip 5.76.1 -> 5.77.1`.
- Build/lint requirements are strict (`npm run build`, `npm run lint` must pass); docs must be updated in the same commit if runtime behavior/defaults change.
- AI SDK integration points to review for breaking changes:
  - `src/llm-providers/base.ts` uses `streamText`/`generateText` from `ai`, `jsonSchema` and `ProviderOptions` from `@ai-sdk/provider-utils`, and `LanguageModel`/`ModelMessage`/`StreamTextResult` types from `ai`.
  - `src/llm-providers/openai.ts`, `openai-compatible.ts`, `anthropic.ts`, `google.ts`, `openrouter.ts`, `ollama.ts` create provider clients and pass providerOptions.
  - `src/llm-providers/test-llm.ts` imports `LanguageModelV2*` types from `@ai-sdk/provider` (likely impacted by SDK major changes).
  - `src/types-shim.d.ts` declares `@openrouter/ai-sdk-provider` types.
  - `src/setup-ai-sdk.ts` toggles `AI_SDK_LOG_WARNINGS` global (verify behavior in v6).

# Decisions (user)
- D1: Pin only major versions for AI SDK + provider packages (no major bumps there).
- D2: Update all other deps to latest.
- D3: Validation = at least build + lint (confirm if Phase1/Phase2 still required after updates).
- D4: Lockfile/docs = update docs whenever behavior/defaults change.

# Plan
- Identify the exact set of “AI SDK + providers” packages to constrain to current major.
- Update those packages to `^<current-major>.x` (or equivalent) without crossing major.
- Update all other packages to latest versions.
- Update `package-lock.json`.
- Run `npm run build` + `npm run lint`.
- Update docs only if runtime behavior/defaults change.

# Implied decisions
- Accept major-version constraints for AI SDK/providers; allow majors for all other deps.

# Testing requirements
- Required: `npm run build`, `npm run lint`.
- Optional: `npm run test:phase1` / `npm run test:phase2:tier1` if you still want them after non-AI updates.

# Documentation updates required
- Update docs if SDK major changes alter behavior/defaults (SPECS/IMPLEMENTATION/AI-AGENT-GUIDE/README as needed).
