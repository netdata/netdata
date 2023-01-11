<!--
title: "Install Netdata with kickstart.sh"
description: "The kickstart.sh script installs Netdata from source, including all dependencies required to connect to Netdata Cloud, with a single command."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kickstart.md
-->
import { OneLineInstallWget, OneLineInstallCurl } from '@site/src/components/OneLineInstall/'

# Install Netdata with kickstart.sh

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=kickstart%20downloads&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_by_url_pattern&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=kickstart%20downloads&precision=0)

This page covers detailed instructions on using and configuring the automatic one-line installation script named
`kickstart.sh`.

The kickstart script works on all Linux distributions and macOS environments. By default, automatic nightly updates are enabled. If you are installing on macOS, make sure to check the [install documentation for macOS](macos.md) before continuing.

> If you are unsure whether you want nightly or stable releases, read the [installation guide](/packaging/installer/README.md#nightly-vs-stable-releases). 
> If you want to turn off [automatic updates](/packaging/installer/README.md#automatic-updates), use the `--no-updates` option. You can find more installation options below.

To install Netdata, run the following as your normal user:

<OneLineInstallWget/>

Or, if you have cURL but not wget (such as on macOS):

<OneLineInstallCurl/>


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

## Optional parameters to alter your installation

The `kickstart.sh` script accepts a number of optional parameters to control how the installation process works:

- `--non-interactive`: Don’t prompt for anything and assume yes whenever possible, overriding any automatic detection of an interactive run.
- `--interactive`: Act as if running interactively, even if automatic detection indicates a run is non-interactive.
- `--dont-wait`: Synonym for `--non-interactive`
- `--dry-run`: Show what the installer would do, but don’t actually do any of it.
- `--dont-start-it`: Don’t auto-start the daemon after installing. This parameter is not guaranteed to work.
- `--release-channel`: Specify a particular release channel to install from. Currently supported release channels are:
    - `nightly`: Installs a nightly build (this is currently the default).
    - `stable`: Installs a stable release.
    - `default`: Explicitly request whatever the current default is.
- `--nightly-channel`: Synonym for `--release-channel nightly`.
- `--stable-channel`: Synonym for `--release-channel stable`.
- `--auto-update`: Enable automatic updates (this is the default).
- `--no-updates`: Disable automatic updates.
- `--disable-telemetry`: Disable anonymous statistics.
- `--repositories-only`: Only install appropriate repository configuration packages (only for native install).
- `--native-only`: Only install if native binary packages are available.
- `--static-only`: Only install if a static build is available.
- `--build-only`: Only install using a local build.
- `--reinstall`: If an existing install is found, reinstall instead of trying to update it in place.
- `--reinstall-even-if-unsafe`: Even try to reinstall if we don't think we can do so safely (implies `--reinstall`).
- `--disable-cloud`: For local builds, don’t build any of the cloud code at all. For native packages and static builds,
    use runtime configuration to disable cloud support.
- `--require-cloud`: Only install if Netdata Cloud can be enabled. Overrides `--disable-cloud`.
- `--install-prefix`: Specify an installation prefix for local builds (by default, we use a sane prefix based on the type of system).
- `--install-version`: Specify the version of Netdata to install.
- `--old-install-prefix`: Specify the custom local build's installation prefix that should be removed.
- `--uninstall`: Uninstall an existing installation of Netdata.
- `--reinstall-clean`: Performs an uninstall of Netdata and clean installation.
- `--local-build-options`: Specify additional options to pass to the installer code when building locally. Only valid if `--build-only` is also specified.
- `--static-install-options`: Specify additional options to pass to the static installer code. Only valid if --static-only is also specified.
- `--prepare-offline-install-source`: Instead of insallling the agent, prepare a directory that can be used to install on another system without needing to download anything. See our [offline installation documentation](/packaging/installer/methods/offline.md) for more info.

Additionally, the following environment variables may be used to further customize how the script runs (most users
should not need to use special values for any of these):

- `TMPDIR`: Used to specify where to put temporary files. On most systems, the default we select automatically
  should be fine. The user running the script needs to both be able to write files to the temporary directory,
  and run files from that location.
- `ROOTCMD`: Used to specify a command to use to run another command with root privileges if needed. By default
  we try to use sudo, doas, or pkexec (in that order of preference), but if you need special options for one of
  those to work, or have a different tool to do the same thing on your system, you can specify it here.
- `DISABLE_TELEMETRY`: If set to a value other than 0, behave as if `--disable-telemetry` was specified.

### Connect node to Netdata Cloud during installation

The `kickstart.sh` script accepts additional parameters to automatically [connect](/claim/README.md) your node to Netdata Cloud immediately after installation. 

> Note: You either need to run the command with root privileges or run it with the user that is running the agent.  More details: [Connect an agent without root privileges](/claim/README.md#connect-an-agent-without-root-privileges) section.

To automatically claim nodes after installation: 

1. Sign in to [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=/spaces)
2. Go to the [Spaces management area](https://learn.netdata.cloud/docs/cloud/spaces#manage-spaces)
3. Click on **Connect Nodes**
4. Find the `token` and `rooms` strings and specify your nodes:

- `--claim-token`: Specify a unique claiming token associated with your Space in Netdata Cloud to be used to connect to the node
  after the install.
- `--claim-rooms`: Specify a comma-separated list of tokens for each War Room this node should appear in.
- `--claim-proxy`: Specify a proxy to use when connecting to the cloud in the form of `http://[user:pass@]host:ip` for an HTTP(S) proxy.
  See [connecting through a proxy](/claim/README.md#connect-through-a-proxy) for details.
- `--claim-url`: Specify a URL to use when connecting to the cloud. Defaults to `https://api.netdata.cloud`.

For example:

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --claim-token TOKEN --claim-rooms ROOM1,ROOM2
```

### Native packages

We publish official DEB/RPM packages for a number of common Linux distributions as part of our releases and nightly
builds. These packages are available for 64-bit x86 systems. Depending on the distribution and release they may
also be available for 32-bit x86, ARMv7, and AArch64 systems. If a native package is available, it will be used as the
default installation method. This allows you to handle Netdata updates as part of your usual system update procedure.

If you want to enforce the usage of native packages and have the installer return a failure if they are not available,
you can do so by adding `--native-only` to the options you pass to the installer.

### Static builds

We publish pre-built static builds of Netdata for Linux systems. Currently, these are published for 64-bit x86, ARMv7,
AArch64, and POWER8+ hardware. These static builds are able to operate in a mostly self-contained manner and only
require a POSIX compliant shell and a supported init system. These static builds install under `/opt/netdata`. If
you are on a platform which we provide static builds for but do not provide native packages for, a static build
will be used by default for installation.

If you want to enforce the usage of a static build and have the installer return a failure if one is not available,
you can do so by adding `--static-only` to the options you pass to the installer.

### Local builds

For systems which do not have available native packages or static builds, we support building Netdata locally on
the system it will be installed on. When using this approach, the installer will attempt to install any required
dependencies for building Netdata, though this may not always work correctly.

If you want to enforce the usage of a local build (perhaps because you require a custom installation prefix,
which is not supported with native packages or static builds), you can do so by adding `--build-only` to the
options you pass to the installer.


## Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "<checksum-will-be-added-in-documentation-processing>" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.


