# Install Netdata on FreeBSD

:::info

This guide is community-maintained and might not always reflect the latest details (like package versions). Double-check before proceeding! Want to help? [Submit a PR!](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md)

:::

## 1. Install dependencies

Run as `root`:

```bash
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof liblz4 libuv json-c cmake gmake
```

Approve any prompts that appear.

## 2. Choose an Installation Method

<details>
<summary><strong>Option A: Kickstart Installer (Recommended)</strong></summary>

The simplest approach is to use our one-line [kickstart installer](/packaging/installer/methods/kickstart.md).

- Prepare the installation command:
    - For Netdata Cloud users: Navigate to your Space, click **Add Nodes** → Copy the command from the "Linux" tab.
    - For standalone installation, use the example below.

- Run the installation command:
   ```bash
   wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh && sh /tmp/netdata-kickstart.sh --claim-token <YOUR_TOKEN> --claim-url https://app.netdata.cloud
   ```

:::note

Replace `<YOUR_TOKEN>` with your actual claim token.

:::

- After installation, access your Netdata dashboard at:

   ```
   http://NODE:19999
   ```

  (`NODE` = your FreeBSD machine's hostname or IP)

</details>

### Option B: FreeBSD Ports Installation

Netdata is also available through the FreeBSD Ports collection:

https://www.freshports.org/net-mgmt/netdata/

<details>
<summary><strong>Option C: Manual Installation (For Advanced Users)</strong></summary>

- Download the latest Netdata release:

   ```bash
   fetch https://github.com/netdata/netdata/releases/latest/download/netdata-latest.tar.gz
   ```

  Or download a specific version:

   ```bash
   fetch https://github.com/netdata/netdata/releases/download/v2.3.2/netdata-v2.3.2.tar.gz
   ```

- Extract the downloaded archive:

   ```bash
   tar -xzf netdata*.tar.gz && rm netdata*.tar.gz
   ```

- Install Netdata to `/opt/netdata`:

   ```bash
   cd netdata-v*
   ./netdata-installer.sh --install-prefix /opt
   ```

- Configure Netdata to start automatically at boot:

   ```bash
   sysrc netdata_enable="YES"
   ```

- Start the Netdata service:

   ```bash
   service netdata start
   ```

</details>

## 3. Updating Netdata Installation

If you enabled auto-updates with `--auto-update`, no further action is needed.

For manual updates:

```bash
cd /opt/netdata/usr/libexec/netdata/
./netdata-updater.sh
```

## Optional Kickstart Parameters

| Option                                               | Description                                                                                                |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------|
| `--non-interactive`                                  | Skip prompts and assume yes.                                                                               |
| `--interactive`                                      | Force interactive prompts.                                                                                 |
| `--release-channel stable`                           | Install stable builds (instead of nightly).                                                                |
| `--no-updates`                                       | Disable auto-updates.                                                                                      |
| `--disable-telemetry`                                | Disable anonymous statistics.                                                                              |
| `--native-only`                                      | Install only if native packages are available.                                                             |
| `--static-only`                                      | Install only if static builds are available.                                                               |
| `--install-prefix /opt`                              | Change installation directory.                                                                             |
| `--prepare-offline-install-source ./netdata-offline` | Prepare offline installation source. See [Offline Install Guide](/packaging/installer/methods/offline.md). |

## Environment Variables (Advanced Users)

| Variable              | Purpose                                                            |
|-----------------------|--------------------------------------------------------------------|
| `TMPDIR`              | Directory for temporary files.                                     |
| `ROOTCMD`             | Command used for privilege escalation (default: `sudo` or `doas`). |
| `DISABLE_TELEMETRY=1` | Disables anonymous telemetry data.                                 |

## Telemetry Notice

Anonymous usage data is collected by default. You can learn more or opt-out [here](/docs/netdata-agent/configuration/anonymous-telemetry-events.md).
