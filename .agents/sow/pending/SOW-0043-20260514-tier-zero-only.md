# SOW-0043 - PromQL is tier-0-only by design

## Status

Status: open

Sub-state: drafted after the conversation that closed SOW-0041
exposed the tier-selection feature as a Netdata-specific affordance
that does not belong on the PromQL endpoint. Reverts SOW-0041's
tier infrastructure and closes SOW-0042 (engine-driven tier
planning + splicing) as not-pursued.

## Requirements

### Purpose

Remove the tier-selection feature from the PromQL endpoint. The
PromQL evaluator always reads from tier 0, with no parameter, no
auto-selection, no plan-aware downgrading. This matches Prometheus's
semantic contract exactly — Prometheus has no tiers, and tier 0 is
structurally the same thing as Prometheus's raw TSDB samples — and
eliminates a class of silent-correctness-drift failure modes that
the auto-select path was at risk of producing.

The Netdata-specific multi-tier query capability remains fully
available through `/api/v1/data` and `/api/v2/data`. The PromQL
endpoint is the prometheus-compatible surface; the other endpoints
are the Netdata-native surface. Different APIs, different
capabilities.

### User Request

Following an investigation into how `/api/v1/data` handles tier
planning, splicing, and grouping (which surfaced architectural
complexity not appropriate to duplicate in the shim), and an
investigation into how Prometheus handles heterogeneous
`update_every` (which surfaced that Prometheus has no tier model
at all), the user wrote:

> I'm leaning towards always using tier 0 no matter what. No option
> available to the user. Same semantics as prometheus.

And after confirming that tier 0 + the 5-minute staleness window
handles heterogeneous `update_every` correctly:

> Ok, lets draft the SOW.

### Assistant Understanding

Facts:

- Prometheus stores raw samples per series at each series'
  collection cadence. There are no tiers, no bucket aggregates, no
  group_factor.
- Query evaluation at grid point `t` per series: find the most
  recent sample with `timestamp <= t` and `timestamp > t -
  staleness_window`. Default staleness is 5 minutes.
- The grid is synthetic — the user picks `step`. Grid points have
  no relationship to sample timestamps.
- Netdata's tier 0 stores every collected sample at its actual
  timestamp at the metric's `update_every`. Structurally identical
  to Prometheus's raw TSDB samples.
- Higher tiers (1, 2, …) store bucket aggregates over
  `update_every × tier_grouping` seconds. A "sample" at tier 1+
  is not an observation; it is a summary statistic
  (`{min, max, sum, count}`) over a time window.
- The PromQL evaluator's `eval/select.rs::eval_vector_select`
  already implements the prometheus contract: scan-once
  two-pointer lookback over the fetched samples, NaN at grid
  points whose lookback window contains nothing, default
  lookback = 5 min.
- Heterogeneous `update_every` works correctly at tier 0 because
  every series's native samples are at most `update_every` old at
  any grid point. As long as `update_every < lookback_ms`, every
  grid point emits a value. This is the Prometheus correctness
  contract verbatim.
- SOW-0041 introduced: `tier_hint` FFI parameter, per-query
  auto-select via the v2 weight function (`shim_pick_tier`,
  `shim_tier_weight`), explicit `?tier=N` URL parameter, and a
  plan-level forcing pass (`Plan::requires_tier_zero`) that
  downgrades to tier 0 when distribution-sensitive operators
  appear. This SOW removes all of that.
- SOW-0042 drafted an engine-driven per-metric tier planner with
  splicing. The complexity-and-correctness-risk analysis
  concluded against that path. SOW-0042 has not been started; it
  is being closed as not-pursued.

Inferences:

- The right "tier choice" for PromQL is no choice. Auto-selection
  policies are themselves a surface for silent correctness drift
  (a future calibration of the weight function could flip auto
  behavior); removing the surface removes the risk.
- An explicit `?tier=N` parameter is also surface area. A user
  who passes `?tier=1` to a query containing `rate(M[10s])` gets
  mathematically broken output if `M`'s tier-1 `update_every` is
  60s. Plan-level forcing catches some of this, not all. The
  product position "PromQL endpoint = prometheus semantics" is
  clearer if there is no override.
- Users who need long-history queries reach for the
  `/api/v1/data` or `/api/v2/data` endpoints, which already
  expose `?tier=N` and the full tier-aware query engine. PromQL
  is not the only API on the agent.

Unknowns:

- None. The decision is product-shaped, not investigative.

### Acceptance Criteria

- All 195/195 unit tests pass after the revert.
- Compliance corpus: same 545/255/212 split (the harness uses
  `MemBackend`; no tier semantics there to begin with).
- Smoke harness: 117/117 pass after any tier-specific checks are
  removed from the harness.
- The C shim's `nd_pds_resolve` signature returns to its
  pre-SOW-0041 shape (no `points_wanted`, no `tier_hint`).
- `nd_pds_open_samples` reads from tier 0 unconditionally; the
  `tier` field on `nd_pds_query` is removed.
- `shim_pick_tier`, `shim_tier_weight`, `nd_pds_chosen_tier` are
  deleted.
- The Rust `EvalContext` no longer has a `tier_hint` field.
- `Plan::requires_tier_zero` and `FuncKind::requires_tier_zero`
  are deleted.
- The FFI entry points `nd_promql_query_instant` and
  `nd_promql_query_range` lose their `tier` parameter.
- The HTTP layer's `parse_tier_hint` is deleted; the PromQL
  endpoints no longer parse `?tier=N` (the parameter is silently
  ignored if a client sends it).
- A new regression test covers heterogeneous `update_every`: a
  synthetic `MemBackend` carrying two series with different
  sample spacing, queried at a step that crosses the cadence
  boundary, produces dense output for both.
- The SOW records a permanent rationale section explaining why
  the tier-selection feature was removed; future contributors who
  read it understand the prometheus-semantics argument and do not
  re-add the feature thinking it is missing.

## Analysis

Sources checked:

- The four SOW-0041 close-conversation messages discussing
  per-query vs per-metric, splicing, and the v2 weight function.
- The SOW-0041 file in `done/` and the SOW-0042 file in
  `pending/`.
- The Prometheus query-evaluation reference (the `engine.go`
  staleness logic) and its documented 5-minute default.
- `src/crates/netdata_promql/src/eval/select.rs::eval_vector_select`
  — confirms the existing tier-0 path already implements the
  prometheus contract.
- `src/database/contexts/promql-data-source.{c,h}` to identify the
  exact symbols to remove.

Current state:

- The shim, the Rust evaluator, and the HTTP layer all carry the
  SOW-0041 tier infrastructure.
- SOW-0042 sits in `pending/` documenting a path that is more
  complex still.

Risks:

- The revert is mechanical, but it touches 17 files (the same
  set SOW-0041 modified). Care is needed to remove every
  reference cleanly without leaving dead imports or stale
  comments.
- A user who has already wired `?tier=N` into their dashboards
  (unlikely — the SOW-0041 change is local-only and never
  reached `origin/master`) would see the parameter silently
  ignored. Acceptable because the branch is unpushed.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The PromQL endpoint should expose Prometheus semantics. Prometheus
has no tiers. Tier 0 in Netdata is structurally identical to
Prometheus's raw TSDB samples. The right answer is to use tier 0
unconditionally and remove the surface area that pretends
PromQL-on-Netdata is different. The complexity that SOW-0041
introduced (and that SOW-0042 would have extended) is genuine
Netdata-specific functionality that belongs on the Netdata-native
APIs (`/api/v1/data`, `/api/v2/data`), not on the prometheus-
compatible endpoint.

Evidence reviewed:

- Prometheus's `engine.go` staleness logic confirms: raw samples,
  5-minute default staleness, per-grid-point lookback.
- The Netdata `eval/select.rs` evaluator already implements this
  contract verbatim at tier 0.
- The SOW-0041 conversation explicitly surfaced that auto-select
  was at risk of silent correctness drift (heterogeneous
  `update_every` × tier-1 bucket size → half-NaN charts) without
  a clear way to test for it.
- No current user of the PromQL endpoint is asking for tier
  selection; the explicit-`?tier=N` infrastructure has zero
  call-sites in the wild.

Affected contracts and surfaces:

- `src/database/contexts/promql-data-source.c` — shim. Revert.
- `src/database/contexts/promql-data-source.h` — shim header.
  Revert.
- `src/crates/netdata_promql/src/storage/{backend,ffi_backend,
  mem_backend,query}.rs` — `Backend::resolve` and
  `NdQuery::resolve` lose `points_wanted` and `tier_hint`.
- `src/crates/netdata_promql/src/eval/context.rs` —
  `tier_hint: i32` field removed; `Default` updated.
- `src/crates/netdata_promql/src/eval/{select,fused,subquery}.rs` —
  call-site reverts.
- `src/crates/netdata_promql/src/plan/ir.rs` —
  `Plan::requires_tier_zero` and `FuncKind::requires_tier_zero`
  deleted.
- `src/crates/netdata_promql/src/lib.rs` — FFI entry points lose
  the `tier` parameter; `run_*_inner` no longer threads
  `effective_tier_hint`.
- `src/crates/netdata_promql/src/discovery.rs` — discovery resolve
  calls revert to 5-arg form.
- `src/crates/netdata_promql/src/testing.rs` — public re-export
  and `EvalContext` literal revert.
- `src/web/api/v3/api_v3_promql.c` — `parse_tier_hint` deleted;
  `handle_instant` and `handle_range` revert to pre-SOW-0041
  signatures.
- `tests/promql-smoke/run-smoke.sh` — any tier-specific checks
  removed if present (verify).
- HTTP-level URL parsing — silently drops the `?tier=N` parameter.

Existing patterns to reuse:

- The git history. SOW-0041's commit (`99e6ac6f97`) is in `done/`.
  The revert is mostly the inverse diff of that commit; this SOW
  produces a *clean* revert (not `git revert`) so the resulting
  commit has a forward-looking explanation rather than a
  backward-looking one.

Risk and blast radius:

- Numerical-output risk: zero. Tier 0 was the default behavior
  before SOW-0041 and remains the only behavior after this SOW.
  No query that previously worked stops working.
- The `?tier=N` URL parameter becomes silently inert. Acceptable
  per the no-current-users argument.
- The branch is local-only (`pql` branch, never pushed). The
  revert is invisible to anyone outside this working tree.

Sensitive data handling plan:

- This SOW touches an internal Rust+C revert. No secrets,
  credentials, or customer data. The new regression test uses
  `MemBackend` with synthetic series; no real-host data is
  involved.

Implementation plan:

1. **C shim revert** (`promql-data-source.{c,h}`):
   - Remove `shim_pick_tier`, `shim_tier_weight`,
     `nd_pds_chosen_tier`.
   - Remove the `tier` field from `nd_pds_query`.
   - Remove `points_wanted` and `tier_hint` from
     `nd_pds_resolve`'s signature.
   - Restore `nd_pds_open_samples`' hard-coded `tier = 0` with
     a forward-looking comment ("Tier 0 by design; see
     SOW-0043.") replacing the prior placeholder comment.
2. **Rust storage revert** (`storage/{backend,ffi_backend,
   mem_backend,query}.rs`):
   - `Backend::resolve` drops both new parameters.
   - All impls and tests follow.
3. **Rust EvalContext revert** (`eval/context.rs`):
   - Remove `tier_hint` field and `Default` initializer.
4. **Rust plan-walker delete** (`plan/ir.rs`):
   - Delete `Plan::requires_tier_zero` and
     `FuncKind::requires_tier_zero`.
5. **Rust call-site reverts** (`eval/{select,fused,subquery}.rs`,
   `discovery.rs`, `testing.rs`).
6. **Rust FFI revert** (`lib.rs`):
   - `nd_promql_query_instant` and `nd_promql_query_range` lose
     the `tier_hint` parameter.
   - `run_*_inner` no longer computes or threads
     `effective_tier_hint`.
7. **HTTP layer revert** (`api_v3_promql.c`):
   - Delete `parse_tier_hint`.
   - `handle_instant` and `handle_range` drop the `tier_str`
     extraction and pass-through.
8. **New regression test**: in
   `eval/select.rs` (mod tests at the bottom), add a unit test
   that builds a `MemBackend` with two series of different
   sample spacing (e.g., one sample every 1000 ms and one every
   10000 ms over an hour), queries them via a vector selector
   at 15 s step, and asserts that both produce dense output
   (no NaN gaps) across every grid point.
9. **SOW disposal**:
   - Move SOW-0042 from `pending/` to `done/` with
     `Status: closed` and a "superseded by SOW-0043" outcome
     note recorded.
   - Add a one-line "superseded by SOW-0043" addendum to
     SOW-0041's Outcome (it stays in `done/` with
     `Status: completed` because the work it did was real even
     if the policy decision is being reversed).
10. **Regression gate**: cargo test, compliance, smoke.
11. **Documentation**: no external doc was added for the
    `?tier=N` parameter (it was deferred in SOW-0041's followups).
    Nothing to revert there.
12. **Close**: single commit covering the revert, the new test,
    the SOW lifecycle changes.

Validation plan:

- Unit tests: 195/195 (note: a few tests will drop due to deleted
  features; the new heterogeneous-`update_every` test offsets).
- Compliance corpus: 545/255/212 unchanged.
- Smoke harness: 117/117 (after removing any tier-related
  smoke checks if present).
- New unit test passes.
- `git diff` against the pre-SOW-0041 state of each file
  confirms only the tier infrastructure was removed; no
  unrelated drift.

Artifact impact plan:

- AGENTS.md: no update needed.
- Runtime project skills: no update needed.
- Specs: add a brief spec note at `.agents/sow/specs/` titled
  "promql-endpoint-uses-tier-0" recording the decision and its
  rationale, so future contributors who read the specs before
  touching the PromQL endpoint see this constraint clearly. One
  page, mostly the prometheus-semantics argument.
- End-user/operator docs: no public docs reference the
  `?tier=N` parameter (deferred in SOW-0041); nothing to update.
- End-user/operator skills: not affected.
- SOW lifecycle: SOW-0042 closed → `done/`; SOW-0041 Outcome
  amended with one line; SOW-0043 completed → `done/`.

Open-source reference evidence:

- The Prometheus query engine
  (https://github.com/prometheus/prometheus/blob/main/promql/engine.go)
  for the staleness handling. No code is copied; the reference
  validates that tier 0's vector-selector behavior already
  matches the documented semantics.

Open decisions:

None. The decision is the SOW.

## Implications And Decisions

No user decisions pending. The conversation that produced this
SOW (the four-message exchange in the chat history) is the
decision record. Recorded here so a future reader sees it as
deliberate.

## Plan

See the 12-step Implementation plan above.

## Execution Log

### 2026-05-14

- SOW drafted following the conversation chain that concluded
  PromQL should be tier-0-only by design.

## Validation

Pending.

## Outcome

Pending.

## Lessons Extracted

These will be expanded at close, but the load-bearing points
this SOW will leave in the project's permanent record are:

1. **The PromQL endpoint is the prometheus-compatible surface.**
   Its job is to behave like Prometheus, not to expose
   Netdata's storage internals. Tier 0 is structurally
   Prometheus's TSDB; that is the semantic contract.
2. **Heterogeneous `update_every` is a non-issue at tier 0.**
   Each series's native samples are dense relative to the
   default 5-minute lookback. The Prometheus contract
   ("scrape interval < staleness window") holds for any
   sensibly configured Netdata collector.
3. **Auto-selection policies for storage tiers are themselves a
   correctness surface.** Any rule that picks among tiers based
   on query properties (range, step, points_wanted) can produce
   silently wrong output when the rule is calibrated for a
   different evaluation model. Removing the rule removes the
   correctness obligation.
4. **The right place for tier-aware queries is the
   Netdata-native API**, not the prometheus-compatible API.
   The product split is clean: PromQL → prometheus semantics;
   v2 data → Netdata semantics including tiering. Users who
   need long-history queries on tiered data reach for v2.
5. **A SOW can record a deliberate non-decision.** The work
   SOW-0041 did (FFI plumbing, plan walker, weight-function
   port) was real engineering. The decision to remove it is
   not a regression on SOW-0041; it is a product-shape choice
   that supersedes SOW-0041's policy. The SOW system's
   `closed` status applies to SOW-0042; SOW-0041 remains
   `completed` because the engineering completed cleanly.

## Followup

- None active. SOW-0042 closes alongside this SOW.

## Regression Log

None yet.
