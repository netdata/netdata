# Anonymous telemetry events

By default, Netdata collects anonymous usage information from the open-source monitoring Agent. For events like start, stop, crash, etc., we use our own cloud function in GCP. For frontend telemetry (page views, etc.) on the dashboard itself, we use the open-source product analytics platform [PostHog](https://github.com/PostHog/posthog).

We are strongly committed to your [data privacy](https://netdata.cloud/privacy/).

We use the statistics gathered from this information for two purposes:

1. **Quality assurance**, to help us understand if Netdata behaves as expected, and to help us classify repeated issues with certain distributions or environments.

2. **Usage statistics**, to help us interpret how people use the Netdata Agent in real-world environments, and to help us identify how our development/design decisions influence the community.

Netdata collects usage information via two different channels:

- **Agent dashboard**: We use the [PostHog JavaScript integration](https://posthog.com/docs/integrations/js-integration) (with sensitive event attributes overwritten to be anonymized) to send product usage events when you access an [Agent's dashboard](/docs/dashboards-and-charts/README.md).
- **Agent backend**: The `netdata` daemon executes the [`anonymous-statistics.sh`](https://github.com/netdata/netdata/blob/6469cf92724644f5facf343e4bdd76ac0551a418/daemon/anonymous-statistics.sh.in) script when Netdata starts, stops cleanly, or fails.

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

We have configured the dashboard to trigger the PostHog JavaScript code only when the variable `anonymous_statistics` is true. The value of this variable is controlled via the [opt-out mechanism](#opt-out).

### Agent Backend - Anonymous Statistics Script

Every time the daemon is started or stopped, and every time a fatal condition is encountered, Netdata uses the anonymous statistics script to collect system information and send it to the Netdata telemetry cloud function via an HTTP call.

**Information collected for all events:**

- Netdata version, build information, release channel, install type
- OS name, version, ID, and detection method
- Kernel name, version, and architecture
- Virtualization and containerization technology
- System CPU information (model, vendor, frequency, core count)
- System disk and RAM information (total sizes)
- Netdata configuration (streaming, memory mode, web enabled, HTTPS, etc.)
- Alert counts (normal, warning, critical)
- Chart and metric counts
- Collector information
- Cloud connection status and availability
- Streaming/mirroring configuration

**For FATAL events only:**

The FATAL event sends the Netdata process and thread name, along with the source code function, source code filename, and source code line number of the fatal error.

:::note

To see exactly what and how is collected, you can review the script template `daemon/anonymous-statistics.sh.in`. The template is converted to a bash script called `anonymous-statistics.sh`, installed under the Netdata `plugins directory`, which is usually `/usr/libexec/netdata/plugins.d`.

:::

## Quick Decision Guide

**What's your installation type?**

```
🐧 Linux (standard)    → Use Method 1 (File-based)
🖥️  Windows             → Use Method 2 (Windows-specific)
🐳 Docker              → Use Method 3 (Environment variable)
📦 Manual/Offline      → Use Method 1 (File-based)
```

:::note

All opt-out methods prevent:

- The daemon from executing the anonymous statistics script
- The PostHog JavaScript snippet (which remains on the dashboard) from firing and sending any data to Netdata PostHog

When opt-out is enabled, the anonymous statistics script exits immediately without sending any data.

:::

## Opt-out Methods

<details>
<summary><strong>Method 1: Create opt-out file (Linux/macOS)</strong></summary><br/>

**When to use**: Standard Linux installations, manual installations, macOS, or offline setups.

**Steps**:

1. Navigate to your Netdata configuration directory (usually `/etc/netdata`)
2. Create an empty file called `.opt-out-from-anonymous-statistics`

```bash
touch /etc/netdata/.opt-out-from-anonymous-statistics
```

:::tip

This method works for all installation types including manual, offline, and macOS installations. The file simply needs to exist—the content doesn't matter.

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

See the [installation documentation](/packaging/installer/README.md) for available installer scripts.

</details>

## Installation-specific paths

| Installation Type | Configuration Directory                |
| ----------------- | -------------------------------------- |
| Linux (standard)  | `/etc/netdata`                         |
| Windows           | `C:\Program Files\Netdata\etc\netdata` |
| Docker            | N/A (use environment variable)         |
| macOS (Homebrew)  | `/usr/local/etc/netdata`               |
| macOS (other)     | `/etc/netdata`                         |

## Verification

To confirm that telemetry is disabled:

1. Check that the `.opt-out-from-anonymous-statistics` file exists in your config directory, **OR**
2. Verify that the `DISABLE_TELEMETRY` environment variable is set

The Agent dashboard will no longer send PostHog events, and the backend statistics script will not execute.

## Related documentation

- [Installation methods](/packaging/installer/README.md) - Complete installation guides
- [Docker installation](/packaging/docker/README.md) - Container-specific setup
- [Windows installation](/packaging/windows/WINDOWS_INSTALLER.md) - Windows Agent setup
