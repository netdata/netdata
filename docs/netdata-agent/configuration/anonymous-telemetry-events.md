# Anonymous telemetry events

By default, Netdata collects usage and stability information from the open-source monitoring Agent. For Agent restarts and crashes, the `netdata` daemon sends a status report to Netdata. For frontend telemetry (page views, etc.) on the dashboard itself, we use the open-source product analytics platform [PostHog](https://github.com/PostHog/posthog).

We are strongly committed to your [data privacy](https://netdata.cloud/privacy/).

We use the statistics gathered from this information for two purposes:

1. **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated issues with certain distributions or environments.

2. **Usage statistics**, to help us interpret how people use the Netdata Agent in real-world environments, and to help us identify how our development/design decisions influence the community.

Netdata collects usage information via two different channels:

- **Agent dashboard**: We use the [PostHog JavaScript integration](https://posthog.com/docs/integrations/js-integration) (with sensitive event attributes overwritten to be anonymized) to send product usage events when you access an [Agent's dashboard](/docs/dashboards-and-charts/README.md).
- **Agent backend**: When the `netdata` daemon starts, it reports how its previous run ended (a restart or a crash) to `agent-events.netdata.cloud` over HTTPS. Builds with Sentry support also send crash reports to Sentry.

## What data is collected

### Agent Dashboard - PostHog JavaScript

When you access an Agent dashboard session by visiting `http://NODE:19999`, Netdata initializes a PostHog session and masks various event attributes.

_Note_: You can see the relevant code in the [dashboard repository](https://github.com/netdata/dashboard/blob/master/src/domains/global/sagas.ts#L107) where the `window.posthog.register()` call is made.

```JavaScript
window.posthog.register({
    distinct_id: machineGuid,
    $ip: "127.0.0.1",
    $current_url: "agent dashboard",
    $pathname: "netdata-dashboard",
    $host: "dashboard.netdata.io",
})
```

In the above snippet, a Netdata PostHog session is initialized and the `ip`, `current_url`, `pathname`, and `host` attributes are set to constant values for all events that may be sent during the session. This way, information like the IP or hostname of the Agent will not be sent as part of the product usage event data.

We have configured the dashboard to trigger the PostHog JavaScript code only when the variable `anonymous_statistics` is true. The value of this variable is controlled via the [opt-out mechanism](#opt-out-methods).

### Agent Backend - Restart and Crash Reports

When the daemon starts, it reads the status file it saved during its previous run and, if reporting is enabled, sends
it to `https://agent-events.netdata.cloud/agent-events`. Each kind of event is deduplicated and reported at most once
per day.

**Information in a report:**

- Netdata version, install type, database mode, and the Agent's uptime and restart count
- How the previous run ended: the exit reason, and the duration of the startup and shutdown steps
- For crashes and fatal errors: the signal, the thread, the source function, filename and line, the error message, and
  a stack trace when one is available
- OS, kernel, architecture, virtualization and container technology, cloud provider, instance type and region, and
  timezone
- Hardware: CPU, RAM and disk sizes, and the vendor and model of the system, board and chassis
- Memory use and database sizes of the Agent
- Identifiers: the Agent's machine GUID, the host's machine and boot IDs and the system UUID, and the Netdata Cloud
  node and claim IDs when the Agent is connected to Netdata Cloud

The `crash reports` option in the `[global]` section of `netdata.conf` controls these reports: `all` (restarts and
crashes), `crashes` (crashes only) or `off`. Its default is `all` when telemetry is enabled or the Agent is connected to
Netdata Cloud (directly or through a Netdata Parent), and `off` otherwise.

**Sentry crash reports:** builds that include Sentry support (some native packages) also send a crash report to
Sentry when the daemon crashes. Sentry is not started when telemetry is disabled.

:::note

To see exactly what is sent, review the status file the Agent saves in its library directory (`status-netdata.json`,
usually under `/var/lib/netdata`), and the code in `src/daemon/status-file.c`.

:::

## Quick Decision Guide

**What's your installation type?**

```
🐧 Linux (standard)    → Use Method 1 (File-based)
🖥️  Windows             → Use Method 2 (Windows-specific)
🐳 Docker              → Use Method 3 (Environment variable)
📦 Manual/Offline      → Use Method 1 (File-based)
🔷 FreeBSD             → Use Method 1 (File-based)
```

:::note

All opt-out methods prevent:

- The daemon from starting Sentry crash reporting
- The PostHog JavaScript snippet (which remains on the dashboard) from firing and sending any data to Netdata PostHog
- The restart and crash reports, unless the Agent is connected to Netdata Cloud (set `crash reports = off` in
  `netdata.conf` to stop them in that case too)

:::

## Opt-out Methods

<details>
<summary><strong>Method 1: Create opt-out file (Linux/macOS/FreeBSD)</strong></summary><br/>

**When to use**: Standard Linux installations, FreeBSD, manual installations, macOS, or offline setups.

**Steps**:

1. Navigate to your Netdata configuration directory
2. Create an empty file called `.opt-out-from-anonymous-statistics`

```bash
# Linux
touch /etc/netdata/.opt-out-from-anonymous-statistics

# FreeBSD
touch /usr/local/etc/netdata/.opt-out-from-anonymous-statistics
```

:::tip

This method works for all installation types including FreeBSD, manual, offline, and macOS installations. The file simply needs to exist—the content doesn't matter.

:::

</details>

<details>
<summary><strong>Method 2: Windows</strong></summary><br/>

**When to use**: Windows Agent installations.

On Windows, the Netdata configuration directory is located at `C:\Program Files\Netdata\etc\netdata`.

**Option A: Create opt-out file**

Run PowerShell as Administrator and execute:

```powershell
New-Item -Path "C:\Program Files\Netdata\etc\netdata\.opt-out-from-anonymous-statistics" -ItemType File -Force
```

**Option B: Set environment variable**

Run PowerShell as Administrator and execute:

```powershell
[Environment]::SetEnvironmentVariable("DISABLE_TELEMETRY", "1", "Machine")
Restart-Service -Name Netdata
```

:::note

After setting the environment variable, restart the Netdata service for the changes to take effect.

:::

</details>

<details>
<summary><strong>Method 3: Docker</strong></summary><br/>

**When to use**: Container-based deployments.

Set the `DISABLE_TELEMETRY` environment variable when creating the container:

```bash
docker run -e DISABLE_TELEMETRY=1 ...
```

Or using Docker Compose:

```yaml
environment:
  - DISABLE_TELEMETRY=1
```

:::note

You can also use `DO_NOT_TRACK=1` as an alternative to `DISABLE_TELEMETRY=1`.

:::

See the [Docker installation guide](/packaging/docker/README.md#create-a-new-netdata-agent-container) for complete configuration.

</details>

<details>
<summary><strong>Method 4: Installer scripts</strong></summary><br/>

**When to use**: Initial installation or manual updates.

Pass the `--disable-telemetry` flag to any installer script:

```bash
./netdata-installer.sh --disable-telemetry
```

Or export the environment variable before running the installer:

```bash
export DISABLE_TELEMETRY=1
./kickstart.sh
```

:::note

You can also use `DO_NOT_TRACK=1` as an alternative to `DISABLE_TELEMETRY=1` when using Netdata installer scripts.

:::

See the [installation documentation](/packaging/installer/README.md) for available installer scripts.

</details>

## Installation-specific paths

| Installation Type | Configuration Directory                |
|-------------------|----------------------------------------|
| Linux (standard)  | `/etc/netdata`                         |
| Windows           | `C:\Program Files\Netdata\etc\netdata` |
| Docker            | N/A (use environment variable)         |
| macOS (Homebrew)  | `/usr/local/etc/netdata`               |
| macOS (other)     | `/etc/netdata`                         |
| FreeBSD           | `/usr/local/etc/netdata`               |

## Verification

To confirm that telemetry is disabled:

1. Check that the `.opt-out-from-anonymous-statistics` file exists in your config directory, **OR**
2. Verify that the `DISABLE_TELEMETRY` environment variable is set in the environment used to start the Netdata daemon

:::note

If you used `DO_NOT_TRACK=1` with Docker or a Netdata installer script, confirm that it resulted in the `.opt-out-from-anonymous-statistics` file being created. The Agent dashboard will no longer send PostHog events, and the daemon will not start Sentry or, unless it is connected to Netdata Cloud, send restart and crash reports.

:::

## Related documentation

- [Installation methods](/packaging/installer/README.md) - Complete installation guides
- [Docker installation](/packaging/docker/README.md) - Container-specific setup
- [Windows installation](/packaging/windows/WINDOWS_INSTALLER.md) - Windows Agent setup
