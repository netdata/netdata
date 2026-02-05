# Anonymous telemetry events

Netdata collects anonymous usage information by default to improve the monitoring Agent. This telemetry helps identify issues and understand how users interact with Netdata.

We are committed to your [data privacy](https://netdata.cloud/privacy/). You can opt out at any time using several methods described below.

## What data is collected

Netdata gathers anonymous statistics via two channels:

### Agent Dashboard

The dashboard uses [PostHog JavaScript](https://posthog.com/docs/integrations/js-integration) to track page views and interactions. Sensitive attributes (IP, hostname, URL) are masked to anonymize the data:

```JavaScript
window.posthog.register({
    distinct_id: machineGuid,
    $ip: "127.0.0.1",
    $current_url: "agent dashboard",
    $pathname: "netdata-dashboard",
    $host: "dashboard.netdata.io",
})
```

### Agent Backend

The `netdata` daemon sends anonymous statistics when it starts, stops, or crashes via the `anonymous-statistics.sh` script.

**Information collected:**

- Netdata version
- OS name, version, and ID
- Kernel name, version, and architecture
- Virtualization and containerization technology

**For FATAL events only:**

- Process and thread name
- Source code function, filename, and line number

:::note

To see exactly how data is collected, review the script template at [`daemon/anonymous-statistics.sh.in`](https://github.com/netdata/netdata/blob/6469cf92724644f5facf343e4bdd76ac0551a418/daemon/anonymous-statistics.sh.in).

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

- The anonymous statistics script from running
- The PostHog JavaScript snippet from firing

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

After setting the environment variable, restart the Netdata service for changes to take effect.

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

See [Docker installation guide](/packaging/docker/README.md#create-a-new-netdata-agent-container) for complete configuration.

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

See [installation documentation](/packaging/installer/README.md) for available installer scripts.

</details>

## Installation-specific paths

| Installation Type | Configuration Directory                |
| ----------------- | -------------------------------------- |
| Linux (standard)  | `/etc/netdata`                         |
| Windows           | `C:\Program Files\Netdata\etc\netdata` |
| Docker            | N/A (use environment variable)         |
| macOS             | `/usr/local/etc/netdata`               |

## Verification

To confirm telemetry is disabled:

1. Check that the `.opt-out-from-anonymous-statistics` file exists in your config directory, **OR**
2. Verify the `DISABLE_TELEMETRY` environment variable is set

The Agent dashboard will no longer send PostHog events, and the backend statistics script will not execute.

## Related documentation

- [Installation methods](/packaging/installer/README.md) - Complete installation guides
- [Docker installation](/packaging/docker/README.md) - Container-specific setup
- [Windows installation](/packaging/windows/WINDOWS_INSTALLER.md) - Windows Agent setup
