# Install Netdata on Offline Systems

Netdata supports offline installation of the Agent using our `kickstart.sh` script.

This method:

- Downloads all required files in advance.
- Works with static builds only (for now).
- Does *not* support automatic updates on offline systems.

> Note  
> Local package tools like `apt-offline` may work for DEB/RPM installs — but we don’t officially support them.

---

## Prepare the Offline Installation Source

You need:

| Requirement | Purpose |
|-------------|---------|
| `curl` or `wget` | Download the script |
| `sha256sum` or `shasum` | Verify downloads |
| POSIX-compliant shell | Required to run the script |

Run the following on any internet-connected machine:

### With `wget`

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

---

### With `curl`

```bash
curl https://get.netdata.cloud/kickstart.sh > /tmp/netdata-kickstart.sh
sh /tmp/netdata-kickstart.sh --prepare-offline-install-source ./netdata-offline
```

---

> [!TIP]
> The folder name `netdata-offline` is just an example — use any name you want.

---

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

   > ⚠️ Warning  
   > Don't rename or modify the files.

---

2. On the offline system, run:

```bash
cd netdata-offline
sudo ./install.sh
```

---

## Customize the Installation (Optional)

The `install.sh` script accepts all the same options as `kickstart.sh`.

Example — disable telemetry:

```bash
sudo ./install.sh --disable-telemetry
```

More options:  
[View all kickstart.sh parameters →](https://learn.netdata.cloud/packaging/installer/methods/kickstart#optional-parameters-to-alter-your-installation)

---

## Automatic Updates

> Note  
> Automatic updates are *disabled* by default for offline installations — since there’s no network connection.