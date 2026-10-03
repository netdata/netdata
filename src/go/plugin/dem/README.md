# DEM plugin

`dem.plugin` runs native Netdata jobs for browser real user monitoring. It is experimental and built explicitly with
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
