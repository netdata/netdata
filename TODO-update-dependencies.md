# TODO — Dependency Refresh (November 9, 2025)

## 1. TL;DR
- Costa asked for `npm outdated` plus upgrading every dependency/devDependency to the latest compatible release available on 9 Nov 2025.
- Current repo already contains many staged TODOs; this work is specific to dependency bumps and must keep lint/build green per project rules.
- Updating core SDK providers (Vercel AI SDK, MCP SDK, OpenTelemetry, ESLint stack, etc.) risks cascading type or runtime changes, so we need a careful, test-backed approach.

## 2. Analysis
- `npm outdated` (run 2025-11-09) reports 16 packages behind: key ones include `@ai-sdk/*` (anthropic/openai/provider-utils), `@modelcontextprotocol/sdk`, `@openrouter/ai-sdk-provider`, most OpenTelemetry exporters/sdks, `ai`, `eslint`, `knip`, and `ollama-ai-provider-v2`.
- Package versions are mostly patch-level bumps (no major semver jumps) except `@ai-sdk/provider-utils` (3.0.15 → 3.0.16) and `ai` (5.0.86 → 5.0.89) which are still within same major versions but may alter types.
- The repo enforces strict lint/build gating plus deterministic harness tests (docs/TESTING.md). Dependency updates can break TypeScript types or ESLint config quickly, so we must expect follow-up fixes.
- Telemetry stack (`@opentelemetry/*`) spans multiple coordinated packages; they all need to move together to 0.208.x / 1.38.0 / 2.2.0 versions to avoid peer mismatch warnings.
- Current lockfile (`package-lock.json`) must stay in sync with `package.json`; npm v10 will rewrite integrity hashes, so changes must be committed.

## 3. Decisions Needed From Costa
1. Confirm expectation: upgrade *all* listed packages to their absolute latest patch/minor versions (per `npm outdated`), even if TypeScript or lint fixes are required.
2. Decide whether we should include transitive dependency auditing (e.g., `npm audit fix`) as part of this task or strictly limit to direct deps.
3. Confirm if Phase 1 deterministic harness (`npm run test:phase1`) should be run post-upgrade, or if lint+build suffices (tests can take longer but provide safety).

## 4. Plan
1. **Inventory & Backup** — Save the current outdated output and inspect `package.json` / `package-lock.json` for direct dependencies; note any pinned versions that might resist `npm update`.
2. **Upgrade Direct Dependencies** — Use targeted `npm install <package>@latest` (grouped logically) to bump each outdated dependency/devDependency, ensuring telemetry packages move together to avoid mixed ranges.
3. **Lockfile Verification** — Review `package-lock.json` changes to ensure only intended packages moved; no inadvertent removal of scripts/configs.
4. **Code & Config Fixes** — Address any TypeScript or lint errors introduced by new versions (e.g., updated types in AI SDK/Otel/ESLint). Keep changes scoped and documented.
5. **Validation** — Run `npm run lint`, `npm run build`, and (if Costa approves) `npm run test:phase1`; capture outputs for the final report.
6. **Documentation Check** — Update docs (e.g., `docs/AI-AGENT-GUIDE.md`, README) if dependency bumps change stated version numbers or capabilities.

## 5. Implied Decisions / Work Items
- Need to inspect release notes (especially AI SDK + MCP SDK) to confirm no new config fields or breaking changes must be reflected in docs.
- Evaluate whether OpenTelemetry patch bump requires env/config updates (e.g., exporter option names) and adjust docs accordingly.
- Ensure `ollama-ai-provider-v2` upgrade doesn’t change CLI flags; update `docs/AI-AGENT-GUIDE.md` if behavior differs.
- Re-run `npm outdated` at the end to prove workspace is fully up-to-date.

## 6. Testing Requirements
- Mandatory: `npm run lint`, `npm run build` (per quality requirements).
- Recommended: `npm run test:phase1` (deterministic harness) if runtime changes occur.
- Spot-check `npm run start -- --version` (or similar) if CLI behavior might change due to dependency updates.

## 7. Documentation Updates Required
- README.md (dependency/version claims) — verify if stated provider/library versions need refresh.
- docs/AI-AGENT-GUIDE.md and docs/SPECS.md — update any references to specific SDK/Otel versions or behavior that changed with new releases.
- CHANGELOG or release notes (if maintained elsewhere) — mention dependency upgrade sweep and any notable impacts.

