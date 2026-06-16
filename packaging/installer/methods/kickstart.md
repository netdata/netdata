import { OneLineInstall } from '@site/src/components/OneLineInstall/'
import { Install, InstallBox } from '@site/src/components/Install/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with kickstart.sh

`kickstart.sh` is the recommended way to install Netdata.

This installation script works on all major Linux distributions. It automatically detects the best way to install Netdata for your system.

<details><summary>What does kickstart.sh actually do?</summary>

1. Detects your OS and environment
2. Checks for an existing Netdata installation
3. Installs using:
    - Native packages (preferred)
    - Static build (fallback)
    - Build from source (last resort)
4. Installs an auto-update cron job (unless disabled)
5. Optionally connects your node to Netdata Cloud

</details>  

## Quick Overview

| Task                  | Command / Location             | Notes                                   |
|-----------------------|--------------------------------|-----------------------------------------|
| Install Netdata       | Run `kickstart.sh`             | Choose nightly or stable release        |
| Connect to Cloud      | Use claim token                | Connect node to Netdata Cloud           |
| Customize install     | Pass flags to control behavior | Directory, release, update control      |
| Export config for IaC | Copy config from Cloud UI      | For automation & Infrastructure as Code |

## Run the One-Line Install Command

To install and connect to Netdata Cloud in a single step from your terminal:

<Tabs>
  <TabItem value="wget" label="wget">
    <OneLineInstall
      method="wget"
      privacyMd="[anonymous statistics?](/docs/netdata-agent/configuration/anonymous-telemetry-events.md)"
      connectMd="[connect](/src/claim/README.md)"
    />
  </TabItem>

  <TabItem value="curl" label="curl">
    <OneLineInstall
      method="curl"
      privacyMd="[anonymous statistics?](/docs/netdata-agent/configuration/anonymous-telemetry-events.md)"
      connectMd="[connect](/src/claim/README.md)"
    />
  </TabItem>
</Tabs>

:::tip

Pick **Stable** or **Nightly**: Check the [guide](/packaging/PLATFORM_SUPPORT.md) for differences.

:::

<details><summary>🔍 Where to find your claim token</summary>

1. Log in to [Netdata Cloud](https://app.netdata.cloud)
2. Navigate to your Space
3. Go to **Space Settings** → **Nodes**
4. Click **Add Node** → Copy Claim Token

</details>

## Optional Parameters for kickstart.sh

Use these flags to customize your installation.

| Category                | Parameter              | Purpose                             |
|-------------------------|------------------------|-------------------------------------|
| **Directory Options**   | `--install-prefix`     | Custom install directory            |
|                         | `--old-install-prefix` | Clean previous install directory    |
| **Interactivity**       | `--non-interactive`    | No prompts (good for scripts)       |
|                         | `--interactive`        | Force interactive prompts           |
| **Release Channel**     | `--release-channel`    | `nightly` or `stable`               |
|                         | `--install-version`    | Install specific version            |
| **Auto-Updates**        | `--auto-update`        | Enable updates                      |
|                         | `--no-updates`         | Disable updates                     |
| **Netdata Cloud**       | `--claim-token`        | Provide claim token                 |
|                         | `--claim-rooms`        | Assign node to specific Cloud Rooms |
| **Reinstall/Uninstall** | `--reinstall`          | Reinstall existing Netdata          |
|                         | `--uninstall`          | Uninstall Netdata completely        |

## Environment Variables

These environment variables provide additional customization options (most users won't need these):

| Variable            | Purpose                                      | Default Behavior                            |
|---------------------|----------------------------------------------|---------------------------------------------|
| `TMPDIR`            | Specify directory for temporary files        | System default temp directory               |
| `ROOTCMD`           | Command to run with root privileges          | Uses `sudo`, `doas`, or `pkexec` (in order) |
| `DISABLE_TELEMETRY` | Disable telemetry when set to non-zero value | Telemetry enabled                           |

:::note

The user running the script needs write and execute permissions in the temporary directory specified by TMPDIR.

:::

## Verify Script Integrity

Before running the installation script, you can verify its integrity using the following command:

```bash
[ "@KICKSTART_CHECKSUM@" = "$(curl -Ss https://get.netdata.cloud/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`. We recommend verifying script integrity before installation, especially in production environments.

## Notes & Best Practices

- Stop the Agent with `sudo systemctl stop netdata` before reinstalling
- Customize install location or behavior with flags
- Always verify the downloaded script for security
- Use the `--non-interactive` flag in CI/CD pipelines

## Troubleshooting

### SSL certificate verification fails during the one-line download

When you run the one-line install command, `curl` or `wget` may fail to download `kickstart.sh` or the install artifacts from `https://get.netdata.cloud` or GitHub releases because your system cannot validate the remote server's TLS certificate. This is a **download-phase** error — it happens *before* Netdata is installed.

The exact message depends on your HTTP client. With `curl`, the common form is:

```text
curl: (60) SSL certificate problem: unable to get local issuer certificate
```

Some `curl` builds report the underlying OpenSSL verify result with its numeric code:

```text
curl: (60) SSL certificate OpenSSL verify result: unable to get local issuer certificate (20)
```

The `(20)` is OpenSSL's verify result code, meaning the local trust store has no certificate that chains back to the issuer of the server's certificate. With `wget`, the same condition looks like:

```text
ERROR: cannot verify get.netdata.cloud's certificate, issued by '...': Unable to locally verify the issuer's authority.
To connect to get.netdata.cloud insecurely, you can use `--no-check-certificate'.
```

It occurs when the system's CA certificate store cannot validate the remote certificate. Common causes:

- The `ca-certificates` package is **missing or outdated** (common on minimal containers, freshly provisioned VMs, or older distributions that no longer receive trust-root updates).
- The host is **older or air-gapped** and never received updated trust roots.
- A **TLS-inspecting proxy, firewall, or MITM appliance** re-signs traffic with its own root CA, which is not present in the host's trust store.

#### Fix 1: Update or reinstall the system CA certificate store

Refresh the system CA bundle, then rebuild the trust store:

| Distribution family    | Refresh the CA package                                       | Rebuild the trust store               |
|------------------------|--------------------------------------------------------------|---------------------------------------|
| Debian / Ubuntu        | `sudo apt-get install --reinstall -y ca-certificates`        | `sudo update-ca-certificates`         |
| RHEL / Fedora / CentOS | `sudo dnf reinstall -y ca-certificates`                      | `sudo update-ca-trust`                |
| openSUSE / SLES        | `sudo zypper install -f ca-certificates-mozilla`             | `sudo update-ca-certificates`         |
| Alpine                 | `sudo apk add --force-reinstall ca-certificates`             | `sudo update-ca-certificates --fresh` |
| Arch Linux             | `sudo pacman -S --noconfirm ca-certificates`                 | `sudo update-ca-trust`                |

Then re-run the one-line install command. If your distribution is not listed, consult its documentation for the equivalent package and trust-store rebuild command.

#### Fix 2: Use the offline installer on air-gapped hosts

Refreshing the CA store requires internet access. If the target host has no connectivity, do not attempt the download-based install — use the [Offline Installation Guide](/packaging/installer/methods/offline.md) to prepare a self-contained install source on an online machine and transfer it to the offline host.

#### Fix 3: Install a TLS-inspecting proxy's root CA

If your network uses a proxy, firewall, or security appliance that intercepts and re-signs TLS connections, install that appliance's root CA certificate into the system trust store so the host can validate the re-signed certificates. See [Using custom CA certificates with Netdata](/docs/netdata-agent/configuration/using-custom-ca-certificates-with-netdata.md) for per-distribution instructions on installing a certificate in the system certificate store.

:::note

**This is a download-phase error, not an agent-runtime trust issue.** It occurs while *downloading* `kickstart.sh` and its artifacts. It is separate from configuring TLS trust for the **running Netdata Agent** (streaming to a Parent, exporting metrics, or collecting from secure endpoints). For agent-runtime TLS trust configuration, see [Using custom CA certificates with Netdata](/docs/netdata-agent/configuration/using-custom-ca-certificates-with-netdata.md).

:::

## Related Docs

- [Connect to Netdata Cloud](/src/claim/README.md)
- [Release Channels & Versions](/packaging/PLATFORM_SUPPORT.md)
- [Uninstall Guide](/packaging/installer/UNINSTALL.md)
- [Offline Installation Guide](/packaging/installer/methods/offline.md)
