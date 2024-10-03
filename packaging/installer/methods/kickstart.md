import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'
import { Install, InstallBox } from '@site/src/components/Install/'
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Install Netdata with kickstart.sh

![last hour badge](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=kickstart%20downloads&value_color=orange&precision=0) ![today badge](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=kickstart%20downloads&precision=0)

`kickstart.sh` is the recommended way of installing Netdata.

This script works on all Linux distributions and macOS environments, by detecting the optimal method of installing Netdata directly to the operating system.

## Installation

> :bulb: Tip
>
> If you are unsure whether you want nightly or stable releases, read the [related section](/packaging/installer/README.md#nightly-vs-stable-releases) of our Documentation, detailing the pros and cons of each release type.

To install Netdata, run the following as your normal user:

<Tabs>
  <TabItem value="wget" label="wget">

  <OneLineInstallWget/>

  </TabItem>
  <TabItem value="curl" label="curl">

  <OneLineInstallCurl/>

  </TabItem>
</Tabs>

> :bookmark_tabs: Note
>
> If you plan to also connect the node to Netdata Cloud, make sure to replace `YOUR_CLAIM_TOKEN` with the claim token of your space,
> and `YOUR_ROOM_ID` with the ID of the Room you are willing to connect the node to.

## Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "@KICKSTART_CHECKSUM@" = "$(curl -Ss https://get.netdata.cloud/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## What does `kickstart.sh` do?

The `kickstart.sh` script does the following after being downloaded and run using `sh`:

- Determines what platform you are running on.
- Checks for an existing installation, and if found updates that instead of creating a new install.
- Attempts to install Netdata using our [official native binary packages](#native-packages).
- If there are no official native binary packages for your system (or installing that way failed), tries to install
  using a [static build of Netdata](#static-builds) if one is available.
- If no static build is available, installs required dependencies and then attempts to install by
  [building Netdata locally](#local-builds) (by downloading the sources and building them directly).
- Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated with new nightly
  versions, unless you override that with an [optional parameter](#optional-parameters-to-alter-your-installation).
- Prints a message whether installation succeeded or failed for QA purposes.

## Start stop or restart the Netdata Agent

You will most often need to _restart_ the Agent to load new or edited configuration files.

> **Note**  
> Stopping or restarting the Netdata Agent will cause gaps in stored metrics until the `netdata` process initiates collectors and the database engine.
>
> You do not need to restart the Netdata Agent between changes to health configuration files, see the relevant section on [reloading health configuration](/src/health/REFERENCE.md#reload-health-configuration).

### Using `systemctl` or `service`

This is the recommended way to start, stop, or restart the Netdata daemon.

- To **start** Netdata, run `sudo systemctl start netdata`.
- To **stop** Netdata, run `sudo systemctl stop netdata`.
- To **restart** Netdata, run `sudo systemctl restart netdata`.

If the above commands fail, or you know that you're using a non-systemd system, try using the `service` command:

- Starting: `sudo service netdata start`.
- Stopping: `sudo service netdata stop`.
- Restarting: `sudo service netdata restart`.

### Using the `netdata` command

Use the `netdata` command, typically located at `/usr/sbin/netdata`, to start the Netdata daemon:

```bash
sudo netdata
```

If you start the daemon this way, close it with `sudo killall netdata`.

### Shutdown using `netdatacli`

The Netdata Agent also comes with a [CLI tool](/src/cli/README.md) capable of performing shutdowns. Start the Agent back up using your preferred method listed above.

```bash
sudo netdatacli shutdown-agent
```

## Starting Netdata at boot

In the `system` directory you can find scripts and configurations for the
various distros.

### systemd

The installer already installs `netdata.service` if it detects a systemd system.

To install `netdata.service` by hand, run:

```sh
# stop Netdata
killall netdata

# copy netdata.service to systemd
cp system/netdata.service /etc/systemd/system/

# let systemd know there is a new service
systemctl daemon-reload

# enable Netdata at boot
systemctl enable netdata

# start Netdata
systemctl start netdata
```

### init.d

In the system directory you can find `netdata-lsb`. Copy it to the proper place according to your distribution's documentation. For Ubuntu, this can be done via running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-lsb /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
update-rc.d netdata defaults
```

### openrc / Gentoo Linux

In the `system` directory you can find `netdata-openrc`. Copy it to the proper
place according to your distribution documentation.

### CentOS / Red Hat Enterprise Linux

For older versions of RHEL/CentOS that don't have systemd, an init script is included in the system directory. This can be installed by running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-init-d /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
chkconfig --add netdata
```

_There have been some recent work on the init script, see the following PR <https://github.com/netdata/netdata/pull/403>_

### Other operating systems

You can start Netdata by running it from `/etc/rc.local` or your system's equivalent.

## Optional parameters to alter your installation

The `kickstart.sh` script accepts a number of optional parameters to control how the installation process works:

### destination directory

- `--install-prefix`
  Specify an installation prefix for local builds (by default, we use a sane prefix based on the type of system).
- `--old-install-prefix`
  Specify the custom local build's installation prefix that should be removed.

### interactivity

The script automatically detects if it is running interactively, on a user's terminal, or headless in a CI/CD environment. These are options related to overriding this behavior.

- `--non-interactive` or `--dont-wait`
  Don’t prompt for anything and assume yes whenever possible, overriding any automatic detection of an interactive run. Use this option when installing Netdata agent with a provisioning tool or in CI/CD.
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

By default the script will prefer native builds when they are available, and then static builds. It will fallback to build from source when all others are not available.

- `--native-only`
   Only install if native binary packages are available. It fails otherwise.
- `--static-only`
  Only install if a static build is available. It fails otherwise.
   When installing a static build, the parameter `--static-install-options` can provide additional options to pass to the static installer code.
- `--build-only`
  Only install using a local build. It fails otherwise.
  When it builds from source, the parameter `--local-build-options` can be used to give additional build options.

### automatic updates

By default the script installs a cron job to automatically update Netdata to the latest version of the release channel used.

- `--auto-update`
  Enable automatic updates (this is the default).
- `--no-updates`
  Disable automatic updates (not recommended).

### Netdata Cloud related options

By default, the kickstart script will provide a Netdata agent installation that can potentially communicate with Netdata Cloud, if of course the Netdata agent is further configured to do so.

- `--claim-token`
  Specify a unique claiming token associated with your Space in Netdata Cloud to be used to connect to the node after the install. This will enable, connect and claim the Netdata agent, to Netdata Cloud.
- `--claim-url`
  Specify a URL to use when connecting to the cloud. Defaults to `https://app.netdata.cloud`. Use this option to change the Netdata Cloud URL to point to your Netdata Cloud installation.
- `--claim-rooms`
  Specify a comma-separated list of tokens for each Room this node should appear in.
- `--claim-proxy`
  Specify a proxy to use when connecting to the cloud in the form of `http://[user:pass@]host:ip` for an HTTP(S) proxy. See [connecting through a proxy](/src/claim/README.md#automatically-via-a-provisioning-system-or-the-command-line) for details.
- `--claim-only`
  If there is an existing install, only try to claim it without attempting to update it. If there is no existing install, install and claim Netdata normally.

### anonymous telemetry

By default, the agent is sending anonymous telemetry data to help us take identify the most common operating systems and the configurations Netdata agents run. We use this information to prioritize our efforts towards what is most commonly used by our community.

- `--disable-telemetry`
  Disable anonymous statistics.

### reinstalling

- `--reinstall`
  If there is an existing install, reinstall it instead of trying to update it. If there is not an existing install, install netdata normally.
- `--reinstall-even-if-unsafe`
  If there is an existing install, reinstall it instead of trying to update it, even if doing so is known to potentially break things (for example, if we cannot detect what type of installation it is). If there is not an existing install, install Netdata normally.
- `--reinstall-clean`
  If there is an existing install, uninstall it before trying to install Netdata. Fails if there is no existing install.

### uninstall

- `--uninstall`
  Uninstall an existing installation of Netdata. Fails if there is no existing install.

### other options

- `--dry-run`
  Show what the installer would do, but don’t actually do any of it.
- `--dont-start-it`
  Don’t auto-start the daemon after installing. This parameter is not guaranteed to work.
- `--distro-override`
  Override the distro detection logic and assume the system is using a specific Linux distribution and release. Takes a single argument consisting of the values of the `ID`, `VERSION_ID`, and `VERSION_CODENAME` fields from `/etc/os-release` for the desired distribution.

The following options are mutually exclusive and specify special operations other than trying to install Netdata normally or update an existing install:

- `--repositories-only`
  Only install repository configuration packages instead of doing a full install of Netdata. Automatically sets --native-only.
- `--prepare-offline-install-source`
  Instead of installing the agent, prepare a directory that can be used to install on another system without needing to download anything. See our [offline installation documentation](/packaging/installer/methods/offline.md) for more info.

### environment variables

Additionally, the following environment variables may be used to further customize how the script runs (most users
should not need to use special values for any of these):

- `TMPDIR`: Used to specify where to put temporary files. On most systems, the default we select automatically
  should be fine. The user running the script needs to both be able to write files to the temporary directory,
  and run files from that location.
- `ROOTCMD`: Used to specify a command to use to run another command with root privileges if needed. By default
  we try to use sudo, doas, or pkexec (in that order of preference), but if you need special options for one of
  those to work, or have a different tool to do the same thing on your system, you can specify it here.
- `DISABLE_TELEMETRY`: If set to a value other than 0, behave as if `--disable-telemetry` was specified.

## Native packages

We publish [official DEB/RPM packages](/packaging/installer/methods/packages.md) for a number of common Linux distributions as part of our releases and nightly
builds. These packages are available for 64-bit x86 systems. Depending on the distribution and release they may
also be available for 32-bit x86, ARMv7, and AArch64 systems. If a native package is available, it will be used as the
default installation method. This allows you to handle Netdata updates as part of your usual system update procedure.

If you want to enforce the usage of native packages and have the installer return a failure if they are not available,
you can do so by adding `--native-only` to the options you pass to the installer.

## Static builds

We publish pre-built [static builds](/packaging/makeself/README.md) of Netdata for Linux systems. Currently, these are published for 64-bit x86, ARMv7,
AArch64, and POWER8+ hardware. These static builds are able to operate in a mostly self-contained manner and only
require a POSIX compliant shell and a supported init system. These static builds install under `/opt/netdata`. If
you are on a platform which we provide static builds for but do not provide native packages for, a static build
will be used by default for installation.

If you want to enforce the usage of a static build and have the installer return a failure if one is not available,
you can do so by adding `--static-only` to the options you pass to the installer.

## Local builds

For systems which do not have available native packages or static builds, we support building Netdata locally on
the system it will be installed on. When using this approach, the installer will attempt to install any required
dependencies for building Netdata, though this may not always work correctly.

If you want to enforce the usage of a local build (perhaps because you require a custom installation prefix,
which is not supported with native packages or static builds), you can do so by adding `--build-only` to the
options you pass to the installer.
