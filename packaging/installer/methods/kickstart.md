import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'
import { Install, InstallBox } from '@site/src/components/Install/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with kickstart.sh

![last hour badge](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=kickstart%20downloads&precision=0) ![today badge](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=kickstart%20downloads&precision=0)

**`kickstart.sh` is the recommended way of installing Netdata.**

This script works on all Linux distributions and macOS environments, by detecting the optimal method of installing Netdata directly to the operating system.

## What does `kickstart.sh` do?

The `kickstart.sh` script does the following after being downloaded and run using `sh`:

- Determines what platform you’re running on.
- Checks for an existing installation, and if found updates that instead of creating a new installation.
- Attempts to install Netdata using our [official native binary packages](/packaging/installer/methods/packages.md).
- If there are no official native binary packages for your system (or installing that way failed), tries to install using a [static build of Netdata](/packaging/makeself/README.md) if one is available.
- If no static build is available, installs required dependencies and then attempts to install by building Netdata locally (by downloading the sources and building them directly).
- Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated with new nightly versions, unless you override that with an [optional parameter](#optional-parameters-to-alter-your-installation).
- Prints a message whether installation succeeded or failed for QA purposes.

## Installation

> **Tip**
>
> If you are unsure whether you want nightly or stable releases, read the [related section](/docs/netdata-agent/versions-and-platforms.md) of our Documentation, detailing the pros and cons of each release type.

To install Netdata, run the following as your normal user:

<Tabs>
  <TabItem value="wget" label="wget">

  <OneLineInstallWget/>

  </TabItem>
  <TabItem value="curl" label="curl">

  <OneLineInstallCurl/>

  </TabItem>
</Tabs>

> **Note**
>
> If you plan to also connect the node to Netdata Cloud, make sure to replace `YOUR_CLAIM_TOKEN` with the claim token of your space,
> and `YOUR_ROOM_ID` with the ID of the Room you’re willing to connect the node to.

## Optional parameters to alter your installation

The `kickstart.sh` script accepts a number of optional parameters to control how the installation process works:

### destination directory

- `--install-prefix`
  Specify a custom installation directory for local builds. If not provided, a default directory will be used based on your system.
- `--old-install-prefix`
  Specify the previous custom installation directory to be removed during the update process.

### interactivity

The script automatically detects if it is running interactively, on a user's terminal, or headless in a CI/CD environment. These are options related to overriding this behavior.

- `--non-interactive` or `--dont-wait`
  Don’t prompt for anything and assume yes whenever possible, overriding any automatic detection of an interactive run. Use this option when installing Netdata Agent with a provisioning tool or in CI/CD.
- `--interactive`
   Act as if running interactively, even if automatic detection indicates a run is non-interactive.

### release channel

By default, the script installs the nightly channel of Netdata, providing you with the most recent Netdata. For production systems where stability is more important than new features, we recommend using the stable channel.

- `--release-channel`
  Specify a particular release channel to install from. Currently supported release channels are:
  - `nightly`: Installs a nightly build (this is currently the default).
  - `stable`: Installs a stable release.
  - `default`: Explicitly request whatever the current default is.
- `--nightly-channel`
  Synonym for `--release-channel nightly`.
- `--stable-channel`
  Synonym for `--release-channel stable`.
- `--install-version`
  Specify the exact version of Netdata to install.

### install type

By default, the script will prefer native builds when they’re available, and then static builds. It will fallback to build from source when all others aren’t available.

- `--native-only`
   Only install if native binary packages are available. It fails otherwise.
- `--static-only`
  Only install if a static build is available. It fails otherwise.
   When installing a static build, the parameter `--static-install-options` can provide additional options to pass to the static installer code.
- `--build-only`
  Only install using a local build. It fails otherwise.
  When it builds from source, the parameter `--local-build-options` can be used to give additional build options.

### automatic updates

By default, the script installs a cron job to automatically update Netdata to the latest version of the release channel used.

- `--auto-update`
  Enable automatic updates (this is the default).
- `--no-updates`
  Disable automatic updates (not recommended).

### Netdata Cloud related options

By default, the kickstart script will provide a Netdata Agent installation that can potentially communicate with Netdata Cloud if the Netdata Agent is further configured to do so.

- `--claim-token`
  Specify a unique claiming token associated with your Space in Netdata Cloud to be used to connect to the node after the installation. This will connect and connect the Netdata Agent to Netdata Cloud.
- `--claim-url`
  Specify a URL to use when connecting to the Cloud. Defaults to `https://app.netdata.cloud`. Use this option to change the Netdata Cloud URL to point to your Netdata Cloud installation.
- `--claim-rooms`
  Specify a comma-separated list of tokens for each Room this node should appear in.
- `--claim-proxy`
  Specify a proxy to use when connecting to the Cloud in the form of `http://[user:pass@]host:ip` for an HTTP(S) proxy. See [connecting through a proxy](/src/claim/README.md#automatically-via-a-provisioning-system-or-the-command-line) for details.
- `--claim-only`
  If there is an existing installation, only try to connect it without attempting to update it. If there is no existing installation, install and connect Netdata normally.

### anonymous telemetry

By default, the Agent is sending anonymous telemetry data to help us take identify the most common operating systems and the configurations Netdata Agents run. We use this information to prioritize our efforts towards what is most commonly used by our community.

- `--disable-telemetry`
  Disable anonymous statistics.

### reinstalling

- `--reinstall`
  If there is an existing installation, reinstall it instead of trying to update it. If there is not an existing installation, install netdata normally.
- `--reinstall-even-if-unsafe`
  If there is an existing installation, reinstall it instead of trying to update it, even if doing so is known to potentially break things (for example, if we can’t detect what type of installation it is). If there is not an existing install, install Netdata normally.
- `--reinstall-clean`
  If there is an existing installation, uninstall it before trying to install Netdata. Fails if there is no existing installation.

### uninstall

- `--uninstall`
  Uninstall an existing installation of Netdata. Fails if there is no existing install.

### other options

- `--dry-run`
  Simulates the installation process without making any changes to your system. This allows you to review the steps and potential impacts before proceeding with the actual installation.
- `--dont-start-it`
  Don’t auto-start the daemon after installing. This parameter is not guaranteed to work.
- `--distro-override`
  Override the distro detection logic and assume the system is using a specific Linux distribution and release. Takes a single argument consisting of the values of the `ID`, `VERSION_ID`, and `VERSION_CODENAME` fields from `/etc/os-release` for the desired distribution.

The following options are mutually exclusive and specify special operations other than trying to install Netdata normally or update an existing install:

- `--repositories-only`
  Only install repository configuration packages instead of doing a full install of Netdata. Automatically sets --native-only.
- `--prepare-offline-install-source`
  Instead of installing the Agent, prepare a directory that can be used to install on another system without needing to download anything. See our [offline installation documentation](/packaging/installer/methods/offline.md) for more info.

### environment variables

Additionally, the following environment variables may be used to further customize how the script runs (most users
shouldn’t need to use special values for any of these):

- `TMPDIR`: Used to specify where to put temporary files. On most systems, the default we select automatically
  should be fine. The user running the script needs to both be able to write files to the temporary directory,
  and run files from that location.
- `ROOTCMD`: Used to specify a command to use to run another command with root privileges if needed. By default,
  we try to use sudo, doas, or pkexec (in that order of preference). However, if you need special options for one of
  those to work, or have a different tool to do the same thing on your system, you can specify it here.
- `DISABLE_TELEMETRY`: If set to a value other than 0, behave as if `--disable-telemetry` was specified.

## Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "@KICKSTART_CHECKSUM@" = "$(curl -Ss https://get.netdata.cloud/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.
