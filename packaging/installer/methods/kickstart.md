<!--
title: "Install Netdata with kickstart.sh"
description: "The kickstart.sh script installs Netdata from source, including all dependencies required to connect to Netdata Cloud, with a single command."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/methods/kickstart.md
-->

# Install Netdata with kickstart.sh

![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-3600&label=last+hour&units=installations&value_color=orange&precision=0) ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.requests_per_url&options=unaligned&dimensions=kickstart&group=sum&after=-86400&label=today&units=installations&precision=0)

This page covers detailed instructions on using and configuring the automatic one-line installation script named
`kickstart.sh`.

This method is fully automatic on all Linux distributions. To install Netdata from source, including all dependencies
required to connect to Netdata Cloud, and get _automatic nightly updates_, run the following as your normal user:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

> See our [installation guide](../README.md) for details about [automatic updates](../README.md#automatic-updates) or
> [nightly vs. stable releases](../README.md#nightly-vs-stable-releases).

## What does `kickstart.sh` do?

The `kickstart.sh` script does the following after being downloaded and run using `bash`:

-   Detects the Linux distribution and **installs the required system packages** for building Netdata. Unless you added
    the `--dont-wait` option, it will ask for your permission first.
-   Checks for an existing installation, and if found updates that instead of creating a new install.
-   Downloads the latest Netdata source tree to `/usr/src/netdata.git`.
-   Installs Netdata by running `./netdata-installer.sh` from the source tree, using any [optional
    parameters](#optional-parameters-to-alter-your-installation) you have specified.
-   Installs `netdata-updater.sh` to `cron.daily`, so your Netdata installation will be updated with new nightly
    versions, unless you override that with an [optional parameter](#optional-parameters-to-alter-your-installation).
-   Prints a message whether installation succeeded or failed for QA purposes.

## Optional parameters to alter your installation

The `kickstart.sh` script passes all its parameters to `netdata-installer.sh`, which you can use to customize your
installation. Here are a few important parameters:

-   `--dont-wait`: Enable automated installs by not prompting for permission to install any required packages.
-   `--dont-start-it`: Prevent the installer from starting Netdata automatically.
-   `--stable-channel`: Automatically update only on the release of new major versions.
-   `--nightly-channel`: Automatically update on every new nightly build.
-   `--disable-telemetry`: Opt-out of [anonymous statistics](/docs/anonymous-statistics.md) we use to make
    Netdata better.
-   `--no-updates`: Prevent automatic updates of any kind.
-   `--reinstall`: If an existing install is detected, reinstall instead of trying to update it. Note that this
    cannot be used to change installation types.
-   `--local-files`: Used for [offline installations](offline.md). Pass four file paths: the Netdata
    tarball, the checksum file, the go.d plugin tarball, and the go.d plugin config tarball, to force kickstart run the
    process using those files. This option conflicts with the `--stable-channel` option. If you set this _and_
    `--stable-channel`, Netdata will use the local files.

### Claim node to Netdata Cloud during installation

The `kickstart.sh` script accepts additional parameters to automatically [claim](/claim/README.md) your node to Netdata
Cloud immediately after installation. Find the `token` and `rooms` strings by [signing in to Netdata
Cloud](https://app.netdata.cloud/sign-in?cloudRoute=/spaces), then clicking on **Claim Nodes** in the [Spaces management
area](https://learn.netdata.cloud/docs/cloud/spaces#manage-spaces).

- `--claim-token`: The unique token associated with your Space in Netdata Cloud.
- `--claim-rooms`: A comma-separated list of tokens for each War Room this node should appear in.
- `--claim-proxy`: Should take the form of `socks5[h]://[user:pass@]host:ip` for a SOCKS5 proxy, or
  `http://[user:pass@]host:ip` for an HTTP(S) proxy.See [claiming through a
  proxy](/claim/README.md#claim-through-a-proxy) for details.
- `--claim-url`: Defaults to `https://app.netdata.cloud`.

For example:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --claim-token TOKEN --claim-rooms ROOM1,ROOM2 --claim-url https://app.netdata.cloud
```

## Verify script integrity

To use `md5sum` to verify the integrity of the `kickstart.sh` script you will download using the one-line command above,
run the following:

```bash
[ "271aef84d0bbdabb337571a3963549c7" = "$(curl -Ss https://my-netdata.io/kickstart.sh | md5sum | cut -d ' ' -f 1)" ] && echo "OK, VALID" || echo "FAILED, INVALID"
```

If the script is valid, this command will return `OK, VALID`.

## What's next?

When you're finished with installation, check out our [single-node](/docs/quickstart/single-node.md) or
[infrastructure](/docs/quickstart/infrastructure.md) monitoring quickstart guides based on your use case.

Or, skip straight to [configuring the Netdata Agent](/docs/configure/nodes.md).

Read through Netdata's [documentation](https://learn.netdata.cloud/docs), which is structured based on actions and
solutions, to enable features like health monitoring, alarm notifications, long-term metrics storage, exporting to
external databases, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fpackaging%2Finstaller%2Fmethods%2Fkickstart&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
