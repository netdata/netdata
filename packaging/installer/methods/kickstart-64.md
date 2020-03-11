<!--
---
title: "Install Netdata with kickstart-static64.sh"
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kickstart-64.md
---
-->

# Install Netdata with kickstart-static64.sh

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

This page covers detailed instructions on using and configuring the installation script named `kickstart-static64.sh`.

This method uses a pre-compiled static binary to install Netdata on any Intel/AMD 64bit Linux system and on any Linux
distribution, even those with a broken or unsupported package manager.

To install Netdata from a binary package and get _automatic nightly updates_, run the following as your normal user:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

> This script installs Netdata at `/opt/netdata`.

> See our [installation guide](../README.md) for details about [automatic updates](../README.md#automatic-updates) or
> [nightly vs. stable releases](../README.md#nightly-vs-stable-releases).

## What does `kickstart-static64.sh` do?

The `kickstart.sh` script does the following after being downloaded and run:

-   Detects the Linux distribution and **installs the required system packages** for building Netdata. Unless you added
    the `--dont-wait` option, it will ask for your permission first.
-   Downloads the latest Netdata binary from the [binary-packages](https://github.com/netdata/binary-packages)
    repository. You can also run any of these `.run` files with [makeself](https://github.com/megastep/makeself).
-   Installs Netdata by running `./netdata-installer.sh` from the source tree, including any options you might have
    added.
-   Installs `netdata-updater.sh` to `cron.daily` to enable automatic updates, unless you added the `--no-updates`
    option.
-   Prints a message about whether the insallation succeeded for failed for QA purposes.

If your shell fails to handle the above one-liner, you can download and run the `kickstart-static64.sh` script manually.

```sh
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

## Optional parameters to alter your installation

The `kickstart-static64.sh` script passes all its parameters to `netdata-installer.sh`, which you can use to customize
your installation. Here are a few important parameters:

-   `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--stable-channel`: Automatically update only on the release of new major versions.
-   `--nightly-channel`: Automatically update on every new nightly build.
-   `--disable-telemetry`: Opt-out of [anonymous statistics](../../../docs/anonymous-statistics.md) we use to make
    Netdata better.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--local-files`: Used for [offline installations](offline.md). Pass four file paths: the Netdata
    tarball, the checksum file, the go.d plugin tarball, and the go.d plugin config tarball, to force kickstart run the
    process using those files. This option conflicts with the `--stable-channel` option. If you set this _and_
    `--stable-channel`, Netdata will use the local files.

## Verify script integrity

To use `md5sum` to verify the intregity of the `kickstart-static64.sh` script you will download using the one-line
command above, run the following:

```bash
[ "33ecd20452f569c1d9972bcefdf04692" = "$(curl -Ss https://my-netdata.io/kickstart-static64.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## What's next?

When you finish installing Netdata, be sure to visit our [step-by-step tutorial](../../../docs/step-by-step/step-00.md)
for a fully-guided tour into Netdata's capabilities and how to configure it according to your needs. 

Or, if you're a monitoring and system administration pro, skip ahead to our [getting started
guide](../../../docs/getting-started.md) for a quick overview.
