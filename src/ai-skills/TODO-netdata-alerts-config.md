# TODO - netdata-alerts-config completeness verification

## TL;DR
- Ensure all Netdata alert documentation is correct and aligned with code.
- Apply every finding from the completeness review across all relevant docs (not just the AI skill file).
- Add deterministic alert ordering: load files sorted from disk, build dependency graph, and reorder alert evaluation accordingly.

## Analysis
- Required lines are misstated: file says `warn/crit` required and `lookup/calc` required, but docs/code allow helper alerts with only `lookup`/`calc` and no `warn/crit` (e.g., `cpu_user_mean`). Evidence: `src/health/REFERENCE.md:333-341`, `src/health/REFERENCE.md:1205-1215`.
- Status constants conflict between docs and code; file matches code but REFERENCE is outdated (no `$RAISED`, shifted values). Evidence: `src/health/REFERENCE.md:999-1013` vs `src/health/rrdcalc.h:20-27`.
- `lookup` syntax missing grouping options in parentheses (e.g., `countif(>5)`, `percentile(95)`, `trimmed-mean(5)`), defaults for empty options, and operator set. Evidence: `src/health/health_config.c:190-282`, `src/health/health-config-unittest.c:220-280`.
- `lookup` grouping methods list is incomplete: missing aliases and extra methods (`avg`, `mean`, `cv`, `rsd`, `coefficient-of-variation`, `ses`, `ema`, `ewma`, `des`, `incremental_sum` plus canonical names). Evidence: `src/web/api/queries/query-group-over-time.c:60-340`.
- `lookup` options list incomplete/incorrect: parser accepts `abs`/`absolute_sum`, `match_ids`, `match_names`, and treats `sum` as default. Evidence: `src/health/health_config.c:310-364`.
- Label matching semantics incomplete: parser supports multiple label keys on one line (AND across keys) and multiple values per key (OR), and trims spaces around `=`; commas are not supported. Evidence: `src/health/health_prototypes.c:286-340`, `src/health/health_prototypes.c:347-370`, `src/health/health_prototypes.c:503-522`.
- Pattern matching evaluation is first-match-wins (including negation): the first matching pattern returns positive/negative and stops evaluation. Evidence: `src/libnetdata/simple_pattern/simple_pattern.c:285-299`.
- Line continuation with `\` is supported but not documented in the file. Evidence: `src/health/health_config.c:610-626`.
- Expression evaluation behavior is more specific than the file states: logical operators short-circuit, `nan` is false and `inf` is true in boolean contexts, and final `nan/inf` fails the expression. Evidence: `src/libnetdata/eval/eval-evaluate.c:54-140`, `src/libnetdata/eval/eval-evaluate.c:300-340`.
- Legacy keys `charts:` and `families:` are accepted but ignored by the parser. Evidence: `src/health/health_config.c:818-826`.
- Docs list `$collected_total_raw` as a built-in chart variable, but it is not provided by core alert variables; only per-dimension `_raw` values and custom chart vars exist. Evidence: `src/health/rrdvar.c:159-270`.
- Alert naming rules are incomplete: names are sanitized by `rrdvar_fix_name` and may be auto-renamed; docs also forbid names equal to chart/dimension/family/variable names. Evidence: `src/health/health_config.c:676-684`, `src/libnetdata/sanitizers/chart_id_and_name.c:1-80`, `src/health/REFERENCE.md:368-387`.
- Defaults missing: `to` defaults to `root`, `exec` defaults to `alarm-notify.sh` from `[health]` in netdata.conf. Evidence: `src/health/health.c:72-92`, `src/health/health_notifications.c:404`.
- `class`/`type`/`component` are free-form (not validated) and default to `Unknown` when omitted; file presents a partial fixed list. Evidence: `src/health/health_config.c:730-740`, `src/health/health_json.c:85-88`, `src/health/REFERENCE.md:430-510`.
- Variable resolution is missing key behavior: cross-context references choose the best match by label similarity (highest overlap), and ties depend on iteration order. Evidence: `src/health/health_variable.c:38-111`, `src/health/health_variable.c:340-411`, `src/health/README.md:392-419`.
- Alert-to-alert references timing is misstated: values for all alerts are computed in a first pass, then warn/crit in a second pass; `calc` may see previous-cycle values depending on evaluation order. Evidence: `src/health/health_event_loop.c:400-446`, `src/health/health_variable.c:340-411`.
- `options` line accepts `no-clear` alias (not documented). Evidence: `src/health/health_config.c:96-110`.
- Quoted values are allowed and stripped for several lines; file says “do NOT add quotes”. Evidence: `src/health/health_config.c:730-824`.
- `foreach` appears in commented stock alert examples but is not parsed as a lookup keyword; this is undocumented/ambiguous. Evidence: `src/health/health.d/ml.conf:24-33`, `src/health/health_config.c:300-380`.
- Alert file loading order is non-deterministic (uses `readdir()` without sorting). Evidence: `src/libnetdata/paths/paths.c:234-270`, `:276-319`.
- Alert prototypes are applied per chart first (chart loop, then prototype loop), not by file order. Evidence: `src/health/health_prototypes.c:622-629`, `:693-700`.
- Alert evaluation iterates the rrdcalc index in insertion order, which follows chart insertion and per-chart prototype application. Evidence: `src/health/health_event_loop.c:293-421`, `src/health/rrdcalc.h:104-114`, `src/libnetdata/dictionary/README.md:194-204`.
- Cross-alert references resolve the stored value (`rc->value`) with no dependency graph enforcement. Evidence: `src/health/health_variable.c:146-148`.
- Variable lookup resolves in order: chart dimensions/vars, host vars, then alert names, then cross-context lookups. This makes static “variable name == alert name” dependency detection unreliable. Evidence: `src/health/health_variable.c:60-148`, `:360-379`.
- Proposed ordering alternative: iterative “weight” pass (count references in calc variables, increase weight per iteration, reorder by weight without explicit graph). This needs clear dependency detection rules and cycle/limit behavior. (Proposal from user)

## Decisions
1. Source of truth when docs conflict with code (e.g., status constants):
   - Decision: Option A (align strictly to code behavior).
   - Implication: This file becomes authoritative, even if REFERENCE.md is outdated.
2. `class`/`type`/`component` guidance style:
   - Decision: Option B (full list + free-form/Unknown default).
3. `lookup` method/option aliases:
   - Decision: Option B (canonical + aliases).
4. Alignment rule in the file:
   - Decision: Option A (keep as MUST).
5. `foreach` keyword in lookup examples:
   - Decision: Option C (remove mention entirely).
6. Fix other docs:
   - Decision: Option B (update all alert docs: REFERENCE + guides + examples).
7. Deterministic file loading order:
   - Decision: pending (sort by filename, full path, or stable directory traversal).
8. Dependency graph scope:
   - Decision: pending (per-host only, per-chart only, or global per-host across charts).
9. Dependency resolution timing:
   - Decision: pending (build once at load vs recompute on reload vs incremental updates).
10. Cycle handling:
   - Decision: pending (detect + warn, break cycles, or allow with stale values).
11. Evaluation ordering semantics:
   - Decision: pending (strict topological order for calc/warn/crit, or only for calc).
12. Ordering algorithm approach:
   - Decision: pending (explicit dependency graph vs iterative weight pass).
13. Dependency sources for reordering:
   - Decision: pending (calc only, calc+warn/crit, or all expressions).

## Plan
- Summarize all gaps with evidence, aligned to docs and code.
- Identify all alert-related docs in this repo and audit them for the findings listed above.
- Update every relevant doc to match code behavior (syntax, methods, options, defaults, behavior notes).
- Ensure examples reflect actual parser behavior (labels, lookup options, required lines).
- Define deterministic ordering requirements and dependency semantics.
- Implement sorted loading (deterministic order) in config loader.
- Build dependency graph for alert references and apply topological ordering for evaluation.
- Handle cycles (documented behavior + runtime logging).
- Update alert ordering docs to reflect new deterministic + dependency behavior.

## Implied Decisions
- Treat code as authoritative; avoid doc/code mismatch notes.

## Testing Requirements
- Documentation updates: none.
- Code changes: add tests for sorted load order and dependency evaluation ordering, plus cycle detection behavior.

## Documentation Updates Required
- Update all alert-related docs to match code:
  - `src/ai-skills/skills/netdata-alerts-config.md`
  - `src/health/REFERENCE.md`
  - `src/health/README.md`
  - Any other repo docs that describe alert syntax/behavior (to be located during audit).
- Update alert ordering docs:
  - `src/health/alert-configuration-ordering.md`
  - `src/health/overriding-stock-alerts.md` (ordering caveats)
