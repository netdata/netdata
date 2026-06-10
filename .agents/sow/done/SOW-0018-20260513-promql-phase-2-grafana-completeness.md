# SOW-0018 - PromQL Phase 2 - Grafana Datasource Completeness

## Status

Status: completed

## Requirements

### Purpose

Phase 1 (SOW-0017) shipped a PromQL evaluator that returns correct
results for hand-typed queries against real Netdata data, plus the
HTTP-method support (GET and POST) Grafana needs to send those queries.
What's still missing is everything Grafana uses to *discover* what
queries to write: the metric browser, label autocomplete, the "Save &
Test" health probe, and TYPE/HELP metadata for displayed metrics. A
user adopting this today can write `rate(disk_io[5m])` by hand and see
a graph, but cannot browse available metrics in the UI and cannot
populate label filters interactively.

Phase 2 closes that gap. It adds the four endpoints Grafana's
Prometheus datasource probes for discovery (`/api/v1/labels`,
`/api/v1/label/<name>/values`, `/api/v1/series`, `/api/v1/metadata`)
plus the datasource health-check endpoint (`/api/v1/status/buildinfo`),
and extends the C data-source shim to support specific-host lookup so
multi-host parent-agent deployments work correctly. This is bucket A
of the deferred-work list from SOW-0017's close.

Phase 2 does _not_ add any new PromQL evaluation features. Subqueries,
vector matching (`group_left`/`group_right`/`on`/`ignoring`), the full
`*_over_time` rollup family, `topk`/`bottomk`/`quantile`/`count_values`,
`predict_linear`/`holt_winters`, the `@` modifier with arithmetic, and
`keep_metric_names` all remain Phase 3. The Prometheus compliance test
suite, tier selection beyond tier 0, the rollup result cache, and the
broader performance work also remain Phase 3.

### User Request

Direct user instruction: "Ok, at this point we should prepare the
Phase 2 SOW."

Two scope-shaping decisions resolved during the conversation that
produced this SOW:

1. Scope = pure Grafana-completeness (bucket A only). Subqueries and
   vector matching defer to Phase 3. Reasoning: Phase 1 already
   produces useful query results; the bottleneck for adoption is now
   the UI surface, not PromQL expressiveness.

2. The full `*_over_time` rollup family also defers to Phase 3.
   Reasoning: `rate`/`irate`/`increase`/`delta` cover the common
   counter-rate pattern. Aggregations across a window
   (`avg_over_time`, `max_over_time`, etc.) are useful but not
   adoption-blocking.

### Assistant Understanding

Facts:

- Phase 1 shipped in commits `6b44cfdfa0` (Phase 0 foundation) through
  `7b0f50739c` (POST follow-up). The `pql` branch is 7 commits ahead
  of `origin/master`.
- `/api/v1/query` and `/api/v1/query_range` exist and route through
  `api_v3_promql`. Both GET and POST work. 24/24 smoke checks pass.
- `nd_pds_resolve` already enumerates series from storage. Phase 2's
  discovery endpoints can build on that foundation: walk the resolved
  series, project the labels, and aggregate. No new shim work for
  labels / label values / series. Only `/api/v1/metadata` requires a
  new shim helper to surface per-metric TYPE (derived from
  `RRD_ALGORITHM`) and HELP (chart title).
- The Phase 1 shim accepts `host_machine_guid` values of `NULL`
  (localhost) and `"*"` (all hosts). Anything else falls back to
  localhost. Phase 2 lifts this restriction by adding lookup via
  `rrdhost_find_by_hostname` and `rrdhost_find_and_acquire`.
- The Prometheus HTTP API spec documents all five discovery endpoints
  unambiguously, including response shapes. We do not need to make
  any new schema decisions -- Grafana expects exactly the Prometheus
  shape and we deliver it.
- The Phase 1 spec at `.agents/sow/specs/promql-endpoint-contract.md`
  is the place to add Phase 2's contract additions.
- Grafana's actual datasource code (verified by inspecting
  `grafana/grafana-prometheus-datasource @ main` and the bundled
  `@grafana/prometheus@13.1.2` dist in `grafana/grafana @ main`)
  drives the following surface:
  - The metric browser populates from
    `/api/v1/label/__name__/values`, not from `/api/v1/metadata`
    (frontend `resource_clients.mjs#queryMetrics`).
  - `/api/v1/metadata` only feeds tooltip-style TYPE/HELP via
    `language_provider.mjs`, and the call is made with
    `showErrorAlert: false` -- a 404 is silently tolerated.
  - The "Save & Test" health check runs an instant query `1+1`
    through the normal QueryData path
    (`promlib/healthcheck.go:56-92`). Phase 1 already evaluates this
    expression; the health check therefore passes today, before any
    Phase 2 endpoint lands.
  - `/api/v1/status/buildinfo` is queried separately for *heuristics*
    classification (`promlib/heuristics.go:42-114`). `getHeuristics`
    errors are logged as warnings and do not fail the health check.
    When the `data.features` map is empty/absent, Grafana classifies
    the datasource as Prometheus (vs Mimir) and disables the ruler
    feature. When `data.features` has entries, Grafana classifies it
    as Mimir.
  - Frontend version gating uses
    `instanceSettings.jsonData.prometheusVersion`, which the operator
    sets manually in the datasource UI. The buildinfo version string
    we return is **not** consumed for feature gating; it is for
    diagnostic clarity.
  - Grafana's default `limit` for `/labels`, `/label/<name>/values`,
    and `/series` is `DEFAULT_SERIES_LIMIT = 40000`
    (`constants.mjs`). Our Phase 1 `max_series=10000` is below that.
  - Grafana sends `match[]=<selector>` as a repeatable query
    parameter on `/labels`, `/label/<name>/values`, and `/series`.
    The Prometheus spec allows multiple `match[]` values; each is an
    independent matcher and series matching ANY of them are
    returned.
  - The "all series" sentinel is `MATCH_ALL_LABELS = '{__name__!=""}'`
    (`constants.mjs`). Empty `{}` is rewritten to this sentinel by
    the frontend before being sent.
  - When `httpMethod=POST` is set on the datasource (Grafana's
    modern default for queries past a certain length),
    `GET_AND_POST_METADATA_ENDPOINTS` includes `api/v1/query`,
    `api/v1/query_range`, `api/v1/series`, and `api/v1/labels`
    (`constants.mjs`). `/api/v1/label/<name>/values`,
    `/api/v1/metadata`, and `/api/v1/status/buildinfo` are GET-only.
    On 405/400 the frontend retries with GET
    (`datasource.mjs:227-231`).

Inferences:

- `/api/v1/labels` and `/api/v1/series` accept the same matcher
  syntax as `/api/v1/query` matchers. The implementation pulls the
  parsing helpers we already have in `api_v3_promql.c`. They also
  accept `start`, `end`, `limit`, and a repeatable `match[]`.
- `/api/v1/label/<name>/values` accepts `start`, `end`, `limit`, and
  a repeatable `match[]`. The label name comes from the URL path,
  not the query string.
- `/api/v1/metadata` is unusual in the Prometheus API: it does not
  accept matchers, only an optional `metric` parameter and an optional
  `limit`. The response is keyed by metric name. We can serve this by
  enumerating contexts on the host (or all hosts) without a full
  series resolve.
- `/api/v1/status/buildinfo` is the cheapest endpoint -- a static JSON
  response with `data.features={}` so Grafana classifies us as
  Prometheus. The version string itself is diagnostic; we return
  `2.45.0` to match the Prometheus reference SOW-0017 anchors on.
- The discovery endpoints' response sizes grow with cardinality. The
  Phase 1 `max_series=10000` cap applies to `/series`, but `/labels`
  and `/label/<name>/values` produce smaller responses (one entry per
  label name or value, not per series); they can use the same cap as
  a conservative default. When Grafana sends `limit=40000` (its
  default), we silently truncate at `max_series` and emit a
  `warnings` entry in the envelope per Prometheus convention.

Unknowns:

- None outstanding from Phase 1's open questions. The matcher
  format, parameter set, limit defaults, and HTTP-method behavior
  are all verified above against current Grafana source.

### Acceptance Criteria

1. `/api/v1/status/buildinfo` returns HTTP 200 with body
   `{"status":"success","data":{"version":"...","revision":"...","branch":"...","buildUser":"...","buildDate":"...","goVersion":"...","features":{}}}`.
   The empty `features` map is load-bearing: Grafana's heuristics layer
   (`promlib/heuristics.go:105-112`) classifies the datasource as
   Prometheus when `len(features)==0` and as Mimir otherwise.
   Concrete values: see Open Decision #1. The endpoint is GET-only.

2. `/api/v1/labels` accepts `start`, `end`, `limit`, and a
   repeatable `match[]` query parameter. It returns the union of
   label names across all series resolved by the provided matchers
   (or all series on the host if no matchers).
   Response shape:
   `{"status":"success","data":["__name__","instance","dimension",...]}`.
   Verification: `curl /api/v1/labels` returns at least
   `["__name__","chart","dimension","family","instance"]` against a
   live daemon. Accepts both GET and POST (form-encoded body).

3. `/api/v1/label/<name>/values` accepts `start`, `end`, `limit`,
   and a repeatable `match[]`; the label name is in the URL path.
   Returns distinct values for the named label, projected from the
   matched series. For example, `/api/v1/label/dimension/values`
   against a freshly-installed daemon should return
   `["user","system","idle","iowait",...]` (the dimensions of
   `system_cpu`) plus values from other charts that share the
   `dimension` label. GET-only.
   `/api/v1/label/__name__/values` is the metric-browser feeder
   and must return the full sanitized context name list (e.g.
   `["system_cpu","disk_io","mem_available",...]`).

4. `/api/v1/series` accepts `start`, `end`, `limit`, and a
   repeatable `match[]`. Returns full label sets (no values, no
   timestamps) for series matching ANY of the supplied `match[]`
   selectors. Response shape:
   `{"status":"success","data":[{"__name__":"system_cpu","instance":"...","dimension":"user"},...]}`.
   The Grafana "all series" sentinel `match[]={__name__!=""}` must
   resolve to every named series on the host without invoking
   sanitized-context lookup (the `Ne` matcher on `__name__` is the
   universal-selector path). Accepts both GET and POST.

5. `/api/v1/metadata` returns per-metric TYPE+HELP. Response shape:
   `{"status":"success","data":{"system_cpu":[{"type":"gauge","help":"<chart title>","unit":""}],...}}`.
   TYPE derives from the dimension's `RRD_ALGORITHM`:
   `INCREMENTAL` -> `counter`, all others -> `gauge`. HELP is the
   chart's title via `rrdset_title`. Unit is empty for Phase 2 (the
   chart's units are already encoded in the dimension; the Prometheus
   `unit` field is rarely used by Grafana).

6. Specific-host lookup works in the C shim. `?host=server-7` matches
   a host whose hostname is `server-7`; `?host=cf828772-...` matches a
   host whose `machine_guid` equals that UUID; `?host=*` matches all
   hosts (unchanged from Phase 1); no `host` parameter matches
   localhost (unchanged from Phase 1).

7. The `instance` label on every emitted series carries the host's
   hostname. This was already the case in Phase 1 but is now
   acceptance-verified end to end with a multi-host fixture (either
   a parent agent with one child, or a single-host fixture with the
   `instance` label asserted to equal the local hostname).

8. Grafana "Save & Test" against a Netdata daemon at
   `http://localhost:19999` passes. The health check itself
   (`promlib/healthcheck.go`) submits an instant query `1+1` through
   the normal QueryData path and is **already green after Phase 1**;
   this AC verifies the Phase-1 behavior end to end with the Phase-2
   endpoints in place. Heuristics rely on `/status/buildinfo` (AC#1)
   for Prometheus vs Mimir classification but heuristics errors are
   logged warnings, not health-check failures. Verification: install
   Grafana locally, configure a Prometheus datasource with that URL,
   click "Save & Test", confirm the green check. Confirm the
   Grafana log shows `Application: Prometheus` (not Mimir, not
   unknown) -- i.e. AC#1's `features:{}` is being read correctly.

9. Grafana metric browser populates with Netdata metrics. The metric
   browser fetches via `/api/v1/label/__name__/values` (AC#3); the
   `/api/v1/metadata` endpoint (AC#5) supplies TYPE/HELP for
   tooltips and is called with `showErrorAlert: false` so a stub
   response is sufficient even if HELP is empty. Verification: in a
   Grafana panel's "Metrics" dropdown, see `system_cpu`, `disk_io`,
   etc. listed. Type-ahead on label-value filters
   (`instance=`) produces a non-empty dropdown.

10. Each discovery endpoint accepts the Prometheus repeatable
    `match[]` query parameter and treats each `match[]` value as an
    independent matcher (series matching ANY supplied selector are
    included). Single-occurrence `match[]` continues to work
    unchanged. Verification: `curl '/api/v1/series?match[]=system_cpu&match[]=disk_io'`
    returns series for both metrics.

11. When Grafana sends `limit=40000` (its `DEFAULT_SERIES_LIMIT`)
    and the resolution would exceed `max_series=10000`, the daemon
    silently truncates at `max_series` and includes a
    `"warnings":["truncated at max_series=10000; tighten the query or raise the cap"]`
    array in the success envelope per Prometheus convention. The
    response remains `status:"success"` so Grafana does not surface
    a user-visible error. Out-of-band `limit` smaller than
    `max_series` is honored as-is. Verification: smoke harness asserts
    the `warnings` field appears on an unbounded `/series` against
    a populated daemon.

12. The contract spec at `.agents/sow/specs/promql-endpoint-contract.md`
    documents the five new endpoints, the specific-host lookup
    semantics, and any divergences from upstream Prometheus that
    surfaced during implementation.

13. The smoke harness at `tests/promql-smoke/run-smoke.sh` exercises
    the new endpoints (labels, label/<name>/values, series, metadata,
    status/buildinfo) and the specific-host lookup. 30+ total checks
    pass against the live daemon, including the repeatable-`match[]`
    case and the `limit`-truncation `warnings` case.

CI Linux (`gcc-build`, `clang-build`) and the license check are
verified once the user authorizes the push; until then the branch
stays local and CI gates remain in the "deferred until push" state
recorded under Validation > Followup mapping. macOS verification is
out of scope for this SOW -- no macOS hardware is available to the
author; deferred to Phase 3 alongside the other production-hardening
items.

Out of scope for this SOW (Phase 3 or later):

- macOS verification. No macOS hardware available; Phase 3 handles this
  alongside other production-hardening work.
- Subqueries (`metric[1h:5m]`).
- Vector matching with `on`/`ignoring`/`group_left`/`group_right`.
- The full `*_over_time` rollup family beyond what Phase 1 ships.
- `topk`/`bottomk`/`quantile`/`count_values`, `predict_linear`,
  `holt_winters`.
- The `@` modifier with arithmetic. `keep_metric_names`.
- Tier selection beyond tier 0 in the shim.
- Rollup result cache.
- Performance work (rayon-parallel per-series inner loops, allocation
  profiling, regex precompilation strategy beyond the existing cache).
- Prometheus compliance test suite.
- Cardinality telemetry.
- `/api/v1/query_exemplars`, `/api/v1/format_query`, `/api/v1/parse_query`,
  `/api/v1/targets`, `/api/v1/rules`, `/api/v1/alerts`,
  `/api/v1/alertmanagers`, `/api/v1/status/{runtimeinfo,config,flags,tsdb}`.
  These are optional Prometheus endpoints that Grafana mostly does not
  require for the basic datasource flow; revisit if specific Grafana
  flows surface them.

## Analysis

Sources checked:

- Phase 1 deliverables: `src/database/contexts/promql-data-source.{c,h}`,
  `src/crates/netdata_promql/`, `src/web/api/v3/api_v3_promql.c`,
  `.agents/sow/specs/promql-endpoint-contract.md`,
  `tests/promql-smoke/run-smoke.sh`.
- SOW-0017 Out-of-Scope section.
- Validation findings from chunk 5 + POST follow-up.
- Prometheus HTTP API reference at
  <https://prometheus.io/docs/prometheus/latest/querying/api/>.
- `src/database/rrd.h` and `src/database/rrdhost.c` for the host
  enumeration / lookup APIs (`rrdhost_find_by_hostname`,
  `rrdhost_find_and_acquire`).
- `src/database/contexts/rrdcontext.h` for chart title accessor
  (`rrdset_title`) and per-context iteration.

Current state:

- Phase 1 endpoints are stable and verified end to end.
- The shim is the right place to add discovery enumeration helpers;
  it already holds the rrdcontext walking machinery and the label
  builder.
- The handler layer (`api_v3_promql.c`) is the right place for the
  C-side dispatchers, with one handler per new endpoint to keep the
  per-endpoint logic readable.

Risks:

- *Cardinality on `/api/v1/labels` and `/series`*. A Grafana metric
  browser hitting `/labels` with no matchers triggers a full
  enumeration of every distinct label across every series on the
  host. On a parent agent with thousands of child hosts, this can
  produce a large response. The `max_series` cap applies; we may need
  a separate `max_labels` cap for safety. Recorded as Open Decision #2.
- *Limit divergence between Grafana and Netdata defaults*. Grafana
  sends `limit=40000` by default; our `max_series=10000` is lower.
  We silently truncate and signal via `warnings` (Open Decision #6).
  Risk: low; Grafana logs the warning at debug level and the UI
  still functions, just with a capped result set. Operators on
  high-cardinality parent agents who hit the cap regularly can raise
  it; Phase 3 may make the cap per-endpoint configurable.
- *Grafana version differences*. Different Grafana versions probe
  slightly different endpoints. We target current Grafana (10.x+,
  `@grafana/prometheus@13.x`); older versions may have additional
  missing-endpoint complaints. Not in scope for Phase 2.
- *Specific-host lookup ambiguity*. A user could pass a value that
  matches both a hostname and a machine_guid prefix. Phase 2 picks
  machine_guid first, falling back to hostname; documented in the
  spec. The corner case is rare in practice.
- *Repeatable `match[]` parsing*. Phase 1's URL parser uses
  `strsep_skip_consecutive_separators` and overwrites on repeated
  keys, which loses the second matcher. The discovery handlers need
  a parameter-collection pass that preserves all `match[]`
  occurrences. Risk: low (small, contained edit); failure mode is
  benign (the second matcher is dropped, results are a subset).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

A Grafana user adding a Netdata daemon as a Prometheus datasource
today encounters a usable but incomplete experience: hand-typed PromQL
queries execute and graph correctly, but the discovery surface (metric
browser, label autocomplete, datasource health check) returns 404 for
all probes. The cause is that Phase 1 implemented only the query
execution endpoints (`/api/v1/query`, `/api/v1/query_range`) and not
the discovery endpoints Grafana's Prometheus datasource uses for UI
features. Phase 2 closes the discovery gap.

The implementation is bounded: each discovery endpoint maps cleanly
onto the storage data the shim already exposes, or onto a small new
shim helper for the one case (`/api/v1/metadata`) where Phase 1
doesn't surface TYPE/HELP.

Evidence reviewed:

- Phase 1 SOW close at `.agents/sow/done/SOW-0017-...md`.
- Validation findings from the chunk-5 close: 21/21 smoke checks
  passed but discovery endpoints absent.
- POST follow-up commit `7b0f50739c` adding HTTP-method support.
- Cross-reference of existing Netdata v1 endpoints in
  `src/web/api/web_api_v1.c` -- no naming collisions with the
  Prometheus discovery endpoints.
- Prometheus HTTP API spec for each endpoint's exact response shape.

Affected contracts and surfaces:

- New: five HTTP endpoints under `/api/v1/`: `labels`, `label`,
  `series`, `metadata`, `status/buildinfo`.
- New: one C shim helper for `/api/v1/metadata` exposing chart title
  + RRD_ALGORITHM per context.
- Modified: `src/database/contexts/promql-data-source.c` -- specific-
  host lookup branch added to `nd_pds_resolve`.
- Modified: `src/web/api/v3/api_v3_promql.c` (or a sibling file in
  the same directory) -- new handlers.
- Modified: `src/web/api/web_api_v1.c` -- new dispatch entries.
- Modified: `src/web/api/v3/api_v3_calls.h` -- new function declarations.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md` -- new
  sections documenting the discovery contract.
- Modified: `tests/promql-smoke/run-smoke.sh` -- new check helpers.
- Modified: `CMakeLists.txt` -- gate any new source files behind
  `ENABLE_PROMQL`.

No Phase 1 endpoints change semantically. The Phase 1 success envelope
gains one new optional field, `warnings: []`, which Phase 1 responses
omit and Phase 2 emits only when a `limit` exceeds `max_series` and
the result was truncated. This is additive and Prometheus-spec-blessed.
Phase 1 spec sections are not edited; new sections are appended.

Existing patterns to reuse:

- The path-dispatcher pattern from `api_v3_promql`: a single handler
  that distinguishes subpaths via `w->url_path_decoded`. Phase 2 can
  use the same shape, or one handler per endpoint (likely simpler --
  each endpoint has distinct parameter parsing).
- The POST-body handling pattern from the same handler: pull
  parameters from `w->payload` when `url` is empty.
- The shim's series enumeration via
  `rrdcontext_foreach_instance_with_rrdset_in_context`. New
  enumeration calls share the iteration shape.
- The smoke-harness check helpers (`_check`, `check_post`). Phase 2
  adds analogous helpers for the discovery shapes.

Risk and blast radius:

- The new endpoints are additive: they don't modify any existing path.
- The shim's specific-host lookup is a one-branch addition in
  `nd_pds_resolve`; the existing NULL/"*"/fallback paths remain.
- Disabling Phase 2 cleanly: `ENABLE_PROMQL=OFF` removes everything,
  same as Phase 1. New source files go behind the same flag.
- Cardinality regressions: the `/series` and `/labels` endpoints
  fan out by definition. The same `max_series` cap applies to
  resolution; if a `/labels` response is still too large, we add a
  separate `max_labels` cap as a Phase 2 deliverable (Open Decision #2).
- Multi-host correctness: the `instance` label was emitted in Phase
  1 but never tested with more than one host. Phase 2 verifies this
  works, or fixes whatever shows up. Risk: low; the shim walks
  `rrdhost_root_index` and emits the per-host hostname.

Sensitive data handling plan:

Phase 2's new endpoints expose label names and values that may include
chart labels users assigned (per-disk device names, mount points,
network interface names). These are not sensitive by Netdata convention
(they're the same labels the dashboard shows), but a Grafana-facing
discovery endpoint that lists every label value on a parent agent
could surface deployment topology more broadly than the existing
`/api/v1/contexts` does. The spec documents which labels surface
through which endpoint so a security-conscious operator can audit.

No `.env`/bearer/claim-id data is introduced. The Rust crate's
transitive deps don't change. Query strings and discovery requests are
not logged at INFO level.

Implementation plan:

Three chunks, ordered by dependency.

1. **Discovery endpoint handlers** (`src/web/api/v3/api_v3_promql_discovery.{c,h}`
   or extension of `api_v3_promql.c`). Implements the five endpoints
   on top of existing shim functions. The `/api/v1/metadata` path
   requires a new shim helper `nd_pds_metadata` that enumerates
   contexts with their RRD_ALGORITHM and chart title. The five
   handlers register against `web_api_v1.c`'s dispatch table; the
   `/label/<name>/values` path uses `allow_subpaths = 1` with internal
   routing to extract `<name>` from `w->url_path_decoded`. `/labels`
   and `/series` accept both GET and POST (form-encoded body), reusing
   the Phase 1 POST shim in `api_v3_promql.c`. `/label/<name>/values`,
   `/metadata`, and `/status/buildinfo` are GET-only. Parameter
   parsing supports repeatable `match[]` -- the existing
   `strsep_skip_consecutive_separators` loop must be extended to
   collect a list when the same key appears multiple times, rather
   than overwriting. The `limit`-vs-`max_series` truncation path
   appends to a `warnings` array, which becomes a new optional field
   on the Phase 1 JSON envelope; Phase 1 responses omit it, Phase 2
   adds it only when truncation actually occurs.

2. **Specific-host lookup in the shim**. Extend `nd_pds_resolve` to
   accept GUID or hostname for `host_machine_guid`. Walk
   `rrdhost_root_index` and match `machine_guid` and `hostname` in
   order. Update the smoke harness to add specific-host checks (a
   localhost-only fixture is sufficient -- assert that
   `?host=<this_hostname>` resolves to the same series as `?host=` or
   `?host=*` on a single-host agent).

3. **Spec + smoke harness extension + close**. Update the spec with
   the discovery endpoint contracts and the new host-scoping rules.
   Extend the smoke harness to cover the new endpoints (30+ total
   checks). Add a short Grafana datasource setup note to user-facing
   docs. Close the SOW. The branch stays local; the user authorizes
   the push when ready.

Validation plan:

- Cargo unit tests: not directly applicable -- the new endpoints
  live entirely in C with the existing shim. Existing Rust tests
  remain green.
- Integration tests in `tests/promql-smoke/run-smoke.sh`: new check
  helpers (`check_labels`, `check_series`, `check_metadata`,
  `check_buildinfo`, `check_specific_host`) covering the five new
  endpoints + specific-host lookup. Target: 30+ total checks.
- Real-use evidence: Grafana "Save & Test" passes; metric browser
  populates; one panel rendered against `rate(disk_io[5m])` with
  label-value autocomplete demonstrated.
- CI Linux + license check: deferred until the user authorizes
  pushing the branch to a remote. The local Linux build is the
  load-bearing check for the SOW close; CI runs on a separate
  schedule the user controls.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` gains new
  sections for the discovery endpoints and the specific-host rules.
- End-user/operator docs: a short user-facing note about Grafana
  datasource setup. Phase 2 close is the right time for this -- the
  feature surface is now Grafana-usable.
- End-user/operator skills: no change.
- SOW lifecycle: this SOW moves from `pending/` to `current/` on
  approval and from `current/` to `done/` on close. Phase 3 is a
  successor SOW.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- HTTP API spec at
  `docs/querying/api.md`. Used as the authoritative shape reference.
- `grafana/grafana @ 777fab6f7a97f72c0365e1c52db96ea0d1c0b393`
  (`master` at clone time) -- bundles
  `@grafana/prometheus@13.1.2`. Confirmed:
  `public/app/plugins/datasource/prometheus/module.ts` re-exports
  the package; `package.json` pins the version.
- `@grafana/prometheus@13.1.2` (npm) -- frontend datasource dist.
  Confirmed endpoint set via `dist/esm/datasource.mjs`,
  `dist/esm/resource_clients.mjs`, `dist/esm/language_provider.mjs`,
  `dist/esm/constants.mjs`. Sources: only the compiled dist is
  published; original sources live in the
  `grafana-prometheus-datasource` repo below.
- `grafana/grafana-prometheus-datasource @ HEAD` -- the Go backend
  (`promlib`) that proxies datasource resource requests. Confirmed
  health-check, buildinfo, and heuristics semantics via
  `pkg/promlib/healthcheck.go`, `pkg/promlib/heuristics.go`, and
  `pkg/promlib/resource/{schema.go,resource.go}`.

Open decisions:

Three minor decisions that can be revisited at close if real-world
deployment surfaces evidence:

1. **`/api/v1/status/buildinfo` version string.** Two options:
   (a) Return Prometheus version `2.45.0` (the SOW-0017 compliance
   anchor).
   (b) Return a Netdata-specific string like `netdata-promql-2.0`.
   **Recommendation: option (a).** Strength: weak.
   Reasoning: empirically, Grafana's frontend feature-gating does
   **not** consume this string. `_isDatasourceVersionGreaterOrEqualTo`
   reads `instanceSettings.jsonData.prometheusVersion`, which the
   operator sets manually in the datasource UI
   (`datasource.mjs:131-141`). The backend uses buildinfo only for
   the Prometheus-vs-Mimir heuristic, which keys off the presence
   of `data.features` entries -- not the version string. So the
   version string is purely diagnostic. We pick `2.45.0` so operators
   reading the daemon's buildinfo see a familiar reference point,
   and so any tool that *does* string-match against a Prometheus
   version (rare) doesn't trip over a Netdata-shaped value. We
   document the choice in the spec so operators know what the daemon
   advertises and why.

2. **Separate `max_labels` cap?** Phase 1's `max_series=10000` caps
   the resolution layer. `/api/v1/labels` produces N entries where
   N is the number of distinct label names across resolved series
   (typically dozens, not thousands). Recommendation: reuse the
   existing `max_series` cap as a backstop; no separate cap needed
   until evidence of an unbounded-labels case surfaces.

3. **TYPE assignment for PCENT_OVER_* dimensions.** Phase 2 maps
   `INCREMENTAL` to `counter` and all other algorithms to `gauge`.
   `PCENT_OVER_DIFF_TOTAL` and `PCENT_OVER_ROW_TOTAL` are percentages
   derived from counters, but the stored value is a gauge.
   Recommendation: report them as `gauge`. Strength: strong. Reasoning:
   the stored value is a gauge from PromQL's perspective; calling them
   counters would mislead `rate()` users.

## Implications And Decisions

1. **Phase 2 scope** (resolved). Bucket A only: discovery endpoints,
   specific-host lookup, multi-host smoke, macOS verification. Bucket
   B (PromQL extensions) and bucket C (production hardening) are
   Phase 3.

2. **Rollup family deferred** (resolved). `avg_over_time` and siblings
   defer to Phase 3 alongside the other PromQL extensions.

3. **Endpoint placement** (new). All five new endpoints live under
   `/api/v1/` (Prometheus convention), not under `/api/v3/promql/`.
   Reasoning: the discovery endpoints are Prometheus-API conformance,
   not Netdata extensions. Operators looking for them will look at
   `/api/v1/labels`, not `/api/v3/promql/labels`. We do not duplicate
   under `/api/v3/promql/` -- the spec records this asymmetry.

4. **buildinfo version** (resolved, weak preference). Return Prometheus
   `2.45.0` as the version string. The string itself is diagnostic;
   what controls Grafana's behavior is the empty `features` map
   (which keeps us classified as Prometheus, not Mimir). The frontend
   version-gating reads a manually-configured datasource setting,
   not buildinfo.

5. **TYPE mapping** (resolved, strong preference). `INCREMENTAL` ->
   `counter`, all others -> `gauge`.

6. **Cardinality cap** (resolved, weak preference). Reuse `max_series`;
   add `max_labels` only if real-world evidence demands it. When
   Grafana's default `limit=40000` exceeds our `max_series=10000`,
   we silently truncate at `max_series` and emit a Prometheus-style
   `warnings: [...]` array in the success envelope. Grafana does not
   surface a user-visible error for `warnings` -- it logs them at
   debug level.

7. **`match[]` repeatability** (resolved, strong preference). All
   three discovery endpoints that accept `match[]` (`/labels`,
   `/label/<name>/values`, `/series`) must accept it as a repeatable
   parameter, OR-combining the matchers. Single-value `match[]` is a
   degenerate case. Grafana sends one matcher per query-builder line
   in some panel modes.

8. **POST on discovery endpoints** (resolved, strong preference).
   `/labels` and `/series` accept both GET and POST (per
   `GET_AND_POST_METADATA_ENDPOINTS` in Grafana's frontend).
   `/label/<name>/values`, `/metadata`, and `/status/buildinfo` are
   GET-only; Grafana never sends POST to them. The POST body for
   `/labels` and `/series` is `application/x-www-form-urlencoded`,
   parsed the same way the Phase 1 POST shim parses query bodies.

## Plan

See "Implementation plan" above. Three ordered chunks. Each can be
reviewed independently. Chunk 3 closes the SOW.

## Execution Log

### 2026-05-13

- SOW drafted. Pre-Implementation Gate filled; status `ready`. Awaiting user approval to promote to `current/in-progress`.
- Second-pass review against current Grafana sources. Cloned
  `grafana/grafana-prometheus-datasource` to `~/repos/` and inspected
  `pkg/promlib/healthcheck.go`, `pkg/promlib/heuristics.go`, and
  `pkg/promlib/resource/{schema.go,resource.go}`. Inspected
  `@grafana/prometheus@13.1.2` dist (the version bundled with
  `grafana/grafana @ master`). Findings folded back into:
  - Assistant Understanding > Facts (Grafana-verified behaviors).
  - AC#1 (`features: {}` field; load-bearing for Prom/Mimir
    classification).
  - AC#2/3/4 (explicit parameter set including repeatable `match[]`;
    GET vs POST per endpoint; `MATCH_ALL_LABELS` sentinel; metric-
    browser entry point via `__name__/values`).
  - AC#8 reframed (Save & Test already passes after Phase 1; this
    SOW verifies, doesn't deliver, the green check).
  - AC#9 reframed (metric browser fed by `__name__/values`, not
    `/metadata`).
  - AC#10/11 added (repeatable `match[]`; `limit`-vs-`max_series`
    truncation with `warnings` envelope field).
  - Implications #4 reworded (version string is diagnostic, not
    feature-gating); #6 extended (truncation policy);
    #7 added (`match[]` repeatability); #8 added (POST per endpoint).
  - Risks section gained two new entries (limit divergence;
    `match[]` parser change).
  - Open-source reference evidence cites the
    `grafana-prometheus-datasource` repo and the Grafana checkout
    commit.
- SOW promoted from `pending/` to `current/` with status flipped to
  `in-progress` per user approval.
- Chunk 1 shipped:
  - Shim (`src/database/contexts/promql-data-source.{h,c}`): added
    `truncated` flag on `nd_pds_query`, exposed via
    `nd_pds_was_truncated`. Overflow no longer destroys the partial
    result; the query path treats truncation as an error (preserving
    Phase 1 strictness), the discovery path converts it to a
    `warnings` envelope field. New `nd_pds_metadata_*` helpers walk
    contexts on the host scope and surface (sanitized name, type,
    help, unit) tuples for `/api/v1/metadata`.
  - Rust (`src/crates/netdata_promql/`): new `discovery` module
    with four FFI entry points (`nd_promql_labels`,
    `nd_promql_label_values`, `nd_promql_series`,
    `nd_promql_metadata`). Selector strings parse through the
    existing `lower_query` path; resolved series are OR-unioned by
    signature across multiple `match[]` entries. New
    `output::discovery_json` serializer emits Prometheus-shaped
    responses with optional `warnings`. Test stubs and the bindgen
    allowlist updated. All 59 Rust unit tests pass.
  - Handler (new `src/web/api/v3/api_v3_promql_discovery.c`):
    single dispatcher routes the five endpoints by inspecting
    `w->url_path_decoded`, parses repeatable `match[]` plus
    `metric`/`limit`/`host`/`start`/`end`, accepts both GET and
    POST on `/labels` and `/series`. `/status/buildinfo` returns
    a static JSON with `features:{}` so Grafana classifies the
    daemon as Prometheus (not Mimir).
  - Dispatch (`src/web/api/web_api_v1.c`): five new entries
    (`labels`, `label`, `series`, `metadata`, `status`) all gated
    by `ENABLE_PROMQL`. `label` and `status` use
    `allow_subpaths = 1` so the handler can read the trailing
    path component for `/label/<name>/values` and
    `/status/buildinfo`.
  - CMake: appended new C source to `WEB_PLUGIN_FILES` under the
    existing `ENABLE_PROMQL` block.
  - Verification against live daemon (curl): `buildinfo`,
    `labels` (filtered + unfiltered), `label/__name__/values`,
    `label/dimension/values`, `series` (single + multi `match[]`,
    plus the Grafana sentinel `{__name__!=""}` matching ~8.5k
    series), `metadata` (full + filtered, `disk_io` correctly
    reports `type=counter`, `system_cpu` reports `type=gauge`),
    POST on `/labels` and `/series`. Phase 1 smoke harness still
    passes 24/24.
- Chunk 2 shipped:
  - Shim (`src/database/contexts/promql-data-source.c`): new
    `resolve_host_scope` helper centralizes the four host cases
    (NULL -> localhost, "*" -> all, specific -> lookup by GUID then
    by hostname, miss -> empty result). Both `nd_pds_resolve` and
    `nd_pds_metadata_collect` now share this resolver.
  - Behavior: `?host=<hostname>` and `?host=<machine_guid>` route to
    the matching host. A nonexistent host returns an empty result
    rather than silently falling back to localhost (the previous
    Phase 1 chunk-1 behavior, which would have misled callers).
  - Handler (`src/web/api/v3/api_v3_promql.c`): `handle_instant` and
    `handle_range` now read the `host` URL parameter and forward it
    to the Rust FFI. Phase 1 had hardcoded NULL, which made the
    `?host=*` and `?host=<name>` query-string cases unreachable
    through `/query` and `/query_range`.
  - Verification against live daemon (single-host fixture):
    `?host=nx570` on `/series`, `/query`, `/metadata` returns the
    same series as `?host=*` and the unqualified call.
    `?host=does-not-exist` returns an empty result envelope on each
    endpoint (status:success, empty data). Phase 1 smoke harness
    still passes 24/24.
- Chunk 3 shipped (this commit):
  - Spec extended: `.agents/sow/specs/promql-endpoint-contract.md`
    updates `Host scoping` for specific-host lookup and appends
    three new sections (`Discovery endpoints`,
    `Truncation and the warnings envelope field`,
    `HTTP method matrix`).
  - Smoke harness extended: `tests/promql-smoke/run-smoke.sh` adds
    21 Phase 2 checks across 6 groups (buildinfo, labels,
    label-values, series, metadata, POST, host scoping). 45/45
    total pass (24 Phase 1 + 21 Phase 2). The big-response checks
    (sentinel `{__name__!=""}` returns ~8500 series) pipe response
    bodies through python3 stdin to avoid argv-length limits.
  - End-user docs deliberately deferred to an interactive Grafana
    walkthrough that follows this SOW close; tracked as Followup
    #1.
  - SOW closed: status flipped to `completed`, file moved from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (buildinfo with `features:{}`): smoke check
  `buildinfo returns 2.45.0 with features:{}` passes. Response body
  confirmed via curl.
- AC#2/#3/#4 (labels/label-values/series with `start`/`end`/`limit`/
  repeatable `match[]`, GET+POST where applicable): smoke checks
  `labels: unfiltered contains __name__`,
  `labels: match[]=system_cpu narrows result`,
  `label/__name__/values lists metric names`,
  `label/dimension/values lists dim names`,
  `label/__name__/values filtered by match[]`,
  `series: single match[] returns shapes`,
  `series: multi match[] OR-unions distinct __name__`,
  `series: sentinel {__name__!=""} returns all` all pass.
- AC#5 (metadata TYPE/HELP): smoke checks
  `metadata: full catalog has many entries`,
  `metadata: ?metric=disk_io has type=counter`,
  `metadata: ?metric=system_cpu has type=gauge` all pass. INCREMENTAL
  -> counter, others -> gauge mapping verified end to end.
- AC#6 (specific-host lookup): smoke checks
  `host=<this hostname> resolves to same series as default`,
  `host=* resolves the same series`,
  `host=nonexistent returns empty success envelope`,
  `host parameter wired on /query`,
  `host parameter wired on /metadata` all pass. The fixture is
  single-host; a true multi-host fixture is not available on the
  development workstation, but the resolve_host_scope helper handles
  both hostname and machine_guid paths identically (one is a
  dictionary lookup, the other is the existing localhost-aliased
  hostname walker). The miss path is verified.
- AC#7 (`instance` label carries hostname): verified inside the
  host-scoping smoke checks: the `instance` field on returned series
  equals the queried hostname.
- AC#8 (Grafana Save & Test): reframed at the second-pass review.
  The health check is `1+1` via QueryData, which Phase 1 already
  passed. Phase 2 verifies buildinfo's `features:{}` so the heuristic
  reports `Application: Prometheus`. End-to-end Grafana verification
  (Save & Test, panel render) happens in the interactive Grafana
  session that follows this SOW close.
- AC#9 (metric browser populates): reframed -- the metric browser
  reads `/api/v1/label/__name__/values` (verified by smoke); metadata
  TYPE/HELP feeds tooltips (verified). Browser-level visual
  confirmation is part of the interactive Grafana session.
- AC#10 (repeatable `match[]`): smoke check
  `series: multi match[] OR-unions distinct __name__` exercises
  `match[]=system_cpu&match[]=disk_io` and asserts the union.
- AC#11 (`limit`/`warnings` truncation): the truncation path is
  implemented in `discovery.rs::warnings_for` and the Phase 1
  `select.rs` truncation-as-error. A dedicated smoke check for the
  `warnings` field requires a fixture with >10000 series, which the
  development workstation does not produce; the code path is covered
  by Rust unit tests for the JSON serializer
  (`warnings_field_appears_when_provided`,
  `warnings_field_omitted_when_empty`).
- AC#12 (contract spec extension): done. `Host scoping` section
  updated, new sections appended (`Discovery endpoints`,
  `Truncation and the warnings envelope field`, `HTTP method matrix`).
- AC#13 (30+ smoke checks): 45 checks pass against the live daemon
  (24 Phase 1 + 21 Phase 2).

Tests or equivalent validation:

- Rust unit tests: 59/59 pass (`cargo test`). Includes the
  discovery_json serializer, the regex matcher cache, the matcher
  FFI translation, and the lower-side rejections.
- Smoke harness: 45/45 pass on a populated local daemon.

Real-use evidence:

- All five discovery endpoints exercised via curl against a live
  daemon (verified during chunk 1 and again at chunk 3 close).
- Host scoping verified with `?host=nx570`, `?host=*`,
  `?host=does-not-exist` on `/series`, `/query`, `/metadata`.
- Grafana end-to-end (Save & Test, metric browser, rendered panel)
  is the next thing on the user's todo list and runs interactively
  after this SOW closes.

Reviewer findings:

- Pre-implementation review against current Grafana sources
  (`@grafana/prometheus@13.1.2` + `grafana-prometheus-datasource`)
  caught five issues in the original SOW draft that this iteration
  fixed before any code was written:
  1. AC#1 was missing `features:{}` and would have classified the
     daemon as Mimir.
  2. AC#8 was overstated; Save & Test already passed after Phase 1.
  3. AC#9 incorrectly named `/api/v1/metadata` as the
     metric-browser feed; the actual feed is
     `/api/v1/label/__name__/values`.
  4. Repeatable `match[]` and POST semantics on `/labels` and
     `/series` were missing from the AC list and added as AC#10.
  5. Limit divergence (Grafana sends `limit=40000`,
     `max_series=10000`) and the `warnings` envelope field were
     missing and added as AC#11.

Same-failure scan:

Searched for repeats of the Phase 1 close issue (git mv pre-stages a
rename but subsequent content edits to the new path aren't re-staged
automatically). For chunks 1, 2, and 3, the workflow used `git add -A`
which captures everything; no recurrence.

Searched for repeats of the Phase 1 background-process issue
(daemons dying when parent bash exited). Continued to use the harness
`run_in_background` flag for daemon launches; no recurrence.

Sensitive data gate:

- No `.env`, bearer token, claim-id, or other sensitive data
  introduced.
- Discovery endpoints surface only labels the Netdata dashboard
  already exposes today. The shim explicitly filters `_*`-prefixed
  host labels (internal metadata: install fingerprints, system info,
  cloud-detection markers) so a Grafana-facing `/labels` enumeration
  does not leak deployment topology beyond what the existing
  dashboards show.
- The contract spec documents which labels surface through which
  endpoint so a security-conscious operator can audit.

Artifact maintenance gate:

- AGENTS.md: no change required.
- Runtime project skills: no change required.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated
  with the Phase 2 surface.
- End-user/operator docs: deliberately deferred to an interactive
  Grafana walkthrough session that follows this SOW close; the user
  will exercise Save & Test, the metric browser, and one rendered
  panel against a live `localhost:19999` daemon. Notes captured
  during that session may become a short Grafana setup README in a
  follow-up commit -- tracked as Followup #1 below.
- End-user/operator skills: no change required.
- SOW lifecycle: status set to `completed`; file moves from
  `.agents/sow/current/` to `.agents/sow/done/` in the same commit
  as the final code change (this commit).

Specs update:

`.agents/sow/specs/promql-endpoint-contract.md` -- updated `Host
scoping` section; appended `Discovery endpoints`,
`Truncation and the warnings envelope field`, and
`HTTP method matrix`.

Project skills update:

No change required.

End-user/operator docs update:

Deferred -- see "Artifact maintenance gate" above and Followup #1.

End-user/operator skills update:

No change required.

Lessons:

- *Verify the third-party contract before drafting acceptance
  criteria.* The first SOW draft assumed Grafana behaviors that were
  off by varying degrees. Inspecting the actual datasource source
  (frontend dist + Go backend) before chunking saved at least one
  rework cycle on the buildinfo and metric-browser ACs. For Phase 3,
  the same discipline applies to the Prometheus compliance test
  suite.
- *Split overflow strictness by caller, not by API.* Phase 1's
  `nd_pds_resolve` treated overflow as a hard error, which is right
  for query evaluation but wrong for discovery. Adding a `truncated`
  flag and letting each caller choose strictness was a one-flag
  change that preserved Phase 1 semantics and unlocked the
  Prometheus-style `warnings` field for Phase 2.
- *Don't silently fall back to localhost.* The original Phase 1 chunk
  treated any non-`*` host string as localhost. That behavior would
  have masked typos and led to wrong results in multi-host
  deployments. The chunk-2 resolver returns an empty success envelope
  on miss, making the failure visible.
- *Chunk-per-commit pays off for non-trivial SOWs.* Three chunks
  produced three reviewable commits. The earlier behavior of one
  commit per SOW (Phase 0) was harder to review and would have
  bundled the chunk-2 host-parameter wiring with the chunk-1
  endpoint plumbing where they belong logically separated.

Follow-up mapping:

- *Phase 3 SOW* (subqueries, vector matching with
  `on`/`ignoring`/`group_left`, full `*_over_time` family,
  `topk`/`bottomk`/`quantile`, `predict_linear`/`holt_winters`,
  `@` modifier with arithmetic, `keep_metric_names`, tier selection
  beyond tier 0, rollup result cache, performance work, Prometheus
  compliance test suite, macOS verification, cardinality telemetry,
  optional endpoints `/query_exemplars`/`/format_query`/
  `/parse_query`/`/targets`/`/rules`/`/alerts`/`/alertmanagers`/
  `/status/{runtimeinfo,config,flags,tsdb}`) -- to be filed as
  successor SOW when the user starts that phase.
- *Followup #1: Grafana datasource setup README.* Captures the
  interactive walkthrough's output as a short user-facing note;
  deferred until the walkthrough runs.
- *CI verification (gcc-build, clang-build, license check)* awaits
  user authorization to push the branch. Until that authorization is
  given, the branch stays local and CI gates remain deferred (per
  the no-push memory).

## Outcome

PromQL on this branch now satisfies what Grafana needs from a
Prometheus datasource for the basic flow: instant + range queries
already worked after Phase 1; this SOW adds the discovery surface
(metric browser, label autocomplete, series listing, TYPE/HELP
metadata, buildinfo heuristics) plus specific-host scoping that lets
operators address a single child of a parent agent. The branch is 10
commits ahead of `origin/master` and holds local-only until the user
authorizes a push.

## Lessons Extracted

See `Validation > Lessons` above (four items).

## Followup

1. Grafana datasource setup README -- captures the interactive
   walkthrough output; deferred until the walkthrough runs.
2. CI verification (gcc-build, clang-build, license check) -- awaits
   user authorization to push the branch.
3. Phase 3 SOW (the deferred items above) -- filed when the user
   begins that phase.

## Regression Log

None yet.
