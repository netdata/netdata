<!--
title: "Libreswan IPSec tunnel monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/libreswan/README.md
sidebar_label: "Libreswan IPSec tunnels"
-->

# Libreswan IPSec tunnel monitoring with Netdata

Collects bytes-in, bytes-out and uptime for all established libreswan IPSEC tunnels.

The following charts are created, **per tunnel**:

1.  **Uptime**

-   the uptime of the tunnel

2.  **Traffic**

-   bytes in
-   bytes out

## Configuration

Edit the `charts.d/libreswan.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/libreswan.conf
```

The plugin executes 2 commands to collect all the information it needs:

```sh
ipsec whack --status
ipsec whack --trafficstatus
```

The first command is used to extract the currently established tunnels, their IDs and their names.
The second command is used to extract the current uptime and traffic.

Most probably user `netdata` will not be able to query libreswan, so the `ipsec` commands will be denied.
The plugin attempts to run `ipsec` as `sudo ipsec ...`, to get access to libreswan statistics.

To allow user `netdata` execute `sudo ipsec ...`, create the file `/etc/sudoers.d/netdata` with this content:

```
netdata ALL = (root) NOPASSWD: /sbin/ipsec whack --status
netdata ALL = (root) NOPASSWD: /sbin/ipsec whack --trafficstatus
```

Make sure the path `/sbin/ipsec` matches your setup (execute `which ipsec` to find the right path).

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Flibreswan%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
