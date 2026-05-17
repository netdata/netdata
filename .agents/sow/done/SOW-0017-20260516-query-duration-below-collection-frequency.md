# SOW-0017 - Query duration below collection frequency returns no data

## Status

Status: completed

Sub-state: completed on 2026-05-17 after implementation, installation, focused unit validation, and live direct-agent API validation.

## Requirements

### Purpose

Make Netdata metric queries fit for troubleshooting narrow historical windows: when data exists in the requested time area, querying a duration shorter than the metric collection frequency must not silently return an empty result only because the requested window sits between collection-aligned timestamps.

### User Request

The user reported a hypothesis:

- Metrics such as SNMP or go.d charts are often collected every 10 seconds.
- A 10-minute query with 60 points should show the 10-second samples exist.
- A 5-second absolute query around 5 minutes in the past, fully inside one 10-second alignment interval, returns empty data even though nearby data exists.
- The first step must verify the hypothesis before creating a SOW or implementing anything.

### Assistant Understanding

Facts:

- Local Netdata Agent was reachable at `127.0.0.1:19999`.
- Local Netdata Agent version was reported by `/api/v1/info` as `v2.10.0-199-g532b0ceef2`.
- `/api/v3/contexts` does not expose collection `update_every`; it exposes context metadata, labels, dimensions, instances, retention, and liveness depending on options.
- `/api/v3/contexts?contexts=smartctl.device_temperature&options=labels,instances,retention,liveness,minify` confirmed the selected context is collected by `_collect_plugin=go.d` and `_collect_module=smartctl`.
- `/api/v1/context?context=smartctl.device_temperature&options=instances` confirmed instances of `smartctl.device_temperature` have `update_every=10`.
- Direct local `/api/v3/data` in this checkout is query-string driven; POSTing the JSON body documented by the skill was ignored for local direct-agent calls.

- The verified symptom is in automatic tier selection/planning, not in collector emission or storage execution, because forced `tier=0` and forced `tier=1` return data for the same narrow window.
- The empty result is not caused by retention absence: tier 0 retention covers the tested timestamps.
- The broader hypothesis that any tier-2-only query shorter than tier-2 resolution returns empty was not reproduced on the local agent. Tier-2-only 5-minute windows returned data for both `system.cpu` and `smartctl.device_temperature`.

Unknowns:

- Relative-window variants were not exhaustively tested. The traced trigger is based on the effective window after query-window normalization, so any request form that produces the same effective window can hit it.

### Acceptance Criteria

- Root cause is traced in code with file:line evidence before patching.
- `/api/v3/data` returns meaningful data for a historical sub-frequency window when a sample exists in the surrounding collection interval, or a deliberate product decision explains why it should not.
- Existing aligned 10-second queries for 10-second go.d metrics continue to return all expected points.
- Tests cover the empty-result regression using a lower-frequency metric or a query-engine fixture.
- Validation records exact commands/results for baseline, failing pre-fix case, and fixed post-fix case.
- Public/end-user skill documentation is changed only if a user/operator workflow changes; internal query-planner findings stay in SOW/spec artifacts.

## Analysis

Sources checked:

- `docs/netdata-ai/skills/query-netdata-agents/SKILL.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-metrics.md`
- `src/web/api/v2/api_v2_contexts.c`
- `src/database/contexts/api_v2_contexts.c`
- `src/web/api/maps/contexts_options.c`
- `src/database/contexts/api_v1_contexts.c`
- `src/web/api/v2/api_v2_data.c`
- `src/web/api/queries/query-window.c`
- `src/web/api/queries/query-plan.c`
- `src/database/contexts/query_target.c`
- `src/database/contexts/rrdcontext.h`
- Local Netdata Agent API at `127.0.0.1:19999`

Current state:

- v3 context metadata can identify go.d/SNMP-style collector ownership via labels, but not collection cadence.
- v1 context metadata exposes per-instance `update_every`.
- v3 data responses expose `db.per_tier[].update_every`, which is database tier resolution for the query, not general context metadata.
- Direct local v3 data request parsing reads URL parameters in `api_v23_data_internal()`; the parser does not read the POST JSON body in the inspected path.

Risks:

- A query-window fix may affect all `/api/v2/data` and `/api/v3/data` users, including Cloud-proxied metrics queries.
- A naive widening fix could return data outside the user's requested interval and surprise callers expecting strict time boundaries.
- A strict no-widening interpretation preserves mathematical precision but makes sub-frequency historical inspection unreliable for real troubleshooting workflows.
- Tier-selection behavior may differ across tier 0, tier 1, and tier 2, so tests must not cover only tier 0.

## Pre-Implementation Gate (Historical Snapshot at Implementation Start)

Status: diagnosis complete for the verified local reproduction

Problem / root-cause model:

- Verified symptom: for a 10-second go.d metric, every tested 5-second absolute query across a full 10-second cadence cycle selects the right target objects but returns no data and performs zero tier queries, while adjacent/wider windows return data from tier 0.
- Corrected phase split: query-target selects the matching context, instances, and dimension; the queryable metric / planner path does not create any tier plan for them.
- Root-cause model: automatic per-metric tier selection gives every overlapping tier a sentinel "unusable" weight when the requested effective duration is shorter than the tier update interval. The same sentinel is also used for non-overlapping tiers. The `>=` tie-break then selects the highest numbered tier among tied sentinel weights.
- Forced-tier controls prove the query engine can answer the exact narrow window when the planner is told to use a covering tier.
- The empty result is only one outcome of the trigger. If the highest tied tier covers the requested window, the query returns data from that higher/coarser tier. If the highest tied tier does not cover the requested window, planning returns false and the response is empty.

Evidence reviewed:

- v3 collector identity:
  - Request: `/api/v3/contexts?contexts=smartctl.device_temperature&options=labels,instances,retention,liveness,minify`
  - Result: selected context has `_collect_plugin=go.d` and `_collect_module=smartctl`.
- v1 update frequency:
  - Request: `/api/v1/context?context=smartctl.device_temperature&options=instances`
  - Result: `smartctl.device_temperature` instances include `update_every=10`.
- Baseline 10-minute query:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958670&before=1778959270&points=60&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - Result: `result.data` length was `60`; first timestamp `1778959270`; last timestamp `1778958680`; tier 0 `update_every=10`.
- Failing 5-second sub-frequency query around 5 minutes in the past:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958983&before=1778958988&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - Result: `result.data` length was `0`; `db.per_tier[0].queries=0`; tier 0 retention covered the window.
- Same failing window with `points=1` and with default points also returned `result.data` length `0`.
- Adjacent wider window:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958980&before=1778958990&points=1&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - Result: `result.data` length was `1`, with a sample at timestamp `1778958990`.
- Wider 30-second window:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958970&before=1778959000&points=3&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - Result: `result.data` length was `3`, with timestamps `1778959000`, `1778958990`, and `1778958980`.
- Full 10-second shifted 5-second-window scan:
  - Common request shape: `/api/v3/data?contexts=smartctl.device_temperature&after=<after>&before=<before>&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - `0-5`: `after=1778958980`, `before=1778958985` -> `result.data` length `0`, tier 0 `queries=0`.
  - `1-6`: `after=1778958981`, `before=1778958986` -> `result.data` length `0`, tier 0 `queries=0`.
  - `2-7`: `after=1778958982`, `before=1778958987` -> `result.data` length `0`, tier 0 `queries=0`.
  - `3-8`: `after=1778958983`, `before=1778958988` -> `result.data` length `0`, tier 0 `queries=0`.
  - `4-9`: `after=1778958984`, `before=1778958989` -> `result.data` length `0`, tier 0 `queries=0`.
  - `5-0`: `after=1778958985`, `before=1778958990` -> `result.data` length `0`, tier 0 `queries=0`.
  - `6-1`: `after=1778958986`, `before=1778958991` -> `result.data` length `0`, tier 0 `queries=0`.
  - `7-2`: `after=1778958987`, `before=1778958992` -> `result.data` length `0`, tier 0 `queries=0`.
  - `8-3`: `after=1778958988`, `before=1778958993` -> `result.data` length `0`, tier 0 `queries=0`.
  - `9-4`: `after=1778958989`, `before=1778958994` -> `result.data` length `0`, tier 0 `queries=0`.
- Planner/debug output:
  - `options=debug` and `options=plan` both map to `RRDR_OPTION_DEBUG` in `src/web/api/maps/rrdr_options.c`.
  - v3 emits request/debug fields with `options=jsonwrap,debug`.
  - v3 emits per-metric `plans` only inside `detailed` output with debug, under `detailed.nodes...dimensions.<metric>.plans`.
  - The failing 5-second query has no selected dimensions, so there are no per-metric plans to print.
  - A nearby non-empty query (`after=1778958980`, `before=1778958990`, `points=1`, `options=jsonwrap,debug,details`) prints per-metric plans such as tier 0 `af=1778958985`, `bf=1778958995`.
- Corrected interpretation from the failing debug response:
  - `summary` and `totals` prove the matching target objects were selected.
  - The failing response has no query-plan objects and `db.per_tier[0].queries=0`, proving no tier query was initialized.
  - Therefore the failure is after target-object selection and before storage execution.
- Tier-2-only hypothesis check, using old data where only tier 2 covered the selected window:
  - Baseline tier-retention request: `/api/v3/data?contexts=system.cpu&after=0&before=0&points=1&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,debug&timeout=30000`
  - Result: `system.cpu` had tier 0 retention beginning at `1778940602`, tier 1 retention beginning at `1778817840`, and tier 2 retention beginning at `1778086800`.
  - One-hour tier-2-only baseline: `/api/v3/data?contexts=system.cpu&after=1778798400&before=1778802000&points=1&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `1`; tier 2 `queries=153`; tier 0 and tier 1 `queries=0`.
  - Five-minute tier-2-only query: `/api/v3/data?contexts=system.cpu&after=1778800200&before=1778800500&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; timestamps were `1778800500`, `1778800440`, `1778800380`, `1778800320`, `1778800260`; tier 2 `queries=153`; tier 0 and tier 1 `queries=0`.
  - Five-second tier-2-only query: `/api/v3/data?contexts=system.cpu&after=1778800203&before=1778800208&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; timestamps were `1778800208`, `1778800207`, `1778800206`, `1778800205`, `1778800204`; tier 2 `queries=153`; tier 0 and tier 1 `queries=0`.
  - Five-minute tier-2-only query for the original go.d context: `/api/v3/data?contexts=smartctl.device_temperature&after=1778800200&before=1778800500&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; tier 2 `queries=18`; tier 0 and tier 1 `queries=0`.
  - Five-second tier-2-only query for the original go.d context: `/api/v3/data?contexts=smartctl.device_temperature&after=1778800203&before=1778800208&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; timestamps were `1778800208`, `1778800207`, `1778800206`, `1778800205`, `1778800204`; tier 2 `queries=18`; tier 0 and tier 1 `queries=0`.
  - Finding: the broad tier-2 hypothesis was not reproduced. Sub-resolution tier-2-only windows can return data when auto-selection lands on tier 2 and tier 2 covers the window.
- Forced-tier controls for the original failing 5-second query:
  - Auto-tier failing request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958983&before=1778958988&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `0`; no per-metric plans; all `db.per_tier[].queries=0`; tier 0 and tier 1 retention covered the window, tier 2 did not.
  - Forced tier 0 request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958983&before=1778958988&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&tier=0&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; tier 0 `queries=18`; per-metric plans used tier 0.
  - Forced tier 1 request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958983&before=1778958988&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&tier=1&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; tier 1 `queries=18`; per-metric plans used tier 1.
  - Forced tier 2 request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958983&before=1778958988&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&tier=2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `0`; no per-metric plans; tier 2 did not cover the window.
- Wrong-tier-but-not-empty control where tier 0, tier 1, and tier 2 all covered the narrow window:
  - Auto-tier request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778935003&before=1778935008&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; tier 2 `queries=7`; tier 0 and tier 1 `queries=0`; per-metric plans used tier 2.
  - Forced tier 0 request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778935003&before=1778935008&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&tier=0&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length was `5`; tier 0 `queries=3`; per-metric plans used tier 0.
  - Finding: when all tiers overlap but the effective window is shorter than every tier update interval, auto-selection picks the highest tier instead of the most detailed covering tier. This may return data, but it is still the wrong tier choice for a narrow troubleshooting window.
- Code evidence:
  - `src/database/contexts/query_target.c:325` to `src/database/contexts/query_target.c:328` uses broad retention matching to decide whether a metric can enter the query target.
  - `src/database/contexts/rrdcontext.h:721` to `src/database/contexts/rrdcontext.h:723` allows a two-`update_every` tolerance when checking retention overlap.
  - `src/web/api/queries/query-plan.c:30` to `src/web/api/queries/query-plan.c:36` returns `-LONG_MAX` for invalid/no-overlap tiers.
  - `src/web/api/queries/query-plan.c:44` to `src/web/api/queries/query-plan.c:52` also returns `-LONG_MAX` when `points_available <= 0`, which happens when the requested window is shorter than the tier update interval.
  - `src/web/api/queries/query-plan.c:90` to `src/web/api/queries/query-plan.c:99` marks non-overlapping tiers with the same `-LONG_MAX` value.
  - `src/web/api/queries/query-plan.c:108` to `src/web/api/queries/query-plan.c:112` chooses the later tier on equal weights because it uses `>=`.
  - `src/web/api/queries/query-plan.c:117` to `src/web/api/queries/query-plan.c:176` has a separate all-invalid fallback to tier 0 for natural update-every selection, but the per-metric selector at `src/web/api/queries/query-plan.c:57` to `src/web/api/queries/query-plan.c:115` does not have the same fallback.
  - `src/web/api/queries/query-plan.c:361` to `src/web/api/queries/query-plan.c:365` returns false if the selected tier does not cover the requested window, so no plan reaches `query_planer_initialize_plans()`.

Trigger conditions:

1. The request does not explicitly select a tier, so `query_metric_best_tier_for_timeframe()` is used.
2. More than one storage tier exists.
3. After `query_target_calculate_window()`, the effective planner window is shorter than the update interval of every tier that overlaps it:
   - for the verified failing request, the API request was `after=1778958983`, `before=1778958988`, `points=5`;
   - query-window normalized it to `after=1778958984`, `before=1778958988`, so the planner duration was `4` seconds for `5` output slots;
   - tier 0 update interval for the metric was `10` seconds, tier 1 was `600` seconds, and tier 2 was `36000` seconds.
4. Because `points_available = (common_last_t - common_first_t) / db_update_every_s`, each overlapping tier with an update interval larger than the effective duration gets `points_available=0` and weight `-LONG_MAX`.
5. Non-overlapping tiers also get weight `-LONG_MAX`.
6. The tie-break uses `>=`, so the highest-numbered tied tier is selected.
7. Outcome split:
   - if the selected highest tier covers the window, the query returns data from that tier;
   - if the selected highest tier does not cover the window, `query_plan()` returns false, no tier query is initialized, and the response is empty.

Tier switching behavior:

- `query_plan()` can switch tiers in both directions in the same query, but only around the initially selected tier and only to fill coverage gaps.
- If the selected tier starts after the requested `after`, the planner searches higher-numbered tiers (`selected_tier + 1` upward) to fill the older/beginning part of the query.
- If the selected tier ends before the requested `before`, the planner searches lower-numbered tiers (`selected_tier - 1` downward) to fill the newer/end part of the query.
- Both checks are independent, so a middle selected tier can have coarser plans prepended and finer plans appended in the same query.
- The plan entries are sorted by start time before execution.
- Limitation: switching is not a per-segment resolution optimizer. If the selected tier covers the whole requested window, the planner does not split the query just because another tier would provide better point density for a subsection.
- Limitation: explicit `tier=` disables switching because `switch_tiers=false`.
- Limitation relevant to the verified bug: the selected tier must overlap the requested window before switching can help. If automatic selection picks a non-overlapping tier, `query_plan()` returns false before the gap-filling loops run.
- Code evidence:
  - `src/web/api/queries/query-plan.c:367` to `src/web/api/queries/query-plan.c:370` starts with one selected-tier plan clipped to that tier's retention.
  - `src/web/api/queries/query-plan.c:377` to `src/web/api/queries/query-plan.c:408` fills the beginning by scanning higher-numbered tiers.
  - `src/web/api/queries/query-plan.c:411` to `src/web/api/queries/query-plan.c:445` fills the end by scanning lower-numbered tiers.
  - `src/web/api/queries/query-plan.c:448` to `src/web/api/queries/query-plan.c:450` sorts multiple plan entries by start time.
  - `src/web/api/queries/query-plan.c:349` to `src/web/api/queries/query-plan.c:353` disables switching for explicit selected-tier requests.

Related tier-selection functions:

- `query_metric_best_tier_for_timeframe()` is the execution planner selector. It is `static` and is called only by `query_plan()` when no explicit `tier=` is selected.
- This execution selector already partially matches the desired model:
  - it excludes zero-overlap tiers before scoring;
  - it computes `min_first_time_s` and `max_last_time_s` across all tiers;
  - it passes that union range plus each tier's `db_update_every_s` into `query_plan_points_coverage_weight()`;
  - therefore each overlapping tier is effectively scored for resolution as if it could cover the full union/requested window.
- The verified bug is in the scoring and tie-break, not in the gap-fill planner:
  - sub-resolution tiers collapse to `-LONG_MAX`;
  - the `+25000 * tier` bias and `>=` tie-break favor higher/coarser tiers.
- `rrddim_find_best_tier_for_timeframe()` is a separate aggregate helper in the same file. It also uses `query_plan_points_coverage_weight()`, but it is not the execution planner selector.
- The only call to `rrdset_find_natural_update_every_for_timeframe()` is from `query_target_calculate_window()` when `natural_points`, explicit `selected-tier`, and `tier > 0` are all set. Because the same `selected-tier` option is passed through, the aggregate `rrddim_find_best_tier_for_timeframe()` branch is not reached from the current code path.
- Implication: changing the shared `query_plan_points_coverage_weight()` is less surgical because it also changes the aggregate helper if that helper becomes reachable later. Changing `query_metric_best_tier_for_timeframe()` is the narrow execution-planner change.
- API/data paths using automatic execution planner selection:
  - `/api/v1/data`, `/api/v2/data`, and `/api/v3/data` when no explicit `tier=` is provided;
  - MCP metric queries when no explicit `tier` parameter is provided;
  - weights queries and value helper paths that call `rrd2rrdr()` without `RRDR_OPTION_SELECTED_TIER`.
- Paths verified not to depend on automatic tier selection:
  - health database lookups pass `points=1` and `RRDR_OPTION_SELECTED_TIER` with tier `0`;
  - exporting reads storage tier 0 directly through `storage_engine_query_init(rd->tiers[0]...)`;
  - explicit API/MCP `tier=` requests bypass automatic selection and disable tier switching.

Why the aggregate helper path exists:

- Historical evidence: commit `3fefd03b94458c9f6ad4164a82b1da6fc4fa435c` (`automatic selection of tier`) introduced automatic tier selection in the old single-chart query engine.
- In that first design, `rrddim_find_best_tier_for_timeframe()` served two real purposes:
  - it selected the execution tier for `rrd2rrdr_do_dimension()`;
  - it selected the natural-points `update_every` through `rrdset_find_natural_update_every_for_timeframe()` whenever natural points and multiple storage tiers were available.
- Historical evidence: commit `41e14c83e22ed54c8e48a7b637315bbb556c3185` (`natural points should only be offered on tier 0, except a specific tier is selected`) changed the natural-points call site from `natural_points && storage_tiers > 1` to `natural_points && selected-tier && tier > 0 && storage_tiers > 1`.
- That change made the automatic natural-points branch effectively unreachable, because `rrdset_find_natural_update_every_for_timeframe()` is now only called when selected-tier is already set, and therefore it returns the explicit tier instead of calling `rrddim_find_best_tier_for_timeframe()`.
- Commit `00712b351b3c83a54a147ca23365458acbef3105` (`QUERY_TARGET: new query engine for Netdata Agent`) ported the logic into the query-target engine:
  - execution-tier selection became `query_metric_best_tier_for_timeframe()`;
  - aggregate natural update-every selection remained as `rrddim_find_best_tier_for_timeframe()` behind `rrdset_find_natural_update_every_for_timeframe()`.
- Implication: the other path exists because it is legacy from the original natural-points auto-tier design and was preserved through the query-target port, but the current call site no longer exercises its automatic branch.
- The user's concern is valid: if a future caller exercises the aggregate helper, it does not have the execution planner's gap-fill semantics. It chooses one tier/update_every for the query window, so coverage-sensitive scoring there could produce poor natural-points granularity decisions unless it is reviewed separately.

Affected contracts and surfaces:

- `/api/v2/data` and `/api/v3/data` query behavior.
- Cloud-proxied metrics queries, if they use the same agent-side query path.
- Query target preparation, storage tier selection, retention matching, and data grouping semantics.
- Public AI skills under `docs/netdata-ai/skills/` are affected only if the user/operator query workflow changes. Internal planner diagnostics are out of scope for those artifacts.

Existing patterns to reuse:

- Existing query target and data API test patterns after they are located.
- Existing `RRDR_OPTION_UNALIGNED` behavior must be preserved.
- Existing v2/v3 data query-string parser in `src/web/api/v2/api_v2_data.c`.

Risk and blast radius:

- High enough to require focused tests before patching: data query semantics are user-facing and shared across dashboards, APIs, Cloud, and troubleshooting workflows.
- Security risk is low for the bug itself; evidence must still avoid raw secrets and sensitive label values.
- Performance risk exists if a fix expands storage scans for many narrow queries.

Sensitive data handling plan:

- Durable artifacts must not include raw secrets, bearer tokens, SNMP communities, customer identifiers, private endpoints, non-private customer-identifying IPs, personal data, or device serial numbers.
- The verification API responses contained hardware labels. This SOW records only sanitized collector identity and metric names, not raw serial numbers or full hardware-identifying label values.
- Future logs or traces added to this SOW must be summarized or redacted before being written.

Implementation plan:

1. Choose the tier-selection fix semantics before patching.
2. Implement the smallest fix in the automatic per-metric tier selector so a covering tier is selected for sub-resolution windows.
3. Add regression tests for sub-frequency historical windows, forced-tier behavior, tier-2-only sub-resolution windows, and unchanged aligned-window behavior.
4. Re-run the exact local API verification commands after the fix.
5. Update query skill/docs if the local direct-agent `/api/v3/data` request contract differs from the current skill text.

Validation plan:

- Run targeted query-engine/data API tests found during tracing.
- Add a regression test that fails before the fix.
- Re-run the exact local API verification commands after the fix.
- Search for same-failure risks around tier 1/tier 2, relative windows, absolute windows, `points=0`, `points=1`, and `points>duration`.

Artifact impact plan:

- AGENTS.md: likely unaffected unless the fix exposes a durable project-wide query workflow rule.
- Runtime project skills: likely unaffected unless a codebase workflow lesson emerges.
- Specs: likely add or update a query/data API spec if no existing spec covers sub-frequency windows.
- End-user/operator docs: possibly affected if public query semantics are documented.
- End-user/operator skills: no planned update for internal query-planner diagnostics. The user clarified that `docs/netdata-ai/skills/` is for user/operator how-tos, not maintainer implementation notes.
- SOW lifecycle: this SOW moved to `current/` after the user explicitly prioritized it, and will move to `done/` with `Status: completed` when committed.

Open-source reference evidence:

- None checked. This is an internal Netdata query-engine/API behavior issue; external observability implementations are not needed until a semantic design fork appears.

Open decisions:

- None before implementation.

## Implications And Decisions

- User decision already applied: verify the hypothesis before creating this SOW.
- User decision on 2026-05-17: choose option `1A`, cleanup the dormant natural-points aggregate tier-selection path instead of leaving it commented or reactivating it.
- Implication: implementation should remove or simplify the unreachable `rrddim_find_best_tier_for_timeframe()` path so future maintainers do not confuse it with the execution planner selector.
- User decision on 2026-05-17: implement the planner change in addition to cleanup.
- User-approved planner semantics: zero-overlap tiers are non-candidates; among overlapping tiers, score point density as if the tier had full-window coverage; select the sparsest tier that can provide at least 50% of requested point density; if no tier can provide 50%, select the densest overlapping tier.
- Boundary: existing explicit `tier=` semantics remain unchanged.
- User clarification on 2026-05-17: do not put internal/developer query-planner findings in `docs/netdata-ai/skills/`; use `.agents/sow/` specs or project runtime skills for maintainer-facing memory.

## Plan

1. Keep unrelated SOWs unchanged and complete the SOW-0017 lifecycle.
2. Remove or simplify the dormant natural-points aggregate tier-selection path.
3. Replace automatic execution-tier scoring with the user-approved point-density semantics.
4. Add focused regression tests or equivalent validation.
5. Re-run local API reproduction URLs after installing/restarting a patched agent, or record the approval blocker.
6. Update internal artifacts before close.

## Execution Log

### 2026-05-16

- Verified the symptom using local direct-agent API requests.
- Created this pending SOW after verification only.
- Verified that tier-2-only sub-resolution windows return data in the tested cases, so the broader tier-2 hypothesis was not reproduced.
- Traced the observed empty result to automatic tier selection choosing a non-covering tier when all tier weights tie at `-LONG_MAX`.
- Verified a second outcome of the same trigger: when all tiers cover a narrow sub-resolution window, automatic selection chooses tier 2 and returns data from tier 2 instead of tier 0.
- No code changes made.

### 2026-05-17

- Worked SOW-0017 after the user explicitly prioritized it; unrelated SOWs are outside this SOW's scope.
- Removed the dormant automatic aggregate-tier path:
  - deleted `query_plan_points_coverage_weight()`;
  - deleted `rrddim_find_best_tier_for_timeframe()`;
  - replaced `rrdset_find_natural_update_every_for_timeframe()` with `query_target_min_update_every_for_tier()`, which only computes the minimum update-every for an explicit selected tier.
- Implemented automatic execution-tier selection using the approved density model:
  - zero-overlap tiers get sentinel weight and cannot win;
  - candidate tiers use fixed-point point-density weights, so sub-resolution windows do not collapse to zero;
  - if any tier can provide at least 50% of requested point density, the sparsest acceptable tier wins;
  - if none can provide 50%, the densest overlapping tier wins.
- Hardened the automatic selector to treat reversed or zero-duration windows as invalid before adjusting `points_wanted`.
- Split pure plan-entry building from storage query initialization so unit tests can assert tier selection and head/tail gap-filling without mocking storage engines.
- Added `query_plan_unittest()` and `-W queryplantest` coverage for:
  - sub-resolution window with a non-overlapping coarser tier;
  - sub-resolution window where all tiers overlap;
  - 50% tolerance choosing a sparse acceptable tier;
  - exact 50% tolerance boundary;
  - under-resolution requests choosing the densest tier;
  - zero-overlap tiers not being candidates;
  - invalid duration returning the first working tier;
  - selected tier covering the full window with one plan;
  - coarser-tier head gap fill;
  - finer-tier tail gap fill;
  - simultaneous head and tail gap fill;
  - explicit selected tier disabling gap fill;
  - no-overlap planning failure;
  - selected-tier natural-points update-every cleanup.
- Added internal spec `.agents/sow/specs/query-planner-tier-selection.md`.
- Did not update `docs/netdata-ai/skills/`; those are end-user/operator skills and this work is maintainer/internal query-planner behavior.
- Ran `./install.sh` after explicit user request, and reran it after the final selector guard. Result: install completed and restarted the local `netdata`; non-fatal `git fetch -t` failed due local GitHub SSH permission, but the installer continued and completed.
- Verified the installed local agent reports `v2.10.0-215-ge6e45f29ee` at `/api/v1/info`.
- Ran `/usr/sbin/netdata -W queryplantest`; result: passed all six focused query planner checks.
- Re-ran the live direct-agent `/api/v3/data` baseline and short-window checks against the installed agent. The original failing 5-second automatic-tier query now returns 5 points and uses tier 0.

## Validation

Acceptance criteria evidence:

- Root cause was traced before patching and is recorded in the Pre-Implementation Gate with code evidence.
- The implementation changes `src/web/api/queries/query-plan.c` so sub-resolution overlapping tiers remain candidates with fractional density weights instead of being assigned the same sentinel as non-overlapping tiers.
- Explicit tier behavior remains unchanged: `query_plan()` still disables tier switching for valid `RRDR_OPTION_SELECTED_TIER`.
- Live post-fix API validation against the installed local agent passed. The original failing automatic-tier 5-second query now returns 5 points and initializes tier 0 storage queries.

Tests or equivalent validation:

- `git diff --check -- src/web/api/queries/query-plan.c src/web/api/queries/query-window.c src/web/api/queries/query-internal.h src/daemon/main.c`
  - Result: passed.
- `cmake --build build-clion --target netdata -j 8`
  - Result: configure/build did not reach the changed code. CMake attempted to refresh bundled Sentry crashpad content and failed fetching `mini_chromium` from the external submodule with HTTP 400.
- `cmake -S . -B .local/build-sow17 -G Ninja -DENABLE_SENTRY=OFF -DENABLE_ML=OFF -DCMAKE_BUILD_TYPE=Debug`
  - Result: failed because the fresh cache enabled `ENABLE_PLUGIN_XENSTAT=ON` and local `xenstat` dependencies were not available.
- `cmake -S . -B .local/build-sow17 -G Ninja -DENABLE_SENTRY=OFF -DENABLE_ML=OFF -DENABLE_PLUGIN_XENSTAT=OFF -DCMAKE_BUILD_TYPE=Debug`
  - Result: passed.
- `cmake --build .local/build-sow17 --target netdata -j 8`
  - Result: passed. The changed files `src/daemon/main.c`, `src/web/api/queries/query-window.c`, and `src/web/api/queries/query-plan.c` compiled and linked into `.local/build-sow17/netdata`.
- `.local/build-sow17/netdata -W queryplantest`
  - Result: passed all six focused query planner checks.
- `cmake --build .local/build-sow17 --target netdata -j 8` after extracting pure plan-entry building and expanding planner coverage
  - Result: passed; `src/web/api/queries/query-plan.c` rebuilt and linked.
- `.local/build-sow17/netdata -W queryplantest` after extracting pure plan-entry building and expanding planner coverage
  - Result: passed all fourteen focused query planner checks.
- `cmake --build .local/build-sow17 --target netdata -j 8` after the final selector guard
  - Result: passed; `src/web/api/queries/query-plan.c` rebuilt and linked.
- `.local/build-sow17/netdata -W queryplantest` after the final selector guard
  - Result: passed all six focused query planner checks.
- `./install.sh`
  - Result: passed twice; local `netdata` was restarted. A non-fatal `git fetch -t` step failed due local GitHub SSH permission, but the install completed both times.
- `curl -sS 'http://127.0.0.1:19999/api/v1/info' | jq -r '.version'`
  - Result: `v2.10.0-215-ge6e45f29ee`.
- `/usr/sbin/netdata -W queryplantest`
  - Result: passed all six focused query planner checks.
- `/usr/sbin/netdata -W queryplantest` after the final reinstall
  - Result: passed all six focused query planner checks.

Real-use evidence:

- Pre-fix local direct-agent `/api/v3/data` calls reproduced the symptom against `smartctl.device_temperature`.
- Post-fix 10-minute baseline:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778958670&before=1778959270&points=60&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned&timeout=30000`
  - Result: `result.data` length `60`; first timestamp `1778959270`; last timestamp `1778958680`; tier 0 `queries=18`, `update_every=10`; tier 1 and tier 2 `queries=0`.
- Post-fix original failing 5-second automatic-tier query:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000&after=1778958983&before=1778958988&points=5`
  - Result: `result.data` length `5`; timestamps `1778958988,1778958987,1778958986,1778958985,1778958984`; tier 0 `queries=18`; tier 1 and tier 2 `queries=0`.
- Post-fix forced tier 0 control for the same 5-second window:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000&after=1778958983&before=1778958988&points=5&tier=0`
  - Result: `result.data` length `5`; timestamps `1778958988,1778958987,1778958986,1778958985,1778958984`; tier 0 `queries=18`; tier 1 and tier 2 `queries=0`.
- Post-fix shifted 5-second automatic-tier scan across one 10-second cadence interval:
  - Common request shape: `/api/v3/data?contexts=smartctl.device_temperature&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000&after=<after>&before=<before>&points=5`
  - `0-5`: `after=1778958980`, `before=1778958985` -> `result.data` length `5`, tier 0 `queries=18`.
  - `1-6`: `after=1778958981`, `before=1778958986` -> `result.data` length `5`, tier 0 `queries=18`.
  - `2-7`: `after=1778958982`, `before=1778958987` -> `result.data` length `5`, tier 0 `queries=18`.
  - `3-8`: `after=1778958983`, `before=1778958988` -> `result.data` length `5`, tier 0 `queries=18`.
  - `4-9`: `after=1778958984`, `before=1778958989` -> `result.data` length `5`, tier 0 `queries=18`.
  - `5-0`: `after=1778958985`, `before=1778958990` -> `result.data` length `5`, tier 0 `queries=18`.
  - `6-1`: `after=1778958986`, `before=1778958991` -> `result.data` length `5`, tier 0 `queries=18`.
  - `7-2`: `after=1778958987`, `before=1778958992` -> `result.data` length `5`, tier 0 `queries=18`.
  - `8-3`: `after=1778958988`, `before=1778958993` -> `result.data` length `5`, tier 0 `queries=18`.
  - `9-4`: `after=1778958989`, `before=1778958994` -> `result.data` length `5`, tier 0 `queries=18`.
- Post-fix older two-tier overlap check:
  - Request: `/api/v3/data?contexts=smartctl.device_temperature&after=1778935003&before=1778935008&points=5&group_by=dimension&aggregation=average&time_group=average&format=json2&options=jsonwrap,minify,unaligned,debug,details&timeout=30000`
  - Result: `result.data` length `5`; tier 0 `queries=0`, `first_entry=1778948390`; tier 1 `queries=18`, `update_every=600`; tier 2 `queries=0`, `last_entry=1778940000`.
  - Interpretation: after install/restart, tier 0 and tier 2 no longer overlap on the local database, so the earlier all-three-overlap control is no longer reproducible. This check validates that when only tier 1 and tier 2 overlap a sub-resolution window, the denser overlapping tier 1 wins.

Reviewer findings:

- Self-review found one additional hardening point: the automatic selector should treat reversed windows the same as zero-duration windows before adjusting `points_wanted`. This guard was added and revalidated.
- External AI reviewers were not run; the user did not request them for this PR.

Same-failure scan:

- Source-code search found no remaining references to `query_plan_points_coverage_weight()`, `rrddim_find_best_tier_for_timeframe()`, or `rrdset_find_natural_update_every_for_timeframe()`.
- Automatic execution-tier selection call sites remain centralized through `query_metric_best_tier_for_timeframe()`.

Sensitive data gate:

- This SOW records no raw secrets, bearer tokens, SNMP communities, private endpoints, customer data, personal names, or device serial numbers.
- Hardware-identifying labels observed during verification were not copied into this durable artifact.

Artifact maintenance gate:

- AGENTS.md: no update. Existing artifact-boundary instructions already distinguish internal specs/project skills from public end-user/operator skills.
- Runtime project skills: no update. This change adds project behavior, not a new "how to work here" workflow.
- Specs: added `.agents/sow/specs/query-planner-tier-selection.md`.
- End-user/operator docs: no update. The behavior is internal planner selection; no user-facing API parameter or documented workflow changed.
- End-user/operator skills: no update. Public skills under `docs/netdata-ai/skills/` are for user/operator work, not maintainer debugging notes.
- SOW lifecycle: `Status: completed` and file move to `.agents/sow/done/` are part of this commit.

Specs update:

- Added `.agents/sow/specs/query-planner-tier-selection.md`.

Project skills update:

- No project skill update needed; no durable assistant workflow changed.

End-user/operator docs update:

- No end-user/operator docs update needed; the user-visible API contract did not gain a new parameter or required workflow.

End-user/operator skills update:

- No end-user/operator skill update needed; this is not a public/operator AI skill workflow.

Lessons:

- Internal query-planner diagnostics and maintainer implementation notes belong in the SOW/spec layer, not under `docs/netdata-ai/skills/`.
- The direct-agent public skills may still need separate product-doc review, but that is not part of this internal planner SOW unless a user/operator workflow changes.

Follow-up mapping:

- No remaining behavioral follow-up identified.

## Outcome

Completed. Automatic tier selection now uses overlapping-tier point density, so sub-resolution query windows pick the densest overlapping tier instead of collapsing overlapping and non-overlapping tiers to the same sentinel score.

## Lessons Extracted

- Sub-resolution query windows must be ordered by fractional point density, not by integer point counts, because integer division collapses valid overlapping tiers to the same sentinel value as non-overlapping tiers.
- Internal maintainer findings belong in `.agents/sow/` specs and SOWs; public skills under `docs/netdata-ai/skills/` should only change for user/operator workflows.

## Followup

No behavioral follow-up identified.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
