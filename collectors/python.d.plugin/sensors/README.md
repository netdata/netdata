<!--
title: "Linux machine sensors monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/sensors/README.md"
sidebar_label: "sensors-python.d.plugin"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Devices"
-->

# Linux machine sensors monitoring with Netdata

Reads system sensors information (temperature, voltage, electric current, power, etc.).

Charts are created dynamically.

## Configuration

Edit the `python.d/sensors.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/sensors.conf
```

### possible issues

There have been reports from users that on certain servers, ACPI ring buffer errors are printed by the kernel (`dmesg`) when ACPI sensors are being accessed.
We are tracking such cases in issue [#827](https://github.com/netdata/netdata/issues/827).
Please join this discussion for help.

When `lm-sensors` doesn't work on your device (e.g. for RPi temperatures), use [the legacy bash collector](https://github.com/netdata/netdata/blob/master/collectors/charts.d.plugin/sensors/README.md)

---


