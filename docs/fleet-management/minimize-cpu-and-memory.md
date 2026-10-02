# Minimize CPU and Memory on Fleet Devices

Keep edge devices focused on their workload by moving historical storage, alert evaluation and anomaly detection to Netdata Parents. Configure the Child to collect the metrics you need, at the interval you need, with enough local history for your connectivity pattern.

## Configure a lightweight Child

For the reduced package prepared with `--keep apps,debugfs`, put this baseline in the Child's `netdata.conf`. It collects system, process and supported hardware metrics every five seconds, keeps a small RAM history, and leaves alerts and anomaly detection to the Parent:

```ini
[global]
    profile = iot

[db]
    db = ram
    update every = 5
    retention = 120

[health]
    enabled = no

[ml]
    enabled = no

[web]
    bind to = localhost

[plugins]
    proc = yes
    diskspace = yes
    apps = yes
    debugfs = yes
    cgroups = no
    tc = no
    idlejitter = no
    statsd = no
```

Add the [streaming configuration](./deployment-and-identity.md#connect-to-a-parent) before starting the Agent. A reduced package supplies the executable files; this configuration selects the collection work. Enable cgroups for container devices and retain the `containers` capability when container names and network attribution are needed.

Use `update every = 300` for five-minute collection or `600` for ten-minute collection. With 120 samples per dimension, the nominal RAM history is ten minutes at five-second collection, ten hours at five-minute collection, or twenty hours at ten-minute collection. Allocation is rounded and charts can use different intervals, so check the actual available history before setting your recovery policy.

Use a release with cadence-aware plugin read timeouts before applying five- or ten-minute collection to every retained plugin. For the pinned **v2.12.0-2-nightly** package, keep `apps` and `debugfs` below the Agent's two-minute plugin-read timeout. To combine five- or ten-minute system collection with process and sensor collection, add these overrides:

```ini
[plugin:apps]
    update every = 60

[plugin:debugfs]
    update every = 60
```

This mixed configuration was run on Pi 3 B+ with the prepared ARMv7 package: system CPU charts updated every 300 seconds, process charts every 60 seconds, and the processes and sensors Functions remained registered. Over six minutes, the Agent and persistent plugins used 0.69% of one CPU core and 28.1 MiB mean PSS.

These plugins continue to collect once a minute, and their streamed charts also update once a minute. The Parent's automatic keepalive therefore uses its 30-second minimum. Setting `apps` or `debugfs` to 300 or 600 seconds in this package disconnects it before its next collection; do not use that configuration for complete process and sensor coverage.

The IoT profile does not select a five- or ten-minute interval automatically. Set it explicitly. At long intervals, allow at least two collection cycles after startup before checking rates and complete chart availability.

## Move analysis to the Parent

Configure these settings in the Child's `netdata.conf` when the Parent provides alerting and anomaly detection:

| Section and setting | What it does |
|:--|:--|
| `[global] profile = iot` | Selects defaults suited to constrained devices |
| `[ml] enabled = no` | Moves anomaly detection work off the Child |
| `[health] enabled = no` | Moves alert evaluation off the Child |
| `[web] bind to = localhost` | Keeps the device's web listener on loopback |

Enable the corresponding features on the Parent, where your team uses dashboards and alerts. If you need local alerts while a device is disconnected, keep Child health evaluation enabled for that device class.

The IoT profile supplies a starting point. Set collection intervals and local storage explicitly to match the device's role. See [Agent configuration](../netdata-agent/configuration/README.md) for the settings and [lightweight Child configuration](../deployment-guides/deployment-strategies.md) for a complete baseline.

## Choose a collection interval

Set `[db] update every` to control the default collection interval:

| Value | Collection interval | Suitable use |
|:--|:--|:--|
| `5` | Five seconds | General device monitoring with quicker fault visibility |
| `1` | One second | Fast-changing workloads and troubleshooting |
| `300` | Five minutes | Periodic health, status and capacity monitoring |
| `600` | Ten minutes | Slowly changing devices on tight power or cellular budgets |

Longer intervals reduce collection work and streamed updates. Use shorter intervals for conditions that need quick detection, such as brief thermal excursions or short service failures. At longer intervals, gauges show periodic readings and counter rates average activity over a longer period.

Collector-specific settings can override the default. Set those intervals consistently and check the chart update intervals at the Parent.

## Choose local history

The Child's local database determines how much history it can send after a connection outage:

| `[db] db` setting | Local history | Use it when |
|:--|:--|:--|
| `none` | No local metric history | You need current monitoring with the smallest local history footprint |
| `ram` | Metric history in memory | You want to recover coverage gaps while the Agent keeps running |
| `dbengine` | Persistent metric history | You need history to survive device or Agent restarts |

With RAM mode, `retention` controls sample entries per dimension. The time covered depends on each chart's interval, and allocation is rounded to memory pages. Longer intervals provide a longer history window for the same entry count. Check the available history at the Parent when setting your outage recovery window.

RAM and `none` modes keep metric samples off the SD card. Use the settings below to prevent local logs from adding continuous writes.

For DBengine disk limits and storage tiers, see [Agent sizing](../netdata-agent/sizing-netdata-agents/README.md). The [disconnection guide](./disconnected-devices-and-failover.md) explains how local history supports recovery.

Stripping symbols reduces package and installation space. Control steady-state CPU and memory through collection intervals, collector selection and local retention; symbol removal alone does not reduce the number of metrics collected.

## Protect SD cards from continuous writes

Netdata can monitor SD-card devices without continuously writing collected metrics to the card. Keep the Child's metric history in RAM, disable local logging, and let the Parent store long-term history, evaluate alerts and run anomaly detection. Collection and streaming continue normally, including live troubleshooting through the Parent.

Use the lightweight Child configuration above with `db = ram` or `db = none`, health disabled and ML disabled. To turn off the Agent's local logs, add:

```ini
[logs]
    daemon = off
    collector = off
    access = off
    health = off
    debug = off
    debug flags = 0

[cloud]
    conversation log = no
```

Disabling these logs removes local Agent log history. Keep any device logs you need for troubleshooting in RAM or forward them to your central logging service. Configure the service manager's output destination too if it persists startup messages or plugin output outside Netdata's logging system. See [logging configuration](../../src/libnetdata/log/README.md).

The metadata database records the device, its charts, dimensions and labels. Expect an initialization burst when Netdata starts. Once the metrics and labels are stable, with health and ML disabled, ongoing collection does not repeatedly write that metadata: it stays quiet until metadata changes. You do not need to disable the metadata database to achieve this behavior.

The Agent also saves a small diagnostic status file every 15 minutes and at lifecycle events. This is separate from the metadata database and metric history. Keeping this runtime state on a RAM-backed filesystem removes those writes from the SD card too; preserve the device's identity across restarts when preparing your image.

This setup protects the SD card from continuous monitoring writes while the Parent retains your fleet's monitoring history. Choose `dbengine` instead when you need local metric history to survive device restarts.

## Keep collection focused

Enable the collectors and jobs that monitor your device workload. Configure supported filters to avoid collecting unnecessary process groups, interfaces, containers or application instances. Fewer metrics reduce collection, memory and streaming work together.

Package preparation removes unused optional plugins. Built-in collectors and retained plugin bundles are controlled through runtime configuration. A streaming chart filter controls what reaches the Parent; use collector settings to control work on the Child.

## Plan device capacity

Use these measured footprints to size the monitoring budget for a device class. The packages contain the Agent, built-in system collectors, `apps` and `debugfs`; the [disk guide](./minimize-disk-footprint.md) gives their complete contents and preparation commands.

The following runs used **v2.12.0-2-nightly**, compressed streaming to a Parent, the configuration above and 120 RAM samples per dimension. CPU includes the Agent and its persistent plugin processes; 100% means one fully occupied CPU core. Memory is their mean proportional set size (PSS), which apportions shared pages instead of counting them repeatedly.

| Device | Package / device OS | One-second collection: CPU / memory | Five-second collection: CPU / memory |
|:--|:--|--:|--:|
| Raspberry Pi 1 B+ | ARMv6 / 32-bit Raspbian 12 | 29.1% / 34.4 MiB | 7.6% / 34.3 MiB |
| Raspberry Pi 2 B | ARMv7 / 32-bit Raspbian 12 | 18.2% / 29.8 MiB | 4.5% / 30.2 MiB |
| Raspberry Pi 3 B+ | ARMv7 / 32-bit Raspbian 12 | 12.0% / 30.3 MiB | 3.1% / 29.6 MiB |
| Raspberry Pi 4 B | ARMv7 / 64-bit Linux | 11.0% / 36.8 MiB | 3.0% / 36.8 MiB |
| Raspberry Pi 5 B | ARMv7 / 64-bit Debian 12 | 5.2% / 57.6 MiB | 0.8% / 55.2 MiB |

Measurements cover 90 seconds at one-second collection and 120 seconds at five-second collection, after a short startup warmup. The process tree was sampled every ten seconds; short-lived helpers can fall between samples. PSS excludes kernel memory and the filesystem cache. They describe these boards and workloads; running applications, kernel features and collector jobs change resource use. Pi 5 also hosted the isolated Parent. ARMv7 execution on the two 64-bit operating systems was verified on these installations; choose a package supported by your own device OS.

On Pi 1, using `db = none` at one-second collection used 29.1 MiB of process-tree PSS, compared with 34.4 MiB for `db = ram`. `none` still streamed metrics to the Parent; the Child's local charts API had no stored charts. Choose RAM history when recovery after a connection outage matters.

The CPU and memory measurements used the baseline configuration, before applying the logging settings in [Protect SD cards from continuous writes](#protect-sd-cards-from-continuous-writes). Four boards used NFS roots and Pi 4 used local storage; these runs measured CPU and memory, rather than SD-card endurance.

## Watch resource use from the Parent

Monitor the Agent and its plugin processes for CPU, memory and local storage growth. Check resource use during normal operation, reconnection and live troubleshooting. Use these observations to adjust the device class's collection and retention settings.

For additional settings, see [performance optimization](../netdata-agent/configuration/optimize-the-netdata-agents-performance.md). Once configured, use the [fleet monitoring guide](./monitor-the-fleet.md) to keep track of monitoring overhead across the fleet.
