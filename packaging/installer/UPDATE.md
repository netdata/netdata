<!--
---
title: "Update Netdata"
description: "We actively develop Netdata to add new features and remove bugs. Here's how to stay up-to-date with the 
latest nightly or major releases."
date: 2020-03-12
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/UPDATE.md
---
-->

# Update Netdata

We actively develop Netdata to add new features and remove bugs, and encourage all users to ensure they're using the
most up-to-date version, whether that's nightly or major releases.

Before you update Netdata using one of the methods below, check to see if your Netdata agent is already up-to-date by
opening the update modal in the dashboard. Click the **Update** button in the top navigation to open it. The modal tells
you whether your agent is up-to-date or not.

![Opening the update
modal](https://user-images.githubusercontent.com/1153921/76559153-d5cced00-645b-11ea-8dcd-893f8d16f7a8.gif)

If your agent can be updated, use one of the methods below. **The method you chose for updating Netdata depends on how
you installed it.** Choose from the following list to see the appropriate update instructions for your system.

-   [One-line installer script (`kickstart.sh`)](#one-line-installer-script-kickstartsh)
-   [`.deb` or `.rpm` packages](#deb-or-rpm-packages)
-   [Pre-built static binary for 64-bit systems
    (`kickstart-static64.sh`)](#pre-built-static-binary-for-64-bit-systems-kickstart-static64sh)
-   [Docker](#docker)
-   [macOS](#macos)
-   [Manual installation from Git](#manual-installation-from-git)

## One-line installer script (`kickstart.sh`)

If you installed Netdata using our one-line automatic installation script, run it again to update Netdata. Any custom
settings present in your Netdata configuration directory (typically at `/etc/netdata`) persists during this process.

This script downloads the latest Netdata source (either the nightly or stable version), compiles Netdata, and updates it
via reinstallation.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

> ⚠️ If you installed Netdata with any optional parameters, such as `--no-updates` to disable automatic updates, and
> want to retain those settings, you need to set them again during this process. See the [`kickstart.sh`
> documentation](methods/kickstart.md#optional-parameters-to-alter-your-installation) for more information on these
> parameters and what they do.

## `.deb` or `.rpm` packages

If you installed Netdata with `.deb` or `.rpm` packages, use your distribution's package manager update Netdata. Any
custom settings present in your Netdata configuration directory (typically at `/etc/netdata`) persists during this
process.

Your package manager grabs a new package from our hosted repository, updates Netdata, and restarts it.

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

This script downloads the latest Netdata source (either the nightly or stable version), compiles Netdata, and updates it
via reinstallation.

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

> ⚠️ If you installed Netdata with any optional parameters, such as `--no-updates` to disable automatic updates, and
> want to retain those settings, you need to set them again during this process. See the [`kickstart-static64.sh`
> documentation](methods/kickstart-64.md#optional-parameters-to-alter-your-installation) for more information on these
> parameters and what they do.

## Docker

Docker-based installations do not update automatically. To update an agent running in a Docker container, you must pull
the [latest image from Docker hub](https://hub.docker.com/r/netdata/netdata), stop and remove the container, and
re-create it using the latest image.

First, pull the latest version of the image.

```bash
docker pull netdata/netdata:latest
```

Next, to stop and remove any containers using the `netdata/netdata` image. Replace `netdata` if you changed
it from the default in our [Docker installation instructions](../docker/README.md#run-netdata-with-the-docker-command).

```bash
docker stop netdata
docker rm netdata
```

You can now re-create your Netdata container using the `docker` command or a `docker-compose.yml` file. See our [Docker
installation instructions](../docker/README.md#run-netdata-with-the-docker-command) for details. For example, using the
`docker` command:

```bash
docker run -d --name=netdata \
  -p 19999:19999 \
  -v /etc/passwd:/host/etc/passwd:ro \
  -v /etc/group:/host/etc/group:ro \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /etc/os-release:/host/etc/os-release:ro \
  --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  netdata/netdata
```

## macOS

If you installed Netdata on your macOS system using Homebrew, you can explictly request an update:

```bash
brew upgrade netdata
```

Homebrew downloads the latest Netdata via the
[formulae](https://github.com/Homebrew/homebrew-core/blob/master/Formula/netdata.rb), ensures all dependencies are met,
and updates Netdata via reinstallation.

## Manual installation from Git

If you installed Netdata manually from Git using `netdata-installer.sh`, you can run that installer again to update your
agent. First, run our automatic requirements installer, which works on many Linux distributions, to ensure your system
has the dependencies necessary for new features.

```bash
bash <(curl -sSL https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh)
```

Then, navigate to the directory where you first cloned the Netdata repository, pull the latest source code, and run
`netdata-install.sh` again. This process compiles Netdata with the latest source code and updates it via reinstallation. 

```bash
cd /path/to/netdata/git
git pull origin master
sudo ./netdata-installer.sh
```

> ⚠️ If you installed Netdata with any optional parameters, such as `--no-updates` to disable automatic updates, and
> want to retain those settings, you need to set them again during this process.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FUPDATE&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
