<!--
title: "ZFS Dataset monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/zfsdstat/README.md
sidebar_label: "ZFS Dataset"
-->

# ZFS Dataset monitoring with Netdata

Collects ZFS dataset statistics for all datasets in the system.

The following charts will be created:

1.  **ZFS Dataset Bandwidth**

-   reads
-   writes

2.  **ZFS Dataset IOPS**

-   reads
-   writes

3.  **ZFS Unlinks**

-   unlinks
-   unlinked

## Configuration

Edit the `charts.d/zfsdstat.conf` configuration file using `edit-config` from the your agent's [config
directory](/docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/zfsdstat.conf
```

This is the internal default for `charts.d/zfsdstat.conf`

```sh
# how frequently to collect ZFS dataset data
zfsdstat_update_every=1

# the priority of zfsdstat related to other charts
# 2900 should place it right beneath the ZFS filesystem charts
zfsdstat_priority=2900
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fzfsdstat%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
