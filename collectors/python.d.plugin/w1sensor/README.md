<!--
title: "1-Wire Sensors monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/w1sensor/README.md"
sidebar_label: "1-Wire sensors"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Remotes/Devices"
-->

# 1-Wire Sensors collector

Monitors sensor temperature.

On Linux these are supported by the wire, w1_gpio, and w1_therm modules.
Currently temperature sensors are supported and automatically detected.

Charts are created dynamically based on the number of detected sensors.

## Configuration

Edit the `python.d/w1sensor.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/w1sensor.conf
```

An example of a working configuration can be found in the default [configuration file](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/w1sensor/w1sensor.conf) of this collector.

### Troubleshooting

To troubleshoot issues with the `w1sensor` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `w1sensor` module in debug mode:

```bash
./python.d.plugin w1sensor debug trace
```

