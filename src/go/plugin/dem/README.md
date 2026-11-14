# DEM plugin

`dem.plugin` runs native Netdata jobs for browser real user monitoring, Playwright journeys and desktop Lighthouse audits.
Linux builds include the plugin by default; the browser runtime is an additional, explicit installation.
Use `ENABLE_PLUGIN_DEM=OFF` (source installer: `--disable-plugin-dem`) to omit the plugin. The integration metadata beside each collector is the
source of operator configuration documentation. The [architecture guide](ARCHITECTURE.md) explains how the plugin
is built, from the overall data flow to package ownership and runtime lifetimes.

The stock configuration starts one loopback receiver and no sites. Configuration follows the normal Go Agent layout:
`dem.conf` for plugin policy and `dem/receiver.conf` / `dem/rum.conf` for collectors. The receiver is a canonical single
object (`dem:collector:receiver`); each RUM monitor is an independent job (`dem:collector:rum:<name>`). Native `name` is the
stable beacon/history key; `display_name` is presentation metadata. Renaming a RUM job starts a new history namespace.

## Organize monitors on a site vnode

A DEM site is a configured Virtual Node (vnode). Set the optional `vnode` reference on independent RUM, journey and
Lighthouse jobs to place their charts on that node. Omit it to keep charts on the collecting Agent and leave the job
unassigned to a site. A site with only synthetic monitors needs no RUM job or receiver.

Create a static vnode through the plugin's `dem:vnode` dynamic configuration entry, or define one in a YAML file under
`vnodes/` in the Netdata configuration directory. For example, `/etc/netdata/vnodes/sites.yaml`:

```yaml
- hostname: storefront
  guid: 12345678-1234-4234-8234-123456789abc
  labels:
    environment: production
```

Generate a unique GUID for each site, for example with `uuidgen`. For file-defined static vnodes, `hostname` is also
the reference name used by jobs. A dynamically configured vnode uses its configuration entry's name as the reference.
Use the reference name in `vnode`, not the GUID. See [node identities](../../../../docs/learn/node-identities.md#virtual-nodes-vnodes)
for the standard configuration and identity rules.

These entries place two independent jobs on Storefront:

```yaml
# dem/rum.conf
jobs:
  - name: storefront-rum
    vnode: storefront
    display_name: Storefront
    allowed_origins: [https://shop.example.org]
```

```yaml
# dem/journey.conf
jobs:
  - name: storefront-checkout
    vnode: storefront
    script_path: /etc/netdata/dem/checkout.spec.ts
    check_every: 300
    update_every: 10
    timeout: 120
```

The RUM installation key remains `storefront-rum`, so its script URL ends in `/rum/storefront-rum.js`. Each job keeps
its own configuration, DynCfg entry, schedule, secrets and enable/disable controls. Names must be distinct within each
collector module. Guided site setup creates or selects a vnode and submits ordinary independent job operations;
partial success must show which operations need attention. A vnode does not own child jobs, browser workers or shared
retention budgets, and referencing it does not deploy jobs to other Agents.

DEM Functions remain global on the collecting Agent. Invoke their existing names through that Agent or Cloud and
select a site by its configured vnode name, for example `synthetics-runs vnode:storefront` or
`rum-pages vnode:storefront`. The name is the exact resource name used by the job's `vnode` option, not its display
hostname. Name lookup uses current vnode configuration, including when all jobs are disabled or no jobs reference it.
Renamed or removed names stop resolving; reusing a name selects its current GUID, without aliases to earlier identities.

Use `vnode_guid` with the recorded canonical GUID to query an earlier identity or retained history after its vnode is
removed, for example `synthetics-runs vnode_guid:12345678-1234-4234-8234-123456789abc`. Supply only one selector;
empty or unknown vnode names and requests containing both selectors are rejected, even if either value is empty.
Omitting both selectors, or supplying only an empty `vnode_guid`, includes all vnodes and unassigned data.
These selectors apply to all six RUM Functions and `synthetics-checks`, `synthetics-runs`, and `synthetics-artifacts`;
`synthetics-run` and `synthetics-artifact` keep their existing detail parameters.

The selectors filter data under the existing Agent-level access model; they are not a per-site authorization boundary.
Functions are not registered on each vnode. Frontend consumers retain the collecting Agent relationship when selecting
a site, including for archived vnodes and retained history.

Functions and retained evidence expose `vnode_guid`; charts use normal vnode placement without an extra GUID label.
Existing RUM `site` fields identify the physical RUM job. RUM observations capture the resolved GUID when admitted;
synthetic attempts capture it when submitted to the execution queue, before waiting for browser admission. Saved and
buffered investigation records, runs and captures retain that GUID after reassignment. A changed RUM GUID starts a fresh rolling measurement window and Pages view; hostname or label
edits with the same GUID do not reset measurements. Transport and history counters remain job-wide. Old evidence without
a captured GUID stays unassigned.

Vnode placement does not combine RUM populations, change artifact ownership or increase browser capacity. Execution,
receiver service and retention remain shared by the collecting Agent.

## Investigate journey failures

`synthetics-runs` includes the first unexpected test failure, the last observed running step and pending-before-cleanup context when the runtime
supports structured journey evidence. `synthetics-run` also returns `run.failure_evidence`: test observations,
step identities and parent relationships, source locations, bounded error excerpts, and the first failure's step path.
The overall outcome and termination reason remain separate. An earlier assertion can be retained even when a later
test exhausts the execution budget. Failed configuration rehearsals use the same diagnostic summary.

For the first timed-out or interrupted test, `run.failure_evidence.stop_context` preserves
pending activity observed just before Playwright starts cleanup, even after ordinary step
limits fill. The “Pending before cleanup” column describes that observation, not the cause or
exact location of the timeout. Later cleanup errors do not replace it. Concurrent branches
show their common ancestry and disclose incomplete detail; missing observations and
cleanup-only timeouts can show only the test with an incomplete-path marker. Source locations
identify the displayed pending step, or the test declaration when no step is known; they do
not identify where the timeout occurred.

Evidence status is `not_recorded` for older history or attempts without runtime evidence, `unavailable` when negotiation was not reported, `unsupported` when the
runtime declined the feature, `partial` when reporting stopped, `complete` when its completion marker arrived,
and `invalid` when optional reporting was malformed. A test can have a complete report even when a later test is
interrupted. Completion describes reporting; it does not prove that every test ran. Truncation is separate and can
coexist with complete reporting. Missing evidence never means zero errors.

Detail is bounded: up to 64 tests, 128 user steps and 64 other steps, with a separate first-failure reserve.
Source labels are relative to the journey's directory; external locations are labelled `[external]`.
Error excerpts can include Playwright's call log, but are redacted and clipped. Deep ancestry, unusually large text
or exhausted detail budgets are explicitly marked. A last-observed step describes a received observation, not
current browser activity. Captures keep their existing access rules and publication requirements.

## Optional journey browser error summaries

Set `browser_error_summary: true` in a journey job to collect browser error counts and bounded primitive samples,
including on passing tests. It defaults to `false`. The `synthetics-runs` columns `browser_summary_status` and
`browser_errors` show reporting status and observed counts; `synthetics-run` returns typed `run.browser_summary`
with coverage, aggregate counts, interesting test rows, samples and explicit omissions. Failed configuration
rehearsals include a text summary; successful rehearsals retain their existing success response and do not save runs.

Coverage is limited to contexts supplied by the journey's Playwright fixtures, including their pages and popups.
Manually created contexts and contexts supplied by replacement fixtures are outside this scope; worker errors
are not comprehensive. The categories are uncaught errors, console errors, failed requests and HTTP responses with status 400
or higher. Missing or incomplete observations never establish zero errors. Counts continue after sample limits;
report completeness, coverage and sampling omissions are separate facts. Browser anomalies do not change test
outcomes, trigger retries or extend deadlines. Summary finalization does not wait for pending samples.

Console samples contain only whole primitive values or explicit omissions; objects are not rendered. Unavailable,
pending, unsupported and oversized values remain explicit. No headers, response bodies or cookies are captured.
Structured URLs have credentials, query strings and fragments removed, and recognized embedded HTTP(S) URLs in
text receive best-effort stripping. Known configured secret values are redacted before clipping or byte checks.
Arbitrary page-derived text is not anonymized and can contain sensitive content.

Owned detail is bounded to 64 interesting test rows and four representative samples per category, with sixteen
samples total. Each test has three candidate opportunities per category, at most three console sampling calls and
four arguments per call. Pending, rejected and duplicate candidates consume their opportunities. Counts are not
estimates of distinct incidents, and differences between counts and retained samples are not lost-event counts.
After the row limit, a previously unrepresented category can retain its first sample in the same pool.

Enabling observation can add substantial CPU and memory cost on pages with heavy console traffic, even when the
messages are not errors. Sample limits do not bound Playwright's message dispatch or handle memory. See
[measured browser-summary costs](PERFORMANCE.md#optional-browser-error-summary-costs) before enabling it on noisy
journeys or increasing concurrency.

## Receive browser reports

Expose the receiver through HTTPS, configure its public URL and actual trusted proxy addresses, then install
`<script async src="https://rum.example.org/rum/shop.js"></script>` on the configured website. Public URLs may include a
reverse-proxy path prefix. The receiver serves the pinned Faro 2.11.0 SDK and optional tracing bundle; visitors need no
SDK CDN connection. Client IP addresses are used for rate limiting/geolocation and are not stored.

`dem-history-status` is always announced. The six RUM Functions appear once a RUM site is active or RUM history is
retained; the five synthetics Functions appear once a journey or Lighthouse job is active, or synthetic history or
published captures are retained. Retained data makes the corresponding group available after restart even without
jobs. A receiver alone does not enable RUM Functions. Once announced, a group stays available for that framework run,
even after the last job stops or its data expires. A process restart reconstructs availability from data present when
the stores open. If startup retention then expires that data, its group still appears for that process lifetime.

RUM and synthetic Function providers are independent of collector selection. Disabling `rum` or `journey`, selecting
only `lighthouse`, or disabling all collectors preserves investigation of retained history while the plugin is enabled.
Setting `enabled: no` disables the whole plugin. Handler instances belong to each framework run generation; the command
keeps shared stores open until a clean shutdown.

Six process Functions use the regular Netdata Function transport: `rum-sites`, `rum-pages`, `rum-live`, `rum-sessions`,
`rum-errors`, and `rum-session-events`. Viewing them, and the `synthetics-*` Functions, requires a signed-in member of
the node's Netdata Cloud Space with permission to view sensitive data, on a paid plan or an active trial; collection and
history storage are not restricted.
Runtime observations include only admitted site jobs. Consumers join native
DynCfg state to show disabled or failed configurations. Journal history remains queryable after a site stops. History range
arguments select when the Agent originally received an observation, using inclusive Unix seconds: both endpoints include
their entire second. Negative values are offsets from one captured now; explicit zero means epoch. With omitted bounds,
RUM uses the 15 minutes ending at `before` (or now). Invalid or pre-epoch ranges fail.
Delayed session promotion preserves that original receipt time, so late-saved context can change an earlier range. A
past range shows currently retained evidence, not what was known at the time. Browser clocks do not define the native
history axis.

`rum-live` returns an opaque string `next` cursor, passed unchanged as `after` on the next request. Each event has a
string identity combining site, runtime generation and sequence. Cursors track independent streams so a busy site cannot
advance a quiet site's position. A replaced site's runtime starts a fresh stream. Missing Web Vitals are `null` in
Functions and gaps in charts; an available but quiet receiver measures zero traffic. Receiver outages leave site jobs
running with explicit ingress availability and gaps in browser product measurements.

On Linux, DEM requires a valid machine ID in `/etc/machine-id` or `/var/lib/dbus/machine-id` to open its history store.
Minimal or custom container environments must provide that OS identity before starting the plugin.

Investigation history uses pure-Go journal files under `${NETDATA_LIB_DIR}/dem/journal`, through the existing
`systemd-journal-sdk`. Events include their session and error metadata, so investigation does not depend on a separate
parent record. `rum-sessions` reports sessions with activity originally received in the selected range; document views, application views, errors, frustration
counts and activity spans describe retained events in that range, rather than lifetime totals. `rum-errors` groups
retained occurrences by fingerprint; affected-session, top-page and browser statistics are `null` until a `fingerprint`
filter selects the group to investigate. `rum-session-events` returns the full retained and pending timeline for a
session, ordered by Agent observation time in Unix microseconds, without range filtering. Activity-only presence records
contribute to summaries but stay out of timelines. Session rows sort by `last_observed_us` at microsecond precision
before limiting; first/last observations and their span describe only the selected activity. A gap between observations
does not prove continuous presence.

One process-owned retention worker applies one shared history budget across RUM, synthetic runs, disabled jobs and
retained machine identities. Defaults are 30 days and 1 GiB; `history.max_bytes` must be at least 8 MiB. Accounting uses
journal file lengths, including preallocation. Whole files expire after their latest saved record ages out; size pressure
removes the oldest files sooner and can shorten the investigation window. High-volume RUM can evict older synthetic runs.
New files span less than 24 hours of saved time. With normally advancing clocks and timely successful hourly maintenance,
age alone can retain older records for approximately one extra day plus one hour. Existing files keep their actual spans.
Query time ranges still use original observation/run time, so saved-time retention does not establish query coverage.

The byte target is not a physical disk cap: active growth, allocation rounding and admitted appends can exceed it;
open query readers can keep removed files allocated. Unsafe files and failed cleanup can leave excess until resolved.
Hourly sweeps do not force archives; idle active files are sealed when needed for expiry. Valid policy edits apply at the
next sweep. Invalid edits retain the last valid policy and report a warning and status error. Artifacts have a separate
retention policy, defaulting to 7 days and 2 GiB.

`dem-history-status` reports the effective policy, sampled file bytes/count, observation time, latest cleanup attempt,
last successful cleanup, and separate inventory, cleanup, writer and policy reload errors. It is a read-only member
Function that, like every DEM Function, requires a signed-in member of the node's Space with permission to view
sensitive data, on a paid plan or an active trial. It works even with all collectors
disabled, with no arguments, payload or time filters. Unknown or unattempted values are `null`; an inventory failure
does not erase cleanup or writer facts. These facts help diagnose storage pressure and maintenance failures, but cannot explain whether an absent event expired, was sampled out or was never collected.
Unusable history at startup still stops the plugin before Functions register; inspect process logs in that case.

Queued events flush every five seconds or 500 records.
Flushes sync pending appends; clean idle ticks skip sync. A crash can lose records still queued or
not synced. Queue overflow and append failures count dropped records; sync failures produce a warning. Failed retention
sweeps retry after five seconds, independently of healthy appends. An uncertain journal mutation or sync failure makes shared history unavailable until
restart verification; subsequent reads, writes, sync and retention fail rather than expose a partial result. RUM measurements
and enabled exports continue independently, and synthetic current diagnosis reports history errors. This is sampled
investigation history, not a lossless event archive.

Stock health templates evaluate the current LCP, INP and CLS populations once per minute. They require at least 30
observations for that metric and complete local measurement state. More than 25% above the good threshold warns;
more than 25% above the poor threshold is critical. Exactly 25% does not trigger. These alerts describe recent observed
experiences: a brief poor burst can trigger, and a low-traffic metric can remain ineligible. They do not average p75
values or establish that a problem persisted for ten minutes. Missing, stale or incomplete input is undefined, not
recovery. Notification recovery delay does not change these evaluation semantics. Quiet traffic alone has no stock
alarm because the Agent has no expected-traffic schedule.

Customize alerts through normal Netdata health configuration, using `_collect_plugin=dem` and `_collect_job=<site>`
chart-label filters for site-specific policies. The plugin does not generate health files or reload health configuration.
Terminal/debug runs use private temporary journal and identity state, removed after readers and workers join; they leave
live state untouched. Failed temporary history is preserved for investigation. An existing experimental `history.db` is not imported or deleted.

New journals use `dem.journal` for active files and `dem@…journal` for finalized archives. Startup verifies preexisting
active files before any journal reuse or archival, including retained machine directories. Finalized archives from the
current schema and synchronized writer lifecycle avoid a full integrity scan at every restart. Unexpected archive state,
old or partial schemas, detected index damage and quarantined `.journal~` files fail startup and remain intact for
operator recovery; there is no automatic migration, repair or deletion. External file modification is outside this
archive-provenance guarantee.

The browser normalization, rolling aggregation, OTLP and query behavior originated in the experimental
[Netdata digital-experience POC](https://github.com/netdata-labs/dem), commit
`b3de4662f567dc63d33fee6201f8c2313568301b`. Its private configuration controller, scheduler, protocol emitter and root
state reconciliation are replaced by the native Agent framework. Embedded third-party asset notices remain beside their source.

## Install and verify browser collection

Set receiver `public_url` to the HTTPS base visitors can reach, for example `https://rum.example.org/edge`.
A site's `public_url` can override it. `rum-sites` offers the resulting `script_url` and HTML `snippet`; both are
`null` when neither URL is configured. The receiver can still start and receive requests. A listener address or
forwarded Host header never becomes an installation address; `trusted_proxies` applies to client IP provenance.

Route the complete prefixed `/rum/` namespace to the receiver: the external script, SDK assets and collection POSTs.
The browser resolves these requests relative to the external script element's URL. Use a canonical direct URL:
a redirect of only the bootstrap does not redirect its sibling requests. Install a classic external script, including
an asynchronously inserted external tag; copying the script body into an inline script, module or tag-manager eval
is unsupported. Configure the tag manager to insert the external script element instead.

Allow the receiver's script URLs in the site's applicable `script-src-elem` or `script-src` policy, and collection URL
in `connect-src`. A nonce on the installed script is propagated to the SDK scripts; `strict-dynamic` trust follows
normal browser rules. The receiver permits cross-origin embedding of its public scripts, including on cross-origin
isolated pages. Collection still enforces the site's allowed origins. Policies requiring Trusted Types script URLs
need deliberate website integration; the bootstrap does not create a permissive policy. Inspect browser CSP errors
rather than infer policy from a fetched homepage. HTTPS pages require HTTPS receiver URLs.

To troubleshoot, use the website's browser Network and Console panels:

1. Confirm the external site script loads, then the receiver-hosted SDK (and optional tracing SDK). Browser-console
   messages identify SDK load or initialization failures; a tracing **load** failure permits core RUM to continue.
2. Inspect the `/rum/<site>/collect` POST and any preceding OPTIONS request. A rejected preflight can prevent the POST.
3. Check `rum-sites` for a new accepted-payload timestamp. For a deterministic sampling check, explicitly set
   `measure_sample_rate: 1` and use a fresh eligible browser session. A fresh session at a fractional rate may still
   be sampled out; automation is excluded unless `bots: include` is configured.

| Collection POST response | Meaning / action |
| --- | --- |
| 202 | Payload accepted into live processing; does not guarantee durable history or exporter delivery. |
| 204 | Intentionally excluded bot or disabled collection. Check the current site policy. |
| 400 | Invalid payload. Check that the installed bootstrap and served SDK belong to this receiver release. |
| 403 | Origin refused. Compare the observed origin with the intended website before editing allowed origins. |
| 404 | Site runtime is absent or the proxy route is wrong. Check native job state and request path. |
| 413 | Payload exceeds the receiver body limit. |
| 429 | Receiver rate limit reached. Check traffic and rate settings. |

`rum-sites` exposes independent observations: receiver serving state, collection enabled, receiving runtime generation,
last accepted payload and last rejected origin. Timestamps use Unix microseconds; timestamps and age fields are null when unseen. A rejected-origin observation can come from OPTIONS and can remain after successful collection.
The rejection rate counts origin, rate and size rejections, excluding malformed payloads and bot exclusions. Silence
is unknown; these fields do not certify website deployment, real human traffic or an outage. `/rum/ping` checks only
whether the requesting browser can reach the receiver. Demo traffic cannot verify production installation.

Runtime receipt evidence resets when the site runtime is replaced or restarted. Already-open pages can continue
using earlier sampling/capture settings; receipt by a new runtime does not prove that page loaded its new bootstrap.
A zero collection rate prevents new SDK loads and receiver processing. Browser-side policy changes reach pages on their next load.

SDK assets have immutable byte-identified URLs; the per-site bootstrap revalidates on each load. All backends serving
a public receiver route must serve the assets referenced by its bootstrap; coordinate routing and deployment across
replicas. Agent builds and runtime need no SDK download. Shipped license notices are available through each asset's
`rel="license"` response link. Source and update verification live in [the asset maintenance guide](rum/faro/assets/README.md).

## Browser measurement contract

A document experience starts at the initial document activation or a back-forward cache restore. The bootstrap assigns
an activation identity before the SDK queues reports. `rum.document_views` counts these explicit activations;
`rum.application_views` counts explicit application-view occurrences separately. SPA navigation does not create a
new document experience. Web Vitals remain associated with the document-entry page, even if first reported after a
route change. The application-view dimension describes route activity and errors; it does not imply route-level Web
Vitals.

Each recent document experience contributes its latest reported value for each Web Vital. Repeated CLS or INP updates
replace that experience's earlier value instead of adding visitors to the population. SDK metric identity and increasing
report revisions survive normalization, live queries, retained history and enabled event-log exports. A `rum-live` row
is an observation, not an independent pageview: use its kind, experience, metric and revision fields when updating a
visualization. Later reports retain their own user and application-view context while measurement dimensions remain
attributed to the original document.

The configured `window` uses receiver arrival time, including for delayed reports. A genuinely newer revision extends
that metric's presence in the window; a duplicate does not. Duplicate detection retains evidence for 30 minutes,
unless the site job restarts, is replaced, or changes its vnode association. Identified SDK session lifecycle events
follow the same rule, so replay does not renew observed-session windows. It is best effort, not exactly-once delivery.
Reports arriving after retained evidence expires can be counted again. Late observations are not rejected merely to
claim perfect deduplication.
Errors, console logs, spans and ordinary custom events are not covered by this identity-based replay suppression;
repeated delivery of these records can count again and refresh observed-session activity.

Population charts show exact retained observations and good/needs-improvement/poor counts. Percentiles are unavailable
when there are no observations. A zero CLS or zero timing is a valid observation. Canonical measurements remain in the
configured receipt window (five minutes by default); replay identities remain for 30 minutes. There is no independent
aggregate count or byte cap. Memory grows with admitted distinct state and retained text over these horizons, plus
investigation detail and query state. This is time retention, not a fixed memory guarantee or a qualified full-scale
workload claim. Restarting or replacing the site job, or changing its vnode association, starts fresh in-memory
windows; retained investigation history is independent and preserves its original vnode attribution.

`rum.observed_sessions` counts distinct browser session identities seen in the receipt window. It does not estimate
people, concurrency or time on site. Page session counts use the same window. JavaScript errors are events: several can
occur in one document, so errors divided by document views is not a failure percentage. Missing identity excludes the
corresponding measurement and increments an invalid-measurement diagnostic; no transport-gap fallback invents views.

Breakdown traffic counts and vital populations use the same current window. Top groups and the pooled complement are
computed from current observations. `bucket=value` distinguishes a literal label named `other` from `bucket=other`.
The Pages Function exposes all retained document-entry groups, including groups outside the chart top list. Percentiles
cannot be averaged across time windows, groups or sites to obtain a combined percentile.

Navigation charts report document load time and **DOMContentLoaded handler duration**. Same-site fetch timing includes
reported same-site fetch/XHR resource entries, not all backend API requests or server availability. Resource counts
separate same-site, cross-site and unknown ownership; host buckets mark unavailable hosts as `bucket=unknown`,
distinct from a real host named `unknown`; duration populations include only entries with a valid present
duration. Performance entries for DEM delivery are excluded. Browser support, sampling and delivery can all leave gaps.

## Browser capture contract

Core capture supports page and SPA-view activity, Web Vitals and attribution selectors, navigation/resource timing,
uncaught exceptions and unhandled rejections with structured error frames, and the browser/OS/device, release,
session and view dimensions used by native diagnosis. Selectors locate the element involved; they are not DOM snapshots
or entered form values. Unsupported SDK metadata is removed before transmission and again at native normalization.

| Evidence | Browser/native capture | Live and measurements | Retained native history | Optional export |
|---|---|---|---|---|
| Core activity, vitals, timings and errors | Enabled for collected sessions | Relevant measurements and live entries | Selected investigation entries | Supported event logs; request spans only with tracing |
| Application user ID | Explicit `setUser` ID only | No live-feed field or measurement dimension | Per-event ID in pending/retained timelines; observed retained IDs for session lookup | No automatic ID export |
| Country | Receiver IP lookup; default `capture.geolocation: country` | Country comparisons | Country on retained entries | Country on supported records |
| Approximate city and coordinates | Only `capture.geolocation: city` | City and map markers | No city or coordinates | No city or coordinates |
| Rage, dead and error clicks | Only `capture.frustration_signals: true` | Heuristic counts and activity; absent when disabled | Selected heuristic entries | Selected events when event logs are enabled |
| Custom-event attributes | Only with event logs enabled | No arbitrary attribute dimensions | No generic attributes | Searchable event attributes |
| Console messages | Only with event logs and `include_console_logs` enabled | No core-error substitution | No console archive | Selected console logs |
| Browser request spans | Only with tracing enabled | Native resource timing remains independent | No span archive | Selected browser spans |

`capture.geolocation` accepts `off`, `country` and `city`. Omission or `null` selects `country`; an empty string is
invalid. Quote `'off'` in YAML because unquoted `off` is a YAML boolean. Country-only capture supplies no map coordinates. City mode provides approximate IP-derived locations,
not browser GPS or precise visitor positions. `off` skips location lookup entirely; network IP handling for request
admission, trusted proxies and rate limiting still operates. Client IP addresses are not stored as telemetry.

Frustration signals default to false, including omission or `null`. Enabling them adds browser interaction listeners
and heuristic evidence; repeated clicks, clicks without a detected response and clicks near an error do not prove user
intent, frustration or causality. A disabled signal is unavailable, not a measured zero.

An application-provided user ID is an explicit capture choice. IDs remain application-controlled values and are not
hashed or anonymized by DEM. Use an internal, non-sensitive ID of at most 128 bytes and clear it on logout. IDs undergo the same bounded text
normalization as other diagnostic strings; numeric and UUID IDs are not generalized as URL paths. Per-event attribution survives login,
logout and user changes. The `user_id` filter matches exact stored, normalized IDs within the selected observation range.
Original IDs changed by normalization cannot be recovered through lookup. Current configured credentials also mask
query output, so a displayed `[REDACTED]` value is not a reliable lookup key or a unique identity.
User names, email fields and arbitrary user attributes are not part of this identity contract.

Recognized structured URL fields lose query strings, fragments and credentials; path grouping and configured `redact_paths` rules normalize
supported URL paths. Stack source URLs retain hashed JavaScript basenames for source attribution; configured path rules
still apply. Targeted text transformations apply to diagnostic strings before grouping, history and export. Exact
configured-secret replacement applies to values of at least four bytes; shorter strings remain unchanged unless they
match a credential pattern such as `token=...` or `Bearer ...`. Use strong destination credentials.
These limited transformations are not a general anonymization, data-loss-prevention or consent system: application IDs,
error text, selectors and explicitly exported attributes can still be identifying. Choose application instrumentation
and optional capture settings accordingly. Sampling and the independent event-log/trace switches retain their existing
roles; capture choices do not imply consent or reconstruct evidence that was never collected.

## GeoIP sources and availability

Geography is optional: browser collection continues when no usable database is available. DEM reads local MMDB files;
Agent packaging, the topology IP intelligence downloader, or the operator owns acquisition and updates. DEM makes no
GeoIP downloads and does not depend on a running NetFlow plugin. Some installations package the files with NetFlow;
not every installation includes them or configures database updates.

Receiver `geoip_db` selects the source:

- Empty: try `topology-ip-intel/topology-ip-geo.mmdb` under the Agent cache directory, then its stock data directory.
  Runtime `NETDATA_CACHE_DIR` and `NETDATA_STOCK_DATA_DIR` take precedence over compiled directory defaults.
- Explicit path: use only that file, with no automatic cache or stock fallback.

Supported declared database types are `Netdata-Topology-GEO`, `GeoLite2-City`, `GeoLite2-Country`, `GeoIP2-City`,
`GeoIP2-Country`, `DBIP-City-Lite` and `DBIP-Country-Lite`. ASN and unknown/custom types are rejected. Country-only
sources remain useful. Neither the filename nor the Netdata database type establishes city coverage.

The receiver checks immediately after listener setup and every 60 seconds, including when the file is initially
absent. It adopts accepted replacements without a restart. Automatic selection prefers an accepted current cache file,
then an accepted current stock file; an unreadable or rejected cache file does not hide usable stock data. Only after
current candidates fail can DEM retain the previous usable snapshot, and only if its source was not confirmed removed.
Explicit mode can retain a previous snapshot only from its configured source. Confirmed removal revokes that snapshot;
a detected mapped-memory fault also makes it ineligible for reuse. There is no age-based expiration.

Publish each update as a complete separate file, then atomically rename it over the destination on the same filesystem.
Atomic replacement is required for consistent lookups. Do not overwrite or truncate the active file. DEM narrowly
contains detected mapped-memory faults, but this cannot guarantee recovery or detect torn content from every unsupported
in-place write. Give the Netdata service account read access to the provisioned file and its parent directories.

`rum-sites` exposes copied source evidence in `collector.geoip`, independently of each site's capture policy:

| Field | Meaning |
|---|---|
| `selection` | `auto` or `explicit`, matching receiver source selection; omitted until a receiver has been published. |
| `state` | `loaded`, `unavailable`, or `using_previous`. |
| `source` | Active `cache`, `stock`, or `explicit` source; omitted without an active source. |
| `database_type` | Active database's declared supported type; omitted without an active source. |
| `build_at` | Database metadata build time; omitted when unseen. It is not provider freshness. |
| `loaded_at` | Time the active snapshot was accepted; unchanged checks and failed refreshes do not advance it. |
| `last_checked_at` | Most recent source check; omitted before the first check. |
| `reason` | Bounded source-status code, with no filesystem path or raw error. Load errors are detailed in local Agent logs. |
| `lookup_errors` | Error count for the active source generation; ordinary unmatched addresses and absent fields are not errors. |

All three timestamps use Unix microseconds and are omitted when unseen. `loaded` means the declared geography format
is supported and the reader was accepted. It does not mean every record was validated, or establish freshness,
accuracy or coverage. Per-record decoding or invalid-field errors leave affected fields absent and increment the error
count; they do not automatically reject the whole database. The worker publishes refresh and fault changes; receiver
collection updates the copied lookup count, so Function status is not an instantaneous view of every lookup.

Site policy still applies: country is the default, `off` skips lookup, and `city` enables approximate live-only city
and coordinates. DEM never stores client IP addresses as telemetry, invents locations for unmatched addresses, or
re-enriches retained history after a database update. Database availability cannot enable capture a site has disabled.

## Collection and retained detail

RUM has two sampling decisions, both defaulting to 100%. Omitted or `null` rates use that default; explicit zero
means zero at that stage:

- `measure_sample_rate` selects new browser sessions for collection. Received measurements and live activity describe
  admitted traffic; counts are not scaled to estimate all visitors.
- `investigate.sample_rate` selects a stable baseline of received sessions for native history and enabled event-log
  and browser-trace exports. It does not reduce the received measurements or live activity further.
- `investigate.always_keep` adds problem-triggered detail beyond that baseline. It defaults to `errors` and
  `poor_vitals`; an explicit empty list disables these overrides. Overrides bias retained evidence toward problems,
  so retained rows cannot establish the prevalence of failures or enforce a fixed export budget.

For example, retain problem-triggered detail while measuring all received sessions:

```yaml
measure_sample_rate: 1
investigate:
  sample_rate: 0
  always_keep: [errors, poor_vitals]
```

With `sample_rate: 0` and `always_keep: []`, only measurements and live activity remain, even when optional exports
are enabled. With `measure_sample_rate: 0`, the generated bootstrap does not start the SDK, and the receiver returns
204 for otherwise admissible collection attempts without decoding or recording them. Existing origin, size, bot and
rate checks still apply. Intentional discard is neither accepted traffic nor a rejection/export-loss count. The
Sites table reports `collection_disabled`. Browser measurement charts are absent; receiver diagnostics remain
available. Quiet traffic does not raise a stock missing-beacon alarm.

Already open pages can continue sending and propagating trace context until navigation. Positive collection-rate
changes apply to new SDK sampling decisions; valid existing SDK sessions keep their earlier decision. Collection
zero also discards those sessions at the receiver. Sampling is a cost control, not a privacy opt-out or job disablement.

A configured problem promotes an identified session while it remains tracked. Native history receives the available
preceding timeline context (up to 100 entries), the triggering beacon's generated entries, and subsequent detail.
Each entry keeps the user, release and location observed with it; later login or navigation does not relabel earlier
context. Poor vitals retain their name and value even without element attribution. The current beacon's entries are
not truncated by the context ring, but normal admission and delivery bounds still apply.

This is bounded evidence, not a complete session archive: only 2,000 sessions are tracked per site, inactivity
expiry and eviction lose context and promotion state, and reload/restart starts new in-memory state. Promotion does
not reconstruct earlier discarded logs or spans. Browser spans can arrive separately from the error that promotes a
session, so traces can remain fragmented; a trace ID alone does not establish stored span availability.

Observations without a session ID still contribute measurements. At 100% detail they qualify for supported exports
and standalone error history; below 100%, only configured problem-triggering observations qualify. They do not
create a synthetic session or a standalone vital timeline. Function help describes the current policy; historical
evidence can reflect earlier settings, bounded context, retention and delivery loss.

## Optional event logs and browser tracing

Native RUM charts, session timelines and errors work without an OTLP receiver. Two optional features add evidence
for deeper investigation, each disabled by default and configured independently for each site:

- **Event logs** preserve searchable browser events, custom-event attributes and, when explicitly included, console
  info, warn and error messages. Search by site, session and time in the destination's logs to inspect that evidence.
  Console errors remain logs with severity and bounded, redacted error type/stack when supplied; actual uncaught
  exceptions and unhandled rejections remain core RUM errors.
- **Browser tracing** exports sampled browser request spans and propagates trace context. To follow requests into a
  backend, instrument its services, accept propagated context, coordinate sampling and send browser and backend
  spans to the same tracing system. Cross-origin APIs must allow trace headers in CORS. Browser spans alone show
  browser requests; they do not establish backend execution or trace completeness.

Enable local searchable events after preparing a local OTLP/gRPC receiver:

```yaml
jobs:
  - name: shop
    allowed_origins: [https://shop.example.org]
    event_logs:
      enabled: true
      include_console_logs: true
```

For a Netdata receiver, follow [OTLP receiver setup and log verification](../../../../docs/opentelemetry/otlp-ingestion.md).
Open that receiver node's Logs tab, select `otel-logs`, and choose `netdata-rum` in **Services**
(`resource.attributes.service.name`). Narrow the time range and filter `resource.attributes.rum.site` by site key
and `attributes.session.id` by session ID. Inspect `attributes.rum.type` (`console`, `event`, `error`, `pageview` or `vital`),
the message body, `attributes.console.level`, `attributes.event.name` and `attributes.event.attr.<key>`.
Error detail, when supplied, is in `attributes.error.type` and `attributes.error.stack`. Log access requires
Netdata Cloud sign-in. The final frontend stage will verify this complete RUM-to-Logs workflow in the UI.

Alternatively, enable browser tracing with an existing backend tracing receiver, leaving event logs disabled:

```yaml
jobs:
  - name: shop
    allowed_origins: [https://shop.example.org]
    tracing:
      enabled: true
      propagate_to: [https://api.example.org]
      destination:
        endpoint: https://traces.example.org:4317
        auth_token: ${env:TRACE_EXPORT_TOKEN}
```

Both features accept the same complete `destination` object. There is no inheritance between them. The default
endpoint is `http://127.0.0.1:4317`; it does not enable or discover a receiver. Set `http://host:port` for plaintext or
`https://host:port` for TLS, always with an explicit port from 1 to 65535 (bracket IPv6 addresses). The protocol is
OTLP/gRPC, not OTLP/HTTP. Paths, URL credentials, queries, fragments and resolver targets are not supported.
`auth_token` supplies Bearer authentication. With HTTPS, `tls_ca` selects a CA file (otherwise system roots apply),
and paired `tls_cert` / `tls_key` files enable client certificate authentication. Files must be readable by the service.
Destination credentials are used for collector-to-receiver authentication and are omitted from browser snippets and
investigation results.

Omitted or null feature blocks are disabled. An omitted or null destination or endpoint uses the local default;
an explicitly empty endpoint is invalid for an enabled feature. Null booleans mean false, null auth/TLS strings mean
empty, and null `propagate_to` means no additional origins. Disabling a feature retains its well-typed saved settings
without checking destination, TLS or propagation semantics. Native secret references throughout the job must still
resolve before it can start, including references in disabled features.

Investigation sampling applies to exported events and spans. Later promotion of a session does not reconstruct
previously discarded logs or spans. SDK session lifecycle events are ordinary sampled events; the exporter does not
invent a session-start event. Native history and the export destination retain data independently, so neither is a
complete archive of every session. A session can contain multiple traces, and event logs do not guarantee automatic
Logs-to-Traces correlation.

For an external trace receiver, look up the copied trace ID in your existing tracing system. An export endpoint URL
is not a Netdata node identity. Correct Netdata receiver selection and direct browser-to-backend trace navigation
remain part of the final frontend integration; backend export alone does not establish those UI flows.

Export is best effort and does not block native ingestion or history when a receiver is slow or unavailable. The
`rum.otlp` event-log and `rum.spans` browser-span charts appear only for their enabled feature, including before first
traffic. Their diagnostic dimensions mean:

| Dimension | Meaning |
|---|---|
| `sent` | Records accepted by the receiver, without a guarantee of queryability or durable storage. |
| `errors` | Records whose attempted export failed or was rejected; a timeout may have an unknown delivery outcome. |
| `dropped` | Records lost locally because a queue was full or shutdown ended before export was attempted. |

Intentional disablement and sampling are not export loss. Zero traffic proves neither receiver health nor failure.
The event-log and browser-span warnings each report the average combined rate of failed delivery and local loss
over the last five minutes;
they have no instance when the corresponding feature is disabled. Check the destination, authentication and TLS
settings when they fire. Failed batches are not a durable retry archive, though future batches can succeed after
receiver recovery.

## Synthetic monitoring

Each `journey` job runs one configured Playwright Test entry and its imports. Each `lighthouse` job runs one desktop
performance lab audit. Both use native job configuration. Stock configuration enables no browser jobs.
`check_every` controls browser execution: 900 seconds for journeys and 1800 seconds for Lighthouse by default.
Admitted attempt timeouts default to 120 and 180 seconds respectively. `update_every` controls chart refreshes and defaults
to 10 seconds for both. When updating an existing test configuration, move its old browser interval from `update_every`
to `check_every`; otherwise it will only change chart refresh frequency.

A job queues its first check immediately. The first recurring check is spread deterministically between one and two
check intervals after activation; later due times follow a fixed cadence. A busy job keeps at most one active and one
pending monitoring occurrence. A newer due occurrence replaces the older pending one without losing its admission
position. Replaced occurrences count as coalesced; they never ran and do not establish website failure. There is no
catch-up backlog or automatic attempt retry. Equal-job turns do not imply equal CPU time or per-site quotas.

Charts show the latest completed observation, its age, and independent active/pending and delay measurements.
Refreshing a chart does not start or count another check. Observations older than two check intervals become `stale`;
missing or stale duration, test and Lighthouse measurements remain gaps. Outcome counters count each completed attempt
once, including attempts between chart refreshes, and reset on native job restart/replacement. These job diagnostics
continue across vnode changes. Reassignment clears latest measurements, age and last success/failure until a check
submitted for the new association completes. Earlier queued or running attempts keep their captured GUID in history
and cannot repopulate current observations, including after switching back to the original vnode. History retains per-run
evidence under its normal retention and availability policy. Stopping or replacing a job cancels its queued/active work
and joins execution before the replacement starts.

`browser.concurrency` in `dem.conf` limits simultaneous browser attempts in one plugin process. It defaults to 1 and
accepts a positive integer; restart the plugin to apply changes. Monitoring, journey rehearsals and in-process runtime
qualification share this allowance. Ready jobs take turns, and monitoring and rehearsals for the same job cannot overlap.
Waiting time is separate from execution time; cancellation while waiting starts no browser. A fresh browser is created
for each run.

Choose concurrency for the Agent's CPU and memory budget and the accuracy needed from Lighthouse measurements.
The setting is not a hard memory limit or a count of every child process. Concurrent activation checks share one
runtime contract probe per runtime consumer; probes run outside browser slots so a long audit cannot block activation.
A later activation checks the runtime again. Standalone administration commands have their own process budget.

See [DEM performance and sizing](PERFORMANCE.md) for a measured mixed browser/RUM workload, its resource use,
startup behavior and measurement limits.

Browser jobs require Linux amd64 or arm64 and an unprivileged plugin identity. Chromium sandboxing stays enabled;
containers need compatible namespace/seccomp support. RUM does not require a browser runtime.

### Runtime setup and retirement

The independent runtime owns Node, Chromium, Playwright, Lighthouse and their JavaScript adapters. Native Linux
installations use the optional `netdata-dem-runtime` package; static, Docker and source installations use signed
managed bundles or protected preloads. This tree includes the builder, native recipes and an independent release
workflow. Production publication remains gated on confirmed infrastructure, source/license review and platform qualification.

For native installations, explicitly install `netdata-dem-runtime` with the package manager. Its hooks require a live
systemd host and coordinate Agent stop, consumer drainage, qualification and restoration; image/chroot installation
is not supported yet. DEM never invokes the package manager. See the [native package guide](../../../../packaging/dem-runtime/native/README.md).

On hosts with AppArmor, the native package supplies a profile for its root-owned Chromium executable. This permits
the user namespaces Chromium needs for its sandbox; it does not add browser confinement. Package setup loads the
profile before qualification. Administrator policy remains authoritative, and a policy or qualification failure keeps
browser admission closed while restoring eligible non-browser monitoring.

For static/source installations, use the installed `netdata-dem-runtime.sh` helper or kickstart's explicit
`--install-dem-runtime` option. The administrator-run helper prepares the necessary AppArmor launcher allowance,
then runs signed runtime installation and qualification as the configured nonroot service account. For Docker, run
`sudo sh ./kickstart.sh --install-dem-runtime --docker-container NAME` on the local daemon host while the Agent is
running. After setup succeeds, activate the runtime with `docker restart NAME`. No host Agent is required. See
[browser runtime host setup](../../../../packaging/installer/DEM-RUNTIME.md) for online/offline commands, preloads,
custom paths, status and removal.

Ubuntu 24.04 restricts unprivileged user namespaces by default. The generated allowance covers the registered root's
stable browser launchers, including future runtime generations. It enables Chromium's sandbox without adding AppArmor
confinement or exclusive signed-byte eligibility: service-owned files can be replaced, and other unconfined processes
may adopt profiles. Administrator edits and disable markers remain authoritative. Containers still need compatible
namespace/seccomp settings. Do not disable Chromium's sandbox or global host restrictions to make qualification pass.

If qualification reports a Chromium sandbox error, inspect the host's kernel audit messages, for example
`sudo journalctl -k --since '10 minutes ago'`, for AppArmor namespace denials. Correct the administrator-owned policy,
then retry the failed installation or qualification. Successful CLI qualification must still be followed by a check
through the actual Agent service; a terminal and a service can have different security policies.

The low-level managed commands below run as the consuming service account, after any required host policy is prepared
through the helper described above. Enabling a browser monitor does not download the runtime.
A missing installation leaves browser monitors waiting for their dependency; it does not create website failures.
RUM does not need the browser runtime.

```sh
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, use the administrator-configured signed latest release:
dem.plugin runtime install --allow-download
# Disable automatic update preparation for a controlled/offline deployment:
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst --manual-updates
dem.plugin runtime status
dem.plugin runtime qualify
```

Install, update and repair verify the archive, prepare its private paths, then run both a local sandboxed journey and
Lighthouse audit before selecting the release. Online operations fetch the package public keyring from
`https://repository.netdata.cloud/netdatabot.gpg.key`; offline operations require `runtime.offline_keyring` to name
an administrator-provisioned OpenPGP public keyring. `qualify` repeats these checks for the selected release.
It tests the calling account and environment, not a different service's AppArmor, SELinux or container policy.

Ctrl-C or SIGTERM asks the runtime CLI to cancel at the next point where it can stop safely. If qualification
launched a browser, the CLI waits for its supervisor to confirm that the browser and its descendants have exited.
A process stuck in uninterruptible kernel I/O can keep this wait open. Further interrupts do not bypass it.
Forcing the CLI to exit with SIGKILL leaves drainage unverified. Preserve the execution's files and investigate
the stuck process before retrying removal or repair.

**Restart DEM after installation, update or repair, including the first installation.** Each process keeps the runtime it
acquired at startup. A process started without a runtime stays unavailable until restarted. Runtime releases can
update their components without an Agent upgrade while preserving execution contract 2. Runs record the actual
release version, archive digest and execution contract for investigation.

Explicit installation enables automatic preparation by default and prints this policy before proceeding. Use
`--manual-updates` on install or repair to disable it. The running plugin checks roughly daily with jitter, downloads
and qualifies a newer signed release beside the working release, and reports that restart is required. It never
restarts the Agent. Security fixes remain inactive until the administrator or deployment system restarts DEM.
Qualification shares the browser execution allowance, so monitors can wait while these local checks run. Failures preserve
the selected release and appear as update maintenance errors, separately from website measurements.

Signatures authenticate publication order and complete preparation instructions. They cannot detect withheld updates
or guarantee freshness on a new installation. Online trust follows the canonical HTTPS key endpoint and system TLS
trust. A key fetch/verification failure blocks that preparation attempt without stopping the installed runtime.
No stale cached key is used as a fallback; offline keyring updates remain an administrator responsibility.
Normal updates compare with the currently selected sequence; explicit older-release repair resets that baseline.

```sh
dem.plugin runtime update --release /absolute/path/new-release.json --archive /absolute/path/new-runtime.tar.zst
# Stop every DEM/standalone consumer first:
dem.plugin runtime uninstall
```

Uninstall refuses while any generation has a running consumer or unresolved execution. Refusal leaves the installation
intact. If child completion cannot be proved, execution protection remains; neither a timeout nor a missing PID allows
removal. A failed selection write after publication reports uncertainty; inspection/reconciliation establishes the
selected state before cleanup proceeds. A successful update does not replace files used by running processes.
The low-level command removes runtime state only. For a managed runtime with an Agent-linked policy registration, use the
administrator-owned `netdata-dem-runtime.sh uninstall --agent /path/to/netdata` to complete policy cleanup after
drainage. Protected preloads additionally require `--root /absolute/preload/path --deployments-stopped`;
For Docker, use the host retirement command printed at setup. Other explicit policy-only
registrations use `policy-remove --deployment TOKEN --deployments-stopped` on the host.
See [runtime and host-policy removal](../../../../packaging/installer/DEM-RUNTIME.md#status-retry-and-removal) for the retirement steps.

If selection or generation metadata is damaged, `status` still reports the available inventory and identifies the
metadata errors, with a nonzero exit status. A healthy selected release remains identified even when an older release
is damaged. Incomplete ownership or storage inspection is reported as unknown, not as an empty installation.

Stop all DEM and standalone consumers and let their executions drain before repairing the managed store:

```sh
dem.plugin runtime repair --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, explicitly request configured latest with --allow-download.
```

Repair removes damaged generations and rebuilds the requested release, including a same-release reinstall. It uses
the same signed descriptor, service account, store and browser qualification as install. Repair may deliberately
select an older authenticated release; ordinary updates reject lower publication sequences. A healthy selection of a
different release stays selected if repair fails; ordinary retention still applies to other unselected releases.
If repair clears the selection being rebuilt, failed preparation or qualification leaves it unavailable. The old
selection cannot be rolled back; correct the reported problem and retry repair, then restart DEM after success.
The command reports candidate progress separately from the final selected installation.

Repair and uninstall can handle damaged executable metadata only when independent ownership records prove removal
is safe. They refuse active consumers, preparation, unresolved executions, missing safety structure and storage access
errors. There is no force-delete option; dead PIDs and timeouts do not prove execution completion. Do not manually
edit selection records or remove generation directories. Repair applies only to private managed stores; immutable,
administrator-owned preloads must be replaced through their separate lifecycle. Arbitrary external deletion of
individual ownership records or copying a live store is outside this recovery guarantee.

The store defaults to `NETDATA_LIB_DIR/dem/runtime`; override `cache_path` in `dem.conf` or `--cache` for administration.
Use the same installation directory and service account for the CLI and running plugin. `--config-dir` selects an
administrator configuration directory; all explicit paths must be absolute. Normal plugin configuration lookup rules
apply. Terminal sessions ignore configuration-root environment variables, so use `--config-dir` when needed.

```yaml
runtime:
  mode: managed
  cache_path: ''
  max_bytes: 4294967296
  previous_retention_days: 7
  release_url: https://artifacts.example.invalid/dem-runtime/latest/arm64.json
  offline_keyring: /absolute/path/netdatabot.gpg.key  # required only for offline operations
```

The release URL above is an example; use the confirmed publication URL for the host architecture. Online preparation
uses the canonical package-key HTTPS endpoint, including when `--release` is combined with `--allow-download`.
`--release` plus `--archive` is fully offline and uses only `offline_keyring`; this local trust is never an online fallback.
The descriptor and archive cannot supply their own verification key. Explicit offline installation still enables future
automatic online preparation unless `--manual-updates` is selected; protected preloads remain manual. The release URL
and publication infrastructure must be configured before online setup succeeds. Ordinary execution and status do not
fetch a key, and an already installed runtime can continue operating offline.

The selected runtime stays installed until explicitly uninstalled, even with no browser monitors. The 4 GiB logical
allowance includes preparation workspace and reservations. Processes and executions protect older releases. Unprotected
old releases are cleaned hourly, with a seven-day grace for the most recently used obsolete release; storage pressure
can shorten that grace. Protected files can prevent a new installation. This is not a physical disk quota.
`status` reports the selected installation and storage inventory without launching a browser; it does not report the
runtime held by another running process.

The portable archive supplies both the native package payload and managed installations on Linux amd64/arm64.
Native packages own their installation lifecycle; Docker, static and source installations use the managed store or a
preload. Persist the store in Docker to retain installation across container replacement. No host Node/npm/browser
installation is required or discovered. Host sandbox prerequisites still apply to each deployment.

For an immutable image or read-only deployment, create a protected preload at its final path:

```sh
dem.plugin runtime preload --root /opt/netdata-dem-runtime --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
```

```yaml
runtime:
  mode: preloaded
  preload_path: /opt/netdata-dem-runtime
```

The root must be new or empty. The command verifies and qualifies the complete release, then seals its identity and
final location into the root. Keep it private and owned by the consuming service account; it may be mounted read-only.
Preloads are outside managed retirement. Stop **every** consumer before replacement or removal, prepare replacements
at a new final path, and restart DEM to switch. Moving a prepared root is unsupported. Preloads use this explicit
consumer lifecycle; package-owned native runtimes use the coordinated package hooks described above.

Configure exactly one of inline `script` or absolute `script_path` in `dem/journey.conf`. Standard named imports from
`@playwright/test` and destructured CommonJS `require` use the real pinned package. JS/TS, mjs/mts and cjs/cts entries,
relative helpers, hooks, fixtures, assertions and `test.step` are supported. The configured entry is selected explicitly;
neighboring specs/configuration are not discovered. Project aliases and custom package trees are not an initial contract.
Callable `require('@playwright/test')` shorthand and JSX/TSX are unsupported. One Chromium project/worker runs without
retries or repetition; script retry overrides are rejected. Scripts are trusted code running under the plugin identity.

```yaml
jobs:
  - name: checkout
    script: |
      import {test, expect} from '@playwright/test';
      test('checkout loads', async ({page}) => {
        await page.goto('https://shop.example.org/checkout');
        await expect(page.getByRole('heading', {name: 'Checkout'})).toBeVisible();
      });
    screenshot_on_failure: true
    trace_on_failure: false
```

Native secret resolution accepts named `DEM_SECRET_` environment entries through `secrets` in the journey form.
Secrets travel to the admitted runner through private stdin and its controlled environment. Text diagnosis redacts
known secret values and common credential patterns. Screenshots, trace ZIPs and HTML reports may contain sensitive
page data; ZIPs can retain raw resolved credentials. Text redaction does not sanitize artifact content.

A journey succeeds only when all ordinary declared tests actually pass. Unexpected failure or timeout dominates
incomplete coverage. Skipped, expected-failure, empty or incomplete suites are inconclusive unless there is a real
unexpected failure/timeout. Errors loading/running the monitor are execution inability. Cancellation and an interrupted
start are distinct from a failed target. A workflow failure means a declared check failed, not proof of a service-wide outage.

Journey timeouts start when a browser slot is admitted, including runtime startup. The runner reserves up to five
seconds inside the effective budget for Playwright's final diagnostic report (one fifth of budgets below 25 seconds).
An earlier caller deadline shortens that budget. When the work interval expires, the runner requests a graceful
Playwright stop. A cooperative interrupted test can report its locator error and an opted-in screenshot; its actual
status remains `interrupted`, not a fabricated test timeout. Script-specified test timeouts remain supported.
Lighthouse does not use this reporting reserve.

Run detail and run lists expose `termination_reason` independently of the first reported `error`:

| Reason | Meaning |
| --- | --- |
| `execution_budget` | The journey runner exhausted its work interval and requested a graceful stop before receiving the terminal report. |
| `test_timeout` | Playwright reported a timed-out test. |
| `suite_timeout` | Playwright reported a suite timeout. |
| `execution_deadline` | The configured hard execution deadline expired before verified shutdown. |
| `caller_deadline` | An earlier caller deadline expired before verified shutdown. |
| `cancelled` | The caller cancelled, or Playwright reported interruption without a runner budget stop. |

Other outcomes and older history may have no termination reason. A previously reported assertion error can coexist
with a later timeout reason. A terminal report received before the graceful-stop timer is preserved, but shutdown
still must finish within the hard deadline. Hung hooks, blocked workers and forced termination may prevent reporting
or screenshots. Caller cancellation takes precedence and does not publish interrupted-test screenshots; already
reported ordinary failure captures retain their existing policy. Captures are never recovered by scanning output files.

The execution timeout does not bound the entire call: verified process-tree drainage precedes artifact publication and
history completion, which use the existing separate ten-second completion budget. The browser slot remains held until
completion. Cleanup uncertainty preserves work and closes admission; it never authorizes publication.

Use the journey configuration's **Test** action to execute an unsaved draft on the selected Agent with its resolved
secrets and installed browser runtime. Test returns pass/fail; failed, skipped/incomplete, interrupted or unavailable
execution is not success. It does not save the draft, change monitoring observations, publish metrics or alerts, or
retain captures. `screenshot_on_failure` and `trace_on_failure` apply to monitoring only. Test actions against the website are real and
are not rolled back. Activation still checks runtime preparation without executing the workflow.

Test shares the browser allowance with monitoring and may wait for another attempt or delay a later one. The caller's
request deadline covers waiting and execution, so it may expire before the configured workflow timeout. Cancellation
requests process-tree shutdown. If drainage cannot be verified, DEM exits and preserves uncertain work instead of
claiming cleanup. A timed-out caller may not receive the final diagnosis, and there is no retained result to retrieve
later. Existing secret-reference redaction can replace detailed errors with a generic failure.

The static `dem_journey_failed` template warns on the latest fresh failed/timed-out observation. A new successful
observation clears it. Unknown, inconclusive, execution error, cancellation and observations older than two check
intervals are UNDEFINED, not recovery. Replacement/restart begins unknown until a new observation. Critical escalation
and configurable sensitivity are left for a separate alert-policy decision. Native job status identifies unavailable
dependencies; plugin logs explain runtime setup/execution errors. The installation CLI reports preparation and
qualification failures.
Functions show current execution and retained diagnosis.

Lighthouse performance is a desktop lab score in points from 0 to 100; FCP, LCP, TBT and Speed Index are milliseconds,
and CLS is dimensionless. Missing values are gaps/null, distinct from measured zero. Audit completion is separate from
page performance. A failed optional report save marks capture unavailable and retains its diagnostic while preserving
completed audit measurements. These lab measurements are not browser field Web Vitals; no lab-score alert threshold is
enabled.

## Synthetic diagnosis and captures

Process Functions use the regular Netdata transport: `synthetics-checks` for active observations, `synthetics-runs`
for retained history, `synthetics-run` for one run's full diagnosis, `synthetics-artifacts` for retained capture sets and
storage status, and `synthetics-artifact` for one set's metadata or bytes.
Use the advertised `job_id`, `run_id` and other parameters. Disabled/failed-startup jobs remain in native configuration
status. Last success/failure and freshness describe the current activation and vnode association; reassignment resets
these observations. They do not reconstruct a historical incident. A running or
waiting attempt does not refresh its previous result. Active rows also expose check cadence, scheduled/admitted/coalesced
counts and queue/admission delays for the current activation; these diagnostics continue across vnode changes.
Run records preserve selected due and original queue timestamps when produced by the monitor scheduler; older and
immediate diagnostic records may omit them. Run summaries are capped at 2000 rows with an explicit truncation flag; refine the start-time range when truncated. Ranges select the attempt’s actual start using inclusive Unix seconds,
including the entire endpoint seconds. Explicit zero means epoch, negative values are offsets from one captured now, and
omitted bounds cover epoch through now. Run timestamps use Unix microseconds, while reporter event `at_ms` uses Unix
milliseconds.

Start and completion records share the plugin-wide journal budget with RUM. Each completion is self-contained;
a start without a retained completion remains unknown. A selected run includes its latest retained completion even if it
finished or was saved after the range, so outcome filtering follows phase resolution. Completion-only retained evidence
remains queryable by its original start. A positive completion timestamp earlier than start after a wall-clock
correction remains valid; independently measured duration is preserved. Identity drill-down ignores picker bounds and
can show newer active evidence. History errors appear in current diagnosis. Publication is
best effort; ambiguous append failures are never replayed, and history is not a lossless archive.

Basic diagnosis includes bounded test, step, error, stdout and stderr events (500 records, 2000 characters per text),
with dropped-event counts. Stdout/stderr use bounded stream redaction: lines longer than 2000 characters are omitted
with a marker, and forced termination can lose buffered text. Structured test/step/error evidence remains incremental.
Browser error summaries are separately opt-in with `browser_error_summary`; scripts can also forward diagnostics with
Playwright APIs. Optional failure PNG screenshots, trace ZIPs and Lighthouse HTML reports are disabled by default.
Enable `screenshot_on_failure` or `save_report` before an incident; retaining traces requires both
`trace_on_failure` and `screenshot_on_failure`. Forced browser termination can prevent capture. HAR, video and
filmstrip are not supported.

Captures live under `${NETDATA_LIB_DIR}/dem/artifacts` with one shared default allowance of seven days and 2 GiB
for retained capture sets. Each set contains the screenshots, traces or reports published for one run. The newest eligible
capture for each monitor is preferred under size pressure, then older captures are removed first if even those sets
do not fit. Age still expires preferred captures; this does not guarantee one capture per monitor or a complete incident
history. A later Lighthouse report can replace an earlier poor-score report; a journey recovery without a screenshot
leaves its previous failure capture eligible. Disabled or deleted monitors retain the same preference, and renaming a
monitor creates a separate group until its old captures expire.

Active or unverified workspace, unclassified files and pending deletions are reported separately and do not consume the
retained-capture allowance. Total disk use can exceed the allowance, including growth between sweeps; sizes are logical
file lengths, not allocated disk space. `synthetics-artifacts` reports global storage observations, effective policy,
cleanup status and policy reload errors even when `job_id` filters its rows. Its newest-first list accepts an optional
`limit` from 1 to 2000 and a returned `next` cursor as `before`; captures can expire between pages.

Capture discovery and downloads remain available after run history expires and after a monitor is disabled or deleted.
Capture states distinguish disabled, not needed, unavailable and published evidence. Retained history may also outlive
its files; absent recorded files mean “expired or unavailable”. Metadata presence alone does not verify file integrity;
fetch verifies size and digest. Capture metadata requires member-equivalent permissions. Downloads require
administrator-equivalent permissions and are limited to 5 MiB; file content is returned as base64 data. Valid captures initially
larger than 5 MiB are omitted; the remaining files publish atomically without truncation. If all are omitted, capture
state is unavailable. Invalid files, copy growth and I/O failures still fail publication of the whole set.

`synthetics-artifact` adds a kind-derived label and the associated test title for screenshots and traces reported by a supporting
runtime. The association identifies a test, not a page, failing step or exact screenshot time. Test labels remain with
retained captures after run history expires; older or omitted associations show `Test not recorded`. Lighthouse reports
belong to their run and have no test.

`synthetics-run` returns `run.captures` while run history remains: per-test accepted/unaccepted screenshot attachment
counts, candidate-to-test links, reporting status/truncation, and unpublished-candidate reasons. Zero accepted means
no usable screenshot was reported; a positive unaccepted count means screenshot attachments were reported but could
not be used, including when other screenshots succeeded. It does not establish page closure or a screenshot timeout.
On successful publication, Go reports `too_large` for each size omission without adding general run-error text.
On a hard publication failure, it reports `publication_failed` for the set, except an identified size-error offender
remains `too_large`; earlier size skips are not separately preserved. It reports `not_eligible` for an interrupted
candidate rejected by stop policy, and `not_in_final_report` for metadata
without an accepted final candidate. Without a final candidate report, earlier captures are not recovered.
These are historical decisions, separate from current artifact availability. An absent `run.captures` means reporting
was not recorded, not zero captures. Metadata may be partial, unsupported,
invalid or truncated; none means zero captures. Labels use exact-secret redaction before clipping; this does not
sanitize screenshot, trace or report content.

The runner retains admission and private files until Linux supervision proves the entire child tree is reaped.
Unverified completion stops further admission and exits the plugin without deleting shared state. Unexpected supervisor
loss does not prove that survivors are gone or prevent an external service restart beside them. Unverified leftover
work is retained for operator handling; PID/time heuristics never grant permission to delete it. Terminal/debug runs
use private storage; verified completed work is cleaned after joined shutdown, while unverified work remains protected.

### Optional journey failure traces

Enable `trace_on_failure` together with `screenshot_on_failure` to retain native Playwright trace ZIPs for failing
journeys. It is disabled by default. Traces add DOM snapshots, network data and a visual action timeline. They record
on passing attempts too, although those ZIPs are discarded; enabling adds CPU, memory and temporary disk use.
Under DEM's configured defaults, eligible unexpected failures, timeouts and interruptions can retain traces;
expected failures produce no automatic trace, and DEM does not retain traces for unexpected passes.

ZIPs can contain raw passwords from resolved `DEM_SECRET_*` values, cookies, form input, headers and request/response
bodies. Text-summary redaction does not apply to ZIPs. DEM configures dedicated source files and attachments off;
scripts can override this with `test.use({ trace: 'on' })`, invoke tracing APIs or supply their own trace attachments.
Even without overrides, source fragments and credentials can remain. Treat downloaded or shared ZIPs as
credential-bearing data.

The same 5 MiB per-file limit applies: a valid ZIP already over the limit when publication starts is omitted with
`too_large`, and valid screenshots remain. A hard publication error aborts the entire set, including its screenshots.
Timeout capture is best effort; trace export can consume the reporting reserve and lose both the terminal report
and screenshots. Traces do not extend the deadline or trigger retries. Configuration Test does not enable tracing
or publish captures; the script can still direct its own recording.

`synthetics-artifact` labels retained ZIPs `Trace` and includes their test title. Fetch and base64-decode the artifact
bytes, save a `.zip` file, and open it with the standard local Playwright Trace Viewer. Byte access requires existing
administrator-equivalent permissions. `synthetics-run` exposes `run.captures.trace` as `supported`, `unsupported`,
`negotiation_not_reported` or `invalid` when requested. This reports runtime capability, not guaranteed trace retention;
PNG accepted/unaccepted counts keep their screenshot-only meaning.

Downgrading to an Agent without trace support makes mixed capture sets unreadable, including their screenshots.
These sets become unclassified and are excluded from retention cleanup until re-upgrade. Preserved files become
readable again afterward. The shared retention allowance and manifest version remain unchanged.
