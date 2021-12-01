<!--
title: "1-Wire Sensors monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/w1sensor/README.md
sidebar_label: "1-Wire sensors"
-->

# 1-Wire Sensors monitoring with Netdata

Monitors sensor temperature.

On Linux these are supported by the wire, w1_gpio, and w1_therm modules.
Currently temperature sensors are supported and automatically detected.

Charts are created dynamically based on the number of detected sensors.

## Configuration

Edit the `python.d/w1sensor.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/w1sensor.conf
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fw1sensor%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
