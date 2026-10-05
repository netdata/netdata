# DEM plugin

`dem.plugin` runs native Netdata jobs for browser real user monitoring, Playwright journeys and desktop Lighthouse audits. It is experimental and built explicitly with
`ENABLE_PLUGIN_DEM=ON` (source installer: `--enable-plugin-dem`). The integration metadata beside each collector is the
source of operator configuration documentation. The [ownership guide](ARCHITECTURE.md) maps packages and runtime lifetimes.

The stock configuration starts one loopback receiver and no sites. Configuration follows the normal Go Agent layout:
`dem.conf` for plugin policy and `dem/receiver.conf` / `dem/rum.conf` for collectors. The receiver is a canonical single
object (`dem:collector:receiver`); each site is an independent job (`dem:collector:rum:<name>`). Native `name` is the
stable beacon/history key; `display_name` is presentation metadata. Renaming a site starts a new history namespace.

Expose the receiver through HTTPS, configure its public URL and actual trusted proxy addresses, then install
`<script async src="https://rum.example.org/rum/shop.js"></script>` on the configured website. Public URLs may include a
reverse-proxy path prefix. The bootstrap pins Faro 2.11.0 and loads the SDK from jsDelivr; the website's Content Security
Policy must allow the SDK and receiver. Client IP addresses are used for rate limiting/geolocation and are not stored.

RUM and synthetic Function providers are independent of collector selection. Disabling `rum` or `journey`, selecting
only `lighthouse`, or disabling all collectors preserves investigation of retained history while the plugin is enabled.
Setting `enabled: no` disables the whole plugin. Handler instances belong to each framework run generation; the command
keeps shared stores open until a clean shutdown.

Six process Functions use the regular Netdata Function transport: `rum-sites`, `rum-pages`, `rum-live`, `rum-sessions`,
`rum-errors`, and `rum-session-events`. Runtime observations include only admitted site jobs. Consumers join native
DynCfg state to show disabled or failed configurations. Journal history remains queryable after a site stops. History range
arguments filter when records were saved, using Unix seconds (negative values are relative offsets). A delayed session
promotion can save older events in the current range. Timelines display original observation time in Unix microseconds.

`rum-live` returns an opaque string `next` cursor, passed unchanged as `after` on the next request. Each event has a
string identity combining site, runtime generation and sequence. Cursors track independent streams so a busy site cannot
advance a quiet site's position. A replaced site's runtime starts a fresh stream. Missing Web Vitals are `null` in
Functions and gaps in charts; an available but quiet receiver measures zero traffic. Receiver outages leave site jobs
running with explicit ingress availability and gaps in browser product measurements.

Investigation history uses pure-Go journal files under `${NETDATA_LIB_DIR}/dem/journal`, through the existing
`systemd-journal-sdk`. Events include their session and error metadata, so investigation does not depend on a separate
parent record. `rum-sessions` reports sessions with activity saved in the selected range; document views, application views, errors, frustration
counts and activity spans describe retained events in that range, rather than lifetime totals. `rum-errors` groups
retained occurrences by fingerprint; affected-session, top-page and browser statistics are `null` until a `fingerprint`
filter selects the group to investigate. `rum-session-events` returns the full retained and pending timeline for a
session, ordered by original observation time, without range filtering.

One process-owned retention worker applies the whole-plugin `history.days` and `history.max_bytes` policy, including
disabled sites. It removes oldest archived files by their saved-time head and committed journal bytes; it protects the active
file. Filesystem preallocation can use more disk space than committed bytes. These are whole-file retention targets, not exact event TTLs or a hard instantaneous disk ceiling. The worker
archives idle activity without creating an empty replacement file and applies policy changes at the next hourly sweep;
invalid changes retain the last valid policy and produce a warning. Queued events flush every five seconds or 500 records.
Flushes sync pending appends and retry failed syncs; clean idle ticks skip sync. A crash can lose records still queued or
not synced. Queue overflow and append failures count dropped records; sync failures produce a warning. Failed retention
close/reopen retries after five seconds so a transient filesystem error does not suspend history writes until the next
hourly sweep. This is sampled investigation history, not a lossless event archive.

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
live state untouched. An existing experimental `history.db` is not imported or deleted.

The browser normalization, bounded aggregation, OTLP and query behavior originated in the experimental
[Netdata digital-experience POC](https://github.com/netdata/digital-experience), commit
`b3de4662f567dc63d33fee6201f8c2313568301b`. Its private configuration controller, scheduler, protocol emitter and root
state reconciliation are replaced by the native Agent framework. Embedded third-party asset notices remain beside their source.

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
that metric's presence in the window; a duplicate does not. Duplicate detection retains evidence for up to 30 minutes,
subject to capacity and runtime restart. Identified SDK session lifecycle events follow the same rule, so replay does
not renew observed-session windows. It is best effort, not exactly-once delivery. Reports arriving after retained
evidence expires can be counted again. Late observations are not rejected merely to claim perfect deduplication.
Errors, console logs, spans and ordinary custom events are not covered by this identity-based replay suppression;
repeated delivery of these records can count again and refresh observed-session activity.

Population charts show retained observations, good/needs-improvement/poor counts and capacity-loss evidence. Percentiles
are unavailable when there are no observations or the local population is incomplete. A zero CLS or zero timing is a
valid observation. Lost-report fields conservatively remain nonzero until the latest relevant loss leaves the window;
they can include older loss from a continuous overload episode. They are neither exact missing-window counts nor part
of the percentile denominator. The cumulative drop diagnostic counts actual discarded reports. Restart starts fresh
in-memory windows; retained investigation history is independent.

`rum.observed_sessions` counts distinct browser session identities seen in the receipt window. It does not estimate
people, concurrency or time on site. Page session counts use the same window. JavaScript errors are events: several can
occur in one document, so errors divided by document views is not a failure percentage. Missing identity excludes the
corresponding measurement and increments an invalid-measurement diagnostic; no transport-gap fallback invents views.

Breakdown traffic counts and vital populations use the same current window. Top groups and the pooled complement are
computed from current observations. `bucket=value` distinguishes a literal label named `other` from `bucket=other`.
The Pages Function exposes all retained document-entry groups, including groups outside the chart top list; loss
fields reveal bounded-state truncation. Percentiles cannot be averaged across time windows, groups or sites to obtain a
combined percentile.

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
logout and user changes. The `user_id` filter matches exact stored, normalized IDs within the selected saved-time range.
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
performance lab audit. Both use native job configuration and scheduling. Stock configuration enables no browser jobs.
Journey defaults to 900 seconds between attempts and a 120-second admitted timeout; Lighthouse defaults to 1800/180 seconds.
One plugin-wide admission slot serializes expensive runs. Waiting time is separate from execution time; cancellation
while waiting starts no browser. A fresh browser is created for each run.

Browser jobs currently require Linux amd64 or arm64 and an unprivileged plugin identity. Prepare Node 24 or newer,
full Chromium, its system libraries and compatible sandbox support. Chromium sandboxing stays enabled; setup refusal
appears as execution inability. Containers need namespace/seccomp support for the sandbox. The plugin does not install
packages or download browsers. Restart the plugin after changing `runtime` paths in `dem.conf`.

The installed `plugins.d/dem/package.json` and `package-lock.json` pin `@playwright/test`/Playwright 1.63.0,
Lighthouse 13.4.1 and Chrome Launcher 1.2.1. Copy both files into a prepared dependency directory, then run `npm ci
--omit=dev --ignore-scripts` there with the compatible Node version. Prepare the full Chromium revision 1243
(version 153.0.8010.12) from that Playwright release; Lighthouse needs the full browser rather than only its headless
shell. The pinned CLI supports `node node_modules/playwright/cli.js install --no-shell chromium` for operator-managed
installation. Install system libraries using the release's documented platform prerequisites. Make the dependency and
browser files readable/executable by the service account, then set absolute paths:

```yaml
runtime:
  node_path: /usr/bin/node
  dependencies_path: /opt/netdata-dem
  browser_path: /opt/netdata-dem/browsers/chromium-1243/chrome-linux64/chrome
```

The browser executable's directory varies by architecture and installation location; use the actual full Chromium
path. Do not configure the headless-shell executable. Check validates preparation without executing a workflow.
Prepared packages and installed runner assets are read-only during execution; generated config, caches, profiles and
output are confined to a private run directory.

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
```

Native secret resolution accepts named `DEM_SECRET_` environment entries through `secrets` in the journey form.
Secrets travel to the admitted runner through private stdin and its controlled environment. Text diagnosis redacts
known secret values and common credential patterns. Screenshots and HTML reports may contain sensitive page data;
there is no universal image/report sanitization guarantee.

A journey succeeds only when all ordinary declared tests actually pass. Unexpected failure or timeout dominates
incomplete coverage. Skipped, expected-failure, empty or incomplete suites are inconclusive unless there is a real
unexpected failure/timeout. Errors loading/running the monitor are execution inability. Cancellation and an interrupted
start are distinct from a failed target. A workflow failure means a declared check failed, not proof of a service-wide outage.

The static `dem_journey_failed` template warns on the latest fresh failed/timed-out observation. A new successful
observation clears it. Unknown, inconclusive, execution error, cancellation and observations older than two native
intervals are UNDEFINED, not recovery. Replacement/restart begins unknown until a new observation. Critical escalation
and configurable sensitivity are left for a separate alert-policy decision. Native job status shows preparation errors;
Functions show current execution and retained diagnosis.

Lighthouse performance is a desktop lab score in points from 0 to 100; FCP, LCP, TBT and Speed Index are milliseconds,
and CLS is dimensionless. Missing values are gaps/null, distinct from measured zero. Audit completion is separate from
page performance. A failed optional report save marks capture unavailable and retains its diagnostic while preserving completed audit measurements. These lab measurements are not browser field Web Vitals; no lab-score alert threshold is enabled.

## Synthetic diagnosis and captures

Process Functions use the regular Netdata transport: `synthetics-checks` for active observations, `synthetics-runs`
for retained history, `synthetics-run` for one run's full diagnosis, and `synthetics-artifact` for capture metadata or bytes.
Use the advertised `job_id`, `run_id` and other parameters. Disabled/failed-startup jobs remain in native configuration
status. Current-generation last success/failure and freshness do not reconstruct a historical incident. A running or
waiting attempt does not refresh its previous result. Run summaries are capped at 2000 rows with an explicit truncation
flag; refine the saved-time range when truncated. Saved-time range arguments are Unix seconds; observation timestamps
use Unix microseconds, while reporter event `at_ms` uses Unix milliseconds.

Start and completion records share the plugin-wide journal budget with RUM. Each completion is self-contained;
a start without a retained completion remains unknown. History errors appear in current diagnosis. Publication is
best effort; ambiguous append failures are never replayed, and history is not a lossless archive.

Basic diagnosis includes bounded test, step, error, stdout and stderr events (500 records, 2000 characters per text),
with dropped-event counts. Stdout/stderr use bounded stream redaction: lines longer than 2000 characters are omitted with a marker, and forced termination can lose buffered text. Structured test/step/error evidence remains incremental. There is no automatic page-console/network capture; scripts can forward diagnostics with
Playwright APIs. Optional failure PNG screenshots and Lighthouse HTML reports are disabled by default. Enable
`screenshot_on_failure` or `save_report` before an incident if these captures are needed. Forced browser termination
can prevent capture. Trace, HAR, video and filmstrip are not initial capabilities.

Captures live under `${NETDATA_LIB_DIR}/dem/artifacts` with a separate plugin-wide default of up to seven days subject
to 2 GiB. Oldest finalized runs expire together; active or unverified work is protected, so this is a soft disk target.
The inventory exposes retained/protected bytes and cleanup errors. Capture states distinguish disabled, not needed,
unavailable and published evidence. Retained history may outlive its files; absent recorded files mean “expired or
unavailable”. Metadata presence alone does not verify file availability; fetch verifies the manifest, size and digest.
Captures exceeding 5 MiB are marked unavailable and are not published.
Downloads require administrator-equivalent caller permissions and are limited to 5 MiB; HTML is returned as base64 data.

The runner retains admission and private files until Linux supervision proves the entire child tree is reaped.
Unverified completion stops further admission and exits the plugin without deleting shared state. Unexpected supervisor
loss does not prove that survivors are gone or prevent an external service restart beside them. Unverified leftover
work is retained for operator handling; PID/time heuristics never grant permission to delete it. Terminal/debug runs
use private storage; verified completed work is cleaned after joined shutdown, while unverified work remains protected.
