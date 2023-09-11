<!--
title: "Alarms"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/alarms/README.md"
sidebar_label: "Alarms"
learn_status: "Published"
learn_rel_path: "Integrations/Monitor/Netdata"
-->

# Alarms

This collector creates an 'Alarms' menu with one line plot showing alarm states over time. Alarm states are mapped to integer values according to the below default mapping. Any alarm status types not in this mapping will be ignored (Note: This mapping can be changed by editing the `status_map` in the `alarms.conf` file). If you would like to learn more about the different alarm statuses check out the docs [here](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md#alarm-statuses).

```
{
    'CLEAR': 0, 
    'WARNING': 1, 
    'CRITICAL': 2
}
```

## Charts

Below is an example of the chart produced when running `stress-ng --all 2` for a few minutes. You can see the various warning and critical alarms raised. 

![alarms collector](https://user-images.githubusercontent.com/1153921/101641493-0b086a80-39ef-11eb-9f55-0713e5dfb19f.png)

## Configuration

Enable the collector and [restart Netdata](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md).

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `alarms: no` to `alarms: yes`
sudo systemctl restart netdata
```

If needed, edit the `python.d/alarms.conf` configuration file using `edit-config` from the your agent's [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/alarms.conf
```

The `alarms` specific part of the `alarms.conf` file should look like this:

```yaml
# what url to pull data from
local:
  url: 'http://127.0.0.1:19999/api/v1/alarms?all'
  # define how to map alarm status to numbers for the chart
  status_map:
    CLEAR: 0
    WARNING: 1
    CRITICAL: 2
  # set to true to include a chart with calculated alarm values over time
  collect_alarm_values: false
  # define the type of chart for plotting status over time e.g. 'line' or 'stacked'
  alarm_status_chart_type: 'line'
  # a "," separated list of words you want to filter alarm names for. For example 'cpu,load' would filter for only
  # alarms with "cpu" or "load" in alarm name. Default includes all.
  alarm_contains_words: ''
  # a "," separated list of words you want to exclude based on alarm name. For example 'cpu,load' would exclude 
  # all alarms with "cpu" or "load" in alarm name. Default excludes None.
  alarm_excludes_words: ''
```

It will default to pulling all alarms at each time step from the Netdata rest api at `http://127.0.0.1:19999/api/v1/alarms?all`
### Troubleshooting

To troubleshoot issues with the `alarms` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `alarms` module in debug mode:

```bash
./python.d.plugin alarms debug trace
```

