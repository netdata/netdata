# Working with Netdata on Windows

This section introduces Windows-specific workflows for working with a local Netdata installation.

## First step: open the bundled MSYS2 environment

If Netdata is installed in the default location, you can launch its bundled MSYS2 environment by running:

```text
C:\Program Files\Netdata\msys2.exe
```

### Option 1: Run it from the Windows Run dialog

1. Press `Win + R`.
2. Enter `"C:\Program Files\Netdata\msys2.exe"`.
3. Press `Enter`.

### Option 2: Run it from PowerShell

Open PowerShell and run:

```powershell
& "C:\Program Files\Netdata\msys2.exe"
```

### Option 3: Run it from Command Prompt

Open Command Prompt and run:

```cmd
"C:\Program Files\Netdata\msys2.exe"
```

## What to expect

When `msys2.exe` starts, it opens a shell environment for working with the Netdata files installed on your Windows system.

## Writing Windows paths in Netdata config files

When a Netdata setting is consumed from the MSYS side of the Windows installation, write Windows paths in MSYS format instead of native Windows drive-letter format.

For example:

- Windows path: `C:\Program Files\Netdata`
- MSYS-style path: `/c/Program Files/Netdata`

Use this pattern:

- Replace the drive letter `C:` with `/c`
- Replace backslashes `\` with forward slashes `/`
- Keep spaces as they are

This means:

- `C:\Program Files\Netdata\etc\netdata` becomes `/c/Program Files/Netdata/etc/netdata/`
- `C:\Program Files\Netdata\usr\bin\netdata.exe` becomes `/c/Program Files/Netdata/usr/bin/netdata.exe`

## Editing config files with `edit-config`

Inside the Netdata MSYS environment, use the `edit-config` helper script from `/etc/netdata/edit-config` when you want to edit Netdata configuration files.

For example:

```bash
cd /etc/netdata
./edit-config netdata.conf
```

For the complete `edit-config` workflow and the broader explanation of Netdata config directories, see [Netdata Agent Configuration](/docs/netdata-agent/configuration/README.md#edit-configuration-files).

When `edit-config` opens the file on Windows, it uses the `nano` editor.

### Basic `nano` commands for Windows users

- Edit text: just type, and use the arrow keys to move around the file.
- Search: press `Ctrl + W`, type the text you want to find, then press `Enter`.
- Save: press `Ctrl + O`, press `Enter` to confirm the filename, then wait for nano to write the file.
- Exit: press `Ctrl + X`.
- Exit after editing: press `Ctrl + X`. If nano asks whether to save modified content, press `Y` to save your changes or `N` to discard them.

## Custom metrics with PowerShell scripts

You can collect custom metrics from PowerShell scripts using the [Nagios Plugins and Custom Scripts](/src/go/plugin/scripts.d/collector/nagios/integrations/nagios_plugins_and_custom_scripts.md) integration. Netdata automatically invokes `.ps1` scripts through `powershell.exe` with `-NoProfile -ExecutionPolicy Bypass` — your script just needs to follow the Nagios plugin output format (exit code and optional performance data).

### Quick setup

1. **Create your script** — write a `.ps1` file that outputs status text with optional performance data and exits with code `0` (OK), `1` (WARNING), `2` (CRITICAL), or `3` (UNKNOWN). Save it to the Netdata plugin directory (`C:\Program Files\Netdata\usr\libexec\netdata\plugins.d\`):

   ```powershell
   $value = 85
   Write-Host "OK - Value is $value | my_metric=$value;;;0;100"
   exit 0
   ```

2. **Test it manually** from PowerShell:

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\Program Files\Netdata\usr\libexec\netdata\plugins.d\check_example.ps1"
   echo "Exit code: $LASTEXITCODE"
   ```

3. **Configure the collector** — in the Netdata MSYS2 environment, edit `scripts.d/nagios.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files):

   ```bash
   cd /etc/netdata
   ./edit-config scripts.d/nagios.conf
   ```

   Add a job entry pointing to your script:

   ```yaml
   jobs:
     - name: my_check
       plugin: C:\Program Files\Netdata\usr\libexec\netdata\plugins.d\check_example.ps1
       timeout: 10s
       check_interval: 1m
   ```

4. [Restart the Netdata Agent](/docs/netdata-agent/start-stop-restart.md) for the new configuration to take effect.

:::tip

In a Parent-Child streaming setup, the collector runs on the Agent where the script is installed. To collect custom metrics from a child Node, install and configure the script directly on that child Node.

:::

For the complete configuration reference, a full PowerShell example, alert details, and troubleshooting, see the [Nagios Plugins and Custom Scripts](/src/go/plugin/scripts.d/collector/nagios/integrations/nagios_plugins_and_custom_scripts.md) integration documentation.

## Related Windows documentation

- [Install Netdata on Windows](/packaging/windows/WINDOWS_INSTALLER.md)
- [Switching Netdata Install Types and Release Channels on Windows](/docs/install/windows-release-channels.md)
