# Windows Installer Guide

Netdata provides a straightforward Windows installer for easy setup. The installer offers two installation modes, each with specific features outlined below.

**Important Note**: The Netdata Windows Agent is intended for users with paid Netdata subscriptions. If you're using a free account or no account at all, certain features of the Windows Agent will be restricted.

**Key Limitations for Free Users**:

- **Standalone Agents**: The user interface will be locked, and you will not have access to monitoring data.
- **Child Agents**: If the Windows Agent streams data to a Linux-based parent Netdata instance, you will be unable to view the Windows Agent’s monitoring data in the parent dashboard.

## Download the MSI Installer

You can download the Netdata Windows installer (MSI) from the official releases page. Choose between the following versions:

| Version                                                                                          | Description                                                                                                                                 |
|--------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| [Stable](https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi)            | Recommended for most users; offers the most reliable and well-tested features.                                                              |
| [Nightly](https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi) | Contains the latest features but may have bugs or instability. Choose this if you need the newest features and can handle potential issues. |

## Silent Installation (Command Line)

Silent mode allows for automated deployments without user interaction.

> **Note**: Run the installer as an administrator to avoid prompts.

By using silent mode, you implicitly agree to the terms of the [GPL-3](https://raw.githubusercontent.com/netdata/netdata/refs/heads/master/LICENSE) (Netdata Agent) and [NCUL1](https://app.netdata.cloud/LICENSE.txt) (Netdata Web Interface) licenses, and the agreements will not be displayed during installation.

### Installation Options

| Option       | Description                                                                            |
|--------------|----------------------------------------------------------------------------------------|
| `/qn`        | Enables silent mode installation.                                                      |
| `/i`         | Specifies the path to the MSI installer file.                                          |
| `INSECURE=1` | Forces insecure connections, bypassing hostname verification. Use only when necessary. |
| `TOKEN=`     | Sets the Claim Token for your Netdata Cloud Space.                                     |
| `ROOMS=`     | Comma-separated list of Room IDs where your node will appear.                          |
| `PROXY=`     | Specifies the proxy server address for networks requiring one.                         |

### Example Usage

To connect your Agent to your Cloud Space, use the following command:

```bash
msiexec /qn /i netdata-x64.msi TOKEN="<YOUR_TOKEN>" ROOMS="<YOUR_ROOMS>"
```

Replace `<YOUR_TOKEN>` with your Netdata Cloud Space claim token and `<YOUR_ROOMS>` with your Room ID(s).

You can also download and install Netdata in one step with the following command:

```powershell
$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi -OutFile "netdata-x64.msi"; msiexec /qn /i netdata-x64.msi TOKEN=<YOUR_TOKEN> ROOMS=<YOUR_ROOMS>
```

## Graphical User Interface (GUI) Installation

1. **Double-click** the MSI installer to begin the installation process.
2. **Grant Administrator Privileges**: You will be prompted to provide administrator permissions to install the Netdata service.

Once installed, you can access your Netdata dashboard at `localhost:19999`.

## Installing Netdata using Winget

This section provides instructions for installing Netdata using Winget, Microsoft's official Windows package manager.

### Prerequisites

Make sure Winget is installed on your Windows system. Need to install Winget? Check the official [Microsoft documentation](https://learn.microsoft.com/en-us/windows/package-manager/winget/).


### Install Netdata

Install Netdata by running this command in PowerShell:

```powershell
winget install netdata.netdata
```

### Update Netdata

Get the latest Netdata features and security updates with:

```powershell
winget upgrade netdata.netdata
```

### Uninstall Netdata

If you need to remove Netdata from your system, use:

```powershell
winget uninstall netdata.netdata
```

### Troubleshooting Winget Installation

If you encounter issues while installing Netdata using Winget, consider the following troubleshooting steps:

1. **Ensure Winget is up to date**: Run `winget --version` to check the version and update if necessary.
2. **Check network connectivity**: Ensure your system has a stable internet connection.
3. **Verify package identifier**: Ensure you are using the correct package identifier (`netdata.netdata`).
4. **Review Winget logs**: Check the Winget logs for any error messages or details about the installation process.

For further assistance, refer to the [Winget documentation](https://learn.microsoft.com/en-us/windows/package-manager/winget/) or seek help from the Netdata community.

After installation, you can access your Netdata dashboard by opening your browser and going to `localhost:19999`.
