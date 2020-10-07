<!--
title: "Alarms"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/alarms/README.md
-->

# Alarms - graphing active Netdata alarms over time

This collector creates an 'Alarms' section with one line plot showing alarms by status. Alarm states are mapped to integer values according to the below mapping.

```
{'UNDEFINED': 0, 'CLEAR': 0, 'WARNING': 1, 'CRITICAL': 2}
```

## Charts

Below is an example of the chart produced when running `stress-ng --all 2` for a few minutes. You can see the various warning and critical alarms raised. 

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-alarms-collector.jpg)

## Configuration

Enable the collector and restart Netdata.

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `alarms: no` to `alarms: yes`
sudo systemctl restart netdata
```

If needed (typically not), edit the `python.d/alarms.conf` configuration file using `edit-config` from the your agent's [config
directory](https://learn.netdata.cloud/guides/step-by-step/step-04#find-your-netdataconf-file), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/alarms.conf
```

The `alarms` specific part of the `alarms.conf` file should look like this:

```
# what url to pull data from
url: 'http://127.0.0.1:19999/api/v1/alarms?all'
```

It will default to pulling all alarms at each time step from the Netdata rest api at `http://127.0.0.1:19999/api/v1/alarms?all`
