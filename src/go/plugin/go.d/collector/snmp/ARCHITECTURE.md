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
| Normal collector | `collect.go`, `device_refresh.go`, `profile_sets.go`, `collect_snmp.go` | Runtime initialization, system observation, profile selection, and orchestration of profile results into charts and samples |
| Profile acquisition | `ddsnmp/profile_catalog.go`, `ddsnmp/ddsnmpcollector/collector.go` | Consumer-specific profile projection, SNMP acquisition, value processing, virtual metrics, and typed BGP/licensing rows |
| Collector consumers | `bgp_integration.go`, `licensing_integration.go` | Peer/cache updates and typed BGP samples; normalized license inventory and aggregate samples |
| Diagnostic capture | `normal_diagnostics*.go`, `diagnostics/` | Capture adapters plus shared documents, codecs, and file publication |

Acquisition success, emitted samples, stored consumer state, and a Function response are distinct stages. Use the
[diagnostic stage interpretation](/src/go/tools/snmp-diagnostics/README.md#acquisition-processing-and-samples)
when correlating them. File cadence, retention, and publication remain documented under
[Diagnostic Files And Publication](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#diagnostic-files-and-publication).

## Profile Selection

For normal SNMP collection, `setupProfiles` uses `manual_profiles` only when `sysObjectID` is empty. A non-empty but
unmatched `sysObjectID` does not fall back to that list. `snmputils.GetSysInfo` accepts a valid numeric OID sent as
`OctetString` text and treats any other value as empty, so such devices use `manual_profiles`; the probe records
only the value's ASN.1 type. After catalog matching, consumer projection keeps metrics, BGP, and licensing
definitions for normal collection; topology has its own
[composition](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#topology-profile-composition).
A matched profile with no data for the requested consumer can disappear from that consumer's collection set. When
investigating selection, compare the captured profile context's selected profiles and projection, not only profile
names.

`setupProfiles` lives in `profile_sets.go`; catalog matching and projection live in `ddsnmp/profile_catalog.go`.
The captured selection/projection description is assembled by `ProjectedView.Context` in `ddsnmp/profile_context.go`.

## Runtime System Metadata

Normal SNMP polling jobs read the five standard system scalars once per minute, synchronously before metric polling.
The interval is internal and is evaluated on collection ticks; scheduling, request duration and failures can delay a
successful observation. `ping_only` keeps its setup-only SNMP behavior.

`device_refresh.go` separates metadata updates from device-state rebuilding. Name, contact, location and description
changes refresh scalar profile tags/device metadata on the existing DD collector/client, then update the generated
vnode, chart labels and DeviceStore. `RefreshProfileInputs` stages per-profile inputs, retaining usable snapshots on
failed/incomplete acquisition without suppressing healthy metric profiles. It reconciles only system-OID negative
decisions contradicted by authoritative observed values. Existing row caches, connections and Function rate baselines
remain intact. Authored `Config`, including local vnode overrides, never receives acquired identity values.

Only a confirmed valid changed `sysObjectID` triggers full rebuilding. Neither elapsed time nor a changed description
alone triggers profile reselection. Replacements with unchanged `sysObjectID` are an initial detection limitation.
There is no periodic full rebuild or new cache-expiry policy.

A confirmed classification change immediately suspends incumbent profile samples, even if replacement setup fails.
Its old DeviceStore connection is withdrawn from SNMP-family consumers without changing the accepted registration
ID or runtime writer authority; new compatible state makes that registration eligible again.
Retries occur on subsequent system checks; an unanswered/invalid object ID cannot resume incompatible profiles.
Successful replacement uses fresh profile/device/transport state, retires old charts through V1 output, and clears
Function contents/counter baselines under their existing owner locks. Current correctly classified profiles retain
partial-collection availability. System identity establishes vnode routing before optional profile enrichment so
healthy samples cannot fall back to the local Agent. The GUID and compatible chart identities/history remain stable.

Normal initialization and rebuilding compose identity from prepared profile metadata rather than fetching it twice.
Diagnostic input evidence follows retained or installed snapshots; classified replacement starts a new current
diagnostic view, preserving sources for its own system and transport preparation.

## Metric Emission

The normal SNMP collector skips a table metric when its resolved tag map is empty. To keep rows distinct, provide
non-empty identifying tag values; the SNMP row index is not automatically part of the emitted series identity.

The collector's `tableMetricKey` combines the metric name with non-empty public tag values, ordered by tag key.
[Underscore-prefixed tags](/src/go/plugin/go.d/collector/snmp/profile-format.md#underscore-prefixed-tags) do not distinguish chart IDs. Rows that reach
`collectProfileTableMetrics` with the same key accumulate their ordinary numeric values into one sample; multi-value
mapped dimensions use assignment instead. A captured row can therefore be present without having a distinct chart
series, and a final sample can combine several rows.

A failed profile acquisition does not establish row removal. Its existing table charts remain defined while current
measurements are unavailable; successful source observations can retire rows that are actually absent.

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
