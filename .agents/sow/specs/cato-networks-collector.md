# Cato Networks Collector Specification

## Scope

The `cato_networks` Go collector monitors one Cato Networks account per job through the Cato GraphQL API.

The collector is opt-in and requires:

- `account_id`
- `api_key`

The default API endpoint is `https://api.catonetworks.com/api/v1/graphql2`, and operators may override `url` for regional or custom Cato endpoints. Production endpoints must use HTTPS because the collector sends the API key in request headers. HTTP is accepted only for loopback test endpoints.

## Data Sources

The collector uses the tagged Cato Go SDK dependency pinned in `src/go/go.mod`.

Current approved SDK:

- module: `github.com/catonetworks/cato-go-sdk`
- version: `v0.2.5`
- reason: latest released tag verified for this SOW, declares Go `1.23.1`, and does not require a Netdata Go directive bump

The collector uses these Cato GraphQL operations:

- `entityLookup` for site discovery
- `accountSnapshot` for site, interface, device, status, and PoP snapshot data
- `accountMetrics` for bucketed site/interface traffic and quality time-series
- `eventsFeed` for marker-based event counters
- `siteBgpStatus` for per-site BGP peer/session status

`siteBgpStatus` is treated as higher drift risk because the vendor documents the surface as Beta.

The collector normally uses the pinned SDK client path. For `accountSnapshot`, it must fall back to a raw GraphQL decode path when the SDK rejects a Cato connectivity enum value. This is required because SDK `v0.2.5` does not accept `degraded` in the generated `ConnectivityStatus` enum, while real/reference snapshot payloads can contain degraded site connectivity.

## Collection Contract

Default cadence:

- `update_every: 60`
- `discovery.refresh_every: 300`
- `bgp.refresh_every: 300`
- `metrics.time_frame: last.PT5M`
- `metrics.buckets: 5`
- `events.max_pages_per_cycle: 10`
- `events.max_cardinality: 50`
- `retry.attempts: 5`
- `retry.wait_min: 5`
- `retry.wait_max: 30`
- `site_selector: "*"`
- `interface_selector: "*"`
- `limits.max_sites: 500`
- `limits.max_interfaces_per_site: 32`
- `bgp.peer_selector: "*"`
- `bgp.max_peers_per_site: 32`

The collector must not run at a sub-minute default cadence. Cato publishes per-query, per-account rate limits, and `accountMetrics` is a bucketed time-series API rather than a high-frequency sampler.

HTTP/client timeouts that wrap `context.DeadlineExceeded` and HTTP `5xx` responses are retryable while the caller context is still active. Caller cancellation or caller deadline expiry must stop retrying immediately.

Collector-owned HTTP clients must close idle connections during `Cleanup()`.

The collector batches `accountMetrics` calls by `metrics.max_sites_per_query`.

`metrics.group_interfaces: auto` must pass a nil/unset `groupInterfaces` argument to the SDK/API so Cato applies its default. `enabled` and `disabled` must pass explicit true and false values.

Selectors are applied before caps:

- `site_selector` matches Cato site ID or site name.
- `interface_selector` matches Cato interface ID or interface name.
- `bgp.peer_selector` matches BGP peer remote IP or remote ASN.

Selectors use Netdata simple-pattern syntax: space-separated glob terms, with `!` exclusions such as `!lab-* *`. Exclusions win across all identity fields for the entity. If a selector contains only exclusions, the collector treats it as those exclusions plus a final `*` include.

Cardinality caps are deterministic and protect emitted chart instances:

- `limits.max_sites` caps selected sites after `site_selector`.
- `limits.max_interfaces_per_site` caps selected interfaces per selected site after `interface_selector`.
- `bgp.max_peers_per_site` caps selected BGP peers per selected site after `bgp.peer_selector`.
- A cap value of `0` disables that cap.

When a cap is hit, the collector must skip excess entities deterministically, continue collecting selected entities, publish skipped-entity diagnostics, and raise the cardinality-limit health alert. Site/interface/BGP per-entity charts do not synthesize an `other` aggregate because their status, latency, connectivity, and peer-route values are not safely additive. Event cardinality remains different: excess event label combinations collapse into the existing `other` bucket because event counters are additive.

Traffic and quality metrics derived from `accountMetrics` must be emitted only for fields actually returned by Cato. Missing accountMetrics data must leave the corresponding site/interface series absent; it must not be represented as real zero traffic, zero loss, or zero latency.

`metrics.time_frame` validation accepts Cato's documented `last.<ISO-8601 duration>` form such as `last.PT5M` and `utc.<short-time-frame-spec>` form such as `utc.2020-02-11/{04:50:15--16:50:15}`.

After initial site discovery succeeds, later discovery refresh failures must fall back to the last-known-good site list instead of failing the full collection. The collector must still record the `entityLookup` operation failure and should respect `discovery.refresh_every` before retrying discovery again to avoid API/log spam. Before any successful discovery exists, discovery failures remain hard collection failures.

Runtime hard collection failures must still publish collector-health metrics. `Collect()` must commit `collector_collection_success=0`, the collection failure counter, and any operation failure diagnostics, then return nil to the job runtime so the metrix cycle is not aborted. `Check()` must continue returning hard failures for autodetection.

BGP collection is decoupled from the main metric cadence and limited by `bgp.max_sites_per_collection`, because `siteBgpStatus` is site-scoped.

The collector exposes the estimated BGP full-scan window in seconds and the current BGP cached-site count so operators can tell whether missing BGP state is expected during a rolling scan.

BGP progress health fields must reset at the start of each collection cycle. When BGP is enabled but the current cycle fails before BGP collection runs, the collector must publish zero-valued BGP progress gauges instead of reusing the previous cycle's scan window or cached-site count. When BGP collection is skipped because the refresh window is not due and cached BGP state is reused, the collector must publish the current rolling-scan parameters and cached-site count instead of zero progress.

BGP peers are deduplicated by remote IP and remote ASN for each site so duplicate vendor rows do not produce repeated metric labels or duplicate topology actor IDs.

BGP peer rows must include at least one stable remote identity field, `remoteIP` or `remoteASN`. Rows that only include local fields or connection-state fields are malformed for Netdata metrics and topology; the collector must drop them and count an `empty_peer` normalization issue.

`Check()` must not advance the persisted `eventsFeed` marker and must not publish dry-run operation health, failure counters, normalization issues, discovery cache changes, BGP cache changes, BGP scan cursor changes, or topology changes into later `Collect()` cycles. Only `Collect()` may consume events, persist a newer marker, and publish collector-health metrics.

`Collect()` drains marker-based `eventsFeed` pages until the returned `fetchedCount` is below Cato's documented per-fetch maximum, the marker is empty or unchanged, or `events.max_pages_per_cycle` is reached. The marker is committed only after metrics for the cycle are written. If Cato returns the same marker that was used for the request, the page is treated as stalled and must not add records to `events_total` or persist the unchanged marker as progress.

An `eventsFeed` account-level `errorString` is a page failure for this collector because each request asks for one account. The collector must report the account-error diagnostic and must not advance the persisted or in-memory marker for that page.

The default marker file path is derived from account ID, endpoint URL, and vnode. Jobs that intentionally monitor the same account, endpoint, and vnode independently must set distinct `events.marker_file` values.

When marker persistence fails with a retryable temporary/timeout error, the collector must retry marker writes with the configured retry attempts/backoff and caller context before reporting marker persistence unavailable. Permanent local filesystem errors must fail fast. It may still advance the in-memory marker for the running process to avoid repeatedly counting the same EventsFeed page before restart.

Marker read failures during initialization must also report marker persistence unavailable until a later marker write succeeds.

## Metrics

Site scope:

- connectivity status: connected, disconnected, degraded, unknown
- operational status: active, disabled, locked, unknown
- host count
- traffic: upstream/downstream
- packet loss, discarded packet counts, jitter, RTT, last-mile latency, last-mile packet loss

An `accountMetrics` interface named `all` augments site-scope metrics. It must not replace already merged site-scope metric values when the `all` interface omits a field.

Interface scope:

- label identity includes `interface_id` and `interface_name`; `interface_id` is required in labels to avoid collisions when Cato returns multiple interfaces with the same display name
- connected status
- tunnel uptime
- traffic: upstream/downstream
- packet loss, discarded packet counts, jitter, RTT

BGP peer scope:

- session up status
- accepted routes
- route limit
- route limit exceeded
- RIB-out routes

Event scope:

- stateful `events_total` counter grouped by event type, subtype, severity, and status; charted as `events/s`
- event series cardinality is capped by `events.max_cardinality`; excess event combinations collapse into `event_type=other`, `event_sub_type=other`, `severity=other`, `status=other`

`events.max_cardinality = N` allows `N` real event series before excess new combinations collapse into `other`.

Repeated `eventsFeed` records with the same `event_id` or `eventId` must be counted once per collection cycle. Records without an event ID are counted normally.

API query scope:

- rate-limit retry counter by query; charted as `retries/s`
- transient retry counter by query; charted as `retries/s`

Collector health scope:

- last collection success status
- discovered site count
- selected entity gauges for site, interface, and BGP peer entity classes
- skipped entity gauges by entity class and reason (`selector` or `limit`)
- cardinality-limit-hit status by entity class
- EventsFeed marker persistence availability
- BGP rolling scan window and cached-site progress
- last operation success status by operation
- operation failure counters by operation and normalized error class
- operation affected-site counters by operation and normalized error class for partial site-scoped failures
- full collection failure counters by normalized error class
- normalization issue counters by surface and normalized issue class

Operation success status is stateful: operations skipped between refresh windows keep their last observed success/failure value until the operation runs again.

Normalized error classes are `auth`, `rate_limit`, `timeout`, `network`, `tls`, `proxy`, `decode`, `graphql`, `empty`, `pagination`, `canceled`, and `error`.

Normalization issue classes are intentionally low-cardinality. They must not include raw Cato error strings, site names, account IDs, IP addresses, or raw status values. Current issue classes include:

- `unknown_status`
- `unknown_timeseries_label`
- `empty_peer`
- `parse_int`
- `account_error`
- `cardinality_limit`
- `page_cap`
- `marker_stalled`
- `empty_event_type`
- `empty_event_sub_type`
- `empty_event_severity`
- `empty_event_status`
- `complex_event_field`

## Diagnostics Contract

The collector must expose enough local diagnostics for first-run customer failures where Netdata does not have direct access to the Cato account.

At minimum, operator-visible diagnostics must identify:

- whether the full collection completed
- how many sites were discovered
- whether EventsFeed marker persistence is available
- which operation failed: `entityLookup`, `accountSnapshot`, `accountMetrics`, `eventsFeed`, `siteBgpStatus`, or `eventsMarker`
- the normalized failure class
- how many sites were affected by partial `accountMetrics` and `siteBgpStatus` failures
- whether accepted payloads contained normalization issues, including unknown statuses, unknown accountMetrics timeseries labels, empty/malformed BGP peers, integer parse issues, empty/complex EventsFeed fields, or EventsFeed account-level errors
- whether selectors or caps skipped sites, interfaces, or BGP peers

Error messages may be logged, but chart labels must not include raw error strings because those can contain customer-specific details.

Warn-level logs should prefer operation name, count, and normalized class. Raw provider-side error strings should not be emitted at warn level.

Recoverable failures in the collection loop must be warn-level at first observation per failure class and debug-level on repeated cycles until recovery or class change. This includes full collection failure, cached discovery fallback, partial accountMetrics failure, partial BGP failure, EventsFeed failure, EventsFeed account-level errors, and marker write failure.

Unknown non-empty Cato statuses must map to the `unknown` dimension instead of producing all-zero status charts.

Unknown non-empty `accountMetrics` timeseries labels must not be silently ignored. They must be counted as `surface=metrics`, `issue=unknown_timeseries_label`.

EventsFeed field extraction must accept the expected snake_case and camelCase forms for event type and subtype. Missing, empty, or complex event field values must fall back to `unknown`/`other` style low-cardinality labels and increment low-cardinality normalization issues.

`siteBgpStatus` results decoded from SDK `rawStatus` JSON must filter empty peers. Empty or malformed peer payloads must be counted as normalization issues.

BGP polling must not advance the rolling site window when every `siteBgpStatus` request in the current BGP batch fails.

Cached BGP state must be pruned when a site disappears from current discovery.

All dynamic V2 chart templates for sites, interfaces, BGP peers, event label combinations, operation labels, normalization issue labels, retry query labels, and collector entity-selection labels must explicitly set `lifecycle.expire_after_cycles: 5`. This is the same as the current V2 default but is part of this collector's truthfulness contract: when a known entity stops appearing, its chart instance must expire after successful cycles instead of looking live forever. This obsoletion requirement is independent of cardinality caps.

## Topology

The collector exposes standalone topology through the function method:

```text
topology:cato_networks
```

The topology method must be job-selectable, not `AgentWide`, so multi-instance Cato jobs can return their own account topology through the normal `__job` function parameter.

Topology source and layer:

- source: `cato_networks`
- layer: `wan`
- schema version: `v1`

Actors:

- Cato site
- Cato PoP
- BGP peer

Links:

- site to PoP as `cato_tunnel`
- site to BGP peer as `bgp_session`

Cross-source overlay with SNMP, network-viewer, NetFlow, or other topology sources is outside this collector contract.

## Operator Artifacts

The following files are part of the collector contract and must stay synchronized:

- `src/go/plugin/go.d/collector/cato_networks/metadata.yaml`
- `src/go/plugin/go.d/collector/cato_networks/charts.yaml`
- `src/go/plugin/go.d/collector/cato_networks/config_schema.json`
- `src/go/plugin/go.d/config/go.d/cato_networks.conf`
- `src/go/plugin/go.d/README.md`
- `src/health/health.d/cato_networks.conf`
- `src/go/plugin/go.d/collector/cato_networks/README.md`

## Validation Expectations

Before changing this collector, run the narrow package validation at minimum:

```bash
cd src/go
go test ./plugin/go.d/collector/cato_networks -count=1
go vet ./plugin/go.d/collector/cato_networks
```

Tests must include:

- in-memory SDK-typed fixtures for fast normalization and edge-case coverage
- raw GraphQL fixture replay through the pinned SDK/client path
- degraded connectivity replay through the raw SDK/client path, including the `accountSnapshot` raw GraphQL fallback when the SDK enum is stale
- explicit branch coverage for `sdkAPIClient.AccountSnapshot()` falling back to raw GraphQL after SDK enum decode errors
- a schema-shaped BGP fixture because the available Centreon fixture does not cover `siteBgpStatus`
- diagnostics assertions for partial operation failures, unknown timeseries labels, marker write failures, BGP scan progress, and event field normalization issues
- selector and cap tests for sites, interfaces, and BGP peers, including skipped-entity and cardinality-limit diagnostics
- explicit chart-template lifecycle tests for dynamic V2 chart instances
- API retry metric assertions covering snapshot counter totals and deltas for `api_rate_limit_retries_total` and `api_transient_retries_total`
- EventsFeed pagination, page cap, cardinality collapse, marker resume, and stalled-marker tests
- SDK/client-path failure tests for HTTP status failures, HTTP client timeouts, and GraphQL `errors[]` responses
- empty discovery, discovery pagination, marker write failure, unknown status fallback, BGP empty peer, and BGP total-failure rotation tests
- config schema JSON parsing

The raw fixture baseline is `src/go/plugin/go.d/collector/cato_networks/testdata/centreon-cato-api.mockoon.json`, copied from the mirrored Centreon plugins repository. SDK-generated query aliases may require an adapter in tests, but the source fixture must remain available so future SDK bumps can be compared against the original reference payloads.

For registry or dependency changes, also run:

```bash
cd src/go
go test ./plugin/go.d/... -count=1
```

Live Cato tenant validation is required before claiming real-world coverage of optional payload fields, BGP scale behavior, or dashboard topology rendering beyond unit-tested JSON/function behavior.
