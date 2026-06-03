# Install Netdata on Offline Systems

This guide explains how to install Netdata Agent on systems without internet access.

Netdata supports offline installation of the Agent using our `kickstart.sh` script.

This method:

- Downloads all required files in advance.
- Works with static builds only (for now).
- Does *not* support automatic updates on offline systems.

:::note

Local package tools like `apt-offline` may work for DEB/RPM installs — but we don't officially support them.

:::

:::note

For offline installation on Windows, see [Install Netdata on Windows](/packaging/windows/WINDOWS_INSTALLER.md#offline-air-gapped-installation). This guide covers UNIX-like systems only.

:::

## Step 1: Prepare the Offline Installation Package

On your internet-connected machine, you'll need::

| Requirement             | Purpose                    |
|-------------------------|----------------------------|
| `curl` or `wget`        | Download the script        |
| `sha256sum` or `shasum` | Verify script downloads    |
| POSIX-compliant shell   | Required to run the script |

:::note

The preparation step requires root privileges because the script needs to verify system capabilities before packaging. The commands below use `sudo`; if `sudo` is not available on your system, see the [Troubleshooting](#troubleshooting) section below.

:::

Run the following command:

- using `wget`
  ```bash
  wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
  sudo sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
  ```
- or using `curl`
  ```bash
  curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh
  sudo sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
  ```

:::note

The folder name `netdata-offline` is just an example — use any name you want.
To use the nightly channel instead, replace `stable` with `nightly`.

:::

**What's Included**:

The script creates a directory with all necessary files:

```
── netdata-offline
   ├── channel             # Release channel info
   ├── install.sh          # Installation script
   ├── kickstart.sh        # Original kickstart script
   ├── netdata-*.gz.run    # Netdata static packages for different architectures
   └── sha256sums.txt      # Verification hashes
```

## Step 2: Transfer to Offline System

Copy the entire `netdata-offline` directory to your offline system using your preferred method (USB drive, secure copy, etc.).

:::warning

Do not rename or modify any files in the package.
The installation script expects the exact directory structure and filenames.

:::

:::tip

The folder name `netdata-offline` is just an example — use any name you want.

:::

### Output

This will create a directory like:

```
./netdata-offline/
```

It will contain everything required to install Netdata offline.

---

## Choose Release Channel (Optional)

To prepare for a specific channel (`nightly` or `stable`), add:

```bash
--release-channel nightly
```

or

```bash
--release-channel stable
```

Example:

```bash
sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
```

---

## Install Netdata on the Target (Offline) System

1. Copy the entire `netdata-offline` directory to your offline system.

:::warning

Don't rename or modify the files.

:::

2. On the offline system, run:

```bash
cd netdata-offline
sudo ./install.sh
```

The `install.sh` script accepts the [same parameters](/packaging/installer/methods/kickstart.md#optional-parameters-for-kickstartsh) as `kickstart.sh`, allowing you to customize your installation.

## Automatic Updates

:::note

Automatic updates are *disabled* by default for offline installations — since there's no network connection.

:::

## Troubleshooting

### "This script needs root privileges" (F0201)

If you see the following error when running the preparation command:

```
ABORTED This script needs root privileges to install Netdata, but cannot find a way to gain them (we support sudo, doas, and pkexec). Either re-run this script as root, or set $ROOTCMD to a command that can be used to gain root privileges.
```

This means none of the supported privilege escalation tools (`sudo`, `doas`, or `pkexec`) were found on your system, or you are running the script as an unprivileged user without access to any of them.

To resolve this:

1. **Re-run with a privilege escalation tool:** Prefix the command with `sudo`, `doas`, or `pkexec`:

   ```bash
   sudo sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
   ```

2. **Set the `ROOTCMD` environment variable:** If your system uses a non-standard privilege escalation tool, set `ROOTCMD` to point to it:

   ```bash
   ROOTCMD=/usr/bin/doas sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
   ```

See the [Environment Variables](/packaging/installer/methods/kickstart.md#environment-variables) section in the kickstart documentation for full details on `ROOTCMD` and other options.
