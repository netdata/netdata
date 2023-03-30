<!--
title: "CEPH monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/ceph/README.md"
sidebar_label: "CEPH"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Storage"
-->

# CEPH collector

Monitors the ceph cluster usage and consumption data of a server, and produces:

-   Cluster statistics (usage, available, latency, objects, read/write rate)
-   OSD usage
-   OSD latency
-   Pool usage
-   Pool read/write operations
-   Pool read/write rate
-   number of objects per pool

## Requirements

-   `rados` python module
-   Granting read permissions to ceph group from keyring file

```shell
# chmod 640 /etc/ceph/ceph.client.admin.keyring
```

## Configuration

Edit the `python.d/ceph.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ceph.conf
```

Sample:

```yaml
local:
  config_file: '/etc/ceph/ceph.conf'
  keyring_file: '/etc/ceph/ceph.client.admin.keyring'
```




### Troubleshooting

To troubleshoot issues with the `ceph` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `ceph` module in debug mode:

```bash
./python.d.plugin ceph debug trace
```

