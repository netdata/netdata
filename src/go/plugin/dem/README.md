# DEM plugin

`dem.plugin` (Digital Experience Monitoring) measures how websites behave for the people using them. It runs inside
the Netdata Agent and offers two kinds of monitoring:

- **Real user monitoring (RUM).** Visitors' browsers send performance reports (Web Vitals, page and resource timings,
  errors) to a receiver built into the plugin.
- **Synthetic monitoring.** The plugin runs scripted Playwright journeys and desktop Lighthouse audits on a schedule,
  with its own browser runtime.

Each kind is a set of ordinary Netdata collector jobs with their own charts, alerts and investigation Functions. This
page covers what the modules share: requirements, sites, the plugin-wide `dem.conf`, storage and the Functions. Each
module's setup, options, metrics, alerts and troubleshooting are on its integration page.

:::info

DEM is experimental. Linux builds include the plugin by default, and the browser runtime that synthetic monitoring
needs is a separate, explicit installation.

:::

## Modules

| Module | What it does | Integration page | Browser runtime | Configuration |
|---|---|---|---|---|
| `receiver` | One HTTP endpoint that serves the browser script and receives reports for every RUM site | [RUM Receiver](/src/go/plugin/dem/collector/receiver/integrations/rum_receiver.md) | No | `dem/receiver.conf` |
| `rum` | One job per website: measurements, sampled investigation history, optional OTLP event logs and traces | [Real User Monitoring](/src/go/plugin/dem/collector/rum/integrations/real_user_monitoring.md) | No | `dem/rum.conf` |
| `journey` | One job per Playwright Test script, run on a schedule | [Browser Journey](/src/go/plugin/dem/collector/synthetic/integrations/browser_journey.md) | Yes | `dem/journey.conf` |
| `lighthouse` | One job per audited URL: a desktop Lighthouse audit on a schedule | [Lighthouse Desktop Audit](/src/go/plugin/dem/collector/synthetic/integrations/lighthouse_desktop_audit.md) | Yes | `dem/lighthouse.conf` |

- **Stock configuration.** One receiver listening on loopback, and no RUM sites or browser jobs.
- **Configuration.** Use the module's file under the Netdata configuration directory, or Netdata's dynamic
  configuration (DynCfg). The receiver is a single object (`dem:collector:receiver`); every other job is independent
  (`dem:collector:rum:NAME`, and likewise for `journey` and `lighthouse`).
- **Job names.** A job's `name` is its stable key; `display_name` only changes what the UI shows. A RUM job's name is
  also its script URL (`/rum/NAME.js`) and its history key, so renaming a RUM job starts a new history.
- **RUM needs both modules.** RUM sites need the receiver; the receiver alone collects nothing. Synthetic jobs need
  neither.

## Requirements

- **Linux.** The plugin is built on Linux only, and is on by default. To leave it out of a build, use the CMake option
  `ENABLE_PLUGIN_DEM=OFF`, or `--disable-plugin-dem` with the source installer. To turn it off at run time, set
  `enabled: no` in `dem.conf`.
- **A machine ID.** DEM keeps its history in journal files, which need the host's machine ID. Without one, the plugin
  exits at startup and no DEM job runs, RUM included.
- **For journeys and Lighthouse:** Linux amd64 or arm64, the [browser runtime](#browser-runtime), and an unprivileged
  plugin service account. Chromium's sandbox always stays enabled, so containers need namespace and seccomp settings
  that allow it.
- **For RUM:** an HTTPS address that visitors' browsers can reach, usually a reverse proxy in front of the receiver.
  See the [RUM Receiver](/src/go/plugin/dem/collector/receiver/integrations/rum_receiver.md) page.

:::tip

DEM looks for the machine ID in `/etc/machine-id`, then `/var/lib/dbus/machine-id`, and checks both under the Netdata
host prefix first (`/host` in the official Docker image). If the Agent log shows
`opening DEM history: load journal identity: journalhost: machine id: no machine id found`, create the ID on the host
(`sudo systemd-machine-id-setup`), or mount it into the container with `-v /etc/machine-id:/host/etc/machine-id:ro`.
Each integration page's troubleshooting section has the full fix.

:::

## Sites

A DEM site is a [virtual node](/docs/learn/node-identities.md#virtual-nodes-vnodes) (vnode). Set the optional `vnode`
option on RUM, journey and Lighthouse jobs to show their charts and alerts under that node instead of the collecting
Agent. Jobs without it stay on the Agent and are not assigned to a site. A site with only synthetic jobs needs no RUM
job or receiver.

Define the vnode first, either through the plugin's `dem:vnode` dynamic configuration entry or in a YAML file under
`vnodes/` in the Netdata configuration directory. For example, `/etc/netdata/vnodes/sites.yaml`:

```yaml
- hostname: storefront
  guid: 12345678-1234-4234-8234-123456789abc
  labels:
    environment: production
```

Generate a unique GUID for each site, for example with `uuidgen`. Jobs refer to the vnode by name: the `hostname` of a
file-defined vnode, or the entry name of a dynamically configured one. Use the name, never the GUID. These two jobs
place RUM and a checkout journey on the same site:

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
```

What a vnode does and does not do:

- **Jobs stay independent.** Each keeps its own configuration, schedule, secrets and enable/disable controls; job names
  must be distinct within each module. A vnode owns no jobs, and referencing it does not deploy jobs to other Agents.
- **Shared resources stay shared.** The receiver, the browser concurrency limit and retention belong to the collecting
  Agent. A vnode does not combine RUM populations, add browser capacity or change who owns captures.
- **Evidence keeps its vnode.** Saved sessions, runs and captures keep the vnode GUID they were recorded under, even
  after the job moves to another vnode. Moving a RUM job to another vnode restarts its measurements; a synthetic job's
  latest result clears until a check for the new vnode completes. Changing a vnode's labels or display hostname,
  while its reference name and GUID stay the same, resets nothing.
- **Functions stay on the Agent.** The investigation Functions run on the collecting Agent, not on each vnode. Select a
  site with `vnode:NAME` (its configured name) or with `vnode_guid:GUID`, which also reaches a vnode that was renamed or
  removed; give one, never both. The six RUM Functions and `synthetics-checks`, `synthetics-journey-runs`,
  `synthetics-lighthouse-runs` and `synthetics-artifacts` accept them. These selectors filter data; they are not an
  access boundary.

## Plugin configuration

`dem.conf` in the Netdata configuration directory holds the plugin-wide settings. To change it, run
`sudo ./edit-config dem.conf` in that directory. An omitted or empty section uses its defaults.

```yaml
enabled: yes
browser:
  concurrency: 1
history:
  days: 30
  max_bytes: 1073741824
artifacts:
  days: 7
  max_bytes: 2147483648
```

| Option | Default | Meaning | Applies |
|---|---|---|---|
| `enabled` | `yes` | Runs the plugin. `no` stops every DEM module and Function. | Restart |
| `default_run` | `yes` | Runs every module that `modules` does not turn off. | Restart |
| `modules` | empty | Per-module switches, such as `journey: no` under `modules:`. A module turned off keeps its saved history readable. | Restart |
| `browser.concurrency` | `1` | Maximum simultaneous browser attempts in the plugin process. Monitoring, journey **Test** runs and the automatic updater's runtime qualification share it. A positive integer. | Restart |
| `history.days` | `30` | Age target of the shared investigation history, 1-365 days. | Next hourly sweep |
| `history.max_bytes` | `1073741824` (1 GiB) | Size target of the shared investigation history; at least 8 MiB. | Next hourly sweep |
| `artifacts.days` | `7` | Age target of retained screenshots, traces and reports, 1-365 days. | Next hourly sweep |
| `artifacts.max_bytes` | `2147483648` (2 GiB) | Size target of retained screenshots, traces and reports. | Next hourly sweep |
| `runtime.*` | see the runtime guide | How the browser runtime is installed, updated and stored. See [browser runtime host setup](/packaging/installer/DEM-RUNTIME.md#configuration). | Restart |

- **Concurrency.** Ready jobs take turns, and a fresh browser starts for every attempt. A browser slot is not a memory
  limit: size it for the Agent's CPU and memory and for the Lighthouse measurement quality you need.
  [DEM performance and sizing](/src/go/plugin/dem/PERFORMANCE.md) has measured workloads.
- **Invalid edits.** A `dem.conf` edit that fails validation keeps the last valid history and artifacts policy, logs a
  warning and shows the error in `dem-history-status` and `synthetics-artifacts`. An invalid file at startup stops the
  plugin.

## Storage and retention

| Store | Location | Holds | Default targets |
|---|---|---|---|
| Investigation history | `${NETDATA_LIB_DIR}/dem/journal` | Sampled RUM sessions, events and errors; synthetic run records | 30 days, 1 GiB |
| Captures | `${NETDATA_LIB_DIR}/dem/artifacts` | Failure screenshots, journey traces and Lighthouse HTML reports, one set per run | 7 days, 2 GiB |
| Browser runtime (managed mode) | `${NETDATA_LIB_DIR}/dem/runtime` | The installed runtime releases | 4 GiB; see the [runtime guide](/packaging/installer/DEM-RUNTIME.md#managed-store-and-retention) |

`${NETDATA_LIB_DIR}` is usually `/var/lib/netdata`. A cleanup sweep runs at startup and then hourly.

Investigation history:

- **One shared budget.** RUM, synthetic runs, disabled jobs and files written under earlier machine IDs share one
  budget. High-volume RUM can push out older synthetic runs.
- **Whole files expire.** Each file covers less than 24 hours and is removed once its newest record is older than
  `history.days`, so records can stay about one day and one hour longer. Size pressure removes the oldest files
  sooner, which shortens the investigation window.
- **Targets, not disk caps.** Active files, allocation rounding and pending writes can exceed `max_bytes`, and open
  queries can keep removed files allocated. Sizes include file preallocation.
- **Sampled, not lossless.** The plugin writes queued records every five seconds or 500 records, so a crash can lose
  the latest ones. History holds what each job's sampling policy selected, not every event.
- **Damage stops startup.** If the history files fail their startup check, the plugin exits before any Function
  registers and leaves the files intact for recovery; check the Agent log. A journal write with an uncertain outcome
  makes history unavailable until restart, while measurements continue; other write failures only drop the affected
  records (RUM counts them on its `rum.history` chart).

Captures:

- **Newest per job first.** Under size pressure, each job's newest eligible capture set is kept longest; age still
  expires it. Disabled or deleted jobs keep the same preference, and a renamed job starts a separate group.
- **Separate from history.** Captures stay downloadable after their run history expires and after the job is disabled
  or deleted.
- **Not a disk cap.** Work in progress, unrecognized files and pending deletions are counted separately, so total use
  can exceed `artifacts.max_bytes`. `synthetics-artifacts` reports the storage use, the policy in effect and cleanup
  errors.

### History status

`dem-history-status` reports the history policy in effect, the history files' size and count, the latest cleanup
attempt and success, and separate errors for inventory, cleanup, writes and policy reloads. It works with every
collector disabled and takes no parameters. A `null` value means unknown or not yet attempted.

It helps diagnose storage pressure and maintenance failures. It cannot tell whether a missing event expired, was
sampled out or was never collected.

## Functions

DEM's investigation Functions run on the collecting Agent. Open them in Netdata Cloud, or call them through the Agent
or Cloud API.

| Function | Shows | Documented on |
|---|---|---|
| `rum-sites` | Each site's receiver state, collection policy, install snippet and recent traffic | [Real User Monitoring](/src/go/plugin/dem/collector/rum/integrations/real_user_monitoring.md) |
| `rum-pages` | Web Vitals and traffic by page | Real User Monitoring |
| `rum-live` | A live feed of incoming observations | Real User Monitoring |
| `rum-sessions` | Saved browser sessions in a time range | Real User Monitoring |
| `rum-errors` | Saved JavaScript errors, grouped by fingerprint | Real User Monitoring |
| `rum-session-events` | One saved session's timeline | Real User Monitoring |
| `synthetics-checks` | Each journey and Lighthouse job's current state, schedule and latest result | [Browser Journey](/src/go/plugin/dem/collector/synthetic/integrations/browser_journey.md), [Lighthouse Desktop Audit](/src/go/plugin/dem/collector/synthetic/integrations/lighthouse_desktop_audit.md) |
| `synthetics-journey-runs` | Saved journey runs: outcome, tests and failure evidence | Browser Journey |
| `synthetics-lighthouse-runs` | Saved Lighthouse audits and their lab measurements | Lighthouse Desktop Audit |
| `synthetics-run` | One run's full diagnosis | Both synthetic pages |
| `synthetics-artifacts` | Retained capture sets and capture storage | Both synthetic pages |
| `synthetics-artifact` | One run's captures, or one capture's bytes | Both synthetic pages |
| `dem-history-status` | The shared history's policy, size and maintenance errors | [History status](#history-status) |

- **Access.** Viewing any DEM Function requires a signed-in member of the node's Netdata Cloud Space with permission
  to view sensitive data, on a paid plan or an active trial. Downloading capture bytes with `synthetics-artifact`
  also requires administrator access. Collection and history storage run on every plan.
- **Availability.** `dem-history-status` is always available. The RUM Functions appear once a RUM site runs or RUM
  history exists; the synthetic Functions appear once a journey or Lighthouse job runs, or synthetic history or
  captures exist. Saved data stays readable with the collectors disabled; only `enabled: no` turns the Functions off.
- **Parameters.** Pass them as `key:value` (or `key=value`) arguments, or as fields of a JSON request body, at its top
  level or under `selections` as the Netdata Cloud Function table sends them. Arguments win over `selections`, which
  win over top-level fields. Unknown parameters are rejected, except the table's own request fields (`timeout`,
  `last`, `_auxiliary` and an empty `query`) and an unused time range on `info` requests. The `vnode` and
  `vnode_guid` selectors are described under [Sites](#sites).
- **Time ranges.** History ranges select when the Agent received an observation (RUM) or when a run started
  (synthetic), as inclusive Unix seconds. Negative values are offsets from now, `0` means the epoch, and invalid or
  pre-epoch ranges fail. Browser clocks play no part. A past range shows what is retained now, not what was known then:
  a session saved later because of a problem can add context to an earlier range. Each integration page lists its
  Functions' default range.

## Alerts

The stock DEM alerts (RUM Web Vitals, OTLP export failures and journey failures) are listed on each integration page.
To adjust them for one site, use normal Netdata health configuration with the `_collect_plugin=dem` and
`_collect_job` chart labels. DEM does not write health configuration itself.

## Browser runtime

Journey and Lighthouse jobs need a separately installed browser runtime: one signed bundle with its own Node, Chromium,
Playwright and Lighthouse. RUM does not. Enabling a browser job never downloads it; without it, browser jobs wait and
report no website failures. Restart the Agent after installing or updating it; the native package's hooks restart a
running Agent themselves.

[Browser runtime host setup](/packaging/installer/DEM-RUNTIME.md) covers installation for native packages, static and
source installations, Docker and protected preloads, plus automatic updates and the `dem.plugin runtime` command
reference.

## Further reading

- [DEM plugin architecture](/src/go/plugin/dem/ARCHITECTURE.md): how the plugin is built, for developers.
- [DEM performance and sizing](/src/go/plugin/dem/PERFORMANCE.md): measured workloads and sizing guidance.
