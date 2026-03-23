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

## Related Windows documentation

- [Install Netdata on Windows](/packaging/windows/WINDOWS_INSTALLER.md)
- [Switching Netdata Install Types and Release Channels on Windows](/docs/install/windows-release-channels.md)
