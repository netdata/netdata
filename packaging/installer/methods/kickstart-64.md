<!--
title: "Install Netdata with kickstart-static64.sh"
description: "The kickstart-static64.sh script installs a pre-compiled static binary, including all dependencies required to connect to Netdata Cloud, with one command."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kickstart-64.md
-->

# Install Netdata with kickstart-static64.sh

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart64&group=sum&after=-86400&label=today&units=installations&precision=0)

This page covers detailed instructions on using and configuring the installation script named `kickstart-static64.sh`.

This method uses a pre-compiled static binary to install Netdata on any Intel/AMD 64bit Linux system and on any Linux
distribution, even those with a broken or unsupported package manager.

To install Netdata from a static binary package, including all dependencies required to connect to Netdata Cloud, and
get _automatic nightly updates_, run the following as your normal user:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

> This script installs Netdata at `/opt/netdata`.

> See our [installation guide](/packaging/installer/README.md) for details about [automatic
> updates](/packaging/installer/README.md#automatic-updates) or [nightly vs. stable
> releases](/packaging/installer/README.md#nightly-vs-stable-releases).

## What does `kickstart-static64.sh` do?

The `kickstart.sh` script does the following after being downloaded and run:

-   Checks to see if there is an existing installation, and if there is updates that in preference to reinstalling.
-   Downloads the latest Netdata binary from the [binary-packages](https://github.com/netdata/binary-packages)
    repository. You can also run any of these `.run` files with [makeself](https://github.com/megastep/makeself).
-   Installs Netdata by running `./netdata-installer.sh` from the source tree, including any options you might have
    added.
-   Installs `netdata-updater.sh` to `cron.daily` to enable automatic updates, unless you added the `--no-updates`
    option.
-   Prints a message about whether the installation succeeded for failed for QA purposes.

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
-   `--disable-telemetry`: Opt-out of [anonymous statistics](/docs/anonymous-statistics.md) we use to make
    Netdata better.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--reinstall`: If an existing installation is detected, reinstall instead of attempting to update it. Note
    that this cannot be used to switch between installation types.
-   `--local-files`: Used for [offline installations](/packaging/installer/methods/offline.md). Pass four file paths:
    the Netdata tarball, the checksum file, the go.d plugin tarball, and the go.d plugin config tarball, to force
    kickstart run the process using those files. This option conflicts with the `--stable-channel` option. If you set
    this _and_ `--stable-channel`, Netdata will use the local files.

### Connect node to Netdata Cloud during installation

The `kickstart.sh` script accepts additional parameters to automatically [connect](/claim/README.md) your node to Netdata
Cloud immediately after installation. Find the `token` and `rooms` strings by [signing in to Netdata
Cloud](https://app.netdata.cloud/sign-in?cloudRoute=/spaces), then clicking on **Connect Nodes** in the [Spaces management
area](https://learn.netdata.cloud/docs/cloud/spaces#manage-spaces).

- `--claim-token`: The unique token associated with your Space in Netdata Cloud.
- `--claim-rooms`: A comma-separated list of tokens for each War Room this node should appear in.
- `--claim-proxy`: Should take the form of `socks5[h]://[user:pass@]host:ip` for a SOCKS5 proxy, or
  `http://[user:pass@]host:ip` for an HTTP(S) proxy.See [connecting through a
  proxy](/claim/README.md#connect-through-a-proxy) for details.
- `--claim-url`: Defaults to `https://app.netdata.cloud`.

For example:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --claim-token TOKEN --claim-rooms ROOM1,ROOM2 --claim-url https://app.netdata.cloud
```

## Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart-static64.sh` script you will download using the one-line
command above, run the following:

```bash
[ "508e16775bfe8aaf6bc2d0f7b91c323e" = "$(curl -Ss https://my-netdata.io/kickstart-static64.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fkickstart-64&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
