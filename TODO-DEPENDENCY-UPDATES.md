# TODO: Dependency Updates (Non-AI Packages)

## TL;DR

- Run `npm outdated` and update all **non-AI** packages.
- **Do not update AI SDK packages** (wait for OpenRouter release).
- **Exception:** update `ai` to `5.0.121` (per user request).
- Review Dependabot security alerts at `https://github.com/netdata/ai-agent/security/dependabot`.
- After changes: run lint/build and all tests except `phase3:tier2`.

## Status (2026-01-14)

- Updated non-AI dependencies and added `overrides` for `hono@^4.11.4`.
- `npm outdated` now shows only AI SDK packages + `ollama-ai-provider-v2` (both intentionally frozen).
- Tests:
  - `npm run lint` ✅
  - `npm run build` ✅
  - `npm run test:phase1` ✅ (Vitest deprecation warning about `test.poolOptions`)
  - `npm run test:phase2` ✅ (first run failed once; rerun passed)
  - `npm run test:phase3:tier1` ❌ (failed twice on `nova/gpt-oss-20b :: multi-turn :: stream-off`: required sub-agent tool `agent__test-agent2` was not invoked)
  - All other tier1 models passed (`nova/minimax-m2.1`, `nova/glm-4.5-air`, `nova/glm-4.6`, `nova/glm-4.7`).

## Analysis (current state)

- Dependencies and devDependencies live in `package.json`.
- AI-related packages in use (to **freeze**):
  - `ai`
  - `@ai-sdk/anthropic`
  - `@ai-sdk/google`
  - `@ai-sdk/openai`
  - `@ai-sdk/openai-compatible`
  - `@ai-sdk/provider-utils`
  - `@openrouter/ai-sdk-provider`
- Lockfile expected (likely `package-lock.json`) and must be updated together with `package.json`.
- Dependabot alerts page returned **404** from unauthenticated fetch (likely requires repo access).
- Dependabot alerts (via `gh api`) show **two open high-severity alerts** for transitive `hono` `< 4.11.4`.
- `hono@4.11.1` is installed via `@modelcontextprotocol/sdk -> @hono/node-server` (peer dep).  
  - Evidence: `npm ls hono` shows `@modelcontextprotocol/sdk@1.25.2 -> @hono/node-server@1.19.7 -> hono@4.11.1`.
  - `@modelcontextprotocol/sdk` latest is `1.25.2` (already on latest).
  - `@hono/node-server` latest is `1.19.9` (peer depends on `hono@^4`).

## Decisions

1. **Update `ollama-ai-provider-v2`?**  
   - **Evidence:** `npm outdated` shows `ollama-ai-provider-v2` from `1.5.5` → `2.0.0` (major).  
   - **Context:** It is AI-related but not part of the AI SDK set we are freezing.  
   - **Options:**  
     1) **Update to 2.0.0 now**  
        - **Pros:** Keeps provider current; may include fixes/features.  
        - **Cons:** Major-version risk; could break tests or runtime behavior.  
     2) **Defer update (treat as AI-related freeze)**  
        - **Pros:** Avoids unexpected breakage; aligns with “wait for AI SDK release” mindset.  
        - **Cons:** Leaves dependency behind.  
   - **Decision (2026-01-14):** Option 2 (defer).

2. **Update `ai` to 5.0.121?**  
   - **Decision (2026-01-14):** Yes (user requested).

3. **Dependabot alerts access:** The alerts page returned 404 without auth.  
   - **Evidence:** `https://github.com/netdata/ai-agent/security/dependabot` returned 404 on fetch.  
   - **Options:**  
     1) **You review and paste the alert list here**  
        - **Pros:** Fast; no access changes needed.  
        - **Cons:** Manual step for you.  
     2) **Provide authenticated access / run in a browser yourself**  
        - **Pros:** Complete alert details; avoids copy/paste.  
        - **Cons:** Requires access setup outside this session.  
   - **Decision (2026-01-14):** Use `gh`/GitHub MCP/git to check alerts (per user).

4. **Address Dependabot alert for `hono` (CVE-2026-22817 / CVE-2026-22818)?**  
   - **Evidence:** Open alerts #8 and #9 (high severity) in Dependabot; vulnerable `hono < 4.11.4`.  
   - **Context:** `hono` is **transitive** via `@modelcontextprotocol/sdk` → `@hono/node-server` (peer).  
   - **Options:**  
     1) **Add `overrides` in `package.json` to force `hono@^4.11.4`**  
        - **Pros:** Fixes vulnerability without changing direct deps.  
        - **Cons:** Overrides may mask incompatibilities; must verify runtime.  
     2) **Add direct dependency `hono@^4.11.4`**  
        - **Pros:** Explicitly pins the safe version; simple.  
        - **Cons:** Still a new direct dep; possible API break if `hono` behavior changes.  
     3) **Defer (accept alert for now)**  
        - **Pros:** No change risk.  
        - **Cons:** Leaves high‑severity vulnerability open.  
   - **Recommendation:** Option 1 (overrides) or 2 (direct dep) if you want explicit pinning.
   - **Decision (2026-01-14):** Option 1 — add `overrides` to force `hono@^4.11.4`.

5. **Phase3 failure handling (gpt-oss-20b multi-turn)?**  
   - **Evidence:** `npm run test:phase3:tier1` failed twice on `nova/gpt-oss-20b :: multi-turn :: stream-off` with `agent__test-agent2` not invoked.  
   - **Context:** All other tier1 tests passed; this blocks the "all tests must pass" requirement.  
   - **Options:**  
     1) **Investigate now** (snapshot/log/tool filtering path).  
        - **Pros:** Finds root cause; stabilizes tests.  
        - **Cons:** More time and code changes.  
     2) **Defer** and accept failure for now.  
        - **Pros:** No extra changes now.  
        - **Cons:** Requirement not met; risk of hidden regression.  
   - **Decision (2026-01-14):** Defer investigation and ignore the failure as long as it is isolated to `nova/gpt-oss-20b`.

## Plan

1. Run `npm outdated` and capture current outdated packages.
2. Filter out AI SDK packages listed above.
3. Update remaining packages to latest stable versions.
4. Check Dependabot security alerts (may require authenticated access).
5. Re-run lint/build/tests (all except `phase3:tier2`).
6. Report changes, risks, and results.

## Implied Decisions

- Treat the AI SDK set above as frozen until OpenRouter release.
- Update both runtime deps and dev deps for non-AI packages.

## Testing Requirements

- `npm run lint`
- `npm run build`
- `npm run test:phase1`
- `npm run test:phase2`
- `npm run test:phase3:tier1`
- **Do not run** `npm run test:phase3:tier2`

## Documentation Updates Required

- None expected for dependency updates unless behavior changes are observed.
