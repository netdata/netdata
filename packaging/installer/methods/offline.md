<!--
custom_edit_url: "https://github.com/netdata/netdata/edit/master/packaging/installer/methods/offline.md"
sidebar_label: "Offline systems"
learn_status: "Published"
learn_rel_path: "Netdata Agent/Installation/Linux"
description: "Install the Netdata Agent on offline/air gapped systems to benefit from real-time, per-second monitoring without connecting to the internet."
learn_link: "https://learn.netdata.cloud/docs/netdata-agent/installation/linux/offline-systems"
slug: "/netdata-agent/installation/linux/offline-systems"
-->




# Install Netdata on Offline Systems

This guide explains how to install Netdata Agent on systems without internet access.

Netdata supports offline installation of the Agent using our `kickstart.sh` script.

This method:

- Downloads all required files in advance.
- Works with static builds only (for now).
- Does *not* support automatic updates on offline systems.

:::note

Local package tools like `apt-offline` may work for DEB/RPM installs — but we don’t officially support them.

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

Run the following command:

- using `wget`
  ```bash
  wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
  sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
  ```
- or using `curl`
  ```bash
  curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh
  sh /tmp/netdata-kickstart.sh --release-channel stable --prepare-offline-install-source ./netdata-offline
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

Automatic updates are *disabled* by default for offline installations — since there’s no network connection.

:::
