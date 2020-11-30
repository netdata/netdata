<!--
title: "Update the Netdata Agen"
description: "If you opted out of automatic updates, you need to update your Netdata Agent to the latest nightly or stable version."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/UPDATE.md
-->

# Update the Netdata Agent

By default, the Netdata Agent automatically updates with the latest nightly version. If you opted out of automatic
updates, you need to update your Netdata Agent to the latest nightly or stable version.

> 💡 Looking to reinstall the Netdata Agent to enable a feature, update an Agent that cannot update automatically, or
> troubleshoot an error during the installation process? See our [reinstallation doc](/packaging/installer/REINSTALL.md)
> for reinstallation steps.

Before you update the Netdata Agent, check to see if your Netdata Agent is already up-to-date by clicking on the update
icon in the local Agent dashboard's top navigation. This modal informs you whether your Agent needs an update or not.

![Opening the Agent's Update modal](https://user-images.githubusercontent.com/1153921/99738428-add06780-2a87-11eb-8268-0e17b689eb3f.gif)

## Determine which installation method you used

If you are not sure where your Netdata config directory is, see the [configuration doc](/docs/configure/nodes.md). In
most installations, this is `/etc/netdata`.

Use `cd` to navigate to the Netdata config directory, then use `ls -a` to look for a file called `.environment`.

-   If the `.environment` file _does not_ exist, reinstall with your [package manager](#deb-or-rpm-packages).
-   If the `.environtment` file _does_ exist, check its contents with `less .environment`.
    -   If `IS_NETDATA_STATIC_BINARY` is `"yes"`, update using the [pre-built static
        binary](#pre-built-static-binary-for-64-bit-systems-kickstart-static64sh).
    -   In all other cases, update using the [one-line installer script](#one-line-installer-script-kickstartsh).

Next, use the appropriate method to update the Netdata Agent:

-   [One-line installer script (`kickstart.sh`)](#one-line-installer-script-kickstartsh)
-   [`.deb` or `.rpm` packages](#deb-or-rpm-packages)
-   [Pre-built static binary for 64-bit systems (`kickstart-static64.sh`)](#pre-built-static-binary-for-64-bit-systems-kickstart-static64sh)
-   [Docker](#docker)
-   [macOS](#macos)
-   [Manual installation from Git](#manual-installation-from-git)

## One-line installer script (`kickstart.sh`)

If you installed Netdata using our one-line automatic installation script, run it again to update Netdata. Any custom
settings present in your Netdata configuration directory (typically at `/etc/netdata`) persists during this process.

This script will automatically run the update script that was installed as part of the initial install (even if
you disabled automatic updates) and preserve the existing install options you specified.

If you installed Netdata using an installation prefix, you will need to add an `--install` option specifying
that prefix to this command to make sure it finds Netdata.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

> ❗ If the above command fails, you can [reinstall
> Netdata](/packaging/installer/REINSTALL.md#one-line-installer-script-kickstartsh) to get the latest version. This also
> preserves your [configuration](/docs/configure/nodes.md) in `netdata.conf` or other files.

## `.deb` or `.rpm` packages

If you installed Netdata with [`.deb` or `.rpm` packages](/packaging/installer/methods/packages.md), use your
distribution's package manager to update Netdata. Any custom settings present in your Netdata configuration directory
(typically at `/etc/netdata`) persists during this process.

Your package manager grabs a new package from our hosted repository, updates the Netdata Agent, and restarts it.

```bash
apt-get install netdata     # Ubuntu/Debian
dnf install netdata         # Fedora/RHEL
yum install netdata         # CentOS
zypper in netdata           # openSUSE
```

> You may need to escalate privileges using `sudo`.

## Pre-built static binary for 64-bit systems (`kickstart-static64.sh`)

If you installed Netdata using the pre-built static binary, run the `kickstart-static64.sh` script again to update
Netdata. Any custom settings present in your Netdata configuration directory (typically at `/etc/netdata`) persists
during this process.

This script will automatically run the update script that was installed as part of the initial install (even if
you disabled automatic updates) and preserve the existing install options you specified.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

> ❗ If the above command fails, you can [reinstall
> Netdata](/packaging/installer/REINSTALL.md#pre-built-static-binary-for-64-bit-systems-kickstart-static64sh) to get the
> latest version. This also preserves your [configuration](/docs/configure/nodes.md) in `netdata.conf` or other files.

## Docker

Docker-based installations do not update automatically. To update an Netdata Agent running in a Docker container, you
must pull the [latest image from Docker Hub](https://hub.docker.com/r/netdata/netdata), stop and remove the container,
and re-create it using the latest image.

First, pull the latest version of the image.

```bash
docker pull netdata/netdata:latest
```

Next, to stop and remove any containers using the `netdata/netdata` image. Replace `netdata` if you changed it from the
default.

```bash
docker stop netdata
docker rm netdata
```

You can now re-create your Netdata container using the `docker` command or a `docker-compose.yml` file. See our [Docker
installation instructions](/packaging/docker/README.md#create-a-new-netdata-agent-container) for details.

## macOS

If you installed Netdata on your macOS system using Homebrew, you can explictly request an update:

```bash
brew upgrade netdata
```

Homebrew downloads the latest Netdata via the
[formulae](https://github.com/Homebrew/homebrew-core/blob/master/Formula/netdata.rb), ensures all dependencies are met,
and updates Netdata via reinstallation.

## Manual installation from Git

If you installed [Netdata manually from Git](/packaging/installer/methods/manual.md), you can run that installer again
to update your agent. First, run our automatic requirements installer, which works on many Linux distributions, to
ensure your system has the dependencies necessary for new features.

```bash
bash <(curl -sSL https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh)
```

Navigate to the directory where you first cloned the Netdata repository, pull the latest source code, and run
`netdata-install.sh` again. This process compiles Netdata with the latest source code and updates it via reinstallation.

```bash
cd /path/to/netdata/git
git pull origin master
sudo ./netdata-installer.sh
```

> ⚠️ If you installed Netdata with any optional parameters, such as `--no-updates` to disable automatic updates, and
> want to retain those settings, you need to set them again during this process.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FUPDATE&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
