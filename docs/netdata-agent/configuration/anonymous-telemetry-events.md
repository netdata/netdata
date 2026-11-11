# Anonymous telemetry events

By default, Netdata collects usage and stability information from the open-source Netdata Agent. For Agent restarts and crashes, the `netdata` daemon sends a status report to Netdata. For frontend telemetry on the dashboard itself, the dashboard loads third-party services: [PostHog](https://github.com/PostHog/posthog) for product analytics, Google Tag Manager, Reo.dev and Sentry for error reporting.

We are strongly committed to your [data privacy](https://netdata.cloud/privacy/).

We use the statistics gathered from this information for two purposes:

1. **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated issues with certain distributions or environments.

2. **Usage statistics**, to help us interpret how people use the Netdata Agent in real-world environments, and to help us identify how our development/design decisions influence the community.

Netdata collects usage information via two different channels:

- **Agent dashboard**: When you access an [Agent's dashboard](/docs/dashboards-and-charts/README.md), it loads PostHog, Google Tag Manager, Reo.dev and Sentry, which send product usage events and error reports.
- **Agent backend**: When the `netdata` daemon starts, it reports how its previous run ended (a restart or a crash; on the first start, its current status) to `agent-events.netdata.cloud` over HTTPS. Builds with Sentry support also send crash reports to Sentry.

## What data is collected

### Agent Dashboard - third-party services

When you access an Agent dashboard by visiting `http://NODE:19999`, and telemetry is not opted out, the dashboard loads these services in your browser:

| Service | What it is used for |
|---|---|
| [PostHog](https://posthog.com/docs/libraries/js) | Product usage events (page views, feature use) |
| [Google Tag Manager](https://support.google.com/tagmanager/) | Marketing and usage tags |
| [Reo.dev](https://www.reo.dev/) | Developer activity analytics |
| [Sentry](https://docs.sentry.io/platforms/javascript/) | Error and crash reports from the dashboard |

When you sign in to Netdata Cloud from the dashboard, the product usage events are associated with your Netdata Cloud account.

The dashboard loads these services only when the Agent's `anonymous_statistics` setting is true. The value of this setting is controlled via the [opt-out mechanism](#opt-out-methods).

### Agent Backend - Restart and Crash Reports

When the daemon starts, it reads the status file it saved during its previous run and, if reporting is enabled, sends
it to `https://agent-events.netdata.cloud/agent-events`. On the first start, when there is no previous status, it
sends the current one. The daemon suppresses repeat reports with the same deduplication hash (the status, exit
reason, version, host and crash details) for up to 23 hours, while that hash remains in its 50-entry cache.

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
crashes), `crashes` (only events the Agent classifies as crashes) or `off`. Its default is `all` when telemetry is enabled or the Agent is, or was in its
previous run, connected to Netdata Cloud (directly or through a Netdata Parent), and `off` otherwise.

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
- The dashboard from loading PostHog, Google Tag Manager, Reo.dev and Sentry, so it sends them no data
- The restart and crash reports, unless `crash reports` is set explicitly in `netdata.conf`, or the Agent is, or was in
  its previous run, connected to Netdata Cloud. Set `crash reports = off` to stop them in every case

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

If you used `DO_NOT_TRACK=1` with Docker or a Netdata installer script, confirm that it resulted in the `.opt-out-from-anonymous-statistics` file being created. The Agent dashboard will no longer load PostHog, Google Tag Manager, Reo.dev or Sentry, and the daemon will not start Sentry. It also stops sending restart and crash reports, unless `crash reports` is set explicitly or the Agent is, or was in its previous run, connected to Netdata Cloud.

:::

## Related documentation

- [Installation methods](/packaging/installer/README.md) - Complete installation guides
- [Docker installation](/packaging/docker/README.md) - Container-specific setup
- [Windows installation](/packaging/windows/WINDOWS_INSTALLER.md) - Windows Agent setup
