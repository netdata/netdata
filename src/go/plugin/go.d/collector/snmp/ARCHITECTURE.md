# SNMP Collector Architecture

Maintainer reference for the normal SNMP collector and the subpackage boundaries needed to investigate its results.
Paths and symbols below are relative to this collector directory unless a link points elsewhere.

**Place in the documentation set.** This document owns normal profile selection, sample emission, and BGP/licensing
consumer behavior. The project skill `.agents/skills/triage-snmp-diagnostics/SKILL.md` cites its heading anchors; update
those references when moving a section. The [profile guide](/src/go/plugin/go.d/collector/snmp/profile-format.md) owns user-facing profile syntax and
rules. The [diagnostics CLI README](/src/go/tools/snmp-diagnostics/README.md#normal-device-evidence) owns tool usage
and evidence interpretation. Topology production remains in the
[SNMP topology architecture](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md).

## Collection Boundaries

| Layer | Source entry points | Responsibility |
|---|---|---|
| Normal collector | `collect.go`, `profile_sets.go`, `collect_snmp.go` | Runtime initialization, profile selection, and orchestration of profile results into charts and samples |
| Profile acquisition | `ddsnmp/profile_catalog.go`, `ddsnmp/ddsnmpcollector/collector.go` | Consumer-specific profile projection, SNMP acquisition, value processing, virtual metrics, and typed BGP/licensing rows |
| Collector consumers | `bgp_integration.go`, `licensing_integration.go` | Peer/cache updates and typed BGP samples; normalized license inventory and aggregate samples |
| Diagnostic capture | `normal_diagnostics*.go`, `diagnostics/` | Capture adapters plus shared documents, codecs, and file publication |

Acquisition success, emitted samples, stored consumer state, and a Function response are distinct stages. Use the
[diagnostic stage interpretation](/src/go/tools/snmp-diagnostics/README.md#acquisition-processing-and-samples)
when correlating them. File cadence, retention, and publication remain documented under
[Diagnostic Files And Publication](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#diagnostic-files-and-publication).

## Profile Selection

For normal SNMP collection, `setupProfiles` uses `manual_profiles` only when `sysObjectID` is empty. A non-empty but
unmatched `sysObjectID` does not fall back to that list. After catalog matching, consumer projection keeps metrics,
BGP, and licensing definitions for normal collection; topology has its own
[composition](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#topology-profile-composition).
A matched profile with no data for the requested consumer can disappear from that consumer's collection set. When
investigating selection, compare the captured profile context's selected profiles and projection, not only profile
names.

`setupProfiles` lives in `profile_sets.go`; catalog matching and projection live in `ddsnmp/profile_catalog.go`.
The captured selection/projection description is assembled by `ProjectedView.Context` in `ddsnmp/profile_context.go`.

## Metric Emission

The normal SNMP collector skips a table metric when its resolved tag map is empty. To keep rows distinct, provide
non-empty identifying tag values; the SNMP row index is not automatically part of the emitted series identity.

The collector's `tableMetricKey` combines the metric name with non-empty public tag values, ordered by tag key.
[Underscore-prefixed tags](/src/go/plugin/go.d/collector/snmp/profile-format.md#underscore-prefixed-tags) do not distinguish chart IDs. Rows that reach
`collectProfileTableMetrics` with the same key accumulate their ordinary numeric values into one sample; multi-value
mapped dimensions use assignment instead. A captured row can therefore be present without having a distinct chart
series, and a final sample can combine several rows.

`collect_snmp.go` owns these emission decisions; `normal_diagnostics.go` records their relationship to captured samples.
Profile authoring details remain in the [table metric guide](/src/go/plugin/go.d/collector/snmp/profile-format.md#table-metrics-multiple-rows).

## BGP Collection And Retained Rows

Profile syntax and peer identity requirements are documented in [BGP rows](/src/go/plugin/go.d/collector/snmp/profile-format.md#bgp-rows).

The acquisition layer admits a typed BGP row only when it has a signal and the identity required by its row kind.
`bgpRowHasSignals` and `bgpRowIdentityComplete` in `ddsnmp/ddsnmpcollector/collector_bgp.go` define this boundary.
Receiving an identity OID alone does not establish a peer. Successful rows can survive errors elsewhere in the same
profile; the profile-level BGP result can succeed while acquisition/processing reports contain failures.

The normal collector's `bgpIntegration.prepareProfileMetrics` updates peer state from returned profile results. A
returned profile with `BGPCollectError` keeps its previous source-owned peers as stale and contributes no current typed
BGP chart metrics. Sources without that error replace their peer rows, including after partial success. Profiles omitted
before reaching this BGP consumer do not automatically receive this retention behavior. A top-level SNMP collection
error leaves the previous peer cache and marks collection failed instead of refreshing it.

`bgpPeerCache` applies a failure-associated freshness window to retained rows: age by itself does not mark successful
peer data stale. Function requests can suppress expired failed-source entries or report the data unavailable. Compare
source failures and per-entry update times as well as the overall cache time; retained peer state does not imply a
successful current measurement.

## Licensing Normalization And Collection Results

Profile syntax, signal definitions, and date formats are documented in [Licensing rows](/src/go/plugin/go.d/collector/snmp/profile-format.md#licensing-rows).

The normal collector converts typed profile rows through `licenseRowFromTyped` in `licensing.go`. A row needs an
identity and at least one typed signal; vendor text or perpetual/unlimited descriptors alone are insufficient.
Remaining-duration signals become absolute expiry times at normalization. `deriveLicenseUsage` can fill missing used
capacity from capacity minus available, when available is within that capacity, and derive a missing percentage from
used/capacity when capacity is positive and the license is not unlimited.

The normalized state bucket is not simply the raw vendor text or numeric severity. `normalizeLicenseStateBucket` in
`licensing_state.go` gives an ignored raw classification priority; otherwise an expired applicable timer or exhausted
finite usage is broken, and unexpired grace is degraded. An informational raw classification then takes priority over
typed severity; other rows use typed severity when present, followed by raw-state and signal-based fallbacks.

Charts aggregate the accepted normalized rows through `aggregateLicenseRows` in `licensing_aggregate.go`: state charts
count rows by bucket, timer charts use the minimum remaining time for each timer kind, and usage uses the maximum
rounded percentage. Ignored rows contribute only state counts; perpetual expiry and unlimited usage are excluded from
the corresponding timer/usage aggregates. These charts do not describe one selected license from the drill-down.

Whenever `licensingIntegration.collect` runs, it replaces the cached set, including with empty or partial results.
A top-level SNMP error returns before this consumer runs and leaves the old set intact. Licensing has no BGP-style
failure-expiry gate. The `snmp:licenses` Function uses stored state buckets but computes displayed remaining time at
request time; its timer and bucket can therefore reflect different evaluation times until the next normalization.
