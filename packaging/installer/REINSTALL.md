<!--
title: "Reinstall Netdata"
description: "Troubleshooting installation issues or force an update of the Netdata Agent by reinstalling it using the same method you used during installation."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/REINSTALL.md
-->

# Reinstall Netdata

TK

## Determine which installation method you used

First, see our [configuration doc](/docs/configure/nodes.md) to figure out where your Netdata config directory is. In
most installations, this is `/etc/netdata`.

Use `cd` to navigate to the config directory, then use `ls -a` to look for a file called `.environment` in your Netdata
config directory.

-   If the `.environment` file _does not_ exist, reinstall with your [package manager](#deb-or-rpm-packages).
-   If the `.environtment` file _does_ exist, check its contents with `less .environment`.
    -   If `IS_NETDATA_STATIC_BINARY` is `"yes"`, reinstall using the [pre-built static
        binary](#pre-built-static-binary-for-64-bit-systems-kickstart-static64sh).
    -   In all other cases, reinstall using the [one-line installer script](#one-line-installer-script-kickstartsh).

## One-line installer script (`kickstart.sh`)

Run the one-line installer script with the `--reinstall` parameter to reinstall the Netdata Agent. This will preserve
any [user configuration](/docs/configure/nodes.md) in `netdata.conf` or other 

If you used any [optional parameters](#optional-parameters-to-alter-your-installation) during initial installation, you 

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --reinstall
```

## `.deb` or `.rpm` packages


## Pre-built static binary for 64-bit systems (`kickstart-static64.sh`)


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FREINSTALL&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)