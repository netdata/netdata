<!--
title: "Install Netdata on offline systems"
description: "Install the Netdata Agent on offline/air gapped systems to benefit from real-time, per-second monitoring without connecting to the internet."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/offline.md
-->

# Install Netdata on offline systems

The Netdata Agent installs on offline or air gapped systems with a few additional steps.

By default, the `kickstart.sh` and `kickstart-static64.sh` download Netdata assets, like the precompiled binary and a
few dependencies, using the system's internet connection, but the Agent installer can also use equivalent files already
present on the local filesystem.

First, download the required files. If you're using `kickstart.sh`, you need the Netdata tarball, the checksums, the
go.d plugin binary, and the go.d plugin configuration. If you're using `kickstart-static64.sh`, you need only the
Netdata tarball and checksums.

Download the files you need to a system of yours that's connected to the internet. Use the commands below, or visit the
[latest Netdata release page](https://github.com/netdata/netdata/releases/latest) and [latest go.d plugin release
page](https://github.com/netdata/go.d.plugin/releases) to download the required files manually.

**If you're using `kickstart.sh`**, use the following commands:

```bash
cd /tmp

curl -s https://my-netdata.io/kickstart.sh > kickstart.sh

# Netdata tarball
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*tar.gz" | cut -d '"' -f 4 | wget -qi -

# Netdata checksums
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*txt" | cut -d '"' -f 4 | wget -qi -

# Netdata dependency handling script
wget -q - https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh

# go.d plugin 
# For binaries for OS types and architectures not listed on [go.d releases](https://github.com/netdata/go.d.plugin/releases/latest), kindly open a github issue and we will do our best to serve your request
export OS=$(uname -s | tr '[:upper:]' '[:lower:]') ARCH=$(uname -m | sed -e 's/i386/386/g' -e 's/i686/386/g' -e 's/x86_64/amd64/g' -e 's/aarch64/arm64/g' -e 's/armv64/arm64/g' -e 's/armv6l/arm/g' -e 's/armv7l/arm/g' -e 's/armv5tel/arm/g') && curl -s https://api.github.com/repos/netdata/go.d.plugin/releases/latest | grep "browser_download_url.*${OS}-${ARCH}.tar.gz" | cut -d '"' -f 4 | wget -qi -

# go.d configuration 
curl -s https://api.github.com/repos/netdata/go.d.plugin/releases/latest | grep "browser_download_url.*config.tar.gz" | cut -d '"' -f 4 | wget -qi -
```

**If you're using `kickstart-static64.sh`**, use the following commands:

```bash
cd /tmp

curl -s https://my-netdata.io/kickstart-static64.sh > kickstart-static64.sh

# Netdata static64 tarball
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*gz.run" | cut -d '"' -f 4 | wget -qi -

# Netdata checksums
curl -s https://api.github.com/repos/netdata/netdata/releases/latest | grep "browser_download_url.*txt" | cut -d '"' -f 4 | wget -qi -
```

Move downloaded files to the `/tmp` directory on the offline system in whichever way your defined policy allows (if
any).

Now you can run either the `kickstart.sh` or `kickstart-static64.sh` scripts using the `--local-files` option. This
option requires you to specify the location and names of the files you just downloaded. 

> When using `--local-files`, the `kickstart.sh` or `kickstart-static64.sh` scripts won't download any Netdata assets
> from the internet. But, you may still need a connection to install dependencies using your system's package manager.
> The scripts will warn you if your system doesn't have all the dependencies.

```bash
# kickstart.sh
bash kickstart.sh --local-files /tmp/netdata-(version-number-here).tar.gz /tmp/sha256sums.txt /tmp/go.d.plugin-(version-number-here).(OS)-(architecture).tar.gz /tmp/config.tar.gz /tmp/install-required-packages.sh

# kickstart-static64.sh
bash kickstart-static64.sh --local-files /tmp/netdata-(version-number-here).gz.run /tmp/sha256sums.txt
```

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.
