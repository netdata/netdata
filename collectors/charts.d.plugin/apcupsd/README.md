<!--
---
title: "APC UPS monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/charts.d.plugin/apcupsd/README.md
---
-->

# APC UPS monitoring with Netdata

Monitors different APC UPS models and retrieves status information using `apcaccess` tool.

## Configuration

Edit the `charts.d/apcupsd.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/apcupsd.conf
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fapcupsd%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
