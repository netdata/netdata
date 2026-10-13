# Minimize CPU and Memory on Fleet Devices

Keep monitoring small on the device while retaining fleet-wide dashboards, history, alerts, anomaly detection and live troubleshooting. Let the Child collect and stream metrics; let the Parent provide the storage and analysis capacity.

You control the device's monitoring budget through three settings: which collectors run, how often they collect, and how much history stays locally. On a Raspberry Pi 3 B+, five-minute system collection with one-minute process and sensor collection and StatsD disabled used under 1% of one CPU core and about 30 MiB of memory, with troubleshooting available through the Parent.

## Configure a lightweight Child

For a package prepared with `--keep apps,debugfs`, add this baseline to the Child's `netdata.conf`:

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
    statsd = yes
```

This configuration collects system, process and supported hardware metrics every five seconds, stores a short history in RAM, and moves alert evaluation and machine learning to the Parent. Add the [streaming configuration](./deployment-and-identity.md#connect-devices-to-a-parent) to connect it.

For autonomous device actions, set `[health] enabled = yes` on the Child. Local alerts can run custom scripts on alert status transitions, including while the device is disconnected from its Parent. See [device-side automation](./monitor-the-fleet.md#automate-actions-on-the-device) for how to combine local actions with centralized alerting.

StatsD is enabled on `localhost:8125`, ready for your applications to report operating states and measurements. It follows the Agent's collection interval. See the [fleet monitoring guide](./monitor-the-fleet.md#add-your-applications-operating-metrics) for examples. If a device needs only system monitoring and the smallest footprint, set `statsd = no`.

For container devices, enable the built-in cgroups collector and retain `containers` for container names and network attribution.

## Choose how quickly you need new values

Set `[db] update every` to match the device's job:

| Value | Interval | Use it for |
|:--|:--|:--|
| `1` | One second | Fast-changing workloads and detailed troubleshooting |
| `5` | Five seconds | General monitoring with quick fault visibility |
| `300` | Five minutes | Periodic device health, status and capacity |
| `600` | Ten minutes | Slowly changing devices with tight power or cellular budgets |

Longer intervals reduce collection work and streamed updates. Keep faster collection for brief failures or changes that your operators need to see. At longer intervals, gauges show periodic values and counter rates summarize activity over the interval.

### Collect every five or ten minutes

Set the default interval to `300` or `600`. For the **v2.12.0-2-nightly** package used in this guide, keep `apps` and `debugfs` at 60 seconds:

```ini
[plugin:apps]
    update every = 60

[plugin:debugfs]
    update every = 60
```

These settings keep process and sensor collection within that release's plugin timeout, while system metrics follow the slower interval. To use five- or ten-minute intervals for these plugins too, use an Agent version with cadence-aware plugin read timeouts.

On Pi 3 B+, five-minute system collection with these one-minute overrides and StatsD disabled used **0.69% of one CPU core and 28.1 MiB of memory**. The processes and sensors Functions remained available. With the Parent's default automatic keepalives, these one-minute charts set keepalive idle to 30 seconds; the [cellular guide](./minimize-cellular-traffic.md#match-keepalives-to-collection) explains how collection intervals affect bandwidth.

## Choose local history

Local history lets the Child fill coverage gaps after reconnecting:

| `[db] db` | What you get |
|:--|:--|
| `ram` | History in memory for recovery through network outages, with metric samples kept off the SD card |
| `alloc` | History in memory with a smaller allocation at low retention settings |
| `none` | Current streaming with the smallest local history footprint |
| `dbengine` | Persistent history that survives Agent restarts and device power loss |

In RAM mode, `retention` requests sample entries per dimension, rounded up to whole memory pages. On devices with 4 KiB pages, `retention = 120` allocates 1,024 entries per dimension—the same memory as requesting 1,024. That provides about 85 minutes at five-second collection, 3.6 days at five-minute collection or 7.1 days at ten-minute collection. Each chart's interval determines its window; devices with larger memory pages retain more entries at this setting.

With `alloc`, retention uses the requested entry count, with a minimum of five entries per dimension. For example, `retention = 120` keeps about ten hours at five-minute collection or twenty hours at ten-minute collection, using 480 bytes of metric sample storage per dimension. This trades a shorter recovery window for less memory than the RAM example above. Both modes keep metric history off the SD card and lose it when the Agent stops.

On Pi 1, choosing `none` instead of `ram` saved about 5 MiB of memory at one-second collection. Choose `ram` or `alloc` when you want to backfill network outages, or DBengine when recovery must also cover restarts. The [disconnection guide](./disconnected-devices-and-failover.md) shows the replication configuration; [Agent sizing](../netdata-agent/sizing-netdata-agents/README.md) covers persistent storage limits.

## Protect SD cards from continuous writes

Keep monitoring history on the Parent and metric samples in RAM. The Child collects and streams without writing metric history to the SD card. With health and machine learning running on the Parent, the Child focuses on collection and streaming.

The metadata database initializes at startup, then stays quiet while the device's charts, dimensions and labels remain stable. It records monitoring structure and identity changes as they occur.

Disable the Child's Agent logs in `netdata.conf`:

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

Collection, streaming and live Functions continue with these settings. Keep any logs your team needs in RAM or forward them centrally. Apply the same storage policy to service-manager output; see [logging configuration](../../src/libnetdata/log/README.md).

The Agent's diagnostic status file has a small update every 15 minutes and at lifecycle events, separate from metric storage. RAM-backed runtime state can keep those updates off the SD card as well; retain the device identity across image updates.

## Keep the collectors your team needs

Run the collectors and jobs that describe your device's workload. Use supported filters to focus process groups, interfaces, containers and application instances. Fewer metrics reduce collection work, memory and traffic together.

Select plugin bundles when [preparing the package](./minimize-disk-footprint.md), then select jobs and intervals in runtime configuration. Use streaming filters when you want a metric locally but do not need to send it to the Parent.

## Plan the device's monitoring budget

The examples below use Netdata **v2.12.0-2-nightly**, `apps` and `debugfs`, compressed streaming and `[db] retention = 120` rounded to memory pages, with StatsD disabled. CPU covers the Agent and persistent plugins as a percentage of one core; memory covers their combined process footprint (PSS).

| Device | Package / device OS | One-second collection: CPU / memory | Five-second collection: CPU / memory |
|:--|:--|--:|--:|
| Raspberry Pi 1 B+ | ARMv6 / 32-bit Raspbian 12 | 29.1% / 34.4 MiB | 7.6% / 34.3 MiB |
| Raspberry Pi 2 B | ARMv7 / 32-bit Raspbian 12 | 18.2% / 29.8 MiB | 4.5% / 30.2 MiB |
| Raspberry Pi 3 B+ | ARMv7 / 32-bit Raspbian 12 | 12.0% / 30.3 MiB | 3.1% / 29.6 MiB |
| Raspberry Pi 4 B | ARMv7 / 64-bit Linux | 11.0% / 36.8 MiB | 3.0% / 36.8 MiB |
| Raspberry Pi 5 B | ARMv7 / 64-bit Debian 12 | 5.2% / 57.6 MiB | 0.8% / 55.2 MiB |

Moving from one-second to five-second collection sharply reduces CPU use while retaining the same monitoring capabilities. Use these footprints as a starting budget for the device class, then add the application collectors and custom metrics your fleet needs.

## Follow monitoring overhead from the Parent

Use the Parent to follow Agent and plugin CPU, memory, metric counts and storage growth across the fleet. Review changes after image releases or new collector jobs, then adjust the device class's configuration.

See [performance optimization](../netdata-agent/configuration/optimize-the-netdata-agents-performance.md) for additional settings and [fleet monitoring](./monitor-the-fleet.md) for the central operating view.
