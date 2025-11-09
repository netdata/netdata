# TL;DR
- Task: audit `TODO-CHAINS.md` against the latest code to see if deterministic sub-agent chaining already exists.
- Finding: core files (`src/subagent-registry.ts`, `src/frontmatter.ts`) still lack any `next` metadata or chain-runner logic, so the TODO remains fully relevant.

# Analysis
- `src/subagent-registry.ts:25-38` defines `PreloadedSubAgent` / `ChildInfo` with no `next` data, confirming the registry cannot express deterministic successors today.
- `src/subagent-registry.ts:132-329` shows `execute()` validating parameters, running one child agent, and immediately returning its output/accounting with no follow-up execution, so there is no chaining layer yet.
- The registry still enforces a required `reason` field for every call via schema augmentation plus runtime validation (`src/subagent-registry.ts:39-222`), matching the TODO’s concern about deciding what reason text chained runs should use.
- `src/frontmatter.ts:1-142` rejects unknown top-level keys, and `getFrontmatterAllowedKeys()` contains no allowance for `next`, so prompt files cannot declare downstream stages yet.
- Project-wide ripgrep returns no references to `chainTrigger`, `next` metadata, or similar instrumentation outside of `TODO-CHAINS.md`, indicating no partial implementation exists elsewhere.

# Decisions
1. Do we want Phase 1 deterministic chaining to be strictly linear (only the first `next` entry is honored), or should we accept arrays for future branching even if we execute the first item only?
2. Should chained invocations inject a fixed `reason` (e.g., `Chained run from <parent>`) or forward the upstream payload when available?
3. How should upstream failures be surfaced—bubble the downstream error verbatim, wrap it in a structured diagnostic object, or fall back to the parent agent for remediation?
4. When upstream and downstream output formats differ, do we normalize to the downstream schema or always honor the child’s `expectedOutput` regardless of the parent’s defaults?

# Plan
1. Add `next` (string or array) to frontmatter parsing / validation and persist it in the loaded agent metadata.
2. Extend `SubAgentRegistry` preload to resolve `next` entries to actual children, run cycle detection, and stash downstream schema + format metadata.
3. Introduce a deterministic chain runner inside `execute()` that:
   - Aligns upstream result with the downstream input format (JSON parse vs. text wrapping).
   - Injects the approved `reason`.
   - Executes each downstream stage sequentially while aggregating accounting, opTree, and trace info.
4. Augment instrumentation (logs + accounting extras) with `chainTrigger: 'next'` or similar markers, then add Phase 1 harness scenarios for happy-path, schema-mismatch, and downstream-failure cases.

# Implied Decisions
- Chaining remains opt-in per agent; files without `next` continue to rely on the LLM for orchestration.
- Cycle prevention must reuse the current `ancestors` list so a chain cannot re-enter an earlier stage.
- Any automatic JSON parsing must emit concrete Ajv validation errors when the downstream schema rejects the upstream payload.

# Testing requirements
- `npm run lint`
- `npm run build`
- Phase 1 deterministic harness coverage for chained success, invalid payload, and downstream failure.
- Manual smoke via `./run.sh` once chain logic exists.

# Documentation updates required
- README.md, docs/DESIGN.md, docs/SPECS.md to describe deterministic chaining behavior once implemented.
- CLI/frontmatter templates (help output + docs/AI-AGENT-GUIDE.md) to document the new `next` metadata and chain semantics.
