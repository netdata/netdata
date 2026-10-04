# DEM plugin

`dem.plugin` runs native Netdata jobs for browser real user monitoring, Playwright journeys and desktop Lighthouse audits. It is experimental and built explicitly with
`ENABLE_PLUGIN_DEM=ON` (source installer: `--enable-plugin-dem`). The integration metadata beside each collector is the
source of operator configuration documentation.

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
parent record. `rum-sessions` reports sessions with activity saved in the selected range; page views, errors, frustration
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

Stock health templates provide LCP, INP and CLS alerts using the average p75 over 10 minutes, plus a no-beacons alert
using page views over one hour. They apply independently to each native site chart. Customize thresholds or disable
alerts through normal Netdata health configuration, using `_collect_plugin=dem` and `_collect_job=<site>` chart-label
filters for site-specific policies. The plugin does not generate health files or reload Agent health configuration.
Terminal/debug runs use private temporary journal and identity state, removed after readers and workers join; they leave
live state untouched. An existing experimental `history.db` is not imported or deleted.

The browser normalization, bounded aggregation, OTLP and query behavior originated in the experimental
[Netdata digital-experience POC](https://github.com/netdata/digital-experience), commit
`b3de4662f567dc63d33fee6201f8c2313568301b`. Its private configuration controller, scheduler, protocol emitter and root
state reconciliation are replaced by the native Agent framework. Embedded third-party asset notices remain beside their source.

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
