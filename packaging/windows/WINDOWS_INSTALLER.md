# Install Netdata on Windows

Netdata provides a simple Windows installer for quick setup.

:::note

The Windows Agent is available for users with paid Netdata subscriptions.  
Free users will have limited functionality.

:::

## Limitations for Free Users

| Agent Type       | Limitation                                                                            |
|------------------|---------------------------------------------------------------------------------------|
| Standalone Agent | UI is locked — No local monitoring                                                    |
| Child Agent      | No monitoring data in parent dashboard when streaming to a Linux-based Netdata parent |

## Download the Windows Installer (MSI)

Choose the version that suits your needs:

| Version | Download Link                                                                                             | Recommended For                                                  |
|---------|-----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| Stable  | [Download Stable](https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi)            | Most users — stable, well-tested                                 |
| Nightly | [Download Nightly](https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi) | Users who need the latest features and can handle potential bugs |

## Silent Installation (Command Line)

:::warning

Silent installation isn’t supported on Windows Server versions earlier than 2019 due to TLS compatibility issues.

Use the [GUI installer](#graphical-installation-gui) instead.

:::

Use silent mode to deploy Netdata without user interaction (ideal for automation).

:::tip

Run the command prompt as Administrator.

:::

### Installation Command Options

| Option          | Description                                                               |
|-----------------|---------------------------------------------------------------------------|
| `/qn`           | Enables silent mode (no user interaction)                                 |
| `/i`            | Specifies the path to the MSI installer                                   |
| `TOKEN=`        | Claim token from your Netdata Cloud Space                                 |
| `ROOMS=`        | Comma-separated Room IDs for your node                                    |
| `PROXY=`        | (Optional) Proxy address if required                                      |
| `INSECURE=1`    | (Optional) Allow insecure connections (hostname verification disabled)    |
| `REINSTALL=ALL` | (Optional) It forces a complete reinstallation of all Netdata components. |

### Example Command

Install Netdata and connect to your Cloud Space:

```bash
msiexec /qn /i netdata-x64.msi TOKEN="<YOUR_TOKEN>" ROOMS="<YOUR_ROOMS>"
```

Replace:

- `<YOUR_TOKEN>` with your claim token
- `<YOUR_ROOMS>` with your Room ID(s)

### Download & Install in One Command (PowerShell)

```powershell
$ProgressPreference = 'SilentlyContinue'; Invoke-WebRequest https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi -OutFile "netdata-x64.msi"; msiexec /qn /i netdata-x64.msi TOKEN=<YOUR_TOKEN> ROOMS=<YOUR_ROOMS>
```

## Graphical Installation (GUI)

1. Download the `.msi` installer.
2. Double-click to run it.
3. Grant Administrator privileges when prompted.
4. Complete the setup wizard.

## Access Netdata Dashboard

After installation, open your browser and go to:

```
http://localhost:19999
```

## License Information

By using silent installation, you agree to:

- [GPL-3 License](https://raw.githubusercontent.com/netdata/netdata/refs/heads/master/LICENSE) — Netdata Agent
- [NCUL1 License](https://app.netdata.cloud/LICENSE.txt) — Netdata Web Interface

## Automatic Updates

For users who want to keep their Windows agents automatically updated with the latest releases, you can set up automated updates.

:::tip

**What You'll Learn**

How to set up automatic Netdata updates on Windows nodes using PowerShell and Task Scheduler.

:::

### How to set up

This setup will automatically download and install the latest Netdata build (stable or nightly) daily at your preferred time.

**1. Create the directory and updater script**

Run one of these PowerShell commands **as Administrator** (choose stable or nightly):

:::info

**Administrator Rights Required**

Creating directories in ProgramData and running Task Scheduler with the highest privileges requires administrator access.

Right-click on PowerShell and select "Run as administrator" before running these commands.

:::

- Stable version

   ```powershell
   New-Item -Path "$env:PROGRAMDATA\Netdata" -ItemType Directory -Force
   @'
   Invoke-WebRequest https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi -OutFile $env:PROGRAMDATA\Netdata\netdata-x64.msi
   msiexec /qn /i $env:PROGRAMDATA\Netdata\netdata-x64.msi TOKEN="<CLAIM_TOKEN>" ROOMS="<ROOM_ID>"
   '@ | Out-File -FilePath "$env:PROGRAMDATA\Netdata\netdata-updater.ps1" -Encoding UTF8
   ```

- Nightly version

   ```powershell
   New-Item -Path "$env:PROGRAMDATA\Netdata" -ItemType Directory -Force
   @'
   Invoke-WebRequest https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi -OutFile $env:PROGRAMDATA\Netdata\netdata-x64.msi
   msiexec /qn /i $env:PROGRAMDATA\Netdata\netdata-x64.msi TOKEN="<CLAIM_TOKEN>" ROOMS="<ROOM_ID>"
   '@ | Out-File -FilePath "$env:PROGRAMDATA\Netdata\netdata-updater.ps1" -Encoding UTF8
   ```

:::info

**Configuration Required**

Replace `<CLAIM_TOKEN>` with your Netdata Cloud claim token and `<ROOM_ID>` with your room identifier.

:::

**2. Create an entry in `Task Scheduler`**

| Tab          | Setting                              | Value                                                                                |
|--------------|--------------------------------------|--------------------------------------------------------------------------------------|
| **General**  | Run whether user is logged in or not | ✓ Checked                                                                            |
|              | Run with highest privileges          | ✓ Checked                                                                            |
|              | Configure for                        | Windows Vista, Windows Server 2008                                                   |
| **Triggers** | Schedule                             | Daily                                                                                |
|              | Time                                 | Your preferred time (e.g., 7AM UTC)                                                  |
| **Actions**  | Program/Script                       | `powershell`                                                                         |
|              | Arguments                            | `-noprofile -executionpolicy bypass -file %PROGRAMDATA%\Netdata\netdata-updater.ps1` |

:::tip

**Alternative Scheduling Options**

Instead of daily updates, you might prefer:

- Weekly updates (select "Weekly" in **Triggers tab**)
- Multiple times per day for critical systems
- Only on specific days of the week

:::
