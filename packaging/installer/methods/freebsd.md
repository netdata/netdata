<!--
title: "Install Netdata on FreeBSD"
description: "Install Netdata on FreeBSD to monitor the health and performance of bare metal or VMs with thousands of real-time, per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md
sidebar_label: "FreeBSD"
learn_status: "Published"
learn_rel_path: "Installation/Install on specific environments"
-->

# Install Netdata on FreeBSD

> 💡 This document is maintained by Netdata's community, and may not be completely up-to-date. Please double-check the
> details of the installation process, such as version numbers for downloadable packages, before proceeding.
>
> You can help improve this document by [submitting a
> PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/freebsd.md) with your recommended
> improvements or changes. Thank you!

## Install latest version

This is how to install the latest Netdata version on FreeBSD:

Install required packages (**need root permission**):

```sh
pkg install bash e2fsprogs-libuuid git curl autoconf automake pkgconf pidof liblz4 libuv json-c cmake gmake
```

Download Netdata:

```sh
fetch https://github.com/netdata/netdata/releases/download/v1.26.0/netdata-v1.26.0.tar.gz
```

> ⚠️ Verify the latest version by either navigating to [Netdata's latest
> release](https://github.com/netdata/netdata/releases/latest) or using `curl`:
>
> ```bash
> basename $(curl -Ls -o /dev/null -w %{url_effective} https://github.com/netdata/netdata/releases/latest)
> ```

Unzip the downloaded file:

```sh
gunzip netdata*.tar.gz && tar xf netdata*.tar && rm -rf netdata*.tar
```

Install Netdata in `/opt/netdata`. If you want to enable automatic updates, add `--auto-update` or `-u` to install `netdata-updater` in `cron` (**need root permission**):

```sh
cd netdata-v* && ./netdata-installer.sh --install-prefix /opt && cp /opt/netdata/usr/sbin/netdata-claim.sh /usr/sbin/
```

You also need to enable the `netdata` service in `/etc/rc.conf`:

```sh
sysrc netdata_enable="YES"
```

Finally, and very importantly, update Netdata using the script provided by the Netdata team (**need root permission**):

```sh
cd /opt/netdata/usr/libexec/netdata/ && ./netdata-updater.sh
```

You can now access the Netdata dashboard by navigating to `http://NODE:19999`, replacing `NODE` with the IP address or hostname of your system.

![image](https://user-images.githubusercontent.com/2662304/48304090-fd384080-e51b-11e8-80ae-eecb03118dda.png)

Starting with v1.30, Netdata collects anonymous usage information by default and sends it to a self hosted PostHog instance within the Netdata infrastructure. To read
more about the information collected and how to opt-out, check the [anonymous statistics
page](https://github.com/netdata/netdata/blob/master/docs/anonymous-statistics.md).

## Updating the Agent on FreeBSD
If you have not passed the `--auto-update` or `-u` parameter for the installer to enable automatic updating, repeat the last step to update Netdata whenever a new version becomes available. 
The `netdata-updater.sh` script will update your Agent.

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
- `--native-only`: Only install if native binary packages are available.
- `--static-only`: Only install if a static build is available.
- `--build-only`: Only install using a local build.
- `--disable-cloud`: For local builds, don’t build any of the cloud code at all. For native packages and static builds,
    use runtime configuration to disable cloud support.
- `--require-cloud`: Only install if Netdata Cloud can be enabled. Overrides `--disable-cloud`.
- `--install-prefix`: Specify an installation prefix for local builds (by default, we use a sane prefix based on the type of system).
- `--install-version`: Specify the version of Netdata to install.
- `--old-install-prefix`: Specify the custom local build's installation prefix that should be removed.
- `--local-build-options`: Specify additional options to pass to the installer code when building locally. Only valid if `--build-only` is also specified.
- `--static-install-options`: Specify additional options to pass to the static installer code. Only valid if --static-only is also specified.

The following options are mutually exclusive and specifiy special operations other than trying to install Netdata normally or update an existing install:

- `--reinstall`: If there is an existing install, reinstall it instead of trying to update it. If there is not an existing install, install netdata normally.
- `--reinstall-even-if-unsafe`: If there is an existing install, reinstall it instead of trying to update it, even if doing so is known to potentially break things (for example, if we cannot detect what tyep of installation it is). If there is not an existing install, install Netdata normally.
- `--reinstall-clean`: If there is an existing install, uninstall it before trying to install Netdata. Fails if there is no existing install.
- `--uninstall`: Uninstall an existing installation of Netdata. Fails if there is no existing install.
- `--claim-only`: If there is an existing install, only try to claim it without attempting to update it. If there is no existing install, install and claim Netdata normally.
- `--repositories-only`: Only install repository configuration packages instead of doing a full install of Netdata. Automatically sets --native-only.
- `--prepare-offline-install-source`: Instead of insallling the agent, prepare a directory that can be used to install on another system without needing to download anything. See our [offline installation documentation](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/offline.md) for more info.

Additionally, the following environment variables may be used to further customize how the script runs (most users
should not need to use special values for any of these):

- `TMPDIR`: Used to specify where to put temporary files. On most systems, the default we select automatically
  should be fine. The user running the script needs to both be able to write files to the temporary directory,
  and run files from that location.
- `ROOTCMD`: Used to specify a command to use to run another command with root privileges if needed. By default
  we try to use sudo, doas, or pkexec (in that order of preference), but if you need special options for one of
  those to work, or have a different tool to do the same thing on your system, you can specify it here.
- `DISABLE_TELEMETRY`: If set to a value other than 0, behave as if `--disable-telemetry` was specified.
