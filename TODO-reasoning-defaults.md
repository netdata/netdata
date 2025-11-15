# TODO – Reasoning Defaults & Overrides

## TL;DR
- Align frontmatter/CLI semantics with Costa’s specification: `reasoning` supports `none|minimal|low|medium|high|default|unset`, with `none`/`unset` forcing reasoning off (maps to `reasoningValue=null`), `default`/missing deferring to fallback defaults, and numeric levels requesting explicit effort.
- Introduce a `--default-reasoning` + `defaults.reasoning` path that only fills in `reasoning` when prompts omit it, while `--override reasoning=<value>` continues to stomp every agent/sub-agent.
- Expand deterministic Phase 1 coverage to exercise all 3×2×4 combinations (frontmatter × default × override) so regressions are caught automatically.

## Analysis
- **Current parsing**: `parseFrontmatter` only recognizes `none|minimal|low|medium|high|default|unset`; however it treats `unset`/`default` identically (effectively “inherit”), so there is no way to explicitly disable reasoning without writing `none`. This contradicts Costa’s request to let `unset` mean “null”.
- **Effective option resolver** (`src/options-resolver.ts`) pulls `reasoning` from (in order) global overrides → CLI → frontmatter → `defaultsForUndefined` (populated by `--default-reasoning` or parent sessions) → `.ai-agent.json defaults`. `normalizeReasoningDirective` treats `default|unset|inherit` as “skip” so `defaultsForUndefined` can run. `reasoningValue` falls back independently via `reasoningTokens`.
- **CLI/config hooks**: `--default-reasoning` already exists in the options registry and we partially parse it, but we only populate `defaultsForUndefined.reasoning` when CLI/config passes a recognized level. There is no way to distinguish “missing frontmatter key” from “explicit `default` string” at load time because the prompt builder always serializes a `reasoning:` line.
- **Tests**: Phase 1 already includes `buildReasoningMatrixScenarios`, but it only emits prompts where `reasoning` is present (`default`, `none`, `high`). It does **not** cover the case where the `reasoning` key is omitted entirely, nor does it verify the semantics for `unset` vs `none` vs `default`. We also lack cases for override directives like `--override reasoning=unset` (inherit) or `--override reasoning=none`.
- **Docs** (`docs/REASONING.md`, `docs/AI-AGENT-GUIDE.md`, README) still claim that `unset` == inherit; they need to be updated to match the agreed semantics once we implement them.

## Decisions (Need Costa’s confirmation)
1. **`unset` meaning** — Confirm that in *frontmatter* (and `.ai-agent.json` defaults) the literal string `unset` should be treated identically to `none`, i.e., we set `reasoningValue=null` and never fall back to defaults.  
   - Suggestion: keep CLI/global override semantics where `reasoning=unset` means “inherit” (no override) so users can remove overrides without stopping reasoning, unless you prefer to align everything.
2. **Missing key vs `default`** — We plan to treat “key absent” and `reasoning: default` the same (both defer to defaults). Please confirm no additional state (e.g., distinguishing true absence) is required.
3. **Default reasoning precedence** — `--default-reasoning none` (or config `defaults.reasoning=none`) will disable reasoning only for prompts that omit it. OK to proceed?
4. **Testing granularity** — Proposed matrix:  
   - Frontmatter selector: missing key, explicit `unset` (→ null), explicit `high`.  
   - Default selector: implicit (unset), explicit `minimal`.  
   - Override selector: absent, `unset` (inherit), `none`, `medium`.  
   This yields 24 cases; confirm this matches expectations or adjust the chosen explicit levels.

## Plan
1. **Frontmatter & config parsing**
   - Update `parseFrontmatter` to treat `reasoning: unset` (and `reasoning: none`) as explicit disable while allowing omission or `default` to mean inherit.
   - Propagate the normalized value so `FrontmatterOptions.reasoning` differentiates “absent” vs “explicit disable”.
2. **Option resolution**
   - Ensure `normalizeReasoningDirective` treats frontmatter-provided `null`/`none` distinctly, while `defaultsForUndefined.reasoning` only runs when frontmatter is absent/inherit.
   - Thread the new default reasoning fallback from CLI/config (`--default-reasoning`, `.ai-agent.json defaults.reasoning`) into `defaultsForUndefined`.
3. **CLI/config plumbing**
   - Keep `--override reasoning=<value>` behavior (stomps everything).  
   - Wire `--default-reasoning` plus config `defaults.reasoning` so they only populate `defaultsForUndefined.reasoning`. Validate inputs (`none|minimal|low|medium|high|default|unset`).
4. **Testing**
   - Rework `buildReasoningMatrixScenarios` to emit prompts without a `reasoning` key when needed, add an explicit `unset` case, and verify `reasoningValue` outcomes via the harness summary map.
   - Add assertions for the new combinations (24 total) covering override permutations.
5. **Docs**
   - Update `docs/REASONING.md`, `docs/AI-AGENT-GUIDE.md`, README (CLI flags section) to document the new semantics and the difference between `--override` vs `--default-reasoning`.
6. **Telemetry/logging**
   - Verify structured logs continue to emit `reasoning=disabled|minimal|...` with the new inheritance path; adjust if necessary.

## Implied Decisions
- Reasoning disablement still sets `reasoningValue=null` so providers know to skip thinking budgets.
- Defaults propagate through parent → child agents via `defaultsForUndefined` only; sub-agents with explicit frontmatter values stay untouched.
- We will treat config `defaults.reasoning=unset` as equivalent to removing the key (inherit) unless instructed otherwise.

## Testing Requirements
- Update Phase 1 deterministic harness (`npm run test:phase1`) with the 24-case matrix; ensure a single scenario failure clearly indicates which combination broke.
- Run `npm run lint` and `npm run build` after changes to satisfy repo quality gates.

## Documentation Updates Required
- `docs/REASONING.md` — describe the revised keyword semantics and interplay between override/default flags.
- `docs/AI-AGENT-GUIDE.md` — update the reasoning section and CLI reference.
- `README.md` / CLI help excerpt — mention `--default-reasoning` and clarify override vs default behavior.
