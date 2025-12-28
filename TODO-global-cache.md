# TODO-global-cache

## TL;DR
Add opt-in global response caching for agents and tools. Cache entries are keyed by stable SHA256 of request payload + agent/tool identity, stored in modular backends (SQLite default at `~/.ai-agent/cache.db`, optional Redis). TTL is configured per agent and per tool using a unified `cache` setting.

## Analysis
### Facts (from code/docs)
- **Sessions are designed to have zero shared state.** Any global cache is a deliberate design change and must be documented/opt‑in.
  - Evidence: `docs/DESIGN.md:3-6`, `docs/MULTI-AGENT.md:7-31`.
- **Agent runs are invoked from headends/CLI via `session.run()`**, so agent‑response caching can be placed at headend/registry boundaries before `run()`.
  - Evidence: `src/headends/rest-headend.ts:309-321`, `src/cli.ts:1829-1836`, `src/agent-registry.ts:93-148`.
- **All tool calls flow through `ToolsOrchestrator.executeWithManagementInternal(...)`**, so tool caching can live there or in provider wrappers.
  - Evidence: `src/tools/tools.ts:232-384`, `src/session-tool-executor.ts:370-396`.
- **Sub‑agent execution is centralized in `SubAgentRegistry.execute(...)`**, making it a natural place for per‑agent caching (if desired).
  - Evidence: `src/subagent-registry.ts:161-230`.
- **Existing `caching` option is only Anthropic cache control**, not response caching.
  - Evidence: `src/options-registry.ts:239-247`, `docs/AI-AGENT-GUIDE.md:70-82`.

### Risks / Considerations
- **Isolation contract break:** global cache contradicts “no shared state.” Must be explicit and documented.
- **Cache key correctness:** key must be stable and include all semantics that affect output; otherwise incorrect reuse.
- **Staleness:** web/search tools can become outdated quickly; TTLs need defaults and overrides.
- **Side‑effect tools:** caching must be opt‑in and probably deny‑by‑default.
- **Multi‑tenant leakage:** cache keys may need tenant/user scoping (Slack user/thread, API key, etc.).

## Decisions (resolved by Costa)
1) **Cache backend**
   - Decision: **Option A** — SQLite at `~/.ai-agent/cache.db` (default) + optional Redis.

2) **Global cache config shape**
   - Decision: internal defaults (no `.ai-agent.json` required) — SQLite at `~/.ai-agent/cache.db`, `maxEntries=5000`.
   - `.ai-agent.json` is optional and only used to override backend/path/limits.
   - Cache is **opt-in via frontmatter/CLI `cache`**; absent TTL means no caching.
   - Decision: if the SQLite backend module is unavailable, caching is **disabled** unless a Redis backend is configured.

3) **TTL units/format (agents + tools)**
   - Decision: unified syntax for agent frontmatter and tool config:
     - `cache: off` (disable)
     - `cache: <milliseconds>` (numeric, same as existing convention)
     - `cache: <N.Nu>` where `u ∈ { ms, s, m, h, d, w, mo, y }`
   - Decision detail: `mo = 30d`, `y = 365d` (fixed days, not calendar-aware).

4) **Agent identity for cache keys**
   - Decision: **Agent registry computes a unique agent hash at load time for all agents**.
   - Hash inputs: **prompt content (after include expansion, before variable expansion) + full agent config**.
   - **Do NOT include prompt path or agent name**; identity is content/config only.

5) **Tool identity for cache keys**
   - Decision: include namespaced tool identity (server/namespace + tool name) plus request payload.
   - Tool cache config shape: `mcpServers.<name>.cache` (server default) + `toolsCache.<tool>=<ttl>` (per tool override).

6) **Scope/partitioning**
   - Decision: **fully global cache** (admins opt-in).

7) **Cache policy for failures**
   - Decision: **cache only successful results** (no negative caching).

8) **Cleanup policy**
   - Decision: **no writes on read**; cleanup **only on write**.
   - Decision: always delete **expired entries** on write.
   - Decision: enforce **max entries** by deleting **oldest rows** after write (count-based only).

9) **Agent cache key request payload**
   - Decision: **do not include conversation history**; cache key includes **agent hash + user prompt + expected format/schema**.

9) **Compression + metadata**
   - Decision: **gzip** compress payloads.
   - Decision: store **compressed size** in metadata (for fast accounting and future cleanup heuristics).

10) **Logging**
   - Decision: **log cache hits only** (no miss logs).
   - Decision: log **current identity** and **stored identity** on hits (agent/tool names + hash/namespace).
   - Decision: log **age** of cached entry on hit.

11) **Integration footprint**
   - Decision: exactly **two hooks per path** (lookup + write) for agents and tools; avoid refactors.

12) **SQLite dependency packaging**
   - Decision: move `better-sqlite3` to **optionalDependencies** so installs can succeed without native build toolchains; cache disables itself when sqlite is missing unless Redis is configured.

13) **Redis error logging**
   - Decision: **log every Redis error** (no rate limiting; accept one log per attempt if Redis is down).

14) **Cache config validation**
   - Decision: **strict parsing** of cache config (parse numeric strings and throw on invalid types).

15) **Cache payload validation**
   - Decision: **tighten cached payload validation** and treat malformed payloads as cache misses.

16) **Sub-agent reason handling**
   - Decision: **reason must not be part of cache keys** and **sub-agents must not see it**; keep reason for user-facing metadata only.

## Plan
1) **Config + schema**
   - Add `cache` to frontmatter + CLI options for agents (override precedence).
   - Add `cache` + `toolsCache` to `.ai-agent.json` tool config.
2) **Cache backend module**
   - Define storage interface; implement SQLite + Redis adapters.
   - Implement gzip compression + metadata storage.
3) **Agent‑response cache integration**
   - Compute agent hash at registry load time.
   - Add lookup + write hooks around `session.run()`.
4) **Tool‑response cache integration**
   - Add lookup + write hooks inside tool execution path.
5) **Cleanup + logging**
   - Cleanup on write (expired + overflow) and log hits with metadata + age.
6) **Tests** (Phase 1 harness)
   - Add scenarios for agent cache hit, miss, TTL expiry, and cache-disabled.
   - Add scenarios for tool cache hit, miss, TTL expiry, and per-tool override precedence.
7) **Docs**
   - Update `docs/AI-AGENT-GUIDE.md`, `docs/SPECS.md`, `docs/AI-AGENT-INTERNAL-API.md`, and related specs.

## Implied Decisions (to confirm)
- Cache keys use **SHA256** of stable JSON.
- Agent cache key includes **request payload** (user prompt + response format) + agent hash.
- Tool cache key includes **tool request payload** (tool name + parameters) + namespaced identity.
- Numeric TTL values are interpreted as **milliseconds** (existing convention).
- Cache stores **final report output + conversation** (if needed for return contract).

## Decisions Required (new)
- None.

## Testing Requirements
- `npm run lint`
- `npm run build`
- Add deterministic Phase 1 harness coverage for agent/tool cache hit/miss/expiry.

## Documentation Updates Required
- `docs/AI-AGENT-GUIDE.md`
- `docs/SPECS.md`
- `docs/AI-AGENT-INTERNAL-API.md`
- `docs/DESIGN.md` / `docs/MULTI-AGENT.md` to note the intentional isolation exception.
