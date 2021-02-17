<!--
title: "Alarm Values"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/alarm_values/README.md
-->

# Alarm Values - graphing Netdata caclulated alarm values over time

This collector creates an 'Alarm Values' menu with one line plot showing calculated alarm values over time. This can be useful when you want to see how specific calculated alarm values have evolved over time as you troubleshoot.

## Charts

Below is an example of the chart produced. In the screenshot we have just selected two `net_packets` related alarm values to look at over time.  

![alarm values collector](https://user-images.githubusercontent.com/2178292/108211532-aec77280-7124-11eb-8db4-dd12f6147bbe.png)

## Configuration

Enable the collector and restart Netdata.

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `alarm_values: no` to `alarm_values: yes`
sudo systemctl restart netdata
```

If needed, edit the `python.d/alarm_values.conf` configuration file using `edit-config` from the your agent's [config
directory](/docs/configure/nodes.md), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/alarm_values.conf
```

The `alarm_values` specific part of the `alarm_values.conf` file should look like this:

```yaml
# what url to pull data from
local:
  url: 'http://127.0.0.1:19999/api/v1/alarms?all'
```

It will default to pulling the latest caclulated alarm values at each time step from the Netdata rest api at `http://127.0.0.1:19999/api/v1/alarms?all`

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Falarms%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()