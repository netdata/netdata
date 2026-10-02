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
DynCfg state to show disabled or failed configurations. SQL history remains queryable after a site stops. History range
arguments use Unix seconds (negative values are relative offsets); event timestamps use Unix microseconds.

`rum-live` returns an opaque string `next` cursor, passed unchanged as `after` on the next request. Each event has a
string identity combining site, runtime generation and sequence. Cursors track independent streams so a busy site cannot
advance a quiet site's position. A replaced site's runtime starts a fresh stream. Missing Web Vitals are `null` in
Functions and gaps in charts; an available but quiet receiver measures zero traffic. Receiver outages leave site jobs
running with explicit ingress availability and gaps in browser product measurements.

Investigation history lives in `${NETDATA_LIB_DIR}/dem/history.db`. One process-owned retention worker enforces the
whole-plugin `history.days` and approximate stored-byte `history.max_bytes` budget, including disabled sites. Policy
changes apply at the next hourly sweep; invalid changes retain the last valid policy and produce a warning. Queue or SQL
failures count dropped records. This is sampled investigation history, not a lossless event archive.

Site alert policies generate only their own `rum.<name>.conf` in `${NETDATA_LIB_DIR}/dem/health.d`. The installer creates
`health.d/dem-generated` pointing to that writable directory because the normal service cannot write under `/etc`.
If the Agent's health configuration or state directory is customized, arrange the corresponding link in the effective
health configuration directory. The `rum.*.conf` namespace in this directory is reserved for generated files. Startup
recovers abandoned generated regular files before jobs start, preserving symlinks, directories and unrelated files.
Native jobs atomically write their policy and remove only their exact file on retirement; bounded health reload failures
are reported and retried. Terminal/debug runs use an in-memory database and do not write health configuration or reload
the running Agent.

The browser normalization, bounded aggregation, OTLP and query behavior originated in the experimental
[Netdata digital-experience POC](https://github.com/netdata/digital-experience), commit
`b3de4662f567dc63d33fee6201f8c2313568301b`. Its private configuration controller, scheduler, protocol emitter and root
state reconciliation are replaced by the native Agent framework. SQLite uses the pure-Go `modernc.org/sqlite` driver;
embedded third-party asset notices remain beside their source.
