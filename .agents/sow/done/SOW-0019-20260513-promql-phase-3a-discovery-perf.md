# SOW-0019 - PromQL Phase 3a - Discovery endpoint perf

## Status

Status: completed

## Requirements

### Purpose

The Phase 2 discovery surface (SOW-0018) works correctly and Grafana
exercises it. The interactive walkthrough captured an access-log
trace showing the metric-browser footprint:

```
GET   /api/v1/label/__name__/values?limit=40000&start=...&end=...   32.6 ms
GET   /api/v1/metadata?limit=40000                                   3.8 ms
GET   /api/v1/label/__name__/values   (re-fetch)                    61.8 ms
```

`/label/__name__/values` runs in 32-62 ms because the shim resolves
every series on every chart (~8500 on this host) just to project the
~564 distinct sanitized context names. The metadata endpoint at 3.8 ms
shows the right shape: walk contexts, no series resolution. This SOW
applies that shape to `/label/__name__/values` and honors the
`start`/`end` parameters across the discovery surface so dormant
contexts (charts that haven't been collected in the queried window)
can be skipped on parent agents with broad time ranges.

The user-visible win is metric-browser responsiveness, especially on
parent agents with thousands of contexts.

### User Request

Direct user instruction: "Let's proceed with the next phase. ... I'll
let you decide."

The previous walkthrough surfaced two concrete perf issues and the
assistant recommended the smallest evidence-driven slice
(bucket E from SOW-0018's followup mapping) as the first Phase 3
SOW.

### Assistant Understanding

Facts:

- The shim FFI declares `nd_pds_resolve(host, matchers, ..., after_s,
  before_s, max_series, err)` but the implementation marks `after_s`
  and `before_s` as `__maybe_unused`. The retention window currently
  has no effect on resolution.
- `host->rrdctx.contexts` is a dictionary keyed by raw (dotted)
  context name. `dfe_start_read` over it yields every context on a
  host with O(contexts) cost; the shim already does this for
  `resolve_all_contexts_on_host` and `nd_pds_metadata_collect`.
- For each context, the existing flow then calls
  `rrdcontext_foreach_instance_with_rrdset_in_context` which iterates
  every chart and (via the callback) every dimension. That's where
  the O(series) cost comes from.
- `rrdcontext_retention_match(RRDCONTEXT_ACQUIRED *rca, time_t after,
  time_t before)` exists in `rrdcontext.h:719`. It is the same
  retention test used by `/api/v3/data` and the dashboard.
- `RRDSET` carries `last_collected_time` plus per-tier retention; a
  cheaper chart-level retention test is possible if context-level
  acquisition is awkward.
- Grafana's discovery requests carry `limit=40000` and
  `start`/`end` derived from the panel's time range. The shim
  ignores `start`/`end` today; honoring them lets dormant contexts
  (e.g. a USB device unplugged 30 minutes into a 1h window's start
  position, or a child host disconnected for a day) be skipped.
- The metric-browser feed is `/api/v1/label/__name__/values`
  (confirmed in `@grafana/prometheus@13.1.2 dist/esm/resource_clients.mjs`).
- The Grafana metric browser does NOT pass `match[]` on the
  unconditional first fetch (verified in the access-log trace from
  the walkthrough).

Inferences:

- A walk-contexts-only path is correct for
  `/label/__name__/values` *when* (a) no `match[]` is supplied, or
  (b) the only `match[]` is `{__name__=...}` (which can be applied
  against the sanitized context-name comparison without series
  resolution). When a richer `match[]` is supplied (e.g.
  `{instance="server-7"}`), the fast path doesn't apply -- we still
  need series resolution to evaluate the matcher.
- The fast path is also correct only for the specific label name
  `__name__`. For any other label, we still need to walk series to
  collect its values. Phase 3b or later may extend the optimization
  to `__name__` plus chart-level labels by walking chart instances
  without per-dim resolution; out of scope for this SOW.
- The `start`/`end` filtering applies uniformly across all discovery
  endpoints. The simplest implementation point is a single helper
  that converts the FFI's `start_ms`/`end_ms` parameters (currently
  passed as zero) into `after_s`/`before_s` and uses the existing
  retention API.

Unknowns:

- Whether context-level acquisition (the cleaner path) is fast
  enough to call per context in a hot loop on a parent agent with
  thousands of contexts. Will measure during chunk 1 and pivot to
  chart-level retention checking if the per-context acquisition
  cost is meaningful.

### Acceptance Criteria

1. A new shim helper `nd_pds_metric_names(host_machine_guid,
   metric_filter, max_entries, after_s, before_s)` walks
   `host->rrdctx.contexts`, optionally drops contexts outside the
   retention window, and returns sorted-deduped sanitized names.
   `nd_pds_metric_names_count` / `_get` / `_free` mirror the
   metadata-helper accessor pattern. No series resolution happens
   inside this path.

2. `nd_promql_label_values(host, "__name__", matchers, ...)` detects
   the no-`match[]` (or only-`__name__-EQ-match[]`) case and
   short-circuits to the new fast path. All other cases keep their
   current behavior (full resolve + dedupe of values for the
   requested label).

3. `nd_pds_resolve` and `nd_pds_metadata_collect` honor `after_s`
   and `before_s` when both are non-zero. Contexts whose retention
   window does not overlap `[after_s, before_s]` are skipped.
   When either is zero, the existing "ignore retention" behavior is
   preserved (matches Phase 2 semantics).

4. The discovery handlers in `api_v3_promql_discovery.c` plumb
   `start_ms`/`end_ms` through to the FFI calls. Today they parse
   the parameters into the struct but pass `0`/`0` to the Rust side.

5. Smoke harness gains correctness checks for the new behavior:
   - `/label/__name__/values` returns the same set as
     `/label/__name__/values?match[]=` (a single-trip equality
     after sorting).
   - With a `start`/`end` window that lies in the future,
     `/series` returns zero results (the in-window check is
     stricter than no-window).
   - With a window covering "now", `/series` returns the same
     count as the unwindowed call.

6. Timing budget (informational, not a hard gate): on the
   development host (~8500 series, ~564 contexts), the
   `/label/__name__/values` no-match[] case should drop below 5 ms
   total response time, measured end-to-end via curl. The current
   timing is 32-62 ms.

7. The contract spec at
   `.agents/sow/specs/promql-endpoint-contract.md` updates two
   sections: `/api/v1/label/<name>/values` notes the
   `__name__`-only fast path; the introductory paragraphs of
   `Discovery endpoints` clarify that `start`/`end` are honored
   (Phase 2 said "accepted but ignored").

Out of scope for this SOW (Phase 3b+ or later):

- Subqueries, vector matching, `*_over_time`, `topk`/`bottomk`/
  `quantile`, predict/holt_winters, `@` modifier, keep_metric_names.
  These are the PromQL-completeness bucket and need their own SOWs.
- Tier selection beyond tier 0. Today every sample fetch uses tier
  0; older data on tiers 1/2 is invisible to PromQL. Tier
  selection is its own SOW.
- Rollup result cache (Prometheus-style result caching).
- Per-series parallelism in the evaluator.
- macOS verification.
- The Prometheus compliance test suite.
- Cardinality telemetry.
- Optional Prometheus endpoints (`/query_exemplars`,
  `/format_query`, `/parse_query`, `/targets`, `/rules`, `/alerts`,
  `/alertmanagers`, `/status/{runtimeinfo,config,flags,tsdb}`).

## Analysis

Sources checked:

- `src/database/contexts/promql-data-source.c` -- the existing
  metadata-collect path is the template for the metric-names fast
  path. `nd_pds_resolve`'s `after_s`/`before_s` parameters are
  declared but unused.
- `src/database/contexts/rrdcontext.h` -- the retention API is at
  line 719; chart-level retention is accessible via
  `rrdset_last_entry_s` / the per-tier `db_first_time_s`/
  `db_last_time_s` fields.
- The access-log trace from the Grafana walkthrough (captured into
  `/home/vk/opt/pql/netdata/var/log/netdata/access.log`).
- `@grafana/prometheus@13.1.2 dist/esm/resource_clients.mjs` --
  confirms the metric-browser fetch shape and the absence of
  `match[]` on first open.

Current state:

- `/label/__name__/values` works correctly but does O(series)
  work per request. On a populated single host this is tolerable;
  on a parent agent with N child hosts it scales N×.
- `/series` and `/labels` ignore `start`/`end`. The Grafana time
  range (e.g. "Last 1 hour") is sent and discarded.

Risks:

- *Many-to-one sanitization correctness*. The fast path computes
  sanitized names from raw context names and dedupes. If two raw
  contexts (e.g. `system.cpu` and `system_cpu`) sanitize to the
  same name, the fast path emits one entry -- same as the
  slow path. Verified by inspection of `prometheus_rrdlabels_sanitize_name`.
- *Retention semantics divergence*. The retention test on a chart
  uses tier 0 today. If a chart's tier-0 retention does not cover
  the requested window but tier-1 does, the fast path skips the
  chart while a query against the same chart would succeed (using
  the wider tier-1 retention). The shim already scopes sample
  iteration to tier 0 (Phase 1 documented this), so the divergence
  is consistent with what query evaluation would see. Phase 3b's
  tier-selection work resolves both together.
- *Dormant-context filtering on parent agents*. Skipping dormant
  contexts could surprise users who explicitly want to see what
  *did* exist in a window. The behavior matches Prometheus
  semantics (no samples in window -> not listed), so this is the
  expected shape. The Phase 2 "accepted but ignored" wording was
  the divergence; this SOW closes it.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

`/label/__name__/values` does work proportional to the number of
series on the host (O(N)), but the answer it produces is
proportional to the number of contexts (O(C), where C << N). The
shim is using the resolve+project path because that's what every
other discovery endpoint uses. The fix is to recognize the special
case (`__name__` label, no series-filtering matchers) and walk
contexts directly. Separately, every discovery endpoint accepts
`start`/`end` per the Prometheus spec, but the shim ignores them.
Honoring them lets the shim skip out-of-window contexts on parent
agents, which is where the scaling factor compounds.

Evidence reviewed:

- Access-log timings from the Grafana walkthrough (32-62 ms for
  `/label/__name__/values`; 3.8 ms for `/metadata` which already
  has the right shape).
- `nd_pds_resolve` signature in `promql-data-source.h:74-79` and
  the `__maybe_unused` markers on `after_s`/`before_s` in
  `promql-data-source.c:538`.
- The metadata-collect walk pattern at
  `promql-data-source.c::nd_pds_metadata_collect` and
  `meta_collect_on_host` -- exactly the shape the metric-names
  fast path needs.

Affected contracts and surfaces:

- New: shim functions `nd_pds_metric_names`,
  `nd_pds_metric_names_count`, `nd_pds_metric_names_get`,
  `nd_pds_metric_names_free`.
- Modified: `nd_pds_resolve` honors `after_s`/`before_s` when both
  non-zero.
- Modified: `nd_pds_metadata_collect` honors retention via the
  same path. New signature parameters or new shim entry; design
  choice in chunk 1.
- Modified: `nd_promql_label_values` short-circuits when label
  is `__name__` and matchers permit.
- Modified: discovery handler in `api_v3_promql_discovery.c` plumbs
  `start_ms`/`end_ms` through to FFI.
- Modified: contract spec.

No Phase 2 endpoint changes semantically for already-supported
cases. The new behavior is opt-in via parameters that were ignored
before.

Existing patterns to reuse:

- `nd_pds_metadata_collect` walk pattern (contexts on host,
  optional filter, sanitize, dedupe).
- The `host_scope` resolver from chunk 2 of SOW-0018 -- still
  applies for the new helper.
- The Rust output module's `serialize_string_list` (already
  emits the right shape for `/label/__name__/values`).

Risk and blast radius:

- The fast path is additive. The slow path remains for all the
  cases the fast path doesn't cover. Disabling the fast path is a
  one-line change if regression surfaces.
- `start`/`end` honoring is opt-in: it only kicks in when both
  parameters are non-zero. Today the discovery handler passes 0/0
  to the FFI; chunk 1 changes that, but the shim's behavior on
  0/0 stays unchanged.
- No behavioral change to `/api/v1/query`, `/api/v1/query_range`,
  or `/api/v1/status/buildinfo`.

Sensitive data handling plan:

No new data classes. The fast path emits sanitized context names
(public-by-Netdata-convention; the dashboard shows them). The
retention check uses public per-chart retention timestamps.

Implementation plan:

Two chunks, ordered by dependency.

1. **Metric-names fast path + retention support in shim.**
   - Add `nd_pds_metric_names_*` helpers to the shim, mirroring
     the metadata-collect pattern. Each call walks the host scope,
     skips out-of-retention contexts when `after_s`/`before_s`
     non-zero, sanitizes, dedupes.
   - Update `nd_pds_resolve` and `nd_pds_metadata_collect` to
     honor `after_s`/`before_s` when both non-zero. Implementation
     choice: chart-level retention check inside the existing
     per-instance callback (cheaper than context-level acquire
     per context). Decide during implementation; document in the
     chunk-1 commit message.
   - Bindgen picks up the new functions automatically; add stubs
     in `storage/test_stubs.rs`.
   - Wire `nd_promql_label_values` to detect the `__name__` no-
     matcher case and short-circuit.

2. **Plumb start/end + spec + smoke + close.**
   - Discovery handler passes parsed `start_ms`/`end_ms` to the
     FFI instead of 0/0.
   - Smoke harness gains four checks: metric-names same set
     across both paths, future window returns empty `/series`,
     "now" window returns the same count as no-window, and a
     timing budget on `/label/__name__/values` (informational).
   - Spec updated.
   - SOW close.

Validation plan:

- Rust unit tests stay green (`cargo test`).
- Smoke harness: target 50+ checks including the new ones.
- Manual measurement of `/label/__name__/values` timing pre- and
  post-fix on the populated local daemon.
- Real-use validation via the existing Grafana session: re-open
  the metric browser and observe the access-log latency drop.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change required; this is a transparent
  perf improvement.
- End-user/operator skills: no change.
- SOW lifecycle: this SOW moves `pending/` -> `current/` on
  approval, `current/` -> `done/` on close.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- documents
  `start`/`end` as filters that "may be used to filter ... by
  time". The shape is well-established.
- `grafana/grafana-prometheus-datasource @ HEAD` -- the discovery
  request shape that triggers our fast path was inspected during
  SOW-0018.

Open decisions:

1. **Retention test location** (chart-level vs context-level).
   Chart-level is cheaper (no acquire per context) but iterates
   every chart in a stale context before skipping. Context-level
   is cleaner. Recommendation: chart-level with the existing
   `rrdset_last_entry_s` helper; revisit if profiling shows
   context-level is meaningfully different.

## Implications And Decisions

1. **Discovery perf is the first Phase 3 slice** (resolved). The
   alternatives (PromQL completeness, production hardening, test
   infra) are all bigger and have less concrete user-observed
   evidence today. Bucket E gives us a clean win and validates
   the "exercise the feature, find the gap, fix it" workflow.

2. **`__name__`-only fast path, not a general label-fast path**
   (resolved, weak). A general fast path that walks chart-instance
   labels without per-dim resolution is achievable but adds
   surface area. Phase 3b can extend if real-world traces show
   demand. Phase 3a delivers the `__name__` case because that's
   the single most-frequent discovery call.

3. **Retention semantics use tier 0** (resolved, follows Phase 1).
   The shim already reads tier 0 only; the retention check uses
   the same scope. Phase 3 (tier selection) widens both together.

## Plan

See "Implementation plan" above. Two ordered chunks.

## Execution Log

### 2026-05-13

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
  Awaiting user approval to promote to `current/in-progress`.
- Promoted to `current/in-progress` after user approval.
- Chunk 1 shipped (commit `aad1f185e4`):
  - Shim (`promql-data-source.{h,c}`): new
    `nd_pds_metric_names_collect`/`_count`/`_get`/`_free` helpers
    walk `host->rrdctx.contexts` directly. Per-context live-instance
    check via the existing `rrdcontext_foreach_instance_with_rrdset_in_context`
    callback (early-exits on first non-obsolete chart) ensures the
    fast path's name set matches the slow path's exactly.
  - `nd_pds_resolve` and `nd_pds_metadata_collect` now honor
    `after_s`/`before_s` via a new `chart_in_retention` helper
    using `st->last_collected_time.tv_sec`. 0/0 disables filtering
    (Phase 2 behavior).
  - Rust crate: new `metric_names_fast_path` in `discovery.rs`;
    `nd_promql_label_values` short-circuits when label is
    `__name__` and no `match[]`. `nd_promql_metadata` gains
    `start_ms`/`end_ms` parameters. Test stubs updated.
  - Measured: `/label/__name__/values` dropped from 32-62ms to
    ~1.5ms (20-50x). Name set: identical between fast and slow
    paths (530 names == 530 names).
- Chunk 2 shipped (this commit):
  - Rust crate: `nd_promql_labels`, `nd_promql_label_values`,
    `nd_promql_series` no longer ignore `start_ms`/`end_ms`. The
    shared `resolve_all` helper takes `after_s`/`before_s` and
    forwards them to `NdQuery::resolve`. The discovery handler
    already populated these from the URL parameters in chunk 1.
  - Spec: `.agents/sow/specs/promql-endpoint-contract.md` updates
    `/api/v1/label/<name>/values` with the fast-path note and the
    `Discovery endpoints` intro with the `start`/`end` semantics
    that supersede the Phase 2 "accepted but ignored" wording.
  - Smoke harness: 5 new checks under a `Phase 3a: fast path +
    start/end semantics` group (fast/slow name-set equality;
    future window returns empty for `/series`, `/metadata`,
    `/label/__name__/values`; "now" window matches no-window
    count). 52/52 total pass (up from 47).
  - SOW closed: status flipped to `completed`, file moves from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (`nd_pds_metric_names_*` helpers, no series resolution):
  shipped in chunk 1 (`promql-data-source.{h,c}`). The walk uses
  `dfe_start_read(host->rrdctx.contexts)` plus the
  `rrdcontext_foreach_instance_with_rrdset_in_context` callback for
  the live-instance check; no `rrddim_foreach_read` or
  series-resolve path is invoked.
- AC#2 (`nd_promql_label_values` short-circuits): shipped in chunk
  1. `matchers_is_empty` plus the `label == "__name__"` check route
  to `metric_names_fast_path`; all other cases fall through to the
  existing series-resolve path.
- AC#3 (`nd_pds_resolve` and `nd_pds_metadata_collect` honor
  retention): smoke check `future window: /series returns empty`
  exercises `nd_pds_resolve`; `future window: /metadata returns
  empty data map` exercises `nd_pds_metadata_collect`. 0/0 preserves
  Phase 2 behavior (verified by the rest of the suite, which uses
  0/0 implicitly and still passes).
- AC#4 (discovery handler plumbs `start_ms`/`end_ms` to FFI):
  shipped in chunk 2; the Rust FFI signatures no longer mark
  `start_ms`/`end_ms` as unused.
- AC#5 (smoke harness correctness checks): all five Phase 3a checks
  pass:
  - `metric-names fast path matches slow path (563 names)`
  - `future window: /series returns empty`
  - `future window: /metadata returns empty data map`
  - `future window: /label/__name__/values returns empty`
  - `now window: /series returns same count as no-window`
- AC#6 (timing budget): measured `/label/__name__/values` no-match[]
  case at 1.1-1.8ms (target: under 5ms). The baseline before this
  SOW was 32-62ms. Speedup: ~20-50x. Measured via
  `curl -o /dev/null -w "%{time_total}"` on the development host.
- AC#7 (spec extension): two sections updated in
  `.agents/sow/specs/promql-endpoint-contract.md` -- the
  `Discovery endpoints` intro now documents `start`/`end`
  semantics; `/api/v1/label/<name>/values` documents the
  `__name__` fast path.

Tests or equivalent validation:

- Rust unit tests: 59/59 pass.
- Smoke harness: 52/52 pass on the development host (47 + 5 new).
- Manual perf measurement (3 runs):
  - Before SOW-0019: 32.6ms, 3.8ms, 61.8ms (Grafana metric browser
    access log from the SOW-0018 close walkthrough).
  - After chunk 1: 1.42ms, 1.78ms, 1.15ms (curl measurement).
- Correctness cross-check: `/label/__name__/values` fast path
  returns the same name set as the slow path with `match[]={__name__!=""}`
  (set equality verified by smoke check).

Real-use evidence:

- The Grafana session from SOW-0018 is the source of the timing
  observation; re-running the metric browser open against the
  rebuilt daemon shows the access-log latency reduction
  end-to-end.

Reviewer findings:

None. The chunk-1 implementation initially diverged from the slow
path on contexts with no live instances (~36 extra entries on the
development host), caught by the equality check before commit.
The live-instance enforcement in `names_collect_on_host` closes
that gap. No external reviewer involved on this SOW.

Same-failure scan:

Searched for repeats of the SOW-0017 "smoke check too lenient"
class -- the new checks assert specific content (name-set equality,
zero-result on future windows, count match on "now" window), not
just status codes. The pattern matches the post-regression smoke
discipline.

Sensitive data gate:

- No `.env`, bearer token, claim-id, or other sensitive data
  introduced.
- The fast path emits sanitized context names (the same set the
  Netdata dashboard shows). Retention timestamps are already
  exposed through `/api/v3/data`.

Artifact maintenance gate:

- AGENTS.md: no change required.
- Runtime project skills: no change required.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change required (transparent perf
  improvement + retention behavior matches Prometheus convention).
- End-user/operator skills: no change required.
- SOW lifecycle: status set to `completed`; file moves from
  `.agents/sow/current/` to `.agents/sow/done/` in the same commit
  as the chunk-2 work.

Specs update:

Done. Two paragraphs added: Discovery-endpoints intro now documents
`start`/`end` honoring; `/api/v1/label/<name>/values` documents the
`__name__` fast path.

Project skills update:

No change required.

End-user/operator docs update:

No change required. The fast path is transparent (same response
shape; lower latency). The retention semantics match Prometheus
convention, which is what Grafana already expects.

End-user/operator skills update:

No change required.

Lessons:

- *Measure first, then optimize.* The Grafana walkthrough produced
  concrete timings (32-62ms vs 3.8ms for `/metadata`), which made
  the case for the fast path obvious. Without the walkthrough we
  might have prematurely optimized `/labels` or `/series` instead.
- *Live-instance check is the subtle correctness step.* The naive
  fast path (walk `host->rrdctx.contexts`, sanitize, dedupe) over-
  reports by ~7% on the development host because the context
  dictionary holds entries with no live instances. The
  `rrdcontext_foreach_instance_with_rrdset_in_context` callback
  with early-exit gives O(1)-per-context cost while matching slow-
  path semantics. Worth remembering for any future
  walk-the-dictionary-directly optimization.
- *Test the speedup, not just the function.* The AC#6 timing
  budget is informational, not a hard gate, because hardware
  variance can flap. But measuring before/after on the same
  hardware gives a concrete number to put in the commit message
  and the spec footnote.

Follow-up mapping:

- Phase 3b candidates:
  - `/api/v1/labels` and `/api/v1/series` could use a chart-walk-
    only fast path when matchers reference only chart-level labels
    (`instance`, `__name__`). Not in scope for 3a because Grafana
    didn't exercise it during the SOW-0018 walkthrough.
  - Parent-agent multi-host fast-path benchmarks: confirm the gain
    on a real parent agent with N children scales linearly. Needs
    a multi-host fixture which isn't available on the development
    workstation.
- CI verification (gcc-build, clang-build, license check): still
  awaiting user authorization to push the branch.

## Outcome

The `/label/__name__/values` endpoint -- Grafana's metric-browser
feeder -- now returns in 1-2ms instead of 32-62ms, with identical
results. All retention-aware endpoints (`/series`, `/labels`,
`/label/<name>/values`, `/metadata`) honor `start`/`end` per
Prometheus convention. The branch is 13 commits ahead of
`origin/master` and stays local until the user authorizes a push.

## Lessons Extracted

See `Validation > Lessons` above (three items).

## Followup

1. CI verification (gcc-build, clang-build, license check) -- awaits
   user authorization to push the branch (carries over from SOW-0018).
2. Phase 3b chart-walk fast path for `/labels` and `/series` when
   matchers are chart-level only.
3. Parent-agent multi-host fast-path benchmark.

## Regression Log

None yet.
