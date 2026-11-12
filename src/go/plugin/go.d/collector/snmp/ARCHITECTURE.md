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

Normal SNMP polling jobs read the five standard system scalars once per minute, synchronously before metric polling. The
interval is internal and is evaluated on collection ticks; scheduling, request duration and failures can delay a
successful observation. `ping_only` keeps its setup-only SNMP behavior. There is no periodic rebuild and no public
option.

`device_refresh.go` owns the check. It has one response to a change: `rebuildDevice` replaces every piece of
device-derived state. The trigger is a single predicate, so widening or narrowing it does not change how state is
refreshed.

| Observation | Response |
|---|---|
| No answered field differs | Nothing; ordinary collection continues |
| Request fails, or a field is unanswered | Not a change; the previous value stays |
| Answered `sysName`, `sysDescr`, `sysContact` or `sysLocation` differs | Rebuild. If it fails, the incumbent state keeps collecting and the next check retries |
| Valid `sysObjectID` differs | Suspend the incumbent, then rebuild. Old-profile samples stay suspended until a rebuild succeeds |

A rebuild:

- reselects profiles from the observed identity (both `sysObjectID` and `sysDescr` are selection inputs);
- prepares a new SNMP client with freshly probed transport settings before touching the incumbent, so a failed
  candidate leaves the incumbent intact;
- installs a new DD collector, which discards table caches, missing-OID decisions and prepared profile inputs;
- clears Function contents and counter baselines under their existing owner locks;
- records the existing charts as stale (`resetDevicePresentation`).

Nothing is published at the swap. Publication follows the new state's first profile poll, which runs in the same
collection cycle:

| What | When | After a failed first poll |
|---|---|---|
| Charts the new state reports | Redefined in place with their new labels (`installChart`) | Not reported, so unchanged |
| Stale charts it does not report | Removed after the first poll that succeeds (`retireStaleCharts`); the job runtime prunes them after accepted output | Stay, showing gaps like any failed poll |
| Vnode and DeviceStore registration | `syncDeviceMetadata`, once the new DD collector has prepared profile inputs | The previous publication stays, so consumers never see an identity without its profile metadata |

A partially successful first poll counts as success: charts of a profile that failed in that cycle are removed and
come back when the profile reports, the same way absent table charts behave in ordinary polling.

The vnode GUID, the DeviceStore registration ID and compatible chart identities/history stay stable. Authored
`Config`, including local vnode overrides, never receives acquired identity values.

A changed `sysObjectID` means the incumbent profiles no longer describe the device. The check immediately withdraws
the DeviceStore connection from SNMP-family consumers (the accepted registration ID and runtime writer authority are
kept) and clears Function state, even if the rebuild cannot finish yet. An unanswered or invalid object ID on a later
check cannot resume the incompatible profiles.

Normal initialization uses the same publication point: `syncDeviceMetadata` publishes after a profile poll whenever the
DD collector's prepared-input revision changed. A poll that prepared nothing publishes nothing, so a new job, a device
suspended by a changed `sysObjectID` (which stays withdrawn until then) and a rebuilt device that kept its registration
are each published together with their profile metadata. Samples only come from profiles whose inputs are prepared, so
routing to the vnode exists before any sample leaves the collector and optional profile input failures cannot send
healthy samples to the local Agent. Profile metadata is acquired once, by normal profile preparation, each publication
is a single DeviceStore update, and metadata recovered after a failed preparation publishes without a system-field
change. Diagnostic input evidence follows the installed snapshots; a rebuild starts a new current diagnostic view,
preserving sources for its own system and transport preparation.

A replacement that changes none of the five scalars is not detected.

## Metric Emission

The normal SNMP collector skips a table metric when its resolved tag map is empty. Every required identity
component must be an explicit public tag; the SNMP row index is not automatically part of emitted series identity.

`tableMetricKey` in `metric_ids.go` frames the complete metric name and sorted public tag names and values,
including explicit empty values. A bounded readable prefix and full SHA-256 digest produce the final chart-safe ID.
Dimension subkeys are framed separately for job-scoped sample-map lookup. They do not replace the chart-scoped
Netdata dimension ID: V1 emits `Dim.Name` when set and falls back to `Dim.ID` only when the name is empty. [Underscore-prefixed tags](/src/go/plugin/go.d/collector/snmp/profile-format.md#underscore-prefixed-tags)
are display metadata and do not partition identity. Acquisition preserves semantic tags and typed identity bytes;
only temporary `rm:` tags are removed. The framework sanitizes chart-label values when serializing the wire protocol,
after machine identity has been established.

Rows reaching `collectProfileTableMetrics` with the same exact tuple accumulate ordinary numeric values into one
sample; multi-value dimensions use assignment. Distinct semantic tuples remain distinct even when presentation
sanitation makes their display labels look the same. Historical table IDs are not backfilled after migration.

Table charts absent from the current results are retired, preserving the existing ordinary-poll behavior. A
[device rebuild](#runtime-system-metadata) clears the seen state and records existing charts as stale; fresh DD results
redefine active charts and the rest are removed after the first successful poll. Newly observed composite dimensions
are added, and current-profile dimensions can revive pending incumbent removals after a rebuild. The chart owner
indexes live dimensions so steady collection remains O(observed values), without scanning all retained charts.

`collect_snmp.go` owns emission and `normal_diagnostics.go` records the relation to captured samples. Profile authoring
belongs to the [table metric guide](/src/go/plugin/go.d/collector/snmp/profile-format.md#table-metrics-multiple-rows).

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
