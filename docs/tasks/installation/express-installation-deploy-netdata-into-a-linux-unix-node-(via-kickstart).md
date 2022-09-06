<!--
Title: "Express installation, deploy Netdata into a linux/unix node (via kickstart)"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/installation/express-installation--deploy-netdata-into-a-linux-unix-node-(via-kickstart).md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "installation"
learn_docs_purpose: "Instructions on running the kickstart script on Unix systems."
-->

import { OneLineInstallWget, OneLineInstallCurl } from '../../../src/components/OneLineInstall/'

This page will guide you through installation using the automatic one-line installation script named `kickstart.sh`.

:::info
The kickstart script works on all Linux distributions and, by default, automatic nightly updates are enabled.
:::

## What does `kickstart.sh` do?

The installation script does the following after being downloaded and run using `sh`:

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

## Prerequisites

- Connection to the internet
- A Linux based node
- Either `wget` or `curl` installed on the node  

## Steps

To install Netdata, run the following:

<OneLineInstallWget/>

Or, if you have cURL but not wget:

<OneLineInstallCurl/>

### Optional parameters to alter your installation

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
- `--install`: Specify an installation prefix for local builds (by default, we use a sane prefix based on the type of system), this option is deprecated and will be removed in a future version, please use `--install-prefix` instead.
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

### Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "<checksum-will-be-added-in-documentation-processing>" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## Expected result

The script should exit with a success message.  
To ensure that your installation is working, open up your web browser of choice and navigate to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your node.  
If you're interacting with the node locally and you are unsure of it's IP address, try `http://localhost:19999` first.

If the installation was successful, you will be led to the Agent's local dashboard. Enjoy!

## Example

Here we will install Netdata from the stable release channel:

```bash
root@netdata~ # wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --stable-channel
```

## Related topics

1. [Claim an Agent to the Hub](/docs/tasks/general-configuration/claim-an-agent-to-the-hub.md)
2. [Configure the Agent](/docs/tasks/general-configuration/configure-the-agent.md)
