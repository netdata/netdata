# Install Netdata on Offline Systems

This guide explains how to install Netdata Agent on systems without internet access.

Netdata supports offline installation of the Agent using our `kickstart.sh` script.

This method:

- Downloads all required files on an internet-connected machine
- Transfers these files to the offline system
- Supports static builds only (currently)
- Does not support automatic updates on offline systems

---

## Step 1: Prepare the Offline Installation Package

On your internet-connected machine, you'll need::

| Requirement             | Purpose                       |
|-------------------------|-------------------------------|
| `curl` or `wget`        | Download the kickstart script |
| `sha256sum` or `shasum` | Verify download integrity     |
| POSIX-compliant shell   | Run the installation scripts  |

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

  > [!NOTE]
  > The folder name `netdata-offline` is just an example — use any name you want.
  >
  > To use the nightly channel instead, replace `stable` with `nightly`.

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

---

## Step 2: Transfer to Offline System

Copy the entire `netdata-offline` directory to your offline system using your preferred method (USB drive, secure copy, etc.).

> [!IMPORTANT]  
> Do not rename or modify any files in the package.
> The installation script expects the exact directory structure and filenames.

## Step 3: Install on the Offline System

Navigate to the transferred directory rand un the installation script with elevated privileges:

```bash
cd netdata-offline
sudo ./install.sh
```

The `install.sh` script accepts the [same parameters](/packaging/installer/methods/kickstart.md#optional-parameters-for-kickstartsh) as `kickstart.sh`, allowing you to customize your installation.

## Automatic Updates

For offline installations, automatic updates are **disabled by default** since there's no internet connection to fetch updates.

To update an offline installation, repeat the steps in this guide with a newer version of the installation package.

> [!NOTE]   
> Local package tools like `apt-offline` may work for DEB/RPM installations — but we don’t officially support them.
