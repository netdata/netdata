# Native DEM ownership

## Package boundaries

The root `dem` package composes native collectors, process Function providers, query services and retention.
It does not decode domain journal records, reduce investigation results or serialize Function tables. The command
creates and closes shared resources across framework generations.

| Package | Responsibility |
|---|---|
| `collector/{receiver,rum,journey,lighthouse}` | Native lifecycle, configuration, scheduling callbacks and metric publication |
| `rum/registry` | Admitted sites, exact-generation request/read leases and receiver availability |
| `rum/query` | Copied site observations, live cursors, redaction and active/retained investigation results |
| `rum/history` | RUM record encoding, retained-event reducers and the per-site queued writer |
| `rum/functions` | Function declarations, arguments, permissions, columns and response serialization |
| `synthetic` | Shared request, run, event and artifact values plus request validation |
| `synthetic/registry` | Copied active-job observations, generation fencing and freshness |
| `synthetic/query` | Active-first run lookup, retained runs and recorded-artifact membership/availability |
| `synthetic/history` | Synthetic record encoding and run reduction |
| `synthetic/functions` | Function declarations, arguments, permissions, columns and response serialization |
| `synthetic/runner` | Prepared browser execution, admission, process supervision and private reporter protocol |
| `synthetic/runner/assets` | Immutable Node adapters and pinned dependency manifests |
| `synthetic/artifacts` | Work directories, verified publication, immutable captures, reads and retention |
| `journal` | One shared journal chain, SDK admission, independent snapshots, retention and close |
| `internal/redact` | DEM text and credential redaction |
| `internal/attemptmetrics` | Shared synthetic outcome and duration instruments |

RUM processing is separate from investigation queries and Function presentation:

| Package | Responsibility |
|---|---|
| `rum/beacon` | Normalized browser observations, event kinds and pure sampling and normalization rules |
| `rum/config` | Receiver/site declarations, validation and effective capture and sampling policy |
| `rum/faro` | Faro wire decoding, spans, event classification and pinned bootstrap rendering/assets |
| `rum/httpapi` | HTTP endpoints, exact route leases, origin/proxy/body/rate policy, caching and demo responses |
| `rum/diagnostics` | Last rejected-origin observation owned by one site runtime |
| `rum/aggregate` | One site's rolling measurements and investigation state, behind one lock |
| `rum/otlp` | RUM log/span mapping, export queues, transport and drainage |
| `rum/geoip` | Receiver-owned MMDB reader and RUM location policy |

The top-level `config/` directory holds installed configuration files, not a Go package. Configuration policy does
not read the hostname or generate UI prose; diagnostics derives fallback addresses and Functions owns presentation.

Function adapters depend on query results and shared domain values. Query services combine registry snapshots,
domain history and artifact reads; they do not import Function adapters or acquire browser execution admission.
The history adapters borrow one `journal.Store`; they cannot close it or independently apply retention. Journal
knows SDK fields and snapshots, not RUM or synthetic schemas. Shared Go interfaces live with their consumers;
pure synthetic values do not define executor, persistence or transport interfaces.

Aggregate files separate state/construction, ingestion/results, activity, snapshots/ranking, vital statistics and
cohesive topic operations without creating extra state owners. HTTP never imports the registry or collectors; its
small Processor and Routes interfaces describe its consumers. Diagnostics does not depend on HTTP. Faro is a pure
protocol adapter; its embedded bootstrap template requires no frontend build.

Files follow these boundaries within packages: Function methods keep their columns beside their row builders;
history separates record codecs from reducers; the runner separates configuration, preparation, execution, process
ownership and reporter decoding; artifact files separate work creation, publication, reads, retention and filesystem
checks. Pure DEM helpers stay under their domain or `internal/`; moving code to repository `pkg/` requires a real
shared contract and consumers outside DEM.

## Resource and runtime ownership

The command owns the investigation journal and the plugin-wide retention service. It closes it only after `agenthost.Result` reports
`Err == nil` and `ExitRequired == false`. An error or recovery requiring process exit may leave consumers alive; process
exit owns their handles. Process-service finalizers run before job retirement and cannot close resources used by jobs.

The framework owns desired configuration, preflight, scheduling, admission, retries, status and DynCfg. The constructor
injects domain registries into collectors and query services into process Function providers. Providers construct
fresh handlers for each contained framework run generation and remain available independently of collector selection. The RUM registry contains actual admitted
site registrations and receiver availability. It has no desired config mirror, scheduler or job status.

Receiver `Init` / `Check` validate and prepare without binding. `Run` binds the listener, publishes availability and
signals readiness. Each site initializes its immutable policy, then independently admits its aggregator, route, redactor
and runtime generation. Site readiness does not depend on receiver readiness. Receiver replacement preserves site
aggregation and pending investigation/export queues.

Each package in `collector/` is a registered native collector. Its lifecycle methods show acquisition, readiness,
collection and retirement; its metric definitions show the measurements it publishes. Fixed instruments are prepared
once, with site labels bound during initialization. Dynamic breakdown handles are not retained in an unbounded cache.

One RUM job constructs one aggregator, diagnostic state, route and history writer. It also constructs an event-log
exporter when event_logs is enabled and a browser-trace exporter when tracing is enabled. Both default to disabled.
Each enabled exporter owns its complete destination, gRPC connection, bounded queue and worker; disabled signals
create none of these resources. The destinations have identical security options and do not inherit from each other.
These objects have no site inventory or reconfiguration API. The registry admits references to those exact owners;
it does not construct them. The route records address/rejection observations in the separate diagnostic state.
Aggregation and each exporter reject an observation for another site before changing state.

Each site owns immutable capture policy. Config materializes country-only geolocation and disabled frustration
heuristics without mutating the stored input. Browser bootstrap limits instrumentation and outgoing metadata; the
receiver independently enforces the same policy and normalizes supported URLs, paths and text before aggregation,
retained history or enabled exports. Geolocation off skips lookup while HTTP IP admission remains active. Country mode
retains only country; city mode adds approximate city/coordinates to live observations only. Journals and OTLP never
receive city coordinates. Disabled frustration has no instruments or page counts. Application-provided user IDs use
bounded text normalization without path-style numeric/UUID substitution; per-event attribution and retained observed-ID
membership are distinct from display summaries.
These targeted transformations do not constitute general privacy or consent enforcement.

Faro's `beforeSend` runs before its session sampling hook. Browser shaping clones metadata and preserves the
`isSampled` session attribute until the SDK consumes it; removing it early silently drops telemetry. Session lifecycle
and `view_changed` envelopes carry metadata even on quiet pages, so they stay on the wire. Native history suppresses
generic session lifecycle rows and uses explicit view events to preserve transitions between identically normalized
routes. The pinned-SDK capture and sampling fixtures exercise the real metadata providers and transport hooks,
including selected/excluded session rollover; SDK upgrades MUST verify this ordering against the new bundle.
The decoder targets the pinned SDK's measurement `values` map. Legacy experimental scalar `value.duration` and
`value.value` payloads are unsupported; no compatibility adapter is retained.

The collector's processor calls aggregation and then each enabled exporter directly. Aggregation leaves the normalized Beacon
unchanged and returns acceptance/detail decisions plus a filtered observation containing only newly eligible reports.
Exporters consume that filtered observation, including when it is empty; they MUST NOT fall back to the input batch.
Eligible measurements and live rows are independent of detail selection; investigation sampling controls retained history and OTLP export. Config owns
omitted/null defaults; the aggregator receives literal effective rates, including zero. Receiver collection zero
acknowledges otherwise admissible requests before decode, geo lookup or processing; bootstrap zero avoids SDK startup.
History promotion replays preceding bounded context with each event's original attribution, then emits current
entries independently of the live ring cap. Poor-vital entries do not require element attribution. OTLP exports only
newly selected observations; prior exports and asynchronously arriving spans are not reconstructed. Faro classifies protocol event
names into domain kinds without changing their original names for presentation. HTTP rejection accounting reaches
aggregation only. History enqueue remains nonblocking under the aggregate lock and does not call back into it.

The history writer and each enabled exporter account for their own queues. The trace exporter preserves each
beacon's trace resource attributes. Shared receiver/site policy and destination declarations belong to `rum/config`;
`rum/otlp` implements transport security, signal mapping and drainage. Native `Name` identifies the site;
`DisplayName` is presentation metadata. Runtime export failure does not stop native aggregation or history.

An HTTP request acquires one exact site registration containing both policy and its processor. Bootstrap, preflight, demo and
beacon endpoints use this same admission. Retirement removes future admission, cancels admitted request contexts and
joins their leases. Cancellation interrupts socket reads through a response-controller deadline before closing bodies;
closing a net/http request body alone can wait behind a stalled read. Cancellation callbacks join before leases release.
Concurrent body reads can complete out of receipt-time order. Rolling-window reads filter every expired observation;
session activity and deduplication keep the newest receipt time without rewriting event timestamps.

`rum/aggregate` owns one canonical latest observation per `(experience, vital)` and separate bounded activation,
view/resource identity, activity and observed-session records. Resource observations and replay evidence have separate
retention from document/vital activity, so resource pressure cannot invalidate unrelated measurements. Facet distributions
and top-N complements derive from canonical populations at read time; host/error grouping is bounded by those retained
observations and folded only after ranking. Receipt-ordered retention costs O(log retained) per changed entry without
scanning retained measurements during ingestion. Snapshots scan bounded state and sort populations for percentiles and
ranking; the Sites summary scans only its required activity and session populations.
Measurement retention is independent of the investigation-session LRU and live ring. Pages reads expose all retained
page groups rather than chart top-N only. Capacity loss conservatively invalidates affected results and remains visible
until the newest relevant loss expires. This constant-space loss episode may include older loss during continuous
overload; it is not an exact window-loss count. The drop counter remains cumulative actual loss. Identity loss is scoped
to affected measurements; resource identity loss still invalidates session completeness because resource-only reports
can renew session activity. Unidentified errors, logs, spans and ordinary custom events have no replay suppression.

Faro page metadata captures a frozen activation ID and document-entry URL before queueing. A persisted pageshow starts
a new activation before SDK BFCache reports are generated. Custom metadata providers retain SDK browser/OS/sdk fields;
raw setPage cannot replace the frozen document scope. The FetchTransport subclass changes only its ignore-URL
matcher to an escaped, anchored collector URL; inherited delivery/batching/retry behavior stays intact. Raw URL
strings become regular expressions in the pinned SDK and otherwise mis-handle bot query strings or prefix matches. The WebVitals instrumentation wrapper attaches increasing
revisions in measurement context, never changing metadata for every measurement: Faro groups by metadata, so per-item
metadata would destroy default batching. Explicit document/view/session lifecycle events and performance entries receive identities
before transport; retries preserve identical IDs/revisions. Identity evidence lasts at most 30 minutes subject to
capacity and restart. The SDK has no finite maximum delivery delay, so this is best-effort replay suppression.

Filtered vital reports carry `Vital.Origin` with frozen page/browser/device/country/version dimensions. User identity,
view label and view occurrence remain per-report attributes. These MUST NOT be frozen at the first metric report,
because login/logout and SPA transitions can occur before later revisions. History promotion retains each report's
original attributes. Native live/history and enabled exports preserve the same experience/metric/revision identity.

Collector gauges expose current retained population counts and loss evidence on every eligible collection cycle.
Percentile gauges are omitted for empty or incomplete populations; measurement gauges are omitted when collection is
zero or ingress unavailable. Top-N window counts are gauges, not cumulative counters with changing membership.
Dynamic series use ephemeral metrix handles and a separate bucket label for pooled Other. Stock quality alerts read
coherent fresh population dimensions directly, require 30 observations, and compare threshold counts without averaging
percentiles. Missing/stale/partial populations yield undefined, not a synthetic healthy zero.
Live queries merge per-site stream heads by receipt time while preserving each stream's sequence prefix; concurrent receipt and ingestion order can differ.
All-site Errors, Pages and Sessions use a separate site-scoped row key without replacing their filter values.
Function reads lease domain state only while copying snapshots, and historical queries own independent journal snapshots and check caller cancellation between files and rows.

A retiring site joins its HTTP and Function readers before cancelling the history/export worker context. Export queues
drain using one shared fixed shutdown budget, and accepted history events flush under one detached five-second
admission budget. Every event is self-contained: there are no session/group parent records or cumulative deltas.
A cancelled admission leaves only unattempted entries pending for final drain; attempted appends are never replayed,
including when an error or cancellation races them. Appends, sync, snapshot construction and retention serialize SDK
Log access; the SDK disk operation itself is not interruptible. A non-quiescent exit uses the existing host fail-stop
boundary. Snapshot scans release writer admission and copy SDK payloads before their borrowed lifetime ends.

History selects original Agent receipt (`ObservedUS` / `DEM_OBSERVED_US`) for RUM and immutable attempt start
(`DEM_STARTED_US`) for synthetics. Inclusive whole-second ranges are checked against decoded microsecond clocks after
index selection, without overflowing second-to-microsecond multiplication. RUM counts and spans describe selected
retained activity; user-ID membership filters summaries after aggregation. Session order uses the exposed
`last_observed_us` before limiting. Identity timelines use session/run postings across all retained time, then verify
domain and site/job identity; RUM preserves its occurrence-aware pending/retained union.

The shared append boundary derives `DEM_SCHEMA=1` and one canonical decimal minute field: `DEM_RUM_MINUTE` or
`DEM_SYNTHETIC_MINUTE`, computed as floor(domain microseconds / 60,000,000). Callers cannot supply envelope metadata.
Domain decoders check required scalar multiplicity, identity and timestamp agreement. Each snapshot captures both bucket
FIELD heads and schema posting counts while admission excludes append/rotation/retention; current schema must cover
every committed entry. Opening owns bounded SDK buffers rather than a vector of all ENTRY offsets. Exact minute lookups
serve narrow ranges; broader ranges enumerate existing minute values. The crossover changes cost only. No saved-time
seek, file pruning, candidate cap or hidden full-scan fallback is used. Payload views end with their callback; reducers
retain only domain summaries or requested timeline output.

Cost follows retained file count at capture, native bucket/hash traversal plus selected postings during selection, and
selected records/distinct summary keys during reduction. Broad queries still decode all selected evidence. Error
fingerprint details additionally retain distinct session/page/browser sets. Saved-time retention stays independent of
these query clocks.
Query failures return errors instead of silently labeling corrupt or quarantined history as complete. Damaged
`.journal~` files remain for operator investigation and are outside the SDK retention policy.

Recovery validates every preexisting `dem.journal` with the SDK strict index verifier before `NewLog` can mutate
history. `StrictSystemdNaming` and default archive sync separate active files from `dem@…journal` finalized archives.
Nonempty archives require archived header state and complete current-schema coverage; empty archives require strict
verification because they have no encoder marker. Under the supported single-owner lifecycle, these finalized archives
avoid a whole-graph startup scan. A schema marker alone does not certify arbitrary external modifications. Verification
uses graph-sized memory and fails closed without repair/deletion. An SDK uncertain-write sentinel poisons the shared
store: new reads, writes, sync and retention return the failure; SDK cleanup releases resources without rewriting clean
metadata. Private temporary history is also preserved on failure. Measurements/exports remain independent; existing
startup failure stops the plugin when retained history cannot be opened.

One process worker archives idle history, reopens lazily and applies the plugin-wide age/committed-byte policy even
when all sites are disabled. Failed sweeps retry after five seconds; the store owns the reopened lazy Log before
enforcing retention, and the next append creates its active file. Whole archives expire; the active file and filesystem
preallocation can exceed the configured committed-byte target. Neither retention nor history scans hold a live site lease across disk work. Publication uses
typed metrix snapshots and static chart templates, with no V1 map bridge.

Stock native health templates own alert policy and attach independently to each site's charts. Site jobs publish
measurements only; they do not write health configuration, invoke health reload or recover generated files. The command
creates its history directory as the service account under the existing writable Agent state root.

Setup Functions use explicit site public_url, then receiver public_url, or return no install URL. They do not derive
addresses from listeners/headers, contact websites or parse CSP. The browser derives sibling asset/collect URLs from
its synchronously captured external script element src, preserving proxy prefixes; redirects do not change that src.
The native Document getter obtains the executing script without trusting shadowing named HTML elements.
Faro's exact published bundles and consolidated notices are embedded, content-identified and served as public assets.
The policy bootstrap stays private/no-cache with a body-derived ETag; it changes when effective browser policy changes.

Site runtime generation owns receipt/rejection evidence, not browser policy freshness: older pages can report into a
replacement runtime. The Function publishes generation, collection policy and timestamped facts without inferring an
installed/healthy state. Last accepted time reuses the aggregator's existing receipt state; origin rejection remains
one observation including failed preflight. The receiver's bound Listen address is internal state, not a published
diagnostic or installation URL. There is no outbound setup probe worker.

Receiver and OTLP TLS preparation use the shared context-aware TLS helper, so native preflight cancellation reaches
credential-file reads. Site redaction covers stored/exported payloads and remote OTLP diagnostic messages.

Journey and Lighthouse each own their prepared request, metric store, product instruments and explicit execution flow.
They share an executor, pure request validation and the common outcome/duration instrument helper. `Run` registers one
observation generation before readiness; `Collect` executes through that exact registration and records terminal
outcomes. Retirement fences late callbacks without replacing the collector's registration pointer. Ordinary execution
errors remain measured outcomes; unverified drainage notifies an active `Run`, while the independent engine fatal signal
stops the process even if cancellation already finished `Run`. Framework scheduling owns cadence; the command owns
the execution admission slot, artifact store and journal. Process Functions are registered through separate process
providers and published independently of collector selection. Their narrow source copies active snapshots before
independent history I/O; no Function acquires browser admission.

The real Node adapter selects exactly one configured entry with explicit config/tsconfig and a private reporter pipe.
Standard import mappings target real pinned package entries, never a fake test harness. Retry overrides fail before a
worker starts. Node terminal evidence and Linux process-tree completion are distinct. Go joins stdin/stdout/stderr
owners after verified supervision; false Drained poisons admission and wakes command fail-stop before attempting reader
joins. The command cannot close stores when the host returns forced recovery or the engine reports unverified cleanup.

Run starts/completions are immutable self-contained journal records carrying the same scalar/JSON start clock.
Start-time selection includes both phases even when completion is saved outside the picker. Completion wins over start,
including positive wall-clock rollback; an unmatched start remains unknown. A retained completion does not need its
start record. Outcome filtering follows grouping. Identity detail retains active-first diagnosis. Summary reduction is
O(selected runs) memory and stores no full timelines; SDK snapshots use bounded buffers per retained file.
Current synthetic registry registrations own copied generation observations and reset unknown on replacement. Freshness is two native
intervals and remains independent of an in-flight attempt; history does not drive current incidents.

Chromium receives a short `/tmp/nd-dem-<run-id>` alias for its private work directory because its profile singleton uses
Unix sockets with a small pathname limit. The artifact owner never adopts an existing alias and removes it only after
verified drainage and an exact target check.

Artifact publication starts only after verified tree/reader completion. A Go-owned synced drained marker grants cleanup
permission; work without it remains protected across restart. Captures are constrained beneath output and copied into
synced immutable manifests with actual size/digest. Final publication, fetch, retention and close serialize at the store.
Failed publication remains protected for the current owner; a later owner reconciles only marked drained leftovers.
Expiry removes complete finalized directories without everlasting tombstones. Journal metadata missing its backing
file is reported as expired_or_unavailable. Filesystem traversal/retention is linear in files plus finalized-run sorting;
no metrics collection path scans artifact bytes.
