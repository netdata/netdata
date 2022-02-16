<!--
title: "Reinstall the Netdata Agent"
description: "Troubleshooting installation issues or force an update of the Netdata Agent by reinstalling it using the same method you used during installation."
custom_edit_url: https://github.com/netdata/netdata/edit/master/packaging/installer/REINSTALL.md
-->

# Reinstall the Netdata Agent

In certain situations, such as needing to enable a feature or troubleshoot an issue, you may need to reinstall the
Netdata Agent on your node.

## One-line installer script (`kickstart.sh`)

Run the one-line installer script with the `--reinstall` parameter to reinstall the Netdata Agent. This will preserve
any [user configuration](/docs/configure/nodes.md) in `netdata.conf` or other files.

If you used any [optional
parameters](/packaging/installer/methods/kickstart.md#optional-parameters-to-alter-your-installation) during initial
installation, you need to pass them to the script again during reinstallation. If you cannot remember which options you
used, read the contents of the `.environment` file and look for a `REINSTALL_OPTIONS` line. This line contains a list of
optional parameters.

```bash
wget -O /tmp/netdata-kickstart.sh https://my-netdata.io/kickstart.sh && sh /tmp/netdata-kickstart.sh --reinstall
```

## Troubleshooting

If you still experience problems with your Netdata Agent installation after following one of these processes, the next
best route is to [uninstall](/packaging/installer/UNINSTALL.md) and then try a fresh installation using the [one-line
installer](/packaging/installer/methods/kickstart.md).

You can also post to our [community forums](https://community.netdata.cloud/c/support/13) or create a new [bug
report](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Finstaller%2FREINSTALL&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
