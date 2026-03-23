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

## Related Windows documentation

- [Install Netdata on Windows](/packaging/windows/WINDOWS_INSTALLER.md)
- [Switching Netdata Install Types and Release Channels on Windows](/docs/install/windows-release-channels.md)

