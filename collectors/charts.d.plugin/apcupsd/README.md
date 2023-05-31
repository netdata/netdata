<!--
title: "APC UPS monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/apcupsd/README.md"
sidebar_label: "APC UPS"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# APC UPS collector

Monitors different APC UPS models and retrieves status information using `apcaccess` tool.

## Configuration

If using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), make sure `netdata-plugin-chartsd` is installed.

Edit the `charts.d/apcupsd.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/apcupsd.conf
```


