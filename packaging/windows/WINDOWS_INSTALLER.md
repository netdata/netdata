# Install Netdata on Windows

Netdata provides a simple Windows installer for quick setup.

## Supported Windows Versions and Requirements

The Netdata Windows Agent is a **64-bit (x64)** application. We recommend **Windows 10 / 11 or Windows Server 2019 or newer** as the supported baseline.

The installer is built to run on older 64-bit Windows editions as far back as Windows Vista and Windows Server 2008, but those versions are not tested or officially supported, and several have reached end-of-life.

:::note

Older Windows and Windows Server releases are end-of-life and are more likely to encounter the download/TLS limitations described below. For the oldest versions, see [Will it run on Windows Server 2008?](#will-it-run-on-windows-server-2008).

:::

### Requirements

- **64-bit (x64) Windows.** The installer ships only as `netdata-x64.msi`.
- **Administrator rights.** Installation and the running service require elevated (Administrator) privileges.
- **Network access for download.**

## Access and Limitations

On Windows, whether you can view monitoring data on the local dashboard at <http://localhost:19999> depends on your Netdata Cloud plan and deployment mode. On the free Community tier, standalone Agents collect metrics but the local dashboard is locked — view your data in [Netdata Cloud](https://app.netdata.cloud) instead. For the full details by plan and deployment mode (including air-gapped and streaming setups), see [Access Control and Feature Availability](/docs/netdata-oss-limitations.md).

## Download the Windows Installer (MSI)

Choose the version that suits your needs:

| Version | Download Link                                                                                             | Recommended For                                                  |
|---------|-----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| Stable  | [Download Stable](https://github.com/netdata/netdata/releases/latest/download/netdata-x64.msi)            | Most users — stable, well-tested                                 |
| Nightly | [Download Nightly](https://github.com/netdata/netdata-nightlies/releases/latest/download/netdata-x64.msi) | Users who need the latest features and can handle potential bugs |

## Silent Installation (Command Line)

:::warning

Silent installation isn’t supported on Windows Server versions earlier than 2019 when the workflow depends on downloading the installer over TLS. Use the [GUI installer](#graphical-installation-gui) instead.

:::

Use silent mode to deploy Netdata without user interaction. Run the command prompt as Administrator.

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

### Claim dialog (GUI installer)

During the wizard you'll see a **Connect to the Cloud** dialog. These fields register the Windows Agent with your Netdata Cloud Space — they are the GUI equivalent of the silent-install `TOKEN`, `ROOMS`, `PROXY`, and `INSECURE` options.

| Field       | What it's for                                                                                  |
|-------------|------------------------------------------------------------------------------------------------|
| Claim Token | The claim token from your Netdata Cloud Space.                                                 |
| Rooms ID(s) | The Room(s) you want to add this Agent to.                                                     |
| Proxy URL   | (Optional) A proxy address, if your network requires one to reach Netdata Cloud.               |
| Cloud URL   | The Netdata Cloud endpoint. Defaults to `https://app.netdata.cloud`.                           |
| Insecure    | (Optional) Disables TLS hostname verification when the Agent contacts the Netdata Cloud endpoint. |

:::warning The Claim dialog connects to Netdata Cloud only

The **Cloud URL** and **Insecure** fields connect the Agent to **Netdata Cloud**. They cannot be used to connect this Agent to another local or on-premises Netdata node.

If your goal is to stream metrics to a Parent on your local network, entering a local address such as `http://192.168.0.x:19999` as the Cloud URL and checking **Insecure** will **not** set up streaming. The local dashboard at `http://localhost:19999` is this Agent's own web UI — it is not something you connect to through this dialog.

To stream to a local Parent, configure `stream.conf` on the Child instead. The streaming destination uses Netdata's streaming protocol, not an `http://` URL:

```text
[stream]
    enabled = yes
    destination = PARENT_IP:19999
```

See the [Parent-Child Configuration Reference](../../src/streaming/README.md) for the full setup, and the [Streaming Routing Reference](../../docs/streaming-routing.md) for routing options.

:::

## Offline (Air-gapped) Installation

Use this method to install Netdata on a Windows system with no internet access.

### Step 1: Download the MSI

On an internet-connected machine, download the MSI installer using one of the links from the [Download the Windows Installer](#download-the-windows-installer-msi) table above.

### Step 2: Transfer to the Offline System

Copy the downloaded `.msi` file to the offline Windows system using a USB drive, network share, or other secure transfer method.

### Step 3: Install Without Cloud Parameters

Open a command prompt as Administrator and run a silent install **without** `TOKEN` or `ROOMS` parameters:

```powershell
msiexec /qn /i netdata-x64.msi
```

:::note

This offline method uses `msiexec /qn` with a locally available MSI. On Windows Server versions earlier than 2019, the *automated download* commands in this document may fail due to TLS compatibility issues, so download the MSI on another machine first or use the [GUI installer](#graphical-installation-gui).

:::

## Verify the Installation

After installation, verify that the Netdata service is running:

```powershell
Get-Service netdata
```

Open <http://localhost:19999> to access the Netdata Dashboard. See [Access and Limitations](#access-and-limitations) for when the local dashboard is available.

## License Information

By using silent installation, you agree to:

- [GPL-3 License](https://raw.githubusercontent.com/netdata/netdata/refs/heads/master/LICENSE) — Netdata Agent
- [NCUL1 License](https://app.netdata.cloud/LICENSE.txt) — Netdata Web Interface

## Where is Netdata?

When Netdata is installed on Windows, it automatically registers as a Windows Service and appears in
**Add or remove programs** (also known as **Programs and Features** or **Apps & features** in newer Windows versions).
To start, stop, or restart the service, use the [PowerShell commands described in Service Control](/docs/netdata-agent/start-stop-restart.md#windows).

## Automatic Updates

For users who want to keep their Windows agents automatically updated with the latest releases, you can set up automated updates.

:::caution

Automatic updates require internet access and are not possible on air-gapped systems. To update, repeat the transfer process with a newer MSI.

:::

### How to set up

This setup will automatically download and install the latest Netdata build (stable or nightly) daily at your preferred time.

#### 1. Create the directory and updater script

Run one of these PowerShell commands **as Administrator** (choose stable or nightly). Creating directories in `ProgramData` and running Task Scheduler with the highest privileges requires administrator access.

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

Replace `<CLAIM_TOKEN>` with your Netdata Cloud claim token and `<ROOM_ID>` with your room identifier.

#### 2. Create an entry in `Task Scheduler`

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

## Working with Netdata on Windows

Netdata on Windows includes a bundled MSYS2 environment for working with Netdata configuration files and other internals.

### Open the MSYS2 environment

Launch the bundled MSYS2 shell using one of these methods (the paths below assume Netdata is installed in the default location):

- **Windows Run dialog**: Press `Win + R`, enter `"C:\Program Files\Netdata\msys2.exe"`, and press `Enter`.
- **PowerShell**: `& "C:\Program Files\Netdata\msys2.exe"`
- **Command Prompt**: `"C:\Program Files\Netdata\msys2.exe"`

When `msys2.exe` starts, it opens a shell environment for working with the Netdata files installed on your system.

### Writing Windows paths in MSYS format

Netdata configuration files consumed from the MSYS side require MSYS-style paths instead of native Windows drive-letter format.

Conversion pattern:

- Replace the drive letter with its lowercase equivalent preceded by `/` (e.g., `C:` → `/c`, `D:` → `/d`)
- Replace backslashes `\` with forward slashes `/`
- Keep spaces in the path, but quote or escape the full path when using it in shell commands

Examples:

| Windows path                                   | MSYS-style path                                |
|------------------------------------------------|------------------------------------------------|
| `C:\Program Files\Netdata\etc\netdata`         | `/c/Program Files/Netdata/etc/netdata/`        |
| `C:\Program Files\Netdata\usr\bin\netdata.exe` | `/c/Program Files/Netdata/usr/bin/netdata.exe` |

### Editing configuration files

Inside the MSYS2 environment, use the `edit-config` helper to edit Netdata configuration files:

```bash
cd /etc/netdata
./edit-config netdata.conf
```

For the complete `edit-config` workflow and configuration directory layout, see [Netdata Agent Configuration](/docs/netdata-agent/configuration/README.md#edit-configuration-files).

On Windows, `edit-config` opens files with the `nano` editor.

### Basic nano commands

| Action                | Keybinding                                     |
|-----------------------|------------------------------------------------|
| Edit text             | Type normally, use arrow keys to navigate      |
| Search                | `Ctrl + W`, type search text, `Enter`          |
| Save                  | `Ctrl + O`, `Enter` to confirm filename        |
| Exit                  | `Ctrl + X`                                     |
| Exit with save prompt | `Ctrl + X`, then `Y` to save or `N` to discard |

## Uninstalling Netdata on Windows

The MSI uninstall removes the Netdata binaries and the Windows service, but it does **not** remove your metric database, cache, or edited configuration files. To uninstall the Agent and completely delete the data it leaves behind, see [Uninstall Netdata](/packaging/installer/UNINSTALL.md).

## Related Windows documentation

- [Service Control](/docs/netdata-agent/start-stop-restart.md#windows) — Start, stop, restart, and check status of the Netdata Agent
- [Switching Install Types and Release Channels on Windows](/docs/install/windows-release-channels.md)
- [Uninstall Netdata](/packaging/installer/UNINSTALL.md) — Remove Netdata from Windows, including the metric database and configuration the MSI uninstall leaves behind

## FAQ

### Will it run on Windows Server 2008?

Windows Server 2008 (pre-R2) is the oldest Windows generation the installer targets, so the Agent **may run** on it. It is **not a tested or supported target**:

- Microsoft ended extended support for Windows Server 2008 and 2008 R2 on January 14, 2020; Extended Security Updates (ESU) ended January 2023 for on-premises deployments (extended through January 9, 2024 only for Azure-hosted instances).
- On Windows Server versions earlier than 2019, the automated MSI-download commands in this guide can fail due to TLS compatibility — use the [Graphical Installation (GUI)](#graphical-installation-gui) or a [pre-downloaded MSI](#offline-air-gapped-installation) instead (see also [Silent Installation](#silent-installation-command-line)).

For the best experience, use **Windows Server 2019 or newer**.
