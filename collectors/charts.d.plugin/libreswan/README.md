<!--
title: "Libreswan IPSec tunnel monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/libreswan/README.md"
sidebar_label: "Libreswan IPSec tunnels"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Networking"
-->

# Libreswan IPSec tunnel collector

Collects bytes-in, bytes-out and uptime for all established libreswan IPSEC tunnels.

The following charts are created, **per tunnel**:

1.  **Uptime**

-   the uptime of the tunnel

2.  **Traffic**

-   bytes in
-   bytes out

## Configuration

If using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), make sure `netdata-plugin-chartsd` is installed.

Edit the `charts.d/libreswan.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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


