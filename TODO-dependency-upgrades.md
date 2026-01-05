# TL;DR
- Update Vercel AI SDK core + provider packages to the latest major versions and align code, tests, and docs.
- Dependabot has open PRs for AI SDK core and provider majors; decide scope and merge strategy before coding.

# Analysis
- Current AI SDK dependency versions in `package.json` (rolled back to v5-compatible majors):
  - `ai` is `^5.0.117` and providers are `@ai-sdk/anthropic ^2.0.57`, `@ai-sdk/google ^2.0.52`, `@ai-sdk/openai ^2.0.89`, `@ai-sdk/openai-compatible ^1.0.30`, `@ai-sdk/provider-utils ^3.0.20`. (`package.json` lines 35-38, 56)
  - Other relevant updates: `@openrouter/ai-sdk-provider ^1.5.4`, `ollama-ai-provider-v2 ^1.5.5` (downgraded to keep provider v2), `zod ^4.3.5`. (`package.json` lines 43, 65, 69)
- Additional dependency update: `knip ^5.80.0` (devDependency). (`package.json` line 96)
- AI SDK integration points (likely affected by a major bump):
  - `src/llm-providers/base.ts` imports `streamText/generateText` and `LanguageModel` types from `ai` and `jsonSchema`/`ProviderOptions` from `@ai-sdk/provider-utils`. (`src/llm-providers/base.ts:1-19`)
  - Provider adapters use SDK factories: `createOpenAI`, `createOpenAICompatible`, `createAnthropic`, `createGoogleGenerativeAI`. (`src/llm-providers/openai.ts:1-35`, `src/llm-providers/openai-compatible.ts:1-41`, `src/llm-providers/anthropic.ts:1-34`, `src/llm-providers/google.ts:1-28`)
  - Test provider relies on `@ai-sdk/provider` v2 types. (`src/llm-providers/test-llm.ts:1-3`)
  - Shared types depend on `ReasoningOutput` from `ai`. (`src/types.ts:1-6`)
- Docs currently state AI SDK v5 in the implementation plan; this will need updating if we move to v6. (`docs/IMPLEMENTATION.md:5`)
- Open Dependabot PRs (GitHub API):
  - Merged: #57, #58, #64, #65.
  - Closed: #59 (obsolete due to v5 rollback).
- Build now passes after rollback to AI SDK v5-compatible majors.
- OpenRouter peer mismatch resolved by staying on `ai` v5.
- Upstream OpenRouter PR #307 is open and explicitly targets AI SDK v6 support (beta), with Provider/LanguageModel v3 migrations and `ai` peer dependency `^6.0.0`. (https://github.com/OpenRouterTeam/ai-sdk-provider/pull/307)
- User note (unverified): maintainers are on vacation; PR #307 will be prioritized when they return.

# Decisions (resolved)
- D1: Merge all Dependabot PRs.
- D2: Update **all** dependencies to the latest versions (not just AI SDK packages).
- D3: Run **all tests except Phase 2** (build + lint + full test suite excluding Phase 2).
- D4: Resolve OpenRouter provider peer mismatch with AI SDK v6.
  - Chosen: Option B — stay on `ai` v5 until OpenRouter releases v6 support.
  - Option A: Keep `ai` v6 and accept the peer mismatch for now.
    - Pros: Fully up-to-date core SDK; matches instruction to update latest.
    - Cons: Possible runtime/type incompatibility with `@openrouter/ai-sdk-provider` which declares `ai@^5`.
  - Option B: Downgrade `ai` to v5 to satisfy OpenRouter peer range.
    - Pros: Matches peer requirements; lower risk for OpenRouter provider.
    - Cons: Violates “latest” for core SDK; conflicts with dependabot v6.
  - Option C: Remove/disable OpenRouter provider dependency until it supports v6.
    - Pros: Keeps core SDK latest and avoids mismatch.
    - Cons: Drops OpenRouter support.
- D5: How to handle Dependabot PR #59 (`@ai-sdk/google`), currently not mergeable due to rebase-in-progress (decision needed).
  - Option A: Wait and merge #59 once Dependabot finishes rebasing.
    - Pros: Keeps PR history clean; no manual intervention.
    - Cons: Blocks “merge all PRs” until Dependabot completes.
  - Option B: Manually rebase/merge the PR branch locally and close #59 as resolved.
    - Pros: Unblocks quickly; still aligns with “merge all”.
    - Cons: Manual work; risks minor drift from Dependabot branch.
  - Option C: Close #59 as duplicate because `@ai-sdk/google` is already upgraded in `package.json`.
    - Pros: No extra work; avoids duplicate change.
    - Cons: Technically not “merged,” might be against expectation.
- Decision: D5 Option C chosen — close PR #59 as duplicate/obsolete (v5 rollback).
- D6: Scope of AI SDK rollback to v5 (decision needed).
  - Option A: Roll back the full AI SDK stack to v5-compatible majors (core + providers + provider-utils + openai-compatible).
    - Pros: Expected compatibility with `ai` v5; aligns with OpenRouter peer range.
    - Cons: Conflicts with “latest major” goal for SDK packages.
  - Option B: Keep provider packages at v3 while downgrading only `ai`.
    - Pros: Minimizes package churn.
    - Cons: High risk of incompatibility if providers expect v6 contracts.
- Decision: D6 Option A chosen — roll back the full AI SDK stack to v5-compatible majors.

# Plan
- Merge Dependabot PRs for AI SDK core/providers and zod.
- Run dependency upgrade to latest across `dependencies`, `devDependencies`, and `optionalDependencies`.
- Update code for any AI SDK v6 API changes (providers, base, test provider).
- Update docs that reference AI SDK v5 or changed behavior.
- Run build + lint + all tests except Phase 2 (Phase 1 + Vitest).

# Implied decisions
- Align AI SDK core + providers to compatible majors to avoid API drift.
- Update docs in the same commit if runtime behavior or defaults change.

# Testing requirements
- Required: `npm run build`, `npm run lint`.
- Required: `npm test` (Vitest) and `npm run test:phase1`.
- Skip: all Phase 2 runs.

# Documentation updates required
- `docs/IMPLEMENTATION.md` currently mentions AI SDK v5 (line 5); update to v6 if upgraded.
- Update any other docs that describe SDK behavior or provider APIs if the upgrade changes runtime behavior.
- Updated: `docs/specs/tools-xml-transport.md` to reflect truncated structured-output handling and <think> stripping in error checks.
