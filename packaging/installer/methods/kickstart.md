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
| **Synthetic monitoring** | `--install-dem-runtime` | Explicitly install the optional DEM browser runtime |
| **Netdata Cloud**       | `--claim-token`        | Provide claim token                 |
|                         | `--claim-rooms`        | Assign node to specific Cloud Rooms |
| **Reinstall/Uninstall** | `--reinstall`          | Reinstall existing Netdata          |
|                         | `--uninstall`          | Uninstall Netdata completely        |

:::note

**Managing auto-updates after installation:** `kickstart.sh` enables daily auto-updates by default. To disable, check, or re-enable auto-updates — including why `netdata-updater.conf` cannot disable them — see [Managing Automatic Updates](/packaging/installer/UPDATE.md#managing-automatic-updates) in the Update guide.

:::

## Optional browser runtime for synthetic monitoring

Add `--install-dem-runtime` to an ordinary kickstart install or update to prepare the browser runtime used by DEM journeys and Lighthouse. Without this option, kickstart does not acquire a browser runtime. The option also works with an existing installation supported by kickstart.

```sh
sudo sh ./kickstart.sh --install-dem-runtime
```

For native package installations, kickstart installs the optional `netdata-dem-runtime` package using the same package manager and interaction policy as the Agent. Native runtime packages require the named `netdata` service account. Package hooks qualify the runtime and may restart a previously running Agent. If package installation or runtime readiness fails, kickstart reports failure; it does not switch to managed delivery.

For static and source installations, kickstart runs the installed `dem.plugin runtime install --allow-download` as the configured nonroot Netdata service user. It reads the installed Agent's normal configuration to find its plugin, administrator/stock configuration and persistent-state directories, including custom installation prefixes. The CLI uses the configured publisher URL and verification keys; kickstart does not provide a substitute trust key or release URL.

This managed action downloads signed release metadata and the browser archive, then enables automatic daily preparation of future releases. **An operator must restart the Agent to activate the prepared runtime.** Kickstart does not restart it for managed runtime activation. The Agent's `--no-updates` option controls Agent updates separately; to disable automatic runtime preparation, run the runtime CLI with `install --allow-download --manual-updates` as the service user. See [DEM runtime installation](/src/go/plugin/dem/README.md#runtime-setup-and-retirement) for CLI and offline setup.

The browser runtime requires Linux `x86_64` or `aarch64`, an Agent containing the runtime CLI, and a host where browser sandboxing is allowed. Native package qualification requires a running systemd host. Missing publisher configuration, an unavailable optional package, or failed browser qualification makes explicit setup fail even if Agent installation succeeded. Kickstart does not inspect live service `-c`/`-u` arguments or environment overrides. Such custom launch setups, and plugin paths requiring escaping, must use the runtime CLI directly with the same service identity and configuration as the Agent.

`--install-dem-runtime` cannot be combined with `--offline-install-source`, `--prepare-offline-install-source`, `--repositories-only`, `--claim-only`, or `--uninstall`. Use the signed offline runtime CLI or your package manager for offline runtime installation. `--dry-run` reports the intended action without acquiring or qualifying a runtime.

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

If you see a message like `File not found when checking for remote file at https://repository.netdata.cloud/repos/repoconfig` while running `kickstart.sh`, it is an informational notice. It means a remote file check during installation did not succeed, but this does not stop or break the installation — the installer continues automatically to the next step. The message is not specific to any particular Linux distribution and can appear on any supported system.

### Switching from a vendor-packaged native install

Systems such as XCP-ng and XOA ship Netdata through their own package repositories, and these bundled packages are often older than the latest release. `kickstart.sh` does not recognize these installs and is not designed to safely update, reinstall, or uninstall them. See the "Switching From a Vendor-Packaged Native Install" section under [Switching Install Types](/docs/learn/switching-install-types.md#switching-install-types) for the full procedure to remove the existing package and switch to a static build or another install type.

## Related Docs

- [Connect to Netdata Cloud](/src/claim/README.md)
- [Release Channels & Versions](/packaging/PLATFORM_SUPPORT.md)
- [Uninstall Guide](/packaging/installer/UNINSTALL.md)
- [Offline Installation Guide](/packaging/installer/methods/offline.md)
