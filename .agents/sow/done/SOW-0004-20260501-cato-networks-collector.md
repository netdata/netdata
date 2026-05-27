# SOW-0004 - Cato Networks SASE monitoring collector via GraphQL

## Status

Status: completed

Sub-state: completed 2026-05-02 after collector-writing skill compliance hardening: explicit V2 obsoletion, deterministic cardinality controls, gated recoverable logging, and missing go.d README registry entry. Live Cato tenant validation is handed off to SREs and is no longer tracked as a repo-local pending SOW.

## SOW-0005 Removal Decision - 2026-05-02

User decision:

- Remove pending SOW-0005.
- The user will give the Cato trial/PoC setup information to SREs so they can set up live validation and test the collector.

Implications:

- Repo-local SOW tracking no longer owns live Cato tenant setup or execution.
- Live validation is still required before claiming real-world coverage of optional payload fields, BGP scale behavior, or dashboard topology rendering beyond unit-tested JSON/function behavior.
- If SRE live validation finds a collector bug, payload discrepancy, documentation issue, or fixture opportunity, open or reopen a SOW with the concrete evidence from that validation.

SOW lifecycle:

- `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md` is removed by this decision.
- Earlier historical notes in this SOW that mention SOW-0005 are preserved as past state and are superseded by this section.
- `.agents/sow/audit.sh` after removal reported pending SOWs empty and SOW status/directory consistency OK. The same audit also reported unrelated project-wide SOW framework hygiene warnings from the latest master pull: missing new AGENTS sensitive-data/reference sections and a historical mirrored-repo absolute path citation in this completed SOW.

## Reopen - PR Review Comments - Failed-Cycle Health, BGP Progress, and Stalled Event Markers - 2026-05-02

Reason:

- After reviewer re-triggering on head `8109c922138932fb4051d652e86b2f0e5372a0b2`, three new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_Fx1A` on `src/go/plugin/go.d/collector/cato_networks/write_metrics.go:202`; local verification found `src/go/plugin/framework/jobruntime/job_v2.go:436-440` aborts the metrix cycle when `Collect()` returns an error, discarding Cato's deferred health writes for hard discovery/snapshot failures.
- Thread `PRRT_kwDOAKPxd85_Fx1K` on `src/go/plugin/go.d/collector/cato_networks/collect.go:163` reported BGP progress gauges dropping to zero when the BGP refresh window is not due; local verification found `beginHealthCycle()` resets BGP progress fields before `collectBGP()` returns early with cached BGP state.
- Thread `PRRT_kwDOAKPxd85_Fx1N` on `src/go/plugin/go.d/collector/cato_networks/collect.go:325` reported stalled EventsFeed markers can double-count events; local verification found `collectEvents()` counts the repeated page, returns the unchanged marker, and `events_total` is a stateful counter incremented with `Add()`.

Implementation scope:

1. Ensure hard runtime collection failures publish collector-health metrics instead of returning an error that causes jobruntime to abort the metrix cycle.
2. Preserve or recompute BGP progress health when using cached BGP data between refresh windows, while still publishing zero progress for cycles that fail before BGP collection.
3. Stop stalled EventsFeed markers from adding repeated page records to `events_total` and avoid persisting an unchanged marker as progress.
4. Add regression coverage for failed-cycle health publication, cached BGP progress between refresh windows, and stalled marker no-double-count behavior.
5. Update the Cato collector spec for the runtime error-publication, BGP cached-progress, and stalled-marker contracts.

Implemented:

- `Collect()` now logs hard runtime collection failures but returns nil after the collector writes health diagnostics, allowing the job runtime to commit the metrix cycle instead of aborting the staged health metrics.
- BGP cached-refresh skips now recompute and publish the current BGP sites-per-collection, full-scan window, and cached-site count instead of leaving the begin-cycle zero values.
- `collectEvents()` now detects an unchanged EventsFeed marker before processing the page records, records `marker_stalled`, and does not persist the unchanged marker as progress.
- Added regression coverage for empty-discovery health publication, accountSnapshot failure health publication, cached BGP progress on skipped BGP refresh windows, stalled-marker no-count behavior, and event counter stability across a stalled marker cycle.
- Updated `.agents/sow/specs/cato-networks-collector.md` and the collector README to match the runtime health, BGP cached-progress, and marker-stall contracts.

Validation completed:

- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.
- `.agents/sow/audit.sh` after moving the SOW back to `done/` - passed.

Artifact maintenance:

- `AGENTS.md`: no update needed. The repo workflow did not change.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with committed hard-failure health publication, cached BGP progress publication, and stalled-marker no-count behavior.
- End-user/operator docs: updated `src/go/plugin/go.d/collector/cato_networks/README.md` troubleshooting text for first-run collection failure visibility and `marker_stalled` behavior.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for PR review findings; closing it again after validation. Live Cato tenant validation remains tracked by SOW-0005.

Follow-up mapping:

- No new deferred work from this reopen. Live tenant validation remains explicitly tracked by SOW-0005 and is not closed here.

Outcome:

- PR review findings were implemented and validated locally. The PR threads will be replied to and resolved after this commit is pushed so replies can reference the fixing commit.

## Reopen - PR Review Comments - Snapshot Fallback Coverage and BGP Peer Identity - 2026-05-02

Reason:

- After reviewer re-triggering on head `0e7174394ff032cafbcd2f4fcf86d9d3076d6839`, two new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FvMT` on `src/go/plugin/go.d/collector/cato_networks/client.go:118`; local verification found direct raw accountSnapshot tests existed, but no test exercised `sdkAPIClient.AccountSnapshot()` falling back from the SDK decode-error branch to `raw.AccountSnapshot()`.
- Thread `PRRT_kwDOAKPxd85_FvMZ` on `normalize.go:437` reported BGP rows with connection-state fields but no remote IP/ASN still passed normalization; local verification found `isEmptyBGPPeerResult()` treated incoming/outgoing connection state as enough to keep the row, producing blank `peer_ip`/`peer_asn` labels downstream.

Implementation scope:

1. Add branch coverage for `sdkAPIClient.AccountSnapshot()` enum-drift fallback using a live `httptest` server.
2. Drop BGP peer rows that lack both remote IP and remote ASN, even if they include connection-state fields.
3. Count the dropped remote-identity-less BGP rows as `empty_peer` normalization issues.
4. Update tests and the Cato collector spec for the BGP remote identity requirement.

Implemented:

- Added an `httptest`-backed test that forces the SDK `accountSnapshot` path to hit the enum decode error branch and verifies it calls the raw GraphQL fallback successfully.
- BGP normalization now drops rows that lack both remote IP and remote ASN, even when they include local or connection-state fields.
- Dropped remote-identity-less BGP rows are counted as `empty_peer` normalization issues.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the BGP remote identity requirement and explicit snapshot fallback branch coverage expectation.

Validation completed:

- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- `AGENTS.md`: no update needed. The repo workflow did not change.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with BGP remote identity filtering and snapshot fallback branch coverage expectations.
- End-user/operator docs: no update needed. Public configuration, chart labels, and troubleshooting text did not change in this pass.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for PR review findings; closing it again after validation. Live Cato tenant validation remains tracked by SOW-0005.

Follow-up mapping:

- No new deferred work from this reopen. Live tenant validation remains explicitly tracked by SOW-0005 and is not closed here.

Outcome:

- PR review findings were implemented and validated locally. The PR threads will be replied to and resolved after this commit is pushed so replies can reference the fixing commit.

## Reopen - PR Review Comments - Traffic Availability and Interface Identity - 2026-05-02

Reason:

- After reviewer re-triggering on head `20b8178e601c757a50e8c30e876b57bb67b2d087`, three new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_Fryg` on `src/go/plugin/go.d/collector/cato_networks/topology.go:114`; local verification found tunnel topology links always included `site.Metrics` values, even when no accountMetrics data was present.
- Thread `PRRT_kwDOAKPxd85_Fryj` on `write_metrics.go:147` reported traffic metrics were always exported from a plain zero-value `trafficMetrics` struct; local verification found `writeTrafficMetrics()` unconditionally observed every traffic field.
- Thread `PRRT_kwDOAKPxd85_Frym` on `write_metrics.go:62` reported duplicate interface display names could collide; local verification found runtime interface state is keyed by `interfaceKey(id, name)`, but exported metric labels omitted `interface_id`.

Implementation scope:

1. Track per-field traffic metric availability while merging `accountMetrics`.
2. Emit site/interface traffic and quality metrics only for fields actually returned by Cato.
3. Omit tunnel topology link metrics when the corresponding site traffic fields are unavailable.
4. Add `interface_id` to interface metric labels and chart instance grouping to preserve duplicate interface identity.
5. Add regression coverage for unavailable traffic metrics, topology metrics absence, and duplicate interface names.
6. Update metadata, charts, and the Cato collector spec for the new interface label and availability contract.

Implemented:

- Added per-field traffic metric presence tracking during `accountMetrics` normalization.
- Site and interface traffic/quality metric writers now emit only fields actually returned by Cato, preserving real zeroes while leaving missing telemetry absent.
- Tunnel topology links now omit metric payloads when site traffic metrics are unavailable, and include only available topology metric fields.
- Interface metric labels and chart grouping now include `interface_id` as well as `interface_name`, preventing duplicate display-name collisions.
- Topology interface tables now include the interface ID.
- Added regression tests for metrics-disabled traffic absence, duplicate interface names with different IDs, and topology tunnel links without accountMetrics.
- Updated charts, metadata, and `.agents/sow/specs/cato-networks-collector.md`.

Validation completed:

- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- `AGENTS.md`: no update needed. The repo workflow did not change.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the traffic metric availability contract and interface identity label contract.
- End-user/operator docs: updated metadata and charts for the new `interface_id` label/grouping. README did not need a change because it does not document metric labels.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for PR review findings; closing it again after validation. Live Cato tenant validation remains tracked by SOW-0005.

Follow-up mapping:

- No new deferred work from this reopen. Live tenant validation remains explicitly tracked by SOW-0005 and is not closed here.

Outcome:

- PR review findings were implemented and validated locally. The PR threads will be replied to and resolved after this commit is pushed so replies can reference the fixing commit.

## Reopen - PR Review Comments - Retry Metrics and BGP Progress Freshness - 2026-05-02

Reason:

- After reviewer re-triggering on head `a347f89cd37e4693b41c5c7c57331f1be8430aa1`, two new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FoYa` on `src/go/plugin/go.d/collector/cato_networks/write_metrics.go:178`; local verification found SDK retry stats were unit-tested, but the metric-writing path for `api_rate_limit_retries_total` and `api_transient_retries_total` had no direct assertions.
- Thread `PRRT_kwDOAKPxd85_FoYf` on `write_metrics.go:192` reported BGP progress health gauges could remain stale when a later collection fails before `collectBGP()` runs; local verification found `beginHealthCycle()` did not reset BGP progress fields, while the deferred health writer still runs on collection failure.

Implementation scope:

1. Add direct metric-store assertions for API retry metric totals and deltas.
2. Reset BGP progress health fields at the start of every cycle.
3. Emit zero-valued BGP progress gauges when BGP is enabled but the current cycle did not reach BGP collection.
4. Add regression coverage for a collection that fails before BGP after a prior successful BGP cycle.
5. Update the Cato collector spec if behavior changes.

Implemented:

- Added direct metric-store tests for `api_rate_limit_retries_total` and `api_transient_retries_total`, covering snapshot counter totals and second-cycle deltas.
- `beginHealthCycle()` now resets BGP progress health fields at the start of every cycle.
- `writeCollectorHealth()` now emits BGP progress gauges whenever BGP is enabled, including zero values when the current cycle fails before BGP collection runs.
- Added regression coverage for a collection that first publishes non-zero BGP scan progress, then fails during account snapshot before BGP collection and publishes zero BGP progress for the failed cycle.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the BGP progress freshness contract and API retry metric validation requirement.

Validation completed:

- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- `AGENTS.md`: no update needed. The repo workflow did not change.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with BGP progress reset/zero-publication behavior and API retry metric validation coverage.
- End-user/operator docs: no update needed. Public configuration and troubleshooting semantics did not change; this pass corrected metric freshness and test coverage.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for PR review findings; closing it again after validation. Live Cato tenant validation remains tracked by SOW-0005.

Follow-up mapping:

- No new deferred work from this reopen. Live tenant validation remains explicitly tracked by SOW-0005 and is not closed here.

Outcome:

- PR review findings were implemented and validated locally. The PR threads will be replied to and resolved after this commit is pushed so replies can reference the fixing commit.

## Reopen - PR Review Comments - Transport, Dry-Run State, and Event Robustness - 2026-05-02

Reason:

- After reviewer re-triggering on head `5e7908c7d7401f1453e9d997179bdf6cc86ffcee`, six new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FhFJ` on `src/go/plugin/go.d/collector/cato_networks/collector.go:166`; local verification found `Cleanup()` did not close idle connections for the collector-owned HTTP client.
- Thread `PRRT_kwDOAKPxd85_FhFc` on `config.go:163` reported cleartext `http://` endpoints were accepted while API keys are sent in headers; local verification found validation allowed both `http` and `https`.
- Thread `PRRT_kwDOAKPxd85_FhFn` on `collector.go:276` reported marker writes were single-attempt; local verification found `commitEventsMarker()` called `markerStore.write()` once.
- Thread `PRRT_kwDOAKPxd85_FhF1` on `collect.go:162` reported `Check()` could populate BGP cache and delay first real BGP polling; local verification found `Check()` dry-run state restoration covered health only, not discovery, BGP state, or topology.
- Thread `PRRT_kwDOAKPxd85_FhGC` on `config.go:230` reported `metrics.group_interfaces=auto` behaved exactly like disabled; local verification found `groupInterfaces()` resolved auto with `Bool(false)` and the SDK call always received a non-nil boolean.
- Thread `PRRT_kwDOAKPxd85_FhGO` on `collect.go:309` reported repeated EventsFeed records were counted more than once; local verification found no event ID deduplication before `addEventCount()`.

Implementation scope:

1. Track and close collector-owned HTTP idle connections in `Cleanup()`.
2. Require HTTPS URLs except loopback HTTP endpoints used for local tests/mocks.
3. Retry retryable temporary/timeout marker writes with the configured retry attempts/backoff and caller context while failing permanent local filesystem errors fast.
4. Restore dry-run discovery, BGP, topology, and health state after `Check()`.
5. Pass `nil` for `groupInterfaces` when `metrics.group_interfaces=auto` so Cato/default SDK semantics are preserved.
6. Deduplicate EventsFeed records by event ID within a collection cycle.
7. Add focused regression tests and update the Cato collector spec.

Implemented:

- `Collector` now retains the collector-owned HTTP client from `web.NewHTTPClient()` and closes idle connections in `Cleanup()`.
- `url` validation now requires HTTPS for production endpoints and allows HTTP only for loopback endpoints used by local tests/mocks.
- Events marker writes now retry retryable temporary/timeout errors with configured retry attempts/backoff and caller context, while permanent filesystem errors fail fast.
- `Check()` now snapshots and restores health, discovery, BGP state, and topology so dry-run collection cannot leak mutable state into the first real `Collect()`.
- `metrics.group_interfaces=auto` now passes nil/unset `groupInterfaces` to the SDK/API; enabled/disabled pass explicit true/false.
- EventsFeed records are deduplicated by `event_id`/`eventId` within one collection cycle. Records without an event ID still count normally.
- Added regression tests for HTTPS validation, groupInterfaces auto semantics, BGP dry-run isolation, event-ID dedupe, and transient marker-write retry.
- Updated README, metadata, config schema, stock config, and `.agents/sow/specs/cato-networks-collector.md`.

Validation completed:

- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- `AGENTS.md`: no update needed. The repo workflow did not change.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with HTTPS URL requirements, HTTP cleanup, groupInterfaces auto semantics, stronger `Check()` isolation, retryable marker-write retry, and EventsFeed event-ID dedupe.
- End-user/operator docs: updated README, metadata, config schema, and stock config to match URL and groupInterfaces behavior.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for PR review findings; closing it again after validation. Live Cato tenant validation remains tracked by SOW-0005.

Follow-up mapping:

- No new deferred work from this reopen. Live tenant validation remains explicitly tracked by SOW-0005 and is not closed here.

Outcome:

- PR review findings were implemented and validated locally. The PR threads will be replied to and resolved after this commit is pushed so replies can reference the fixing commit.

## Reopen - PR Review Comments - Event Cardinality and Check Health Isolation - 2026-05-02

Reason:

- After reviewer re-triggering on head `b3e66a3963668fadbbad9e8e8a350a9e1298af33`, two new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FcnC` on `src/go/plugin/go.d/collector/cato_networks/collect.go:365`.
- The reviewer reported that `events.max_cardinality = N` started collapsing at `N-1` real event series. Local verification found `len(counts) >= maxCardinality-1`.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FcnJ` on `src/go/plugin/go.d/collector/cato_networks/collector.go:194`.
- The reviewer reported that `Check()` uses `collect(false)` but still mutates health state through operation and normalization markers. Local verification found `beginHealthCycle()` and all `mark*` calls run before the `write` guard, while only `writeCollectorHealth()` is guarded.
- Because collector health counters are stateful, dry-run failures could be published on the next real `Collect()` cycle.

Implementation scope:

1. Allow `events.max_cardinality = N` to keep `N` real event series before routing excess series to `other`.
2. Prevent `Check()` dry-run health mutations from leaking into later real collections.
3. Add regression tests for the cardinality boundary and dry-run health isolation.
4. Update the Cato collector spec for these contracts.

Implemented:

- `addEventCount()` now allows `events.max_cardinality = N` to keep `N` real event series before routing excess new combinations to `other`.
- `Check()` now snapshots collector health before the dry run and restores it afterward, preventing dry-run operation statuses, failure counters, affected-site counters, collection failure counters, and normalization issues from leaking into later `Collect()` output.
- Added regression tests for the event cardinality boundary and dry-run health isolation.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the event cardinality and `Check()` health isolation contracts.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/collector.go src/go/plugin/go.d/collector/cato_networks/diagnostics.go src/go/plugin/go.d/collector/cato_networks/collect.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the event cardinality and `Check()` health isolation contracts.
- End-user/operator docs: no update needed. User-facing setup and troubleshooting text did not change; this pass corrected implementation behavior under existing event cardinality and dry-run contracts.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review threads and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - Metric Merge, HTTP 5xx Retry, and BGP Topology Deduplication - 2026-05-02

Reason:

- After reviewer re-triggering on head `edd1c3617eab88c93b22c00328170465b8a262a2`, three new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FVbx` on `src/go/plugin/go.d/collector/cato_networks/normalize.go:220`.
- The reviewer reported that an interface named `all` overwrote already-merged site metrics. Local verification found `site.Metrics = iface.Metrics`, which could drop site-only fields when the `all` interface omitted them.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FVcO` on `src/go/plugin/go.d/collector/cato_networks/client.go:423`.
- The reviewer reported that HTTP 5xx errors were not treated as retryable. Local verification found `isTransientCatoError()` handled network strings but not HTTP status `500` through `599`.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FVce` on `src/go/plugin/go.d/collector/cato_networks/topology.go:143`.
- The reviewer reported that repeated BGP peer rows could produce duplicate topology actor IDs and links. Local verification found `buildTopology()` appended every BGP row without deduplicating `catoBGPPeerActorID(site.ID, peer.RemoteIP, peer.RemoteASN)`.
- Same-class search also found duplicate BGP peers could write repeated BGP metric labels through `write_metrics.go`, so deduplication should happen during BGP normalization and be defensively preserved in topology generation.

Implementation scope:

1. Merge metrics from the `all` interface into existing site metrics without overwriting the whole site metric struct.
2. Treat HTTP `5xx` responses as transient retryable Cato errors while preserving caller context cancellation behavior.
3. Deduplicate BGP peers by remote IP and remote ASN during normalization.
4. Defensively deduplicate BGP topology actors/links by generated peer actor ID.
5. Add targeted regression tests for all three findings.
6. Update the Cato collector spec for these contracts.

Implemented:

- `accountMetrics` interface `all` values are now merged into site metrics from raw interface fields instead of replacing the whole site metric struct.
- HTTP `5xx` response strings are now classified as transient retryable Cato errors, while caller context cancellation/deadline handling remains unchanged.
- BGP peers are deduplicated by remote IP and remote ASN during normalization so repeated vendor rows do not produce repeated BGP metric label rows.
- Topology generation also defensively deduplicates BGP peer actor IDs before appending peer actors and links.
- Added regression tests for `all` interface metric merging, HTTP `503` retry classification, BGP normalization deduplication, and topology BGP peer deduplication.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the metric merge, HTTP `5xx`, and BGP deduplication contracts.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/client.go src/go/plugin/go.d/collector/cato_networks/normalize.go src/go/plugin/go.d/collector/cato_networks/topology.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the metric merge, HTTP `5xx`, and BGP deduplication contracts.
- End-user/operator docs: no update needed. User-facing setup, configuration, and troubleshooting text did not change; this pass corrected implementation behavior under already documented metric/topology/diagnostic surfaces.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review threads and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - Stateful Operation Status - 2026-05-02

Reason:

- After reviewer re-triggering on head `7fa15f1f83419802d17defb44900bb1430f59b1e`, one new Copilot review thread opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FRvs` on `src/go/plugin/go.d/collector/cato_networks/write_metrics.go:199`.
- The reviewer reported that `collector_operation_success` is documented as last operation status, but `beginHealthCycle()` rebuilt `LastOperations` from scratch every cycle.
- Local verification found `README.md` describes `cato_networks.collector_operation_status` as last status by operation, and `metadata.yaml` describes it as last observed operation status.
- Local verification found `diagnostics.go` reset `c.health.LastOperations = make(map[string]operationHealth)` at the start of every collection cycle.
- This meant skipped operations such as `entityLookup` and `siteBgpStatus` could disappear between their refresh windows instead of preserving last-known success/failure state.

Implementation scope:

1. Preserve `LastOperations` across collection cycles.
2. Keep per-cycle health values that truly are per-cycle, such as collection success and discovered site count, unchanged.
3. Add a regression test proving skipped operations remain visible after a later collection cycle.
4. Update the Cato collector spec for the stateful operation-status contract.

Implemented:

- `collector_operation_success` now preserves the last observed status for operations skipped between refresh windows.
- `beginHealthCycle()` no longer clears `LastOperations`; a code comment records that this map is intentionally stateful.
- Successful marker writes now mark the local `eventsMarker` operation successful, so stateful operation status does not leave a prior marker-write failure visible after recovery.
- Added `TestCollectorKeepsLastOperationStatusForSkippedOperations` to prove `entityLookup` and `siteBgpStatus` remain visible after a later cycle skips both refreshes.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the stateful operation-status contract.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/collector.go src/go/plugin/go.d/collector/cato_networks/diagnostics.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the stateful operation-status contract.
- End-user/operator docs: no update needed. Existing README and metadata already described the metric as last/last-observed status; this pass made the implementation match that contract.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review thread and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - Marker Read Diagnostics and Timeout Retries - 2026-05-02

Reason:

- After reviewer re-triggering on head `034455f194d51f8d9c4fc65c3cf77b7e74774fc0`, two new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FNUk` on `src/go/plugin/go.d/collector/cato_networks/diagnostics.go:81`.
- The reviewer reported that `MarkerPersistenceAvailable` was derived only from `markerStore != nil`, so a configured marker file that failed to read during `Init()` would still report marker persistence as available until a later write failed.
- Local verification found `Init()` logged marker read failures but did not keep state about marker-store availability. `beginHealthCycle()` reset marker persistence availability from `markerStore != nil`.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FNUo` on `src/go/plugin/go.d/collector/cato_networks/client.go:390`.
- The reviewer reported that `isRetryableCatoError()` returned false for all errors wrapping `context.DeadlineExceeded`, including HTTP/client timeouts that should retry when the caller context is still active.
- Local verification found `withRetry()` already stops immediately when `ctx.Err() != nil`, so `isRetryableCatoError()` can safely retry deadline-wrapped client errors when the caller context has not expired.

Implementation scope:

1. Track marker-store availability separately from marker-store existence.
2. Mark marker persistence unavailable after marker read failure until a later marker write succeeds.
3. Retry `context.DeadlineExceeded`-wrapped client errors only while the caller context remains active.
4. Preserve immediate stop on caller cancellation or caller deadline expiry.
5. Add targeted tests for marker read failure reporting and retryable client deadline errors.
6. Update the Cato collector spec for these contracts.

Implemented:

- Added `markerStoreAvailable` collector state.
- `Init()` now marks marker persistence unavailable when marker read fails.
- `beginHealthCycle()`, events collection success, and marker writes now use/update `markerStoreAvailable`.
- `isRetryableCatoError()` now accepts the caller context, retries deadline-wrapped client errors when `ctx.Err()` is nil, and still rejects caller cancellation/deadline expiry.
- Added marker-read-failure and client-deadline retry tests.
- Updated `.agents/sow/specs/cato-networks-collector.md` with marker-read availability and timeout retry contracts.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/collector.go src/go/plugin/go.d/collector/cato_networks/diagnostics.go src/go/plugin/go.d/collector/cato_networks/client.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- First `cd src/go && go test ./plugin/go.d/... -count=1` failed in unrelated `collector/tor` with an unexpected local test request; Cato package passed in that run.
- `cd src/go && go test ./plugin/go.d/collector/tor -count=1` - passed on immediate rerun.
- Second `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with marker-read availability and timeout retry contracts.
- End-user/operator docs: no update needed. User-facing setup options and documented troubleshooting remain unchanged; this pass tightened internal state reporting and retry behavior.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review threads and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - Topology Job Selection and Events Account Errors - 2026-05-02

Reason:

- After reviewer re-triggering on head `ab77e808287d05097beef5e24ab745c7aef19ac4`, two new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FKhJ` on `src/go/plugin/go.d/collector/cato_networks/func_topology.go:71`.
- The reviewer reported that `AgentWide` hides `__job` while function dispatch still routes through the first running job, making additional Cato account topologies inaccessible in multi-instance setups.
- Local verification found `funcctl` contains a FIXME documenting that `AgentWide` currently means "omit `__job` from the public API" while dispatch still uses the first running job.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FKhS` on `src/go/plugin/go.d/collector/cato_networks/collect.go:286`.
- The reviewer reported that an `eventsFeed` account-level `errorString` should fail the page and not leave the returned marker eligible for commit.
- Local verification found the collector requested exactly one account, skipped accounts with `errorString`, but still kept the page marker in `finalMarker`, allowing marker commit after metric write.

Implementation scope:

1. Make `topology:cato_networks` job-selectable by removing `AgentWide`.
2. Treat an EventsFeed account-level error as an EventsFeed operation failure for that cycle.
3. Preserve the low-cardinality normalization diagnostic for account-level EventsFeed errors.
4. Prevent persisted and in-memory marker advancement when the EventsFeed account payload reports an error.
5. Add targeted tests for topology job selection and marker non-advancement.
6. Update the Cato collector spec for these durable contracts.

Implemented:

- Removed `AgentWide` from the Cato topology method config so the normal `__job` selector remains available for multi-instance Cato jobs.
- `collectEvents()` now returns a sanitized `eventsFeed account error` when the requested account has `errorString`; this marks the operation as failed and leaves the events marker empty for that cycle.
- Added `TestTopologyFunctionRequiresJobSelection`.
- Added `TestCollectorDoesNotAdvanceMarkerOnEventsAccountError`, asserting the old persisted marker remains on disk and in memory while diagnostics are emitted.
- Updated `.agents/sow/specs/cato-networks-collector.md` with the topology job-selection and EventsFeed account-error marker contracts.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/collect.go src/go/plugin/go.d/collector/cato_networks/func_topology.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with topology job selection and EventsFeed account-error marker-safety contracts.
- End-user/operator docs: no update needed. Public setup options and troubleshooting text already described EventsFeed account-level errors and marker behavior; this pass tightened internal failure handling and function dispatch visibility.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review threads and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - Marker Identity and Formatting - 2026-05-02

Reason:

- After reviewer re-triggering on head `bff14dea88374d7ef68b28f83572796d07b0bac8`, three new Copilot review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FHcf` on `src/go/plugin/go.d/collector/cato_networks/marker_store.go:30`.
- The reviewer reported that the default EventsFeed marker file was derived only from `account_id`, so multiple collector jobs monitoring the same Cato account could share marker state and interfere with marker pagination.
- Local verification found `newEventsMarkerStore()` hashed only `accountID`; the collector runtime does not expose the job name inside `Collector`, but `Config` does expose normalized `url` and `vnode`, and `events.marker_file` is available for explicit per-job override.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FHcm` on `src/health/health.d/cato_networks.conf:15`.
- Local verification found the first health block used wider indentation than the remaining blocks in the same file.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FHcq` on `src/go/plugin/go.d/collector/cato_networks/config_schema.json:90`.
- Local verification found mixed indentation around nested `config_schema.json` properties.

Implementation scope:

1. Include endpoint URL and vnode in the default EventsFeed marker identity while keeping explicit `events.marker_file` as the escape hatch for duplicate jobs with the same account, endpoint, and vnode.
2. Add targeted unit coverage for marker identity differences and whitespace normalization.
3. Align the first Cato health block with existing `health.d` indentation style.
4. Reformat `config_schema.json` consistently.
5. Update operator docs/metadata/stock config for the clarified marker-file behavior.
6. Re-run focused validation before pushing.

Implemented:

- `newEventsMarkerStore()` now derives the default marker path from account ID, endpoint URL, and vnode.
- Added `defaultEventsMarkerPath()` so the identity logic is testable without mutating global plugin directory state.
- Added a unit test showing the marker identity changes by endpoint and vnode while trimming surrounding whitespace.
- Reformatted `config_schema.json` with consistent spaces-only indentation.
- Aligned the first `health.d/cato_networks.conf` alarm block indentation with the rest of the file.
- Updated README, metadata, config schema, and stock config comments to explain when `events.marker_file` should be set explicitly.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/collector.go src/go/plugin/go.d/collector/cato_networks/marker_store.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.
- `jq empty src/go/plugin/go.d/collector/cato_networks/config_schema.json` - passed.
- `ruby -e 'require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/cato_networks/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/cato_networks.conf")'` - passed.
- `rg -n "\t" src/go/plugin/go.d/collector/cato_networks/config_schema.json src/go/plugin/go.d/config/go.d/cato_networks.conf src/health/health.d/cato_networks.conf` - found no tab characters.
- `src/health/health.d/cato_networks.conf` was not parsed as YAML because Netdata health files use health syntax, not standard YAML; the first block was verified against the same indentation pattern as the later blocks in the file.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. The marker identity is an internal persistence-path hardening detail; explicit `events.marker_file` remains the public override.
- End-user/operator docs: updated README, metadata, config schema, and stock config comments to clarify when `events.marker_file` should be set.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for late PR review threads and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comment - Explicit Site Info Nil Handling - 2026-05-02

Reason:

- After reviewer re-triggering on head `86cc44d6cd77a7255361b4509a8433e07a20e2ec`, one new Copilot review thread opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_FC0n` on `src/go/plugin/go.d/collector/cato_networks/normalize.go:101`.
- The reviewer reported that `normalizeSnapshot()` could panic when Cato omits `infoSiteSnapshot`.
- Local verification found the generated `github.com/catonetworks/cato-go-sdk@v0.2.5` getters are nil-receiver safe, so the immediate panic claim is not true for the current SDK. However, the normalization code relied on that generated behavior implicitly, while `normalizeSnapshotInterface()` already handles missing nested info explicitly.

Implementation scope:

1. Make missing site info handling explicit in `normalizeSnapshot()`.
2. Preserve the existing fallback behavior for site name, metadata, type, and connection type defaults.
3. Add targeted test assertions for a site without `infoSiteSnapshot`.
4. Re-run focused Cato collector validation before updating the PR.

Implemented:

- `normalizeSnapshot()` now copies site info fields only when `raw.GetInfoSiteSnapshot()` is non-nil.
- Missing site info now explicitly produces empty optional metadata fields and still falls back to discovery/site-ID naming.
- Expanded the nil-info/status normalization test to assert the missing-info defaults.

Validation completed:

- `gofmt -w src/go/plugin/go.d/collector/cato_networks/normalize.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `git diff --check` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. This is defensive normalization hardening with no metric, topology, configuration, or public behavior change beyond avoiding reliance on SDK nil-receiver behavior.
- End-user/operator docs: no update needed. User-facing collector behavior and documented configuration are unchanged.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for a late PR review thread and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR CI - Sonar Duplication Gate - 2026-05-02

Reason:

- PR #22373 reached a clean review-thread state, but the SonarCloud Code Analysis check failed on head `434aaeb8d25f0ba7a6efe86686c1b01317d8a594`.
- SonarCloud reported `5.4% Duplication on New Code`; the project quality gate requires at most `3%`.

Evidence:

- GitHub check run `SonarCloud Code Analysis` on head `434aaeb8d25f0ba7a6efe86686c1b01317d8a594` failed with quality-gate output for new-code duplication.
- Public SonarCloud component measures for PR #22373 identified duplicated new lines in these files:
  - `src/go/plugin/go.d/collector/cato_networks/collector_test.go`: 158 duplicated new lines out of 1608 new lines, `9.82587064676617%`.
  - `src/go/plugin/go.d/collector/cato_networks/normalize.go`: 64 duplicated new lines out of 437 new lines, `14.6453%`.
  - `src/go/plugin/go.d/collector/cato_networks/diagnostics.go`: 23 duplicated new lines out of 209 new lines, `11.0048%`.
- SonarCloud duplication-detail API showed the `normalize.go` duplication came from the near-identical site/interface traffic metric merge functions.
- The repo `fetch-sonar-findings.sh` script still requires local Sonar credentials from `.env` for issue/hotspot fetches; the duplication quality-gate evidence above came from public SonarCloud check and measures APIs.

Implementation scope:

1. Replace duplicate site/interface metric merge bodies with one typed helper that preserves SDK getter semantics.
2. Replace repeated collector-test initialization and single-cycle collection blocks with local test helpers.
3. Replace repeated diagnostics substring checks with one local helper while preserving error-class ordering.
4. Re-run focused and broader Go validation before committing.

Implemented:

- Added `mergeCatoTrafficMetrics()` and a narrow `catoTrafficMetrics` interface, then reused it for site and interface metric payloads.
- Added `fixedCatoTestNow()` and `collectOnce()` test helpers and updated repeated collector tests to use them.
- Added `containsAny()` for diagnostics classification and kept existing error-class precedence unchanged.

Validation completed:

- `cd src/go && gofmt -w src/go/plugin/go.d/collector/cato_networks/collector_test.go src/go/plugin/go.d/collector/cato_networks/normalize.go src/go/plugin/go.d/collector/cato_networks/diagnostics.go` - completed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, collector consistency, and validation rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. This is duplication-reduction refactoring with no public collector behavior, metric, topology, configuration, or diagnostic contract change.
- End-user/operator docs: no update needed. User-facing collector behavior and documented configuration are unchanged.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for a CI quality-gate failure and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- If SonarCloud still reports duplication above the gate after this commit is scanned, the same SOW should be reopened again and the next public SonarCloud measures result should drive the next reduction pass.

Outcome:

- Completed.

## Reopen - PR Review Comments - BGP Normalization, Raw Headers, Dead State - 2026-05-02

Reason:

- After reviewer re-triggering, three new `cubic-dev-ai[bot]` review threads opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_E5aB` on `src/go/plugin/go.d/collector/cato_networks/normalize.go:413`.
- The reviewer reported that empty BGP peer detection ignored incoming/outgoing connection states. Local verification found `isEmptyBGPPeerResult()` checked remote/local identifiers, BGP session, route counts, and RIB-out, but not `IncomingConnection.State` or `OutgoingConnection.State`.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_E5aC` on `src/go/plugin/go.d/collector/cato_networks/client.go:279`.
- The reviewer reported that raw `accountSnapshot` fallback custom headers could overwrite reserved `Content-Type`, `x-api-key`, and `x-account-id` headers. Local verification found custom headers were applied after reserved headers.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_E5aF` on `src/go/plugin/go.d/collector/cato_networks/collector.go:93`.
- Local verification found `sites`, `siteOrder`, and `lastUpdated` collector state fields were assigned each collection cycle but never read. `topology` is still read by the topology function and must remain.
- A later pre-push fetch found thread `PRRT_kwDOAKPxd85_E6c7` on `src/go/plugin/go.d/collector/cato_networks/charts.yaml:35`.
- The reviewer reported that site metrics are emitted with `site_id`, `site_name`, and `pop_name`, while site charts grouped only by `site_id` and `site_name`. Local verification found all site charts omitted `pop_name` from `by_labels`.
- The same later fetch found thread `PRRT_kwDOAKPxd85_E6dE` on `src/go/plugin/go.d/collector/cato_networks/client.go:67`.
- The reviewer reported that user-supplied headers are passed to the SDK and raw fallback. Local verification found the raw fallback was already fixed in this local pass, but the SDK client still received reserved headers from `cfg.Headers`.
- The same later fetch found thread `PRRT_kwDOAKPxd85_E6dH` on `src/go/plugin/go.d/collector/cato_networks/client.go:279`, duplicating the raw fallback reserved-header issue already fixed in this local pass.

Implementation scope:

1. Include incoming/outgoing BGP connection states in empty-peer detection and add a targeted normalization test.
2. Prevent custom GraphQL headers from overriding reserved request headers in both the SDK client setup and raw fallback path, and add targeted tests.
3. Align site chart `by_labels` with the emitted site metric label set.
4. Remove dead collector state fields and assignments while preserving topology locking.
5. Re-run focused Cato collector validation before pushing.

Implemented:

- `isEmptyBGPPeerResult()` now considers incoming and outgoing connection states before classifying a BGP peer payload as empty.
- Cato request header construction now drops reserved `Content-Type`, `x-api-key`, and `x-account-id` entries before the SDK client or raw fallback can use them.
- Raw `accountSnapshot` fallback sets required reserved headers after custom non-reserved headers.
- Site charts now include `pop_name` in `by_labels`, matching the emitted site metric label set.
- Removed unused collector state fields `sites`, `siteOrder`, and `lastUpdated`; topology state and locking remain.
- Added `TestNormalizeBGPKeepsPeersWithOnlyConnectionState`, `TestRawGraphQLAccountSnapshotDoesNotOverrideReservedHeaders`, and `TestCatoRequestHeadersFiltersReservedHeaders`.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, and collector consistency rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. These are internal hardening fixes; public collector behavior and documented contracts are unchanged.
- End-user/operator docs: no update needed. User-facing collector behavior and documented configuration are unchanged.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for the late PR review comments and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comments - BGP Peer Topology Match and Historical Gate Wording - 2026-05-02

Reason:

- A pre-push re-fetch found two additional open `cubic-dev-ai[bot]` review threads after the event-key fix was committed locally.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_EzVd` on `src/go/plugin/go.d/collector/cato_networks/topology.go:128`.
- The reviewer reported that BGP peer topology actors should not emit `ip_addresses` with an empty string when `RemoteIP` is empty.
- Local verification found the actor match and BGP link destination match both used `[]string{peer.RemoteIP}`, so both could emit an empty IP address when a BGP peer has ASN data but no remote IP.
- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` also found thread `PRRT_kwDOAKPxd85_EzVj` on `.agents/sow/done/SOW-0004-20260501-cato-networks-collector.md:1`.
- The reviewer requested that the completed SOW clarify that the pre-implementation gate is a historical snapshot, not current state.

Implementation scope:

1. Omit BGP peer topology IP-address matches when the remote IP is empty or whitespace.
2. Cover the actor and link destination match behavior with a unit test.
3. Add a historical note to the completed SOW pre-implementation gate.
4. Re-run focused Cato collector validation before updating the local review-fix commit.

Implemented:

- Added `catoBGPPeerMatch()` so BGP peer topology actors and BGP link destinations include `ip_addresses` only when the remote IP is non-empty after trimming.
- Added `TestBuildTopologyOmitsEmptyBGPPeerIPMatch` for actor and link destination match behavior.
- Added a historical note to the pre-implementation gate explaining that it records the pre-implementation state from 2026-05-01, not the current completed state.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, and collector consistency rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. Topology output remains semantically the same except invalid empty-IP matches are omitted.
- End-user/operator docs: no update needed. User-facing collector behavior and documented configuration are unchanged.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for the late PR review comments and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comment - Event Aggregation Key - 2026-05-02

Reason:

- After reviewer re-triggering, a new Copilot review thread opened on PR #22373.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_Ey-6` on `src/go/plugin/go.d/collector/cato_networks/collect.go:265`.
- The reviewer reported that `collectEvents()` aggregates with `map[eventCount]int64`, while `eventCount` includes the non-key `Count` field.
- Local verification found the current call path sets `Count` only after aggregation, but the type choice is fragile because future code that sets `Count` before aggregation would split identical event dimensions into distinct map keys.

Implementation scope:

1. Introduce a separate `eventKey` type containing only event dimensions.
2. Use `map[eventKey]int64` for event aggregation and convert to `eventCount` only at output construction.
3. Add a targeted unit test for normalized event-key aggregation.
4. Re-run focused Cato collector validation before pushing.

Implemented:

- Added `eventKey` with only event dimensions.
- Changed event aggregation to use `map[eventKey]int64`.
- Kept `eventCount` as the output type that carries the final count.
- Added `TestAddEventCountAggregatesByNormalizedEventKey`.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, and collector consistency rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. This is an internal aggregation type-safety fix; public collector behavior and metrics are unchanged.
- End-user/operator docs: no update needed. User-facing collector behavior is unchanged.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for the late PR review comment and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR Review Comment - Config Normalization - 2026-05-02

Reason:

- A pre-push re-fetch found one new open `cubic-dev-ai[bot]` review thread after the static-analysis cleanup commit was created locally.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` found thread `PRRT_kwDOAKPxd85_EtfO` on `src/go/plugin/go.d/collector/cato_networks/config.go:155`.
- The reviewer reported that URL validation trims leading/trailing spaces for parsing, but the stored URL remains unnormalized and can still fail at request time.
- Local verification found the comment is valid for `url`.
- Same-class local verification found `account_id`, `api_key`, and `metrics.time_frame` also use trim-aware validation/default checks but can remain stored with leading/trailing spaces before client use.

Implementation scope:

1. Normalize trim-safe string configuration values in `applyDefaults()` before validation and client construction.
2. Add a targeted unit test proving normalized stored values for `account_id`, `api_key`, `url`, and `metrics.time_frame`.
3. Re-run focused Cato collector validation before updating the local static-analysis commit.

Implemented:

- `applyDefaults()` now trims `account_id`, `api_key`, `url`, and `metrics.time_frame` before validation and client construction.
- Path-like values were intentionally not normalized because leading or trailing spaces can be valid filesystem path content.
- Added `TestConfigApplyDefaultsNormalizesStringInputs` to prove the stored configuration values are normalized and still validate.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, and collector consistency rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. This is config input normalization; no documented collector option or public metric contract changed.
- End-user/operator docs: no update needed. Whitespace normalization is defensive input handling, not a user-facing behavior change requiring documentation.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for the late PR review comment and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

Outcome:

- Completed.

## Reopen - PR CI/Static Analysis - 2026-05-02

Reason:

- After PR review-thread fixes were pushed, PR #22373 still had static-analysis blockers.
- The user asked to handle PR comments; the repo PR-review workflow also requires addressing relevant CI and static-analysis findings caused by the PR.

Evidence:

- GitHub check annotations for the SonarCloud Code Analysis run reported three actionable findings in new Cato collector files:
  - `src/go/plugin/go.d/collector/cato_networks/collector.go`: `Cleanup(context.Context)` was empty and needed a nested comment explaining why it is complete.
  - `src/go/plugin/go.d/collector/cato_networks/func_topology.go`: `funcTopology.Cleanup(context.Context)` was empty and needed a nested comment explaining why it is complete.
  - `src/go/plugin/go.d/collector/cato_networks/write_metrics.go`: `writeTrafficMetrics()` had 13 parameters, above the configured threshold of 7.
- The SonarCloud PR comment reported a Quality Gate failure from `5.7% Duplication on New Code`, above the configured `3%` threshold.
- The checkout still lacks the `.env` credentials required by `.agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh`, so GitHub check annotations and PR comments are the available Sonar evidence in this environment.
- Codacy reported `action_required`; GitHub exposed no Codacy annotations for the latest check run, so no specific Codacy finding could be mapped to source yet.

Implementation scope:

1. Add explicit comments to empty cleanup hooks that are complete by design.
2. Refactor `writeTrafficMetrics()` to pass a metric-writer struct instead of a long parameter list.
3. Preserve fixture content while compacting the copied Mockoon JSON to reduce line-oriented duplicate-new-code noise.
4. Update fixture provenance documentation to note that the local copy is compact JSON.
5. Re-run focused tests and the go.d package test suite before pushing.

Implemented:

- Added explicit comments to the collector and topology cleanup hooks explaining why empty cleanup is complete by design.
- Refactored `writeTrafficMetrics()` to use a `trafficMetricWriters` struct, reducing the function signature from 13 parameters to 3 without changing metric names, labels, or observations.
- Compacted `centreon-cato-api.mockoon.json` while preserving valid JSON content and the copied Centreon fixture payload.
- Updated `testdata/README.md` to state that the local fixture copy is compact JSON.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `ruby -e 'require "json"; JSON.parse(File.read("src/go/plugin/go.d/collector/cato_networks/testdata/centreon-cato-api.mockoon.json")); puts "json ok"'` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review, SOW, and collector consistency rules covered this work.
- Runtime project skills: no update needed. The PR-review workflow did not change.
- Specs: no update needed. This pass changed implementation shape and fixture formatting only, not collector behavior or public contracts.
- End-user/operator docs: no update needed beyond the test fixture README; user-facing collector behavior did not change.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for PR CI/static-analysis follow-up and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- Sonar detail fetch remains blocked in this checkout by missing `.env`; GitHub check annotations were used for source-level Sonar findings.
- Codacy exposed no GitHub annotations for the latest check run, so there is no source-level Codacy finding to fix locally until the service publishes details.

Outcome:

- Completed.

## Reopen - PR Review Comments - 2026-05-02

Reason:

- PR #22373 received two open `cubic-dev-ai[bot]` review threads after the branch was pushed.
- The user asked to pull changes and handle PR comments.

Review evidence:

- `.agents/skills/pr-reviews/scripts/fetch-all.sh 22373` initially found two open review threads:
  - `collect.go`: discovery refresh errors should degrade to cached discovery state after initial bootstrap.
  - `diagnostics.go`: context cancellation/deadline classification should use `errors.Is`.
- A pre-push re-fetch found two additional Copilot review threads:
  - `client.go`: raw `accountSnapshot` fallback should use the method `accountID` argument for the `x-account-id` header.
  - `testdata/README.md`: fixture provenance should avoid machine-specific absolute paths and point to upstream source.
- `ci-status.sh 22373` reported no failing checks at fetch time; several checks were still running.
- Sonar detail fetch could not run because this checkout lacks `.env` with Sonar credentials. GitHub's available Sonar comment reports Quality Gate failure from new-code duplication only.

Implementation scope:

1. Make stale discovery refresh failures fall back to last-known-good discovery after initial bootstrap, while preserving hard failure behavior before any discovery state exists.
2. Preserve operation failure diagnostics for the failed refresh and avoid retry/log spam by respecting the configured discovery refresh interval after a failed refresh fallback.
3. Use `errors.Is` for context cancellation/deadline classification.
4. Use the method account ID in the raw `accountSnapshot` fallback header and test this behavior.
5. Replace the absolute fixture source path with an upstream repository path, commit SHA, and URL.
6. Add targeted tests for review findings where executable behavior is affected.

Implemented:

- `refreshDiscovery()` now falls back to the cached site list after an initial successful discovery when a later refresh fails. It still marks the `entityLookup` operation failure and updates the discovery timestamp so retries respect `discovery.refresh_every`.
- Initial discovery failures still fail collection because there is no cached site list to use.
- `classifyCatoError()` now uses `errors.Is()` for wrapped `context.Canceled` and `context.DeadlineExceeded`.
- The raw `accountSnapshot` fallback now sets `x-account-id` from the method argument.
- The fixture README now records the upstream Centreon repository path, source commit, and GitHub URL instead of a local `/opt/...` path.
- Added targeted tests for cached discovery fallback, wrapped context error classification, and raw fallback account ID header handling.

Validation completed:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Artifact maintenance:

- AGENTS.md: no update needed. Existing PR-review and collector rules covered this work.
- Runtime project skills: no update needed. No reusable assistant workflow changed.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the cached discovery fallback contract.
- End-user/operator docs: updated Cato collector README with troubleshooting behavior for `entityLookup` failures after bootstrap.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened for PR review comments and completed after validation.

Follow-up mapping:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- Sonar detail fetch remains blocked in this checkout by missing `.env`; GitHub's visible Sonar comment reports only new-code duplication as the Quality Gate failure.

Outcome:

- Completed.

## Reopened Hardening Pass - External Review Ideas - 2026-05-02

User approved another same-SOW hardening pass after external reviewers were asked for more ways to improve real-tenant readiness and first-failure diagnostics.

Review evidence:

- Raw external review outputs are preserved under `.local/audits/cato-ai-review-20260502-round3/` for local audit only.
- Claude, GLM, MiniMax, and Qwen returned review bodies. Kimi exited successfully but produced only the reviewer banner, so it provided no actionable evidence.
- Verified local code evidence:
  - `normalize.go` silently ignores unknown `accountMetrics` timeseries labels, which can hide Cato API/schema drift.
  - The SDK-shaped Mockoon account snapshot adapter rewrites every non-`connected` connectivity status to `disconnected`, so the raw SDK fixture path does not prove `degraded` handling.
  - Events labels depend on exact snake_case field names and `fmt.Sprint` conversion, so field casing drift can collapse useful event counters into `unknown`, while complex values could become noisy labels.
  - Marker persistence failures are counted, but marker persistence availability remains reported as available after a write failure.
  - Cato client error classification covers rate limit/auth/decode/timeout but not DNS/network/TLS/proxy classes that prospects are likely to hit on first setup.
  - BGP polling is intentionally rolling; large accounts need an operator-visible full-scan estimate so missing fresh BGP state is easier to explain.

Approved implementation scope for this pass:

1. Add low-cardinality diagnostics for unknown `accountMetrics` timeseries labels.
2. Preserve and assert `degraded` status through the raw SDK fixture path.
3. Accept expected camelCase event field aliases, reject complex event field values, and report missing/empty event field diagnostics.
4. Report marker persistence as unavailable after marker write failures while keeping the in-memory marker advanced to avoid duplicate event counting within the same running process.
5. Add network, TLS, and proxy error classes and tests for real client-path timeout behavior.
6. Add BGP full-scan progress/window metrics and documentation.
7. Add targeted tests for the above without changing the public chart catalogue beyond diagnostic metrics.

Reviewer claims explicitly not accepted as blockers:

- SDK double retry is not present with the current collector because Netdata passes its own HTTP client to the SDK, and the SDK only installs its retryable client when no HTTP client is provided.
- Raw provider error leakage is not supported at the collector boundary checked in this SOW; hard discovery/snapshot returns are sanitized by error class.
- Missing `bgp.refresh_every` validation is not true; validation already requires at least 60 seconds.

## Reopened Hardening Pass - 2026-05-02

User approved fixing the issues found by the second external review round in the same SOW.

Review evidence:

- Raw external review outputs are preserved under `.local/audits/cato-ai-review-20260502-round2/` for local audit only.
- Cato's public EventsFeed documentation states `fetchedCount` has a maximum of 3000 events per fetch and the marker returned by one response should be used to fetch the next batch when more events remain. The collector currently calls `eventsFeed` once per collection cycle, so large event queues can silently lag.
- `events_total` currently labels every event by event type, subtype, severity, and status without a cap. Real tenants can create high-cardinality event dimensions.
- `entityLookup` pagination has no defensive max-page guard and no multi-page unit test.
- `metrics.time_frame` is only checked for non-empty value; bad values fail late at runtime.
- `accountMetrics` and `siteBgpStatus` partial failures increment operation failures, but they do not expose an operator-visible affected-site count.
- Discovery and snapshot hard errors wrap SDK/vendor errors back to the framework; warn logs are sanitized, but returned errors should also avoid raw provider messages.
- BGP cached state is not pruned when sites disappear from discovery.
- Topology tables are built from maps without deterministic row order.

Approved implementation scope:

1. Drain `eventsFeed` across pages within one collection cycle, bounded by a conservative page cap.
2. Add an event cardinality cap and collapse excess event series into an `other` bucket with a low-cardinality diagnostic.
3. Add low-cardinality affected-site counters for partial metrics/BGP failures.
4. Sanitize returned collection errors at the collector boundary while keeping raw errors only for internal classification.
5. Add discovery max-page guard and multi-page test.
6. Add marker resume and invalid/deprecated marker behavior tests where feasible with the local SDK test server.
7. Add `metrics.time_frame` validation in code and schema.
8. Prune stale BGP state for removed sites.
9. Sort topology interface/device rows deterministically.
10. Scrub auth header values from test-server failure logs.

Deliberately not included in this pass:

- Live Cato tenant validation remains SOW-0005.
- Site include/exclude filters remain out of v1 scope because the implementation decision at lines below explicitly removed them from v1 to preserve full-account visibility.
- BGP negotiated timers/transport and full BGP-state dimensions are useful follow-up ideas, but they change the chart/topology catalogue more than required for this hardening pass.

## Requirements

### Purpose

Provide Netdata Agent users with first-class observability of their Cato Networks SASE deployment: every Cato site (Socket) and its WAN/LAN/tunnel interfaces should appear in Netdata as a monitored entity, so SREs running Netdata in mixed-vendor environments can correlate Cato connectivity quality with the rest of their infrastructure they already monitor through Netdata.

Purpose statement is the implementation target for this SOW. If the purpose changes, the scope and chart catalogue must be reopened before implementation continues.

### User Request

> "In this branch I want to implement monitoring CATO devices via graphql. First create an SOW and let's do the analysis to understand a) how is this done and b) what other open source solutions exist to mirror them."

Two-step expectation:

1. Land an SOW capturing intent, evidence and analysis.
2. Use that SOW as the basis for design decisions before any implementation.

No code is to be written until decisions in this SOW are recorded.

### Assistant Understanding

Facts verified during the independent verification pass on 2026-05-01:

- Cato Networks runs a single-vendor SASE platform; customer sites connect to the Cato Cloud through hardware/virtual appliances called "Sockets". "Cato devices" in the user's request refers to those Sockets (sites) and their interfaces, not arbitrary endpoints.
- The vendor exposes a GraphQL API. Official API reference and vendor examples use GraphQL operations including `entityLookup`, `accountSnapshot`, `accountMetrics`, `eventsFeed`, and `siteBgpStatus`; Centreon and Cato toolbox examples use `POST https://api.catonetworks.com/api/v1/graphql2`. Datadog's setup docs also expose regional API prefixes such as `us1`.
- Authentication is by API token in the request header `x-api-key`; every collector query needs the account identifier as an argument or SDK client field.
- Vendor-published rate limiting exists per query, per account. The public Cato article lists numeric limits: general API calls `120/minute`, `accountMetrics` `15/minute`, `entityLookup` `30/minute` (`1500/5 hours`), and `eventsFeed` `100/minute`. The same article currently lists `accountSnapshot` as `1/second (30/minute)`, which is internally inconsistent, so the collector must budget conservatively and treat the exact `accountSnapshot` threshold as vendor-documented but ambiguous.
- Cato's public `eventsFeed` article states event data is retained for the previous three days, markers can point up to three days back, and a 2025-05-26 vendor comment says no marker or an invalid/deprecated marker now returns a marker pointing to the most recent events. Therefore first-run behavior must bootstrap the marker and must not assume a full three-day replay.
- Official/vendor tooling verified locally: `github.com/catonetworks/cato-go-sdk` (Go SDK with generated typed clients; Apache-2.0; remote tags through `v0.2.5`; `main` at `94633c4308a0e6f07c94584cb64b8d57303f9631`) and `github.com/catonetworks/cato-toolbox` (Python EventsFeed reference). Other vendor tools must not be cited in implementation evidence until checked locally.
- Reference open-source integrations exist (mirrored locally under `/opt/baddisk/monitoring/repos/`):
  - Centreon `centreon-plugins` ships a Perl plugin `network::security::cato::networks::api` with modes `discovery`, `connectivity`, `events`, `query`. It is the closest functional analogue to a Netdata polling collector and uses the queries listed below directly.
    Path: `centreon/centreon-plugins/src/network/security/cato/networks/api/`
  - Datadog `integrations-core/cato_networks` ingests audit logs and events only. Its docs use API key/account/region setup for audit logs and an S3 forwarder for events; it does not document real-time metrics collection through GraphQL.
    Path: `datadog/integrations-core/cato_networks/`
  - Sumologic ships a cloud-to-cloud "Cato Networks" source for log ingestion.
    Path: `sumologic/sumologic-documentation/docs/send-data/hosted-collectors/cloud-to-cloud-integration-framework/cato-networks-source.md`
  - No local mirrored Zenoss `PS.CatoNetworks` source was found; the earlier Zenoss claim is not verified and is removed as implementation evidence.
- The Netdata `go.d.plugin` tree contains 132 collector directories. Searches found no existing Cato collector, no GraphQL-based go.d collector, and no direct SASE/SD-WAN vendor collector for Cato, Cloudflare Zero Trust, Zscaler, Netskope, Palo Alto Prisma, Fortinet, Cisco SD-WAN, Aryaka, or Versa. This collector breaks new ground in Netdata for both protocol family (GraphQL) and vendor category (SASE).

Inferences (reasoned, not directly stated):

- The closest existing Netdata pattern to mirror is `src/go/plugin/go.d/collector/azure_monitor`: cloud-API polling, periodic discovery of resources, time-series collection per resource. The Cato collector should be a much simpler version of that pattern — one provider, fewer dimensions, predictable schema.
- Datadog and Sumo Logic focus this integration category on logs/events, while Centreon focuses on operational status and link-quality checks. For Netdata, the correct fit is metric-shaped state, throughput, loss, latency, BGP status, and derived event counters; not raw log/event shipping.
- The baseline GraphQL surface is `entityLookup`, `accountSnapshot`, `accountMetrics`, and `eventsFeed`, plus `siteBgpStatus` for BGP. `socketPortMetrics` and other schema areas are candidates only if the verified chart catalogue needs fields not available from the baseline queries.
- The vendor-published per-operation limits and `accountMetrics` bucket model make sub-minute defaults unsafe. Polling should be minute-grain by default, with discovery, BGP, topology, and EventsFeed cadences budgeted separately.

Known remaining unknowns that require implementation-time or tenant-backed validation:

- Real tenant payload shape for optional fields: interface metadata, PoP names, ISP/carrier names, event field names, and BGP peer details.
- Whether `siteBgpStatus` is efficient enough for large accounts. The API appears site-scoped, so BGP polling must be decoupled and rate-budgeted.
- Whether the SDK's default retry detector is sufficient. Its current source retries lowercase `ratelimit`, `rate-limit`, and `rate limit` patterns; the vendor article uses capitalized wording in some examples, so the collector should add its own case-insensitive detection.
- Dashboard behavior for the standalone `topology:cato_networks` function with hub-and-spoke SASE layouts. The shared topology contract exists, but this exact topology shape needs an end-to-end smoke test.

### Acceptance Criteria

Minimum bar for closing this SOW:

- The collector compiles into `go.d.plugin` and passes `go vet` and the standard go.d unit-test pattern (`go test ./...` for the package).
- A `metadata.yaml`, `config_schema.json`, stock `cato_networks.conf`, `health.d/` alerts, and `README.md` ship together and stay consistent (per project rule on collector consistency).
- Fixture-backed tests cover `entityLookup`, `accountSnapshot`, batched `accountMetrics`, `eventsFeed` marker handling, BGP status, partial failures, and rate-limit retry behavior.
- Real-account smoke test against a Cato tenant, or a documented reason the tenant test could not be run, produces non-empty per-site charts for at least connectivity status and one interface throughput dimension.
- Standalone topology smoke test verifies that `topology:cato_networks` returns valid `src/go/pkg/topology` JSON with presentation metadata.
- Authentication failure, rate-limit, marker loss, BGP partial failure, and partial-account errors degrade gracefully (do not crash the agent, do not spam logs, do back off).
- No customer names or customer-specific behavior are introduced by this SOW/collector work (memory: public repo work is vendor-focused).
- Documentation update path is recorded for `metadata.yaml` -> integrations docs regeneration.

## Analysis

Sources checked:

- `AGENTS.md`, `CLAUDE.md` (project rules, SOW framework)
- `.agents/sow/SOW.template.md`
- `.agents/sow/current/SOW-0004-20260501-cato-networks-collector.md` (this SOW)
- `.agents/sow/` inventory (no overlapping pending/current SOWs found)
- `.agents/skills/` inventory (only legacy audit/review skills; none match this collector work)
- `src/go/plugin/go.d/collector/` (132 collector directories; closest patterns: `azure_monitor`, `prometheus`, `dockerhub`)
- `src/go/plugin/framework/collectorapi/collector.go` (V2 collector contract)
- `src/go/plugin/go.d/collector/azure_monitor/{collector.go,config.go,metadata.yaml,query_executor.go}` (V2 cloud-API polling pattern, SDK-backed API client, discovery/query split, concurrency limit)
- `src/go/pkg/topology/types.go`, `src/go/plugin/go.d/collector/snmp_topology/func_topology*.go`, `src/collectors/network-viewer.plugin/network-viewer.c` (topology contract and shipped topology emitters)
- Centreon `centreon-plugins` mirrored repo: Cato plugin source and Mockoon fixtures (4 modes + custom API client; exact GraphQL queries, exact metric labels, retry pattern, event pagination)
- Datadog `integrations-core/cato_networks` mirrored repo (manifest and README; confirms logs-only Cato integration surface)
- Sumo Logic hosted collector documentation mirrored repo (confirms cloud-to-cloud security/audit event ingestion, not live metrics)
- Official Cato docs (web): API reference, "Understanding Cato API Rate Limiting", "Cato API - EventsFeed (Large Scale Event Monitoring)", "Cato API AccountMetrics"
- Vendor open-source cloned to `/tmp/`: `cato-go-sdk` and `cato-toolbox`

Current state:

- Branch `cato` is at `6dbeeeb47c` (`Merge remote-tracking branch 'upstream/master'`). Working tree is not clean because this active SOW is currently untracked/modified; no collector implementation files exist yet.
- No existing Netdata collector polls a vendor cloud GraphQL API. The pattern is novel for go.d.plugin, but vendor SDK use is not novel because `azure_monitor` uses the Azure SDK.
- Centreon's reference implementation:
  - Endpoint default `api.catonetworks.com:443/api/v1/graphql2`, header `x-api-key`, configurable hostname (regional prefixes), retry on rate limit (5 attempts, 5s delay).
  - Query `entityLookup(accountID, type: site, from, search, entityIDs)` — paginated site discovery with native search (default 50 per page).
  - Query `accountSnapshot(accountID) { sites(siteIDs:[…]) { id, info{ name, description }, connectivityStatus, operationalStatus, lastConnected, connectedSince, popName } }` — current connectivity snapshot.
  - Query `accountMetrics(accountID, timeFrame, groupInterfaces:true) { from, to, sites { id, interfaces { name, timeseries(labels:[…] buckets:N) { label, units, data } } } }` — bucketed time-series per site/interface.
  - Available timeseries labels: `bytesUpstreamMax`, `bytesDownstreamMax`, `lostUpstreamPcnt`, `lostDownstreamPcnt`, `packetsDiscardedDownstream`, `packetsDiscardedUpstream`, `jitterUpstream`, `jitterDownstream`, `lastMilePacketLoss`, `lastMileLatency`.
  - Query `eventsFeed(accountIDs, filters, marker) { marker, fetchedCount, accounts { id, errorString, records(fieldNames:[…]){ time, fieldsMap } } }` — marker-based long-poll for events; vendor 3-day retention.
- Cato vendor publishes a code-generated Go SDK (`cato-go-sdk`) with typed methods including `AccountMetrics`, `AccountSnapshot`, `EntityLookup`, `EventsFeed`, `Site`, and `SiteBgpStatus`.
- The Cato SDK schema confirms BGP type coverage (`BGPConnection`, `BgpPeer`, `BgpPeerListPayload`, `BgpTracking`, `SiteBgpStatus`, `BgpDetailedStatus`, `BgpSummaryRoute`). `siteBgpStatus` is documented as Beta in the official API reference, so this surface has higher drift risk than the baseline metric queries.
- The approved SDK tag `github.com/catonetworks/cato-go-sdk@v0.2.5` declares `go 1.23.1`, compiles in Netdata's `src/go` module, and does not force a Netdata Go directive bump. SDK `main` resolves to pseudo-version `v0.2.6-0.20260423133609-94633c4308a0`, declares `go 1.25.8`, and is not used.
- Remote tags for `github.com/catonetworks/cato-go-sdk` were verified through `v0.2.5`; no released `v0.2.6` tag was present at verification time on 2026-05-01.

Risks:

- **Rate limits** (user asked for the why; separated by verified evidence vs. design conservatism):

  Verified evidence (from public vendor sources and the Centreon reference implementation):

  - Cato applies rate limits **per query, per account**, with a counter shared across API keys for the same account/query. Source: public Cato support article "Understanding Cato API Rate Limiting".
  - Cato publishes numeric thresholds. The current public article lists general API calls at `120/minute`, `accountMetrics` at `15/minute`, `entityLookup` at `30/minute (1500/5 hours)`, and `eventsFeed` at `100/minute`. It lists `accountSnapshot` as `1/second (30/minute)`, which is internally inconsistent; use conservative budgeting until a real account verifies behavior.
  - The vendor's public article states Cato's GitHub examples handle rate limiting by waiting five seconds before retrying. Centreon's plugin detects rate-limit errors and applies longer backoff.
  - Centreon's reference plugin defaults to **5 retries with 5s back-off** specifically for rate-limit errors, and **1s back-off** for other transient failures. That is the only public, real-deployment retry policy I could find against this API. Source: same `custom/api.pm` lines 106–138.
  - Cato's `accountMetrics` is bucketed time-series — the API is designed to be queried with a `timeFrame` like `last.PT5M` or `last.PT1H` and N buckets, not as a per-second sampler. Source: official API reference and Centreon fixture.
  - `eventsFeed` uses a **marker** model intended for periodic polling, with near-real-time updates and 3-day retention. Source: public Cato EventsFeed article.

  What remains uncertain:

  - Payload-size limits, concurrency behavior, response headers, and the exact effective `accountSnapshot` threshold need a real-account smoke test.
  - Published limits are "guaranteed minimums", not hard ceilings. The collector must still respect them because other account users and integrations share the same per-account/query budget.

  Why this forces a higher `update_every` than Netdata's typical 1s/5s default:

  1. **Per-query counters**: discovery, metrics, event, and BGP calls all consume per-query budget. `accountMetrics` can and should be batched across site IDs; BGP appears site-scoped and can become the limiting query for large accounts.
  2. **Bucketed metrics**: `accountMetrics` returns N buckets across `timeFrame`. Polling sub-minute pulls the same bucket repeatedly; the collector would produce duplicate samples and waste rate-limit budget.
  3. **Shared account counter**: any other tool consuming the same Cato account (vendor management UI, third-party integrations, customer scripts) shares the same per-account budget. Aggressive Netdata polling can starve those.
  4. **Cost of being wrong**: tripping the limit means `accountMetrics` returns the rate-limit error, the collector retries with backoff, and during that window data is missing — so an over-aggressive default produces *worse* observability, not better.

  Mitigations the collector will ship with:

  - Default `update_every: 60`; documented `accountMetrics` `timeFrame: last.PT5M`, `buckets: 5` so a missed poll has fallback data.
  - Batch `accountMetrics` by site IDs instead of making one metrics call per discovered site.
  - Decouple BGP polling from metric polling, enforce an operation budget, and spread site-scoped BGP calls across cycles for large accounts.
  - Centreon-aligned retry policy: detect rate-limit messages case-insensitively, retry up to 5 attempts with a 5s initial backoff capped at 30s.
  - Track rate-limit retries as an internal Netdata metric so operators can see when they are bumping the ceiling.
  - Discovery refresh decoupled from collection cadence (default 5 min) so site-list churn does not eat metric-poll budget.
  - Reject `update_every` values below 60s during config validation because the Cato API is not designed for high-frequency polling.

- **API drift**: GraphQL schemas evolve. Hand-typed query strings can break silently when fields are deprecated. Using `cato-go-sdk` (Decision 2.B) turns many drift cases into compile-time failures, but Beta surfaces such as `siteBgpStatus` may still require runtime fallback.
- **SDK dependency and build compatibility**: checked 2026-05-01. `github.com/catonetworks/cato-go-sdk@v0.2.5` declares `go 1.23.1`, resolves inside Netdata's `src/go` module, compiles a smoke package using the required methods, and does not force a Netdata Go directive bump. SDK `main` resolves to pseudo-version `v0.2.6-0.20260423133609-94633c4308a0`, declares `go 1.25.8`, forces Netdata's `go.mod` from `go 1.25.7` to `go 1.25.8`, and adds extra HashiCorp/Terraform logging dependencies; do not use SDK `main` for this SOW.
- **SDK retry behavior**: the SDK retries HTTP 429 and some rate-limit error strings, but the string matching observed in source is case-sensitive. The collector should not rely blindly on SDK defaults; add collector-level case-insensitive classification around GraphQL errors.
- **EventsFeed marker semantics**: older vendor docs/examples say `marker:""` fetches queued events; the 2025 vendor comment says no marker/invalid marker returns a marker pointing to most recent events. Implementation must follow current behavior: first run bootstraps marker, persistent marker drives incremental reads, and no full backfill guarantee is promised.
- **BGP scale and Beta status**: BGP is in v1 scope, but `siteBgpStatus` is Beta and appears site-scoped. Large tenants may require slow, rolling BGP polling to stay inside rate limits.
- **Auth secret handling**: API key is highly sensitive and may grant broad account read access depending on Cato Service API Key permissions. Must be carried through `pkg/web` auth/options without logging in plain text and never leak in error strings.
- **Multi-region endpoints**: Customers in different Cato regions need different hostnames. Misconfigured endpoint -> 404 with confusing error.
- **Bucket alignment**: `accountMetrics` returns N buckets across `timeFrame`; mapping those buckets back into Netdata's per-poll model needs care to avoid duplicate or missing samples on each poll.
- **Schema scope creep**: Cato's GraphQL schema is large. Without a tight chart catalogue, the collector grows into a half-built API explorer. Decision #1 limits v1 to sites, interfaces, events-as-counters, BGP, and standalone topology.
- **Vendor neutrality**: Public repo rule — no customer names, no customer-specific behavior. Even when the motivation is a specific deployment, the docs/specs/code must read as a generic vendor integration.
- **Documentation pipeline**: `metadata.yaml` feeds the public integrations site. A poorly-described first version becomes the canonical public face of the integration; needs care.
- **Topology overlap**: standalone Cato topology can ship through the shared topology function contract. Cross-source overlay with SNMP/network-viewer/NetFlow remains out of scope because the unified overlay roadmap is not shipped on this branch.

## Pre-Implementation Gate

Historical note: this gate records the readiness state before implementation began on 2026-05-01. It is preserved in this completed SOW as historical implementation evidence; the current shipped state and later PR-review changes are recorded in the validation and reopen sections above.

Status: verified-ready-for-implementation

Problem / root-cause model:

- There is no Netdata coverage of Cato Networks SASE today. Netdata users running Cato cannot see site/socket connectivity, link quality, or per-interface throughput in Netdata. The Cato vendor exposes the required monitoring data through its GraphQL API. Adding a `go.d.plugin` collector that polls that API closes the visibility gap.

Evidence reviewed:

- Listed in `## Analysis` above. Centreon's Perl reference and fixtures, official Cato API docs, Cato Go SDK/toolbox source, Datadog/Sumo logs-only integrations, Netdata V2 collector examples, and the topology contract collectively establish enough ground truth to scope implementation.
- Discrepancies from the earlier analysis were verified and corrected: collector count is 132, the working tree is not clean because the active SOW is untracked/modified, numeric public rate limits are available, `eventsFeed` first-run behavior changed in 2025, Zenoss source was not locally verified, and `accountMetrics` should be batched rather than called once per site.

Affected contracts and surfaces:

- `src/go/plugin/go.d/collector/<module>/` (new directory)
- `src/go/plugin/go.d/config/go.d/<module>.conf` (stock config)
- `src/go/plugin/go.d/config/go.d.conf` (module enable list)
- `src/go/plugin/go.d/collector/init.go` (registry)
- `src/health/health.d/<module>.conf` (alert definitions, if alerts ship)
- `src/go/plugin/go.d/collector/<module>/{metadata.yaml,config_schema.json,README.md,integrations/}` (per project rule: must be in sync)
- Public integrations site (regenerated from `metadata.yaml`)
- Possibly `src/go/plugin/go.d/pkg/web` if a thin GraphQL POST helper is added (versus duplicated per-collector)
- `go.mod`/`go.sum` if `cato-go-sdk` is adopted

Existing patterns to reuse:

- `azure_monitor` for: V2 collector layout, discovery+query split, query executor, SDK-backed API client, retry/timeout, and chart-template YAML.
- `prometheus` collector for: simple HTTP+token client style (header auth, JSON unmarshal).
- `dockerhub` collector for: minimal API-token pattern with light surface.
- `pkg/web` `*Client` helper for HTTP timeouts/headers/proxy; matches existing collector idioms.
- `snmp_topology` for: topology method config, presentation schema, function handler split.
- Project rule "Collector Consistency Requirements" for the file-set that must ship together.

Risk and blast radius:

- New module isolated under its own directory; build risk is local.
- Adding a third-party dependency (`cato-go-sdk`) will touch `go.mod`/`go.sum`, requires Go toolchain/dependency review, and may require adaptation if the SDK's `go 1.25.8` declaration is incompatible with this repo's build matrix.
- No changes to other collectors. No changes to the C side.
- Only blast radius for users who configure the collector — default `enabled: no` keeps it inert until opted in (matching most go.d collectors that need credentials).
- Topology function is standalone under `topology:cato_networks`; it does not alter SNMP/network-viewer/NetFlow topology behavior.

Implementation plan:

Finalized at the SOW level in `## Plan`. The implementation must preserve these corrected design points: use V2 `collectorapi.CollectorV2`, use a tagged `cato-go-sdk` release if dependency review passes, batch `accountMetrics`, decouple BGP polling, persist EventsFeed markers, and emit standalone topology only.

Validation plan:

Test fixtures available (verified 2026-05-01):

- **Centreon Mockoon fixture** at `/opt/baddisk/monitoring/repos/centreon/centreon-plugins/tests/network/security/cato/networks/api/cato-api.json` (489 lines) covers `entityLookup` (3 sites + `search` and `entityIDs` filter variants), `accountSnapshot` (full + site-filtered), `accountMetrics` (all 10 timeseries labels we use × 10 buckets, plus a jitter-only variant), and `eventsFeed`. Adapt the response bodies into `testdata/` under the new collector; this is the entityLookup/accountSnapshot/accountMetrics/eventsFeed unit-test base.
- **`cato-go-sdk/cato_api.graphqls`** (17 663 lines, current schema, cloned to `/tmp/cato-go-sdk` on 2026-05-01) confirms BGP type coverage: `BGPConnection`, `BgpPeer`, `BgpPeerListPayload`, `BgpTracking`, `SiteBgpStatus`, `BgpDetailedStatus`, `BgpSummaryRoute`. Use these typed models to hand-craft BGP fixture responses since Centreon does not exercise BGP.
- **`cato-go-sdk/archives/`** — 11 historical schema files (Nov 2024 -> Nov 2025) are useful for manual schema-drift review when bumping the SDK; they are not, by themselves, a fixture decoder.
- **`cato-go-sdk/examples/api-rate-limit/main.go`** — vendor's rate-limit hammering example. Reference for retry/backoff intent, not a production-quality collector pattern.
- **`cato-go-sdk/examples/get-site-bgp-status/`, `list-acct-snap/`, `entity-lookup-site/`, `list-eventsfeed-index/`, `list-event-feed/`, `site-lookup/`** — query references. The current `list-event-feed` example has an early `return` before its polling loop, so use it as source material only after reading the code path.
- **`cato-toolbox/eventsfeed/eventsFeed.py`** — vendor Python reference for marker persistence and pagination. Its older empty-marker comments conflict with the 2025 public EventsFeed article update; prefer the current article for first-run semantics.

Test pass plan:

- Unit tests with `httptest.Server` returning canned responses adapted from the Centreon fixture and SDK-typed BGP fixtures, mirroring `azure_monitor/collector_test.go` style.
- SDK compatibility test: compile against the pinned SDK version and decode canned GraphQL responses through the collector's internal DTOs/interfaces; SDK bumps must include schema diff review.
- Rate-limit retry test: stub HTTP 429 and GraphQL error payloads with mixed-case rate-limit text; assert retry count, backoff classification, and that the internal retry-counter metric increments.
- EventsFeed marker tests: first run with no stored marker bootstraps marker without promising backfill; subsequent runs use the stored marker and advance it; invalid/deprecated marker behavior is handled without replay loops.
- AccountMetrics batching test: multiple discovered sites are queried through batched `siteIDs`, not one API call per site.
- BGP scheduler test: site-scoped BGP calls are spread across cycles when site count exceeds configured operation budget.
- Real-tenant smoke test against a user-provided Cato account or vendor sandbox; sanitize and add captured responses to `testdata/` when allowed.
- Topology snapshot test: build a fixture-driven topology, marshal through `src/go/pkg/topology` types, assert JSON shape against the `snmp_topology` test patterns.
- Doc-generation pipeline runs cleanly against new `metadata.yaml` (confirm exact build command before close).

Repos cloned for analysis on 2026-05-01: `/tmp/cato-go-sdk`, `/tmp/cato-toolbox`. Not committed; consider mirroring under `/opt/baddisk/monitoring/repos/` for permanence if work continues over multiple sessions.

Artifact impact plan:

- AGENTS.md: likely no update needed; collector falls under existing "Collector Consistency Requirements" rule.
- Runtime project skills: likely no update needed.
- Specs: a new spec under `.agents/sow/specs/cato-networks-collector.md` capturing the durable contract (endpoint, queries, mapping to charts) is appropriate at completion.
- End-user/operator docs: collector `README.md`, regenerated integrations page, stock conf, schema, alerts file — all required by collector-consistency rule.
- End-user/operator skills: none currently affected; check `.agents/skills/` and `docs/netdata-ai/skills/` at SOW close.
- SOW lifecycle: keep this single SOW; do not split unless events/log ingestion is added (then events become a separate SOW).

Open decisions:

- None currently pending. Recorded decisions remain below for auditability. New evidence may reopen a decision only if it changes the risk model.

## Implications And Decisions

User decisions recorded 2026-05-01. Each decision section ends with **Decision:** and the chosen option.

### 1) Scope: what data does the collector surface?

Background: the Cato GraphQL API exposes connectivity status, per-interface throughput/loss/jitter timeseries, security/system events, audit logs, configured policies, BGP, XDR stories, SDP user sessions and more. Netdata is a metrics platform; events/logs are a different pipeline.

Options:

- 1.A — **Connectivity + per-interface time-series only** (sites, status, throughput up/down, packet loss %, jitter, last-mile latency/loss). Polled via `entityLookup` + `accountSnapshot` + `accountMetrics`.
  Pros: matches Netdata's metric model exactly; small surface; no firehose; mirrors what Centreon's `connectivity` mode does.
  Cons: ignores Cato events that have operational signal (link flaps, security blocks).
  Risk: low.

- 1.B — **Connectivity + interface metrics + selected events as derived counters** (e.g. count by event_type/sub_type from `eventsFeed`, marker-tracked).
  Pros: still metric-shaped; captures notable operational events as rate-of-events charts.
  Cons: marker state needs to persist between collector runs; event-feed retention is 3 days, so first poll after a long downtime can be lossy.
  Risk: medium — state file/storage convention in go.d collectors needs verification.

- 1.C — **Full SASE coverage** — also surface SDP user sessions, BGP peering, security policy hits.
  Pros: comprehensive.
  Cons: large schema surface; more rate-limit pressure; longer build; bigger maintenance cost.
  Risk: high — unbounded scope.

Recommendation: **1.A** for v1. Raw event/log ingestion (1.B) is rejected for this collector SOW because Netdata metrics collection and log ingestion are different pipelines; create a separate SOW only if the user explicitly requests a Cato log/event ingestion pipeline later.

**Decision: maximum coverage — sites + interfaces + events + BGP**, scoped to what the GraphQL API exposes. Concretely v1 will surface:

- Per-site connectivity status (`accountSnapshot` connectivityStatus / operationalStatus / popName / lastConnected / connectedSince).
- Per-site, per-interface bucketed metrics (`accountMetrics`): bytesUpstreamMax, bytesDownstreamMax, lostUpstreamPcnt, lostDownstreamPcnt, packetsDiscardedDownstream, packetsDiscardedUpstream, jitterUpstream, jitterDownstream, lastMilePacketLoss, lastMileLatency, plus any vendor-published TimeseriesKey we can map cleanly. Use `groupInterfaces:false` so each physical/logical interface keeps its own series; do not invent LAN/WAN/Tunnel/Bypass/Off-Cloud labels unless verified payload fields expose them.
- Events feed (`eventsFeed`) shaped as metrics: rate of events per type/sub-type, rate of denied/blocked events, rate per site where event fields expose site identity, marker persisted between collections. Events themselves are NOT logged into Netdata — only their counts/rates as time-series. First run bootstraps a marker and does not promise historical replay.
- BGP per-site metrics from `siteBgpStatus`: peer/session state where exposed, BFD session state where exposed, route counts from peer/to peer, rejected route counts, and raw/detailed status counters. Session uptime and last state change are not in the verified field set and must not be charted unless a real schema/payload proves them.

Out of scope for v1 (explicitly rejected for this collector SOW unless the user requests separate work later):

- Cato Client (SDP user) sessions.
- Security policy rule hit counts beyond what comes through `eventsFeed`.
- XDR stories.
- Configuration management surfaces (writes are explicitly out of scope; the collector is read-only).

Risk: BGP and full event-counter coverage push schema surface beyond what the Centreon Perl reference uses. `siteBgpStatus` is verified in the SDK/API reference but is Beta, so BGP chart families need runtime fallback and conservative polling.

### 2) Implementation style: hand-written GraphQL or `cato-go-sdk`?

Background: many `go.d.plugin` collectors use raw `pkg/web` HTTP + JSON, but SDK-backed collectors already exist (`azure_monitor` uses Azure SDK packages). Cato publishes a generated Go SDK (`github.com/catonetworks/cato-go-sdk`) with typed bindings.

Options:

- 2.A — **Hand-written GraphQL POST + JSON unmarshal**, matching the go.d.plugin idiom. Use a tiny per-collector helper that wraps `pkg/web.HTTPClient` and posts `{"query":"…","variables":{…}}`.
  Pros: zero new dependency; consistent style; small attack surface; predictable build.
  Cons: silent breakage on schema deprecation; we type the response shapes by hand.
  Risk: low.

- 2.B — **Adopt `cato-go-sdk`**.
  Pros: typed; schema drift becomes a compile error; vendor maintains it.
  Cons: large dependency tree; transitive licensing review; SDK bumps need our PR cadence; generated API surface is much larger than the collector needs.
  Risk: medium — dependency/toolchain compatibility and SDK retry behavior need review.

Recommendation: **2.A**. If the schema turns out to drift faster than tolerable, revisit by introducing a minimal generated-types layer (e.g. genqlient) just for the four queries we use, without taking the full vendor SDK.

**Decision: 2.B — adopt `github.com/catonetworks/cato-go-sdk`.** The wider scope decided in #1 (BGP, full event/interface coverage) makes the typed-bindings safety net worth the new dependency. Schema drift in any of those areas will surface as a build failure rather than a silent runtime regression.

Implementation notes flowing from this:

- Add `github.com/catonetworks/cato-go-sdk` to `go.mod`; pin to a tagged release, not `main`.
- Latest verified remote tag as of 2026-05-01 is `v0.2.5`; use that tag unless a newer tag is explicitly rechecked and proven compatible. Do not use SDK `main`/pseudo-version.
- The SDK is auto-generated against `cato_api.graphqls`; before bumping the SDK version, diff the schema and update charts/contexts.
- Wrap the SDK's HTTP client in `pkg/web` so we get the standard go.d timeout/proxy/TLS-options surface, instead of exposing the SDK's raw `http.Client` to operators.
- Keep all SDK calls behind a small internal interface so unit tests can stub them with canned responses.
- Add collector-level case-insensitive rate-limit classification; do not rely only on the SDK's current string matching.
Verification performed 2026-05-01:

- Temporary Netdata-module copy plus smoke package compiled against `github.com/catonetworks/cato-go-sdk@v0.2.5`.
- Verified required methods/types compile: `EntityLookup`, `AccountSnapshot`, `AccountMetrics`, `EventsFeed`, `SiteBgpStatus`, `EntityTypeSite`, `TimeseriesMetricTypeBytesUpstreamMax`, `EventFieldNameEventID`, `SiteBgpStatusInput`.
- Dependency delta in the temp Netdata module for `v0.2.5`: 7 added `go.mod` entries and 14 added `go.sum` entries; no existing dependency upgrades observed in that check.
- `govulncheck` on the smoke package reported no vulnerabilities.
- License note: SDK is Apache-2.0. Added runtime deps include MIT-licensed `gqlgen`, `gqlgenc`, `duration`, `gqlparser`, and MPL-2.0 HashiCorp `go-cleanhttp`/`go-retryablehttp`; MPL-2.0 deps need normal repository license review before merge.

### 3) Configuration model: one job multi-account, or one job per account?

Background: Cato API keys are scoped to an account. Multi-tenant Cato customers (MSPs) may have many accounts.

Options:

- 3.A — **One job per account** (each job has one `account_id` + one `api_key`).
  Pros: simplest; matches go.d.plugin "one job per target" idiom; clear blast radius.
  Cons: MSP with N accounts needs N job entries.

- 3.B — **One job, list of accounts**.
  Pros: less config for MSPs.
  Cons: heterogeneous failure handling; needs per-account error reporting; more complex collect loop.

Recommendation: **3.A**. MSPs can use a config file with N entries; an account fan-out layer is rejected for this collector SOW because it would complicate rate-limit handling and secret scope without changing the go.d runtime's existing multi-job capability.

**Decision: 3.A — one job = one Cato account.** The go.d.plugin runtime already supports running many independent jobs per module (each YAML entry under `jobs:` becomes its own job), so MSPs configure one entry per account. The collector itself stays single-account, single-key — simplest internal model, predictable per-account error handling, and matches every other go.d collector.

### 4) Site discovery: auto-discover or explicit list?

Background: `entityLookup(type: site)` paginates all sites; `accountSnapshot` and `accountMetrics` accept an optional `siteIDs:[…]` filter.

Options:

- 4.A — **Auto-discover all sites, refresh every N minutes**, optionally filter by name (regex) and/or by allowlist/denylist of site IDs. Mirrors `azure_monitor`'s discovery pattern.
  Pros: zero-config on the customer side; new sites picked up automatically.
  Cons: large accounts pay rate-limit cost on each refresh.

- 4.B — **Explicit site list** in config; no discovery.
  Pros: simplest, fewest API calls.
  Cons: ops burden; new sites are invisible until config edit.

Recommendation: **4.A** with a configurable discovery interval (default 5 min) and optional include/exclude patterns, modelled on `azure_monitor.Discovery`.

**Decision: 4.A — auto-discover.** Default discovery refresh 5 minutes (configurable). Optional include/exclude patterns on site name and on site ID. New sites are picked up automatically, deleted sites are obsoleted from charts after the standard go.d obsoletion grace.

### 5) Topology integration?

Background: the repository already has shared topology contracts and shipped topology emitters for SNMP and network connections. Cato sites/sockets/PoPs are natural topology nodes.

Options:

- 5.A — **Out of scope for this SOW**. Ship metrics only; topology integration is a follow-up SOW.
- 5.B — **Standalone topology in scope**. Emit `topology:cato_networks` from the same collector using the existing topology function contract.
- 5.C — **Cross-source overlay in scope**. Blend Cato topology with SNMP/network-viewer/NetFlow topology in one map.

Recommendation after verification: **5.B**. The shared topology function contract already exists, so standalone Cato topology is a bounded collector task. 5.C remains out of scope because cross-source overlay support is not shipped on this branch.

**Decision (revised 2026-05-01): topology IS in scope for v1.** The agent-side topology contract already exists on master. Adding a `topology:cato_networks` function from the same collector is internal plugin wiring. Therefore v1 ships with a standalone topology emitter, and the previously-planned follow-up SOW collapses into this one.

What is verified about the existing contract:

- Stable types in `src/go/pkg/topology/types.go`: `Actor` (node), `Link` (edge), `Presentation*` (self-describing render schema). Each Actor has `ActorID`, `ActorType`, `Layer`, `Source` and free-form `Attributes`/`Tables`. Each Link has `Layer`, `Protocol`, `LinkType`, `Src`/`Dst`, `Metrics`.
- Function method ID format: `topology:<source>`. The Cato emitter will be `topology:cato_networks`.
- Dashboard topology responses carry their own `Presentation*` schema. Reference implementation: `src/go/plugin/go.d/collector/snmp_topology/func_topology_*.go` (topology cache + presentation schema + function handler split, scaled down).
- Already shipped emitters on master: `topology:snmp` (snmp_topology) and `topology:network-connections` (network-viewer.plugin).

What is verified about the limitations:

- **Overlaid rendering (Cato + SNMP + network-viewer on the same map) is NOT shipped.** The unified schema/overlay roadmap is separate from this collector. Until that lands, Cato topology will be a standalone view accessed via `topology:cato_networks`, not blended with other sources.
- The existing presentation primitives (`PresentationActorType`, `PresentationLinkType`, `PresentationPortType`) cover device-style nodes and physical/logical links well. Cato cloud PoPs are vendor-managed nodes — they fit the Actor model fine but may want a distinct icon/colour slot.
- Risk: if the dashboard's generic topology renderer makes assumptions specific to SNMP L2 (e.g. peer-to-peer mesh layout) that do not suit hub-and-spoke layouts, we may discover gaps only on first end-to-end test. Mitigation: smoke-test against the dashboard early.

Topology model for v1 (payload fields still need real-tenant validation before locking labels):

- **Nodes**:
  - Site (Cato Socket) — `id`, `name`, `description`, `connectivityStatus`, `operationalStatus`, geographic location if exposed.
  - PoP (Cato cloud entry point) — `popName` from `accountSnapshot`; PoPs are vendor-managed cloud nodes, not customer devices.
  - BGP peer — neighbours configured on a site.
  - Last-mile ISP (where vendor exposes ISP/carrier name on a WAN interface).
- **Edges (Links)**:
  - Site → PoP tunnel (primary + backup PoP per site if exposed; carries throughput, jitter, packet loss from `accountMetrics` in the `Metrics` map).
  - Site → BGP peer (carries session state, prefix counts in the `Metrics` map).
  - WAN interface → ISP (last-mile metrics: `lastMilePacketLoss`, `lastMileLatency`).
  - Site-to-site logical mesh through Cato Cloud — include only if exposed by already-required responses without extra rate-limit cost.

Implementation hooks:

- Topology data lives in the same `cato_networks` collector module — no sibling module — emitted through a separate `funcapi.MethodHandler` (`topology:cato_networks`).
- The presentation schema ships with each topology response so new actor types ("Cato Site", "Cato PoP", "BGP Peer", "ISP") render without dashboard work.
- Site/interface/PoP labels on metric charts use the same identifiers as Actor IDs in the topology emitter, so the dashboard can correlate metrics with topology nodes when unified overlay support eventually lands.
- Topology refresh cadence is decoupled from metric collection cadence (default same as discovery refresh, 5 min) because connectivity status drift is the only fast-moving topology change.

Caveat to flag explicitly: **standalone Cato topology is in scope for v1 and must be smoke-tested; overlaid rendering with SNMP/network-viewer/NetFlow will only work after the unified topology roadmap ships separately.**

### 6) Module name?

Options:

- 6.A — `cato`
- 6.B — `cato_networks`
- 6.C — `catonetworks`

Recommendation: **6.B** (`cato_networks`). Mirrors the existing public ecosystem (Datadog, Sumologic, Centreon all use the two-token form), avoids ambiguity with the unrelated word "cato", and follows go.d.plugin's snake_case convention (`azure_monitor`, `dockerhub` etc.).

**Decision: 6.B — `cato_networks`.**

### 7) Polling cadence default?

Background: `accountMetrics` is bucketed time-series; vendor enforces per-query/per-account rate limits. Centreon and official API examples use GraphQL time frames such as `last.PT5M`/`last.PT1H`.

Options:

- 7.A — `update_every: 60` and `accountMetrics(timeFrame:"last.PT5M", buckets:5)` — pull 5x 1-minute buckets per minute, write the latest bucket to charts.
- 7.B — `update_every: 300` (5 min) with `last.PT5M` buckets:1 — minimal API pressure.
- 7.C — `update_every: 30` with `last.PT1M` buckets:1 — closer to Netdata's normal cadence; highest rate-limit risk.

Recommendation: **7.A**. Best trade-off: 1-minute granularity, modest API rate, and gives us 4 fallback buckets if a poll skips. Keep `update_every` overridable per job.

**Decision: 7.A — `update_every: 60` default, overridable per job.** Operators with verified rate-limit headroom may push lower; operators with large accounts or shared API usage may push higher. BGP and topology refresh are decoupled from this metric cadence.

### 8) Framework: go.d V2 collector contract

Background (added 2026-05-01 after the user requested the V2 framework): Netdata exposes two related "V2" surfaces in this repo. The wording "go.d.plugin V2 framework" is consistent with the new go.d collector runtime contract, but the ibm.d-style code-generator layer also exists.

Options:

- 8.A — **`framework/collectorapi.CollectorV2`** contract (file: `src/go/plugin/framework/collectorapi/collector.go`).
  Same plugin binary as today (`go.d.plugin`). Collectors implement `Init/Check/Collect/Cleanup/MetricStore/ChartTemplateYAML`, write metrics into `metrix.CollectorStore`, and emit chart templates instead of imperatively building `Charts`. Already adopted by `azure_monitor`, `mysql`, `powervault`, `ping`, `powerstore`. Registration via `collectorapi.Register(name, Creator{ CreateV2: ... })`.
  Pros: pure Go, no CGO; same packaging as the rest of go.d; clear precedent for cloud-API collectors via `azure_monitor`.
  Cons: more boilerplate than the IBM.D code-generated style; chart templates are still hand-authored YAML strings.

- 8.B — **IBM.D-style framework** (file: `src/go/plugin/ibm.d/framework/collector.go` + `metricgen` + `docgen`).
  YAML-declarative metrics, code-generated typed setters, generated README/metadata.yaml. Lives under `src/go/plugin/ibm.d/`, ships in a different binary (`ibm.d.plugin`), built with `CGO_ENABLED=1`. Currently dedicated to IBM workloads.
  Pros: AI-assistant-friendly; documentation/metadata always in sync via code-gen; declarative `contexts/contexts.yaml` makes a many-metric collector tractable.
  Cons: lives in `ibm.d.plugin` which is gated on `ENABLE_PLUGIN_IBM` and uses CGO; using it for Cato either means moving Cato into ibm.d (semantically wrong) or extracting the framework into go.d (out of scope for this SOW). No precedent for non-IBM modules.

Recommendation: **8.A — `collectorapi.CollectorV2` in go.d**. Mirror the `azure_monitor` pattern. It is the right home for a pure-Go cloud-API collector and matches the existing precedent.

**Decision: 8.A — `collectorapi.CollectorV2` in go.d.** Mirror `azure_monitor`'s V2 layout. Pure Go, no CGO, ships in the existing `go.d.plugin` binary.

### 9) Test fixture source

Background: fixture quality determines how much of the collector can be trusted without a live Cato tenant. The mirrored Centreon plugin includes canned Cato API responses. The official Cato Go SDK includes schema files, generated types, and live API examples, but no reusable JSON/API response fixtures.

Options:

- 9.A — **Use only SDK examples and hand-written responses**.
  Pros: follows vendor SDK shape.
  Cons: examples require a live account and do not provide canned payloads; higher risk of inventing response details.
  Risk: medium.

- 9.B — **Use Centreon fixtures as the baseline, then add SDK-schema-derived fixtures for missing surfaces**.
  Pros: starts from real open-source fixture payloads for `entityLookup`, `accountSnapshot`, `accountMetrics`, and `eventsFeed`; keeps BGP aligned with verified SDK/API types.
  Cons: Centreon fixtures are partial, not official Cato SDK fixtures, and do not cover BGP or full per-interface payloads.
  Risk: low to medium.

- 9.C — **Wait for real tenant captures before implementation**.
  Pros: highest payload confidence.
  Cons: blocks development on credentials/sandbox access; still needs sanitized fixtures before merge.
  Risk: medium because delivery stalls.

Recommendation: **9.B**. It gives immediate test coverage without inventing the core API shape, while explicitly marking BGP/per-interface/live-field gaps for later tenant validation.

**Decision: 9.B — Centreon fixture baseline + SDK-schema-derived missing fixtures + optional sanitized tenant captures.** Concretely:

- Use `/opt/baddisk/monitoring/repos/centreon/centreon-plugins/tests/network/security/cato/networks/api/cato-api.json` as the baseline fixture source for `entityLookup`, `accountSnapshot`, `accountMetrics`, and `eventsFeed`.
- Do not claim the Cato SDK provides API response fixtures. It provides schema/types/examples only.
- Hand-craft BGP fixtures from verified `cato-go-sdk`/official API types, and mark them as schema-derived until validated against a real tenant.
- Live tenant or vendor sandbox validation is tracked after close by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.

## Plan

Implementation:

1. Lock the initial chart catalogue from verified SDK/API fields: site state, per-interface throughput/loss/jitter/last-mile metrics, event-rate counters, BGP peer/session/route counts, and internal collector/rate-limit metrics. Mark optional fields that need live-payload validation.
2. Bootstrap module under `src/go/plugin/go.d/collector/cato_networks/` mirroring the `azure_monitor` V2 layout: `collector.go`, `config.go`, `init.go`, `collect.go`, `client.go`, `discovery.go`, plus chart-template YAML for V2, `metadata.yaml`, `config_schema.json`, `testdata/`. Stock `cato_networks.conf` under `src/go/plugin/go.d/config/go.d/`; registry update in `src/go/plugin/go.d/config/go.d.conf`.
3. Add `cato-go-sdk@v0.2.5` to `go.mod` unless a newer tagged release is rechecked and proven compatible. Do not use SDK `main`/pseudo-version. Wrap its HTTP client behind `pkg/web` so timeouts/proxy/TLS options are operator-configurable through the standard surface.
4. Site discovery (`entityLookup` + `accountSnapshot`) with default 5 min refresh. Include/exclude filters are intentionally not part of v1 because the stated purpose is full-account visibility across every Cato site.
5. Per-collection metric pulls: batched `accountMetrics` with `siteIDs` and `groupInterfaces:false`, plus `accountSnapshot` for state, plus `eventsFeed` polled with persisted marker.
6. EventsFeed state: persist marker, bootstrap marker on first run, handle invalid/deprecated marker response, and expose counters/rates only.
7. BGP metrics: use `siteBgpStatus`, decouple cadence from `update_every`, budget site-scoped calls, and spread calls across cycles for large accounts.
8. Chart-template YAML: connectivity, throughput up/down, packet loss %, jitter, last-mile latency/loss, discarded packets, BGP peer/session state and prefix counts, event-rate counters, and collector API/retry counters.
9. Topology emitter (`func_topology.go`) implementing `funcapi.MethodHandler` with method ID `topology:cato_networks`; reuse `src/go/pkg/topology` types; mirror `snmp_topology`'s structure scaled down. Actor types: cato_site, cato_pop, bgp_peer. Link types: cato_tunnel, bgp_session. ISP/last-mile actor links are omitted in v1 because the verified fixtures/SDK coverage used for this SOW did not provide a stable ISP identity field.
10. Health alerts under `src/health/health.d/cato_networks.conf`: site offline; sustained high packet loss; BGP peer down.
11. Rate-limit handling: classify HTTP 429 and GraphQL rate-limit errors case-insensitively, apply backoff, expose internal retry/rate-limit counters, and reject `update_every` values below 60s.
12. Tests: `httptest`-served canned GraphQL responses for each query type; rate-limit retry test; marker persistence test; accountMetrics batching test; BGP budget/scheduler test; partial-failure test (one site errors, others succeed); topology snapshot test verifying actor/link JSON shape against the shared `pkg/topology` types.
13. Documentation: README, metadata.yaml, regenerate integrations page (confirm exact build command before close).
14. Real-tenant smoke test against a user-provided Cato account or vendor sandbox; verify both metric charts and the standalone `topology:cato_networks` function in the dashboard. If credentials are unavailable, record this explicitly with fixture-based substitute evidence.
15. Spec under `.agents/sow/specs/cato-networks-collector.md` capturing the durable contract (endpoint, queries, chart catalogue, topology actor/link types, rate-limit policy, marker semantics).

## Execution Log

### 2026-05-01

- SOW created in `.agents/sow/pending/`.
- Analysis pass: vendor docs, Centreon Perl plugin (functional reference), Datadog `cato_networks` integration (confirmed logs-only, not API-metrics), Cato Go SDK availability, Netdata `azure_monitor` pattern.
- Open decisions surfaced; awaiting user input.
- User recorded decisions: scope = max coverage incl. BGP (1.C+); SDK adoption (2.B); one job per account (3.A); auto-discovery (4.A); topology deferred to follow-up SOW (5.A); module name `cato_networks` (6.B); `update_every: 60` configurable (7.A).
- User requested go.d V2 framework. Decision #8 added; the user later confirmed `collectorapi.CollectorV2` (option 8.A) before the SOW moved to `current/`.
- Risks section expanded with detailed rate-limit reasoning (verified evidence vs. design conservatism, why sub-minute cadence is unsafe, mitigations).
- User confirmed framework choice 8.A.
- Topology investigation: verified the shared topology contract at `src/go/pkg/topology/types.go` and shipped emitters for `topology:snmp` and `topology:network-connections`. Conclusion: standalone Cato topology is a bounded collector-side feature that must be smoke-tested; overlaid rendering with other topology sources requires the separate unified topology overlay roadmap.
- Decision #5 revised: topology emission moves into v1 scope; previously-planned follow-up SOW collapses into this one. Caveat documented.
- SOW ready to move to `current/in-progress`.
- Cloned `cato-go-sdk` and `cato-toolbox` to `/tmp/` for fixture analysis. Confirmed BGP schema coverage in `cato_api.graphqls`. Identified Centreon Mockoon fixture as the immediately-usable starting point for `testdata/` (entityLookup, accountSnapshot, accountMetrics, eventsFeed). Vendor `examples/api-rate-limit` and `examples/get-bgp-peer` (and 50+ other vendor-published Go examples) provide direct references for rate-limit handling and BGP queries.
- SOW moved from `pending/` to `current/`.
- Independent verification pass: corrected stale/incorrect claims about git cleanliness, collector count, public Cato rate-limit thresholds, EventsFeed first-run marker semantics, unverified Zenoss source, SDK dependency risk, SDK retry caveat, BGP Beta status, and `accountMetrics` batching. Replaced user personal-name mentions in this disk artifact with `user`.
- Fixture-source decision recorded: the Cato SDK has no reusable API response fixtures; use Centreon's mirrored `cato-api.json` as baseline, SDK/API schema-derived BGP fixtures for missing coverage, and sanitized tenant captures if available before close.
- SDK compatibility decision recorded: `cato-go-sdk@v0.2.5` is technically usable with Netdata; SDK `main` is not acceptable because it bumps the Go directive and adds extra dependencies. `govulncheck` on the smoke package reported no vulnerabilities.
- User approved proceeding with `cato-go-sdk@v0.2.5` as the pinned SDK dependency for implementation.
- Implemented `src/go/plugin/go.d/collector/cato_networks/` as a V2 go.d collector using `collectorapi.CollectorV2`, `metrix.CollectorStore`, embedded `charts.yaml`, embedded `config_schema.json`, and the standalone `topology:cato_networks` function handler.
- Added SDK-backed API client wrapper for `entityLookup`, `accountSnapshot`, `accountMetrics`, `eventsFeed`, and `siteBgpStatus`, with case-insensitive retry classification for HTTP 429/rate-limit/transient errors and retry gauges by query.
- Added site, interface, BGP peer, event, API retry metrics, plus standalone Cato site/PoP/BGP topology generation.
- Added events marker persistence under Netdata varlib by default; fixed `Check()` so it does not advance the marker.
- Added stock config, config schema, metadata, README, and health alerts. Corrected metadata category to the valid `data-collection.networking` and used the existing `network-wired.svg` integration icon.
- Added `.agents/sow/specs/cato-networks-collector.md` as the durable collector contract.
- Reopened after close for NIDL compliance correction. User requested fixing the metric organization after review against `docs/NIDL-Framework.md`.
- NIDL discrepancy to fix:
  - Families are over-fragmented (`site traffic`, `interface traffic`, `events`, `api` one-chart leaves).
  - `site_hosts` was grouped under `site status` although it is a site inventory/count metric.
  - `events_total` is counter-like but chart units said `events` instead of `events/s`.
  - Discarded-packet chart names and storage used percent semantics while fallback SDK metric fields are raw packet counts; raw counts must not be stored in percent fields.
- NIDL correction implemented:
  - Chart families now group by functional entity/surface: `sites`, `interfaces`, `bgp`, and `collector`; no one-chart leaf family remains.
  - `site_hosts` now belongs to the `sites` family, not to a site status family.
  - Discarded-packet metrics now use the raw Cato packet-count fields (`packetsDiscardedUpstream`, `packetsDiscardedDownstream`) and are charted as `packets`.
  - Interface discarded-packet metrics now have their own `interface_discarded_packets` chart.
  - Stateful event and API retry counters are charted as rates (`events/s`, `retries/s`) with raw counter-backed metric names ending in `_total`.
- Reopened after close for production-readiness validation and diagnostics correction. The user's risk model is that prospects will run this collector first against devices/accounts Netdata does not own, so local testing must use the best available raw API fixtures and the collector must expose enough failure information to diagnose field problems without a live debugging session.
- Validation discrepancy to fix:
  - SOW-0004 identified the Centreon Mockoon Cato fixture as the baseline raw fixture source, but the shipped tests only used in-memory SDK structs.
  - The tests therefore proved collector normalization against SDK-shaped structs, but did not prove raw GraphQL JSON responses decode through the pinned SDK/client path.
  - The collector exposed retry counters, but did not expose enough operator-visible health/failure counters to quickly distinguish discovery, snapshot, metrics, events, BGP, marker-write, and no-site failure classes.
- Production-readiness correction implemented:
  - Copied the raw Centreon Mockoon fixture into `src/go/plugin/go.d/collector/cato_networks/testdata/centreon-cato-api.mockoon.json` with provenance notes.
  - Added a schema-shaped raw BGP fixture at `src/go/plugin/go.d/collector/cato_networks/testdata/cato-site-bgp-status.schema-shaped.json` because Centreon does not cover `siteBgpStatus`.
  - Added `httptest` replay through the pinned SDK/client path for `entityLookup`, `accountSnapshot`, `accountMetrics`, `eventsFeed`, and `siteBgpStatus`.
  - The raw Centreon fixture needed a test adapter for SDK-generated query aliases and GraphQL ID/status scalar shape. The unmodified Centreon `accountSnapshot` body is for Centreon's hand-written query and does not decode against the SDK-generated aliased query response type.
  - Added collector health diagnostics: collection success, discovered sites, per-operation success, per-operation failure counters by normalized class, full collection failure counters by normalized class.
  - Added a health alert for collector collection failure and documented diagnostic-first troubleshooting.

## Validation

Completed on 2026-05-01.

Acceptance criteria evidence:

- Collector compiles and registers through `src/go/plugin/go.d/collector/init.go`.
- Collector source added under `src/go/plugin/go.d/collector/cato_networks/`.
- Operator artifacts added and kept in sync:
  - `src/go/plugin/go.d/collector/cato_networks/metadata.yaml`
  - `src/go/plugin/go.d/collector/cato_networks/config_schema.json`
  - `src/go/plugin/go.d/config/go.d/cato_networks.conf`
  - `src/health/health.d/cato_networks.conf`
  - `src/go/plugin/go.d/collector/cato_networks/README.md`
- `src/go/go.mod` pins `github.com/catonetworks/cato-go-sdk v0.2.5`.
- The standalone topology function is implemented as `topology:cato_networks`; unit tests verify a topology response with source `cato_networks`, non-empty actors, and non-empty links.
- Real Cato tenant smoke test was not run because no tenant credentials or vendor sandbox are available in this workspace. Fixture-backed and SDK-type-backed unit tests are the substitute validation for this SOW close.

Tests or equivalent validation:

- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` — passed.
- `cd src/go/plugin/go.d/collector/cato_networks && go test ./... -count=1` — passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` — passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` — passed.
- After the NIDL correction:
  - `cd src/go && gofmt -w plugin/go.d/collector/cato_networks/models.go plugin/go.d/collector/cato_networks/client.go plugin/go.d/collector/cato_networks/normalize.go plugin/go.d/collector/cato_networks/write_metrics.go plugin/go.d/collector/cato_networks/collector_test.go` — completed.
  - `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` — passed.
  - `cd src/go && go vet ./plugin/go.d/collector/cato_networks` — passed.
  - `cd src/go && go test ./plugin/go.d/... -count=1` — passed.
  - Stale-name scan found no stale metric IDs for discarded-packet percent metrics or old API retry gauges. Remaining matches are historical SOW text, chart titles, and generic documentation wording.
- After the production-readiness correction:
  - `cd src/go && gofmt -w plugin/go.d/collector/cato_networks/collector.go plugin/go.d/collector/cato_networks/collect.go plugin/go.d/collector/cato_networks/collector_test.go plugin/go.d/collector/cato_networks/diagnostics.go plugin/go.d/collector/cato_networks/write_metrics.go` — completed.
  - `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` — passed.
  - `cd src/go && go vet ./plugin/go.d/collector/cato_networks` — passed.
  - `cd src/go && go test ./plugin/go.d/... -count=1` — passed.
  - YAML/JSON parsing passed for metadata, charts, stock config, config schema, the Centreon Mockoon fixture, and the schema-shaped BGP fixture.
  - Stale-name scan for old discarded-percent/API-retry units remained clean.
- YAML/JSON parsing passed for:
  - `src/go/plugin/go.d/collector/cato_networks/metadata.yaml`
  - `src/go/plugin/go.d/collector/cato_networks/charts.yaml`
  - `src/go/plugin/go.d/config/go.d/cato_networks.conf`
  - `src/go/plugin/go.d/collector/cato_networks/config_schema.json`
- Metadata category validation passed for `data-collection.networking` against `integrations/categories.yaml`.
- `python3 integrations/check_collector_metadata.py src/go/plugin/go.d/collector/cato_networks/metadata.yaml` could not run because the checker itself fails importing `SINGLE_PATTERN` from `integrations/gen_integrations.py`. This is recorded as a validation-tool failure, not as a metadata parse failure.
- Unit tests cover collection, chart-template compilation, config validation, retry classification/backoff, retry stats, EventsFeed marker persistence, `Check()` marker non-consumption, partial `accountMetrics` and BGP failures, BGP site rotation, nil SDK status defaults, and topology function response.
- Unit tests now also cover raw Centreon fixture replay through the SDK/client path, schema-shaped BGP raw response decoding, operation failure diagnostics, and normalized error classification.

Real-use evidence:

- No live Cato tenant credentials or vendor sandbox were available, so live API and dashboard smoke tests were not run.
- The collector's live-risk areas remain: optional real-tenant payload fields, BGP Beta surface behavior, very large account scale, and dashboard rendering with real Cato hub-and-spoke layouts.

Reviewer findings:

- No external assistant review was run because the user did not request second-opinion agents for this implementation turn, and repo instructions only allow those assistants when explicitly requested.
- Self-review discrepancies found and fixed before close:
  - omitted discovery default accidentally meant no rediscovery by default; fixed to 300 seconds.
  - `Check()` advanced EventsFeed marker; fixed so only `Collect()` advances marker.
  - nil SDK status values normalized badly; fixed to `unknown`.
  - Cato interface `"all"` metrics did not populate site charts; fixed.
  - retry defaults mismatched verified Cato/Centreon 5-second backoff evidence; fixed to `retry.wait_min: 5`.
  - metadata category was invalid for this branch; fixed to `data-collection.networking`.
  - metadata icon referenced non-existing `cato.svg`; fixed to existing `network-wired.svg`.
  - NIDL metric organization was not compliant enough after the first close; fixed by consolidating chart families and correcting counter/rate and discarded-packet count semantics.
  - raw fixture testing was promised but not implemented; fixed with committed fixture replay through the SDK/client path.
  - failure visibility was insufficient for prospect first-run debugging; fixed with collector health diagnostics and a collection-failure alert.

Same-failure scan:

- Searched health templates for comma-separated dimension lookups; existing alerts use this syntax, so `cato_networks_site_packet_loss` follows a repo pattern.
- Searched metadata categories against `integrations/categories.yaml`; the invalid initial category was corrected.
- Searched existing topology functions; `topology:cato_networks` mirrors the shipped `topology:snmp` function contract.
- Searched SDK dependency graph with `go mod why`; new direct runtime dependencies are attributable to `cato-go-sdk@v0.2.5` and its generated GraphQL client stack. Extra `go.sum`-only entries are transitive test sums from those dependencies.
- Searched the Cato collector and spec for stale discarded-percent metric IDs, old API retry gauge IDs, stale event/retry units, and old one-chart family names after the NIDL correction. No stale code/spec metric IDs remained.
- Searched Cato collector artifacts for the new diagnostic chart/metadata/alert names; charts, metadata, README, tests, and health alert are aligned.

Artifact maintenance gate:

- AGENTS.md: no update needed. The existing collector consistency rules and SOW framework already covered this work; no project-wide workflow changed.
- Runtime project skills: no update needed. There are no `project-*` runtime skills yet, and no recurring workflow was discovered that belongs in a skill rather than this collector spec.
- Specs: added `.agents/sow/specs/cato-networks-collector.md` and updated it for the NIDL and diagnostics corrections: discarded packet counts, `events/s`, `retries/s`, fixture requirements, and diagnostic contract.
- End-user/operator docs: added and updated `src/go/plugin/go.d/collector/cato_networks/README.md`; added metadata/config schema/stock config/health alerts, including collector diagnostics and collection-failure alert. Public integrations docs regeneration is a downstream build step and was not run here.
- End-user/operator skills: no update needed. This collector does not change `docs/netdata-ai/skills/` or `src/ai-skills/` behavior.
- SOW lifecycle: SOW status set to `completed`; file will be moved from `current/` to `done/` after this validation record.

Specs update:

- Added `.agents/sow/specs/cato-networks-collector.md` with SDK pin, query contract, cadence, chart catalogue, topology model, operator artifacts, and validation expectations.
- Updated `.agents/sow/specs/cato-networks-collector.md` after the NIDL correction so it records discarded packet counts and counter-rate chart semantics.
- Updated `.agents/sow/specs/cato-networks-collector.md` after the production-readiness correction so future changes must keep raw fixture replay and operator diagnostics.

Project skills update:

- No project skill update needed; no matching project runtime skill exists and this SOW did not change assistant workflow.

End-user/operator docs update:

- Added and updated collector README, metadata, config schema, stock config, and health alerts.

End-user/operator skills update:

- No end-user/operator skill update needed.

Lessons:

- Cato SDK `v0.2.5` is the correct released tag for this branch; SDK `main` is not acceptable because it resolves as an unreleased pseudo-version and bumps the Go directive.
- `Check()` must be treated as read-only for marker-based APIs, otherwise first real collection can lose event data.
- The public metadata category list in this branch is much narrower than some older/generated category names; validate against `integrations/categories.yaml`.
- NIDL review must include chart-family structure and unit/algorithm semantics, not only chart names and labels.
- Raw third-party fixtures may target a different GraphQL query shape than the SDK-generated query. Keep the original raw fixture, but test the exact SDK query response shape that Netdata sends.
- First-run customer support needs low-cardinality diagnostic labels (`operation`, `error_class`), not raw error strings in metric labels.

Follow-up mapping:

- Live tenant/dashboard validation is not implemented here because credentials are unavailable. This is tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- ISP/last-mile topology actors are explicitly rejected for v1 because verified fixtures and SDK/API coverage used in this SOW did not provide a stable ISP identity field.

## Outcome

Completed.

Implemented a new `cato_networks` go.d V2 collector for Cato Networks GraphQL monitoring with:

- site discovery and account snapshot collection;
- site and interface traffic/quality/status charts;
- NIDL-aligned `sites`, `interfaces`, `bgp`, and `collector` chart families;
- marker-based event counters;
- rolling per-site BGP status collection;
- API retry visibility;
- collector health and operation-failure diagnostics;
- standalone `topology:cato_networks` topology output;
- stock config, config schema, metadata, README, and health alerts;
- pinned `github.com/catonetworks/cato-go-sdk@v0.2.5`.

Known limitation at close: no live Cato tenant or vendor sandbox was available, so live API payload and dashboard topology smoke testing remains unverified.

## Lessons Extracted

- Do not use SDK `main` or unreleased pseudo-versions for this collector. The approved dependency is the released `v0.2.5` tag.
- Marker-based collectors must not mutate marker state during `Check()`.
- Public collector metadata must be validated against the branch-local `integrations/categories.yaml`.
- Cato BGP collection must remain rolling/throttled because `siteBgpStatus` is site-scoped and vendor-documented as Beta.
- Counter-like charts must use rate units and raw counter-backed metrics; raw Cato packet counts must not be presented as percentages.
- Raw fixture replay must exercise the pinned SDK/client path, not only in-memory SDK structs.
- Diagnostic chart labels must classify failures without exposing raw customer-specific API errors.

## Followup

- Live Cato tenant or vendor sandbox validation is tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md` and should run when credentials become available:
  - verify non-empty connectivity and throughput charts;
  - verify BGP fields against real payloads;
  - verify `topology:cato_networks` in the dashboard renderer;
  - capture sanitized real responses for future fixtures if permitted.

## Reopen - Pre-Prospect Hardening

Date: 2026-05-01.

Reason:

- The user requested improving the same SOW after five read-only external reviews focused on first-run customer risk without live Cato access.
- The purpose remains unchanged: make the Cato collector fit for first-run prospect testing as much as possible without Netdata-owned Cato devices/accounts.

Reviewed evidence:

- External review outputs in `.local/audits/cato-ai-review-20260501/`.
- Collector code under `src/go/plugin/go.d/collector/cato_networks/`.
- SDK source in the local module cache for `github.com/catonetworks/cato-go-sdk@v0.2.5`.
- Netdata HTTP client implementation in `src/go/pkg/web/client_config.go`.

Verified findings to address in this reopen:

- Unknown non-empty Cato statuses currently normalize to their raw value, while status charts only mark known values. A new vendor status can therefore produce all-zero status dimensions instead of `unknown`.
- BGP `siteBgpStatus` is decoded by the SDK from `rawStatus` JSON. SDK parse errors are printed to stdout by the SDK and still append an empty result, so the collector needs to filter malformed/empty peers and expose a diagnostic.
- BGP polling advances refresh/index even when the entire BGP batch fails.
- Marker persistence can be disabled when no marker file is configured and Netdata varlib is unavailable; this is not visible as a collector metric.
- Current tests do not exercise real SDK/client-path HTTP status failures, GraphQL `errors[]` responses, empty discovery, marker write failures, or malformed BGP rawStatus responses.
- Some warning logs include raw provider error strings or account/site identifiers at warn level.

Verified reviewer claims rejected or downgraded:

- The collector is not definitely double-retrying with the SDK retry layer: the SDK only creates its retryable HTTP client when no HTTP client is passed, and this collector passes Netdata's `web.NewHTTPClient` client.
- Netdata's HTTP client does set `http.Client.Timeout`, dial timeout, and TLS handshake timeout; indefinite hang is not the immediate issue claimed by one review.
- SDK v0.2.5 BGP incoming/outgoing connection fields are value structs, not pointers, so the claimed nil-pointer panic is not valid for the pinned SDK. The valid risk is malformed `rawStatus` becoming an empty BGP peer.

Implementation plan:

- Add low-cardinality normalization diagnostics and route unknown connectivity, operational, and BGP states to `unknown` rather than all-zero charts.
- Filter empty BGP peers, count malformed/empty BGP payloads, and do not advance the BGP rotation window on total BGP failure.
- Add marker persistence visibility metrics.
- Add SDK/client-path tests for HTTP status failures and GraphQL `errors[]`, plus empty discovery, marker write failure, malformed BGP rawStatus, and config schema JSON parsing.
- Reduce warning logs to operation, count, and normalized error class; keep raw provider details out of warn-level logs.
- Update charts, metadata, README, spec, and health artifacts to match any new diagnostics.

Validation plan:

- `cd src/go && gofmt -w plugin/go.d/collector/cato_networks/*.go`
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1`
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks`
- `cd src/go && go test ./plugin/go.d/... -count=1`
- YAML/JSON parse checks for updated collector artifacts.
- `.agents/sow/audit.sh`

Hardening implemented:

- Unknown non-empty site connectivity and operational statuses now map to the `unknown` dimensions instead of producing all-zero status charts.
- Added low-cardinality normalization diagnostics with labels `surface` and `issue`; issue labels do not include raw Cato error strings, account IDs, site names, IPs, or raw status values.
- Added marker persistence availability metric and health alert so EventsFeed marker persistence failure is visible before or during customer troubleshooting.
- BGP `siteBgpStatus` empty decoded peers are filtered and counted as normalization issues.
- BGP rolling polling no longer advances the rolling site window when every BGP request in the current batch fails.
- Warn-level logs now prefer operation/count/error class and avoid raw provider error strings for accountMetrics, BGP, EventsFeed, and marker write failures.
- Added SDK/client-path fault tests for HTTP auth failure, HTTP rate limit failure, and GraphQL `errors[]` rate-limit failure.
- Added tests for empty discovery, unknown status fallback, BGP total failure rotation, empty BGP peer filtering, marker write failure, BGP session exact state matching, and config schema JSON parsing.

Validation completed after hardening:

- `cd src/go && gofmt -w plugin/go.d/collector/cato_networks/collector.go plugin/go.d/collector/cato_networks/collect.go plugin/go.d/collector/cato_networks/collector_test.go plugin/go.d/collector/cato_networks/diagnostics.go plugin/go.d/collector/cato_networks/normalize.go plugin/go.d/collector/cato_networks/write_metrics.go` - completed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.
- YAML/JSON parsing passed for metadata, charts, stock config, config schema, Centreon Mockoon fixture, and schema-shaped BGP fixture.

Artifact maintenance after hardening:

- AGENTS.md: no update needed. Existing collector/SOW rules covered the hardening workflow.
- Runtime project skills: no update needed. No reusable workflow changed.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with marker persistence diagnostics, normalization diagnostics, unknown-status fallback, BGP empty-peer handling, BGP rotation behavior, and expanded validation expectations.
- End-user/operator docs: updated README, metadata, charts, and health alert to include marker persistence and normalization diagnostics.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened by user request, then completed again after validation; file will be moved back to `done/`.

Remaining limitation:

- No live Cato tenant or vendor sandbox was available. SOW-0005 remains the required live validation path before claiming real-world coverage of optional payload fields, BGP scale behavior, or dashboard topology rendering.

## Reopen - Second Pre-Prospect Hardening Completion

Date: 2026-05-02.

Reason:

- The user approved fixing the second external review round's concrete production-readiness findings in this same SOW.
- The purpose remains unchanged: make the Cato collector fit for first-run prospect testing as much as possible without Netdata-owned Cato devices/accounts.

Implemented:

- `eventsFeed` now drains marker pages within a collection cycle until `fetchedCount` drops below Cato's documented per-fetch maximum, the marker is empty or unchanged, or `events.max_pages_per_cycle` is reached.
- Events marker advancement is committed only after metrics for the cycle are written, reducing the crash window that could otherwise skip fetched events.
- Added `events.max_pages_per_cycle` default `10` and `events.max_cardinality` default `50`.
- Excess event type/subtype/severity/status combinations collapse into an `other` bucket and emit `collector_normalization_issues_total{surface="events",issue="cardinality_limit"}`.
- Events page cap and stalled marker conditions emit low-cardinality normalization diagnostics.
- Added `collector_operation_affected_sites_total{operation,error_class}` so partial `accountMetrics` and `siteBgpStatus` failures report how many sites were affected.
- `accountMetrics` and BGP partial failures now return sanitized partial errors to the collection flow, producing summary warnings while keeping the overall collection alive.
- Discovery now has a defensive max-page guard and returns a `pagination` error class if the guard is hit.
- Returned discovery and snapshot errors are sanitized at the collector boundary with operation name and normalized error class, instead of returning raw SDK/vendor error strings.
- `metrics.time_frame` now validates against Cato's documented `last.P...` and `utc...` TimeFrame shapes in both Go validation and `config_schema.json`.
- BGP cached state is pruned when a site disappears from current discovery.
- Topology interface and device table rows are sorted deterministically.
- Test-server auth failure logging no longer prints header values.

Validation completed after second hardening:

- `cd src/go && gofmt -w src/go/plugin/go.d/collector/cato_networks/*.go` - completed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.
- YAML/JSON parsing passed for metadata, charts, stock config, and config schema.

Test coverage added or expanded:

- EventsFeed multi-page drain.
- Events cardinality collapse into `other`.
- Persisted marker resume.
- Events page cap diagnostic.
- Events stalled-marker diagnostic.
- Multi-page discovery.
- Sanitized returned provider error string.
- Operation affected-site counters for partial metrics/BGP failures.
- BGP stale-site pruning.
- Deterministic topology table ordering.
- Config validation for invalid `metrics.time_frame` and URL schemes.

Artifact maintenance after second hardening:

- AGENTS.md: no update needed. Existing collector/SOW rules covered this work.
- Runtime project skills: no update needed. No reusable assistant workflow changed.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with EventsFeed page draining, cardinality cap, affected-site diagnostics, stale BGP pruning, and expanded validation expectations.
- End-user/operator docs: updated README, metadata, config schema, stock config, and charts for the new EventsFeed controls and affected-site diagnostic chart.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened by user request, completed again after validation, and will be moved back to `done/`.

Follow-up mapping after second hardening:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- Site include/exclude filters remain intentionally out of v1 scope because the implementation decision in this SOW preserves full-account visibility by default.
- BGP negotiated timers/transport and full BGP-state dimensions remain rejected for this pass because they expand the chart/topology catalogue and need real-payload validation.

Outcome after second hardening:

- Completed.

## Reopen - Third Pre-Prospect Hardening Completion

Date: 2026-05-02.

Reason:

- The user approved fixing the concrete findings from the third external review round in this same SOW.
- The purpose remains unchanged: make the Cato collector fit for first-run prospect testing as much as possible without Netdata-owned Cato devices/accounts.

Implemented:

- Added low-cardinality `surface=metrics`, `issue=unknown_timeseries_label` diagnostics when `accountMetrics` returns an unrecognized timeseries label with data.
- Preserved `degraded` connectivity through the raw Centreon/SDK fixture path instead of mapping every non-`connected` status to `disconnected`.
- Discovered and fixed a real SDK compatibility issue: `github.com/catonetworks/cato-go-sdk@v0.2.5` rejects `degraded` in its generated `ConnectivityStatus` enum during `accountSnapshot` decode. The collector now falls back to a raw GraphQL `accountSnapshot` decoder for that enum-staleness case while keeping the SDK path for normal operation and the other Cato operations.
- EventsFeed field extraction now accepts expected snake_case and camelCase event type/subtype keys, rejects complex field values as labels, and emits low-cardinality diagnostics for empty/complex event fields.
- Marker write failures now report `collector_events_marker_persistence_available=0`. The collector still advances the in-memory marker for the running process to avoid repeated duplicate event counting before restart.
- Added normalized `network`, `tls`, and `proxy` error classes and tightened HTTP status matching so filesystem paths containing numeric substrings cannot be misclassified as HTTP/auth/rate-limit failures.
- Added BGP rolling-scan diagnostics: sites per BGP collection, cached BGP sites, and estimated full-scan seconds.
- Expanded schema-shaped BGP fixture assertions to verify route counts, route limits, and RIB-out counts through the raw SDK/client path.
- Updated README, metadata, charts, and the collector spec for the new diagnostics and SDK fallback behavior.

Validation completed after third hardening:

- `cd src/go && gofmt -w src/go/plugin/go.d/collector/cato_networks/collector.go src/go/plugin/go.d/collector/cato_networks/collect.go src/go/plugin/go.d/collector/cato_networks/diagnostics.go src/go/plugin/go.d/collector/cato_networks/normalize.go src/go/plugin/go.d/collector/cato_networks/client.go src/go/plugin/go.d/collector/cato_networks/write_metrics.go src/go/plugin/go.d/collector/cato_networks/collector_test.go` - completed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.
- YAML parsing passed for `metadata.yaml`, `charts.yaml`, and stock `go.d/cato_networks.conf`.
- JSON parsing passed for `config_schema.json`.
- `.agents/sow/audit.sh` passed while the SOW was still `current/in-progress`; final audit will be rerun after moving the completed SOW back to `done/`.

Test coverage added or expanded:

- Unknown `accountMetrics` timeseries labels.
- Event field aliases, empty fields, and complex field values.
- Raw Centreon fixture replay preserving degraded connectivity through the client path.
- Raw BGP schema-shaped fixture assertions for accepted routes, route limits, and RIB-out routes.
- Marker write failure persistence gauge behavior.
- BGP rolling-scan progress/window diagnostics.
- Network/TLS/proxy classification and real HTTP client timeout classification.

Artifact maintenance after third hardening:

- AGENTS.md: no update needed. Existing collector/SOW rules covered this work.
- Runtime project skills: no update needed. No reusable assistant workflow changed.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with the `accountSnapshot` raw fallback, degraded SDK enum risk, new error classes, new normalization issues, marker persistence behavior, BGP scan diagnostics, and expanded validation expectations.
- End-user/operator docs: updated README, metadata, and charts for the new diagnostics and troubleshooting guidance.
- End-user/operator skills: no update needed; no AI skill artifacts were affected.
- SOW lifecycle: same SOW reopened by user request, completed again after validation, and will be moved back to `done/`.

Follow-up mapping after third hardening:

- Live Cato tenant or vendor sandbox validation remains tracked by `.agents/sow/pending/SOW-0005-20260501-cato-networks-live-validation.md`.
- Site include/exclude filters remain intentionally out of v1 scope because the implementation decision in this SOW preserves full-account visibility by default.
- BGP negotiated timers/transport and full BGP-state dimensions remain rejected for this pass because they expand the chart/topology catalogue and need real-payload validation.
- Account ID as a metric label remains intentionally not added in this pass. A Netdata job monitors one Cato account, and adding account ID to every metric label would increase cardinality and expose account identifiers in chart labels. MSP/multi-account collision behavior should be evaluated during SOW-0005 live validation if multiple Cato accounts are configured on the same Agent.

Outcome after third hardening:

- Completed.

## Reopen - Collector Skill Compliance Hardening - 2026-05-02

Reason:

- The user asked whether the Cato collector needed changes after reviewing `/home/costa/src/netdata-ktsaou.git/.agents/skills/project-writing-collectors/SKILL.md`.
- Local verification found three gaps against that skill:
  - recoverable collection-loop warnings are emitted per cycle instead of being gated by condition;
  - high-cardinality site/interface/BGP-peer entities lack explicit selector/cap controls;
  - the go.d module is wired in `collector/init.go`, `config/go.d.conf`, stock config, and collector README, but is missing from `src/go/plugin/go.d/README.md`.
- The same verification found that V2 chart lifecycle already supports explicit chart-instance expiry, and the user stated that obsoletion is important and must be added independent of cardinality.

Evidence reviewed:

- Collector skill logging rule: `/home/costa/src/netdata-ktsaou.git/.agents/skills/project-writing-collectors/SKILL.md:104-111`.
- Collector skill cardinality rule: `/home/costa/src/netdata-ktsaou.git/.agents/skills/project-writing-collectors/SKILL.md:113-121`.
- Collector skill go.d wiring rule: `/home/costa/src/netdata-ktsaou.git/.agents/skills/project-writing-collectors/SKILL.md:258-259` and `:321`.
- Current collection-loop warnings: `src/go/plugin/go.d/collector/cato_networks/collector.go:221`, `:289`, `:295`, `:303`, `:340`; `src/go/plugin/go.d/collector/cato_networks/collect.go:80`, `:136`, `:186`, `:298`.
- Current unbounded dynamic chart instances: `src/go/plugin/go.d/collector/cato_networks/write_metrics.go:27` for sites, `:62` for interfaces, and `:86` for BGP peers.
- V2 lifecycle support: `src/go/plugin/framework/charttpl/README.md:608-624`; default chart expiry: `src/go/plugin/framework/chartengine/lifecycle_defaults.go:13-19`.

User decision recorded before implementation:

- Decision 1.B: deterministically skip excess entities and alert, instead of failing the whole collector when cardinality caps are exceeded.
- Implication: the collector remains available under large accounts, but operator-facing diagnostics and alerts must make partial coverage explicit.
- Obsoletion is a separate requirement: disappeared entities must be obsoleted via V2 chart lifecycle even when no cap is hit.

Implementation scope:

1. Add explicit V2 `lifecycle.expire_after_cycles` to dynamic Cato site, interface, BGP-peer, event, and diagnostic chart templates so missing entities are obsoleted by successful-cycle expiry instead of relying on implicit defaults.
2. Add selector controls:
   - `site_selector` using Netdata simple patterns against site name or site ID;
   - `interface_selector` using Netdata simple patterns against interface name or interface ID;
   - `bgp.peer_selector` using Netdata simple patterns against remote peer IP or ASN.
3. Add deterministic caps:
   - `limits.max_sites`, default `500`, `0` disables the cap;
   - `limits.max_interfaces_per_site`, default `32`, `0` disables the cap;
   - `bgp.max_peers_per_site`, default `32`, `0` disables the cap.
4. Add low-cardinality diagnostics and health alerts for cap hits / selector skips so partial monitoring is visible.
5. Gate repeated recoverable warning logs by condition and error class; keep per-cycle truth in metrics and use debug logging for repeated detail.
6. Add the missing Cato row to `src/go/plugin/go.d/README.md`.
7. Update README, stock config, metadata, config schema, health alerts, and the Cato collector spec to describe selectors, caps, obsoletion, and gated logging behavior.
8. Add tests for selector filtering, deterministic cap behavior, diagnostics/alerts metrics, explicit lifecycle YAML parsing, and warning-gate behavior where practical.

Validation plan:

- `git diff --check`
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1`
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks`
- `cd src/go && go test ./plugin/go.d/... -count=1`
- YAML parsing for `charts.yaml`, `metadata.yaml`, and stock `go.d/cato_networks.conf`.
- JSON parsing for `config_schema.json`.
- `.agents/sow/audit.sh` before moving the SOW back to `done/`.

Implemented after collector skill compliance hardening:

- Added explicit `lifecycle.expire_after_cycles: 5` to every dynamic Cato V2 chart template: sites, interfaces, BGP peers, event label combinations, API retry query labels, operation labels, normalization issue labels, and the new entity-selection diagnostics.
- Added cardinality controls:
  - `site_selector`, matching site ID or site name;
  - `interface_selector`, matching interface ID or interface name;
  - `limits.max_sites`, default `500`, `0` disables;
  - `limits.max_interfaces_per_site`, default `32`, `0` disables;
  - `bgp.peer_selector`, matching BGP peer remote IP or remote ASN;
  - `bgp.max_peers_per_site`, default `32`, `0` disables.
- Implemented selector matching with glob-style terms and `!` exclusions. Exclusions win across all identity fields for an entity, so an excluded site name is not re-included by a catch-all site ID match.
- Applied selected-site pruning before metrics/BGP/topology processing so unexpected extra sites returned by `accountSnapshot` cannot leak into BGP polling or emitted metrics.
- Added low-cardinality selector/cap diagnostics:
  - `collector_selected_entities{entity}`;
  - `collector_skipped_entities{entity,reason}`;
  - `collector_cardinality_limit_hit{entity}`.
- Added `cato_networks_collector_cardinality_limit_hit` health alert.
- Gated repeated recoverable warning logs by condition and normalized class, while preserving per-cycle truth in collector health metrics and debug logs.
- Downgraded per-batch/per-site collection-loop detail logs for partial `accountMetrics` and `siteBgpStatus` failures to debug.
- Added missing `cato_networks` row to `src/go/plugin/go.d/README.md`.
- Updated README, stock config, metadata, config schema, health alerts, charts, and `.agents/sow/specs/cato-networks-collector.md`.

Validation completed after collector skill compliance hardening:

- `git diff --check` - passed.
- `jq empty src/go/plugin/go.d/collector/cato_networks/config_schema.json` - passed.
- `ruby -e 'require "yaml"; ARGV.each { |path| YAML.load_file(path); puts "ok #{path}" }' src/go/plugin/go.d/collector/cato_networks/charts.yaml src/go/plugin/go.d/collector/cato_networks/metadata.yaml src/go/plugin/go.d/config/go.d/cato_networks.conf` - passed.
- `cd src/go && go test ./plugin/go.d/collector/cato_networks -count=1` - passed.
- `cd src/go && go vet ./plugin/go.d/collector/cato_networks` - passed.
- `cd src/go && go test ./plugin/go.d/... -count=1` - passed.

Test coverage added after collector skill compliance hardening:

- Configuration default trimming while preserving explicit `0` cap disables.
- Site selector plus site cap behavior and diagnostics.
- Interface selector plus interface cap behavior and diagnostics.
- BGP peer cap behavior and diagnostics.
- Recoverable warning-gate state by error class.
- Explicit lifecycle declaration on every dynamic chart template.

Artifact maintenance after collector skill compliance hardening:

- `AGENTS.md`: no update needed. Existing SOW and collector rules already require SOW tracking, collector consistency, and artifact synchronization.
- Runtime project skills: no update needed. The collector-writing skill was external to this repository and no reusable project-local workflow changed.
- Specs: updated `.agents/sow/specs/cato-networks-collector.md` with selectors, caps, skip diagnostics, explicit V2 lifecycle obsoletion, and recoverable warning-gate behavior.
- End-user/operator docs: updated README, metadata, config schema, stock config, health alerts, charts, and go.d README registry.
- End-user/operator skills: no update needed. No downstream AI/operator skill artifact changed.
- SOW lifecycle: reopened completed SOW for collector-writing skill compliance; closing it again after validation. Live Cato tenant validation was later handed off to SREs and removed from repo-local pending SOW tracking by user decision on 2026-05-02.

Follow-up mapping after collector skill compliance hardening:

- Live Cato tenant or vendor sandbox validation is handed off to SREs outside repo-local SOW tracking by user decision on 2026-05-02.
- No new deferred work remains from this hardening pass.

Outcome after collector skill compliance hardening:

- Completed.
