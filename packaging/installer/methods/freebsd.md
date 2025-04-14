# Install Netdata on FreeBSD

> 💡 This guide is community-maintained and might not always reflect the latest details (like package versions).  
> Double-check before proceeding!  
> Want to help? [Submit a PR!](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md)

---

## 1. Install dependencies

Run as `root`:

```bash
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof liblz4 libuv json-c cmake gmake
```

Approve any prompts that appear.

---

## 2. Install Netdata (Recommended)

Use our one-line [kickstart installer](/packaging/installer/methods/kickstart.md).

If you're using Netdata Cloud:

- In your Space, click **Add Nodes** → Copy the suggested command from the "Linux" tab.

Example command:

```bash
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --claim-token <YOUR_TOKEN> --claim-url https://app.netdata.cloud
```

> Replace `<YOUR_TOKEN>` with your claim token.

Once installed, access Netdata at:

```
http://NODE:19999
```

(`NODE` = your FreeBSD machine's hostname or IP)

---

## 3. Alternative: Install via FreeBSD Ports

You can also install Netdata using FreeBSD Ports:

https://www.freshports.org/net-mgmt/netdata/

---

## 4. Manual installation (Advanced)

Download the latest Netdata release:

```bash
fetch https://github.com/netdata/netdata/releases/latest/download/netdata-latest.tar.gz
```

Or fetch a specific version:

```bash
fetch https://github.com/netdata/netdata/releases/download/v2.3.2/netdata-v2.3.2.tar.gz
```

Extract the archive:

```bash
tar -xzf netdata*.tar.gz && rm netdata*.tar.gz
```

Install Netdata to `/opt/netdata`:

```bash
cd netdata-v*
./netdata-installer.sh --install-prefix /opt
```

Enable Netdata to start automatically:

```bash
sysrc netdata_enable="YES"
```

Start Netdata:

```bash
service netdata start
```

---

## 5. Updating Netdata on FreeBSD

If you enabled auto-updates with `--auto-update`, you're done!

Otherwise, update manually:

```bash
cd /opt/netdata/usr/libexec/netdata/
./netdata-updater.sh
```

---

## Optional Kickstart Parameters

| Option                         | Description                                                                                     |
|--------------------------------|-------------------------------------------------------------------------------------------------|
| `--non-interactive`            | Skip prompts and assume yes.                                                                    |
| `--interactive`                | Force interactive prompts.                                                                      |
| `--release-channel stable`     | Install stable builds (instead of nightly).                                                     |
| `--no-updates`                 | Disable auto-updates.                                                                           |
| `--disable-telemetry`          | Disable anonymous statistics.                                                                   |
| `--native-only`                | Install only if native packages are available.                                                  |
| `--static-only`                | Install only if static builds are available.                                                    |
| `--install-prefix /opt`        | Change installation directory.                                                                 |
| `--prepare-offline-install-source ./netdata-offline` | Prepare offline installation source. See [Offline Install Guide](/packaging/installer/methods/offline.md). |

---

## Environment Variables (Advanced Users)

| Variable        | Purpose                                                    |
|-----------------|------------------------------------------------------------|
| `TMPDIR`        | Directory for temporary files.                             |
| `ROOTCMD`       | Command used for privilege escalation (default: sudo/doas).|
| `DISABLE_TELEMETRY=1` | Disable anonymous telemetry data.                   |

---

## Telemetry Notice 📊

Starting with Netdata v1.30, anonymous usage data is collected by default.

Learn more or opt-out:  
[Anonymous Telemetry Events](/docs/netdata-agent/configuration/anonymous-telemetry-events.md)