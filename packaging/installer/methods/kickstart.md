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

:::note

**Managing auto-updates after installation:** `kickstart.sh` enables daily auto-updates by default. To disable, check, or re-enable auto-updates — including why `netdata-updater.conf` cannot disable them — see [Managing Automatic Updates](/packaging/installer/UPDATE.md#managing-automatic-updates) in the Update guide.

:::

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

If `curl` fails to download the install script with `curl: (60) SSL certificate OpenSSL verify result: unable to get local issuer certificate (20)`, this is an OS-level certificate-trust issue: the host's CA certificate store is missing or outdated. Update your system CA certificates using your package manager (for example `sudo apt-get install --reinstall ca-certificates` on Debian/Ubuntu or `sudo dnf reinstall ca-certificates` on RHEL/Fedora), then re-run the install command.

## Related Docs

- [Connect to Netdata Cloud](/src/claim/README.md)
- [Release Channels & Versions](/packaging/PLATFORM_SUPPORT.md)
- [Uninstall Guide](/packaging/installer/UNINSTALL.md)
- [Offline Installation Guide](/packaging/installer/methods/offline.md)
