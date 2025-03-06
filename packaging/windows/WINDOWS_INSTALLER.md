# Netdata Windows Installer

Netdata offers a convenient Windows installer for easy setup. This executable provides two distinct installation modes, outlined below.

The Netdata Windows Agent is designed for users with paid Netdata subscriptions. If you are using a free (non-paid Netdata account), or no Netdata account at all, the Windows Agent will have restricted functionality. Specifically:

- **Standalone Agents**: The user interface within the Windows Agent will be locked, preventing access to monitoring data.
- [**Child Agents**](/docs/observability-centralization-points/metrics-centralization-points/README.md): Even when a Windows Agent streams data to a Linux parent Netdata instance, the Windows Agent's monitoring data will be locked and inaccessible within the parent dashboard's user interface.

## Download the MSI Installer

You can download the Netdata Windows installer (MSI) from the official releases page:

| Version                                                                                          | Description                                                                                                                                                               |
|--------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Stable](https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi)            | This is the recommended version for most users as it provides the most reliable and well-tested features.                                                                 |
| [Nightly](https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi) | Offers the latest features but may contain bugs or instabilities. Use this option if you require access to the newest features and are comfortable with potential issues. |

## Silent Mode (Command line)

This section provides instructions for installing Netdata in silent mode, which is ideal for automated deployments.

> **Info**
>
> Run the installer as admin to avoid the Windows prompt.
>
> Using silent mode implicitly accepts the terms of the [GPL-3](https://raw.githubusercontent.com/netdata/netdata/refs/heads/master/LICENSE) (Netdata Agent) and [NCUL1](https://app.netdata.cloud/LICENSE.txt) (Netdata Web Interface) licenses, skipping the display of agreements.

### Available Options

| Option       | Description                                                                                      |
|--------------|--------------------------------------------------------------------------------------------------|
| `/qn`        | Enables silent mode installation.                                                                |
| `/i`         | Specifies the path to the MSI installer file.                                                    |
| `INSECURE=1` | Forces insecure connections, bypassing hostname verification (use only if absolutely necessary). |
| `TOKEN=`     | Sets the Claim Token for your Netdata Cloud Space.                                               |
| `ROOMS=`     | Comma-separated list of Room IDs where you want your node to appear.                             |
| `PROXY=`     | Sets the proxy server address if your network requires one.                                      |

### Example Usage

To connect your Agent to your Cloud Space:

```bash
msiexec /qn /i netdata-x64.msi TOKEN="<YOUR_TOKEN>" ROOMS="<YOUR_ROOMS>"
```

Where:

- `<YOUR_TOKEN>`: Your Space claim token from Netdata Cloud.
- `<YOUR_ROOMS>`: Your Room ID(s) from Netdata Cloud.

This command downloads and installs Netdata in one step:

```powershell
$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi -OutFile "netdata-x64.msi"; msiexec /qn /i netdata-x64.msi TOKEN=<YOUR_TOKEN> ROOMS=<YOUR_ROOMS>
```

## Graphical User Interface (GUI)

1. **Double-click** the installer to begin the setup process.
2. **Grant Administrator Privileges**: You'll need to provide administrator permissions to install the Netdata service.

Once installed, you can access your Netdata dashboard at `localhost:19999`.
