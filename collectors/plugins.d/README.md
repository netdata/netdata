# Netdata External Plugins

`plugins.d` is the netdata internal plugin that collects metrics
from external processes, thus allowing netdata to use **external plugins**.

## Provided External Plugins

plugin|language|O/S|description
:---:|:---:|:---:|:---
[apps.plugin](../apps.plugin/)|`C`|linux, freebsd|monitors the whole process tree on Linux and FreeBSD and breaks down system resource usage by **process**, **user** and **user group**.
[charts.d.plugin](../charts.d.plugin/)|`BASH`|all|a **plugin orchestrator** for data collection modules written in `BASH` v4+.
[fping.plugin](../fping.plugin/)|`C`|all|measures network latency, jitter and packet loss between the monitored node and any number of remote network end points.
[freeipmi.plugin](../freeipmi.plugin/)|`C`|linux|collects metrics from enterprise hardware sensors, on Linux servers.
[node.d.plugin](../node.d.plugin/)|`node.js`|all|a **plugin orchestrator** for data collection modules written in `node.js`.
[python.d.plugin](../python.d.plugin/)|`python`|all|a **plugin orchestrator** for data collection modules written in `python` v2 or v3 (both are supported).


## Motivation

This plugin allows netdata to use **external plugins** for data collection:

1. external data collection plugins may be written in any computer language.
2. external data collection plugins may use O/S capabilities or `setuid` to
   run with escalated privileges (compared to the netdata daemon).
   The communication between the external plugin and netdata is unidirectional
   (from the plugin to netdata), so that netdata cannot manipulate an external
   plugin running with escalated privileges.

## Operation

Each of the external plugins is expected to run forever.
Netdata will start it when it starts and stop it when it exits.

If the external plugin exits or crashes, netdata will log an error.
If the external plugin exits or crashes without pushing metrics to netdata,
netdata will not start it again.

The `stdout` of external plugins is connected to netdata to receive metrics,
with the API defined below.

The `stderr` of external plugins is connected to netdata `error.log`.

## Configuration

This plugin is configured via `netdata.conf`, section `[plugins]`.
At this section there a list of all the plugins found at the system it runs
with a boolean setting to enable them or not. 

Example:

```
[plugins]
	# enable running new plugins = yes
	# check for new plugins every = 60
	
	# charts.d = yes
	# fping = yes
	# node.d = yes
	# python.d = yes
```

The setting `enable running new plugins` changes the default behavior for all external plugins.
So if set to `no`, only the plugins that are explicitly set to `yes` will be run.

The setting `check for new plugins every` controls the time the directory `/usr/libexec/netdata/plugins.d`
will be rescanned for new plugins. So, new plugins can give added anytime. 

For each of the external plugins enabled, another `netdata.conf` section
is created, in the form of `[plugin:NAME]`, where `NAME` is the name of the external plugin.
This section allows controlling the update frequency of the plugin and provide
additional command line arguments to it.

For example, for `apps.plugin` the following section is available:

```
[plugin:apps]
	# update every = 1
	# command options = 
```

- `update every` controls the granularity of the external plugin.
- `command options` allows giving additional command line options to the plugin.


## External Plugins API

Any program that can print a few values to its standard output can become a netdata external plugin.

There are 7 lines netdata parses. lines starting with:

- `CHART` - create or update a chart
- `DIMENSION` - add or update a dimension to the chart just created
- `BEGIN` - initialize data collection for a chart
- `SET` - set the value of a dimension for the initialized chart
- `END` - complete data collection for the initialized chart
- `FLUSH` - ignore the last collected values
- `DISABLE` - disable this plugin

a single program can produce any number of charts with any number of dimensions each.

Charts can be added any time (not just the beginning).

### command line parameters

The plugin **MUST** accept just **one** parameter: **the number of seconds it is
expected to update the values for its charts**. The value passed by netdata
to the plugin is controlled via its configuration file (so there is no need
for the plugin to handle this configuration option).

The external plugin can overwrite the update frequency. For example, the server may
request per second updates, but the plugin may ignore it and update its charts
every 5 seconds.

### environment variables

There are a few environment variables that are set by `netdata` and are
available for the plugin to use.

variable|description
:------:|:----------
`NETDATA_USER_CONFIG_DIR`|The directory where all netdata related user configuration should be stored. If the plugin requires custom user configuration, this is the place the user has saved it (normally under `/etc/netdata`).
`NETDATA_STOCK_CONFIG_DIR`|The directory where all netdata related stock configuration should be stored. If the plugin is shipped with configuration files, this is the place they can be found (normally under `/usr/lib/netdata/conf.d`).
`NETDATA_PLUGINS_DIR`|The directory where all netdata plugins are stored.
`NETDATA_WEB_DIR`|The directory where the web files of netdata are saved.
`NETDATA_CACHE_DIR`|The directory where the cache files of netdata are stored. Use this directory if the plugin requires a place to store data. A new directory should be created for the plugin for this purpose, inside this directory.
`NETDATA_LOG_DIR`|The directory where the log files are stored. By default the `stderr` output of the plugin will be saved in the `error.log` file of netdata.
`NETDATA_HOST_PREFIX`|This is used in environments where system directories like `/sys` and `/proc` have to be accessed at a different path.
`NETDATA_DEBUG_FLAGS`|This is a number (probably in hex starting with `0x`), that enables certain netdata debugging features. Check **[[Tracing Options]]** for more information.
`NETDATA_UPDATE_EVERY`|The minimum number of seconds between chart refreshes. This is like the **internal clock** of netdata (it is user configurable, defaulting to `1`). There is no meaning for a plugin to update its values more frequently than this number of seconds.


### the output of the plugin

The plugin should output instructions for netdata to its output (`stdout`). Since this uses pipes, please make sure you flush stdout after every iteration.

#### DISABLE

`DISABLE` will disable this plugin. This will prevent netdata from restarting the plugin. You can also exit with the value `1` to have the same effect.

#### CHART

`CHART` defines a new chart.

the template is:

> CHART type.id name title units [family [context [charttype [priority [update_every [options [plugin [module]]]]]]]]

 where:
  - `type.id`

    uniquely identifies the chart,
    this is what will be needed to add values to the chart

    the `type` part controls the menu the charts will appear in

  - `name`

    is the name that will be presented to the user instead of `id` in `type.id`. This means that only the `id` part of `type.id` is changed. When a name has been given, the chart is index (and can be referred) as both `type.id` and `type.name`. You can set name to `''`, or `null`, or `(null)` to disable it.

  - `title`

    the text above the chart

  - `units`

    the label of the vertical axis of the chart,
    all dimensions added to a chart should have the same units
    of measurement

  - `family`

    is used to group charts together
    (for example all eth0 charts should say: eth0),
    if empty or missing, the `id` part of `type.id` will be used
    
    this controls the sub-menu on the dashboard

  - `context`

    the context is giving the template of the chart. For example, if multiple charts present the same information for a different family, they should have the same `context`

    this is used for looking up rendering information for the chart (colors, sizes, informational texts) and also apply alarms to it

  - `charttype`

    one of `line`, `area` or `stacked`,
    if empty or missing, the `line` will be used

  - `priority`

    is the relative priority of the charts as rendered on the web page,
    lower numbers make the charts appear before the ones with higher numbers,
    if empty or missing, `1000` will be used

  - `update_every`

    overwrite the update frequency set by the server,
    if empty or missing, the user configured value will be used

  - `options`

    a space separated list of options, enclosed in quotes. 4 options are currently supported: `obsolete` to mark a chart as obsolete (netdata will hide it and delete it after some time), `detail` to mark a chart as insignificant (this may be used by dashboards to make the charts smaller, or somehow visualize properly a less important chart), `store_first` to make netdata store the first collected value, assuming there was an invisible previous value set to zero (this is used by statsd charts - if the first data collected value of incremental dimensions is not zero based, unrealistic spikes will appear with this option set) and `hidden` to perform all operations on a chart, but do not offer it on dashboards (the chart will be send to backends). `CHART` options have been added in netdata v1.7 and the `hidden` option was added in 1.10.

  - `plugin` and `module`

    both are just names that are used to let the user the plugin and its module that generated the chart. If `plugin` is unset or empty, netdata will automatically set the filename of the plugin that generated the chart. `module` has not default.


#### DIMENSION

`DIMENSION` defines a new dimension for the chart

the template is:

> DIMENSION id [name [algorithm [multiplier [divisor [hidden]]]]]

 where:

  - `id`

    the `id` of this dimension (it is a text value, not numeric),
    this will be needed later to add values to the dimension

    We suggest to avoid using `.` in dimension ids. Backends expect metrics to be `.` separated and people will get confused if a dimension id contains a dot.

  - `name`

    the name of the dimension as it will appear at the legend of the chart,
    if empty or missing the `id` will be used

  - `algorithm`

    one of:

    * `absolute`

      the value is to drawn as-is (interpolated to second boundary),
      if `algorithm` is empty, invalid or missing, `absolute` is used

    * `incremental`

      the value increases over time,
      the difference from the last value is presented in the chart,
      the server interpolates the value and calculates a per second figure

    * `percentage-of-absolute-row`

      the % of this value compared to the total of all dimensions

    * `percentage-of-incremental-row`

      the % of this value compared to the incremental total of
      all dimensions

  - `multiplier`

    an integer value to multiply the collected value,
    if empty or missing, `1` is used

  - `divisor`

    an integer value to divide the collected value,
    if empty or missing, `1` is used

  - `hidden`

    giving the keyword `hidden` will make this dimension hidden,
    it will take part in the calculations but will not be presented in the chart


#### VARIABLE

> VARIABLE [SCOPE] name = value

`VARIABLE` defines a variable that can be used in alarms. This is to used for setting constants (like the max connections a server may accept).

Variables support 2 scopes:

- `GLOBAL` or `HOST` to define the variable at the host level.
- `LOCAL` or `CHART` to define the variable at the chart level. Use chart-local variables when the same variable may exist for different charts (i.e. netdata monitors 2 mysql servers, and you need to set the `max_connections` each server accepts). Using chart-local variables is the ideal to build alarm templates.

The position of the `VARIABLE` line, sets its default scope (in case you do not specify a scope). So, defining a `VARIABLE` before any `CHART`, or between `END` and `BEGIN` (outside any chart), sets `GLOBAL` scope, while defining a `VARIABLE` just after a `CHART` or a `DIMENSION`, or within the `BEGIN` - `END` block of a chart, sets `LOCAL` scope.

These variables can be set and updated at any point.

Variable names should use alphanumeric characters, the `.` and the `_`.

The `value` is floating point (netdata used `long double`).

Variables are transferred to upstream netdata servers (streaming and database replication).

## data collection

data collection is defined as a series of `BEGIN` -> `SET` -> `END` lines

> BEGIN type.id [microseconds]

  - `type.id`

    is the unique identification of the chart (as given in `CHART`)

  - `microseconds`

    is the number of microseconds since the last update of the chart. It is optional.

    Under heavy system load, the system may have some latency transferring
    data from the plugins to netdata via the pipe. This number improves
    accuracy significantly, since the plugin is able to calculate the
    duration between its iterations better than netdata.

    The first time the plugin is started, no microseconds should be given
    to netdata.

> SET id = value

   - `id`

     is the unique identification of the dimension (of the chart just began)

   - `value`

     is the collected value, only integer values are collected. If you want to push fractional values, multiply this value by 100 or 1000 and set the `DIMENSION` divider to 1000.

> END

  END does not take any parameters, it commits the collected values for all dimensions to the chart. If a dimensions was not `SET`, its value will be empty for this commit.

More `SET` lines may appear to update all the dimensions of the chart.
All of them in one `BEGIN` -> `END` block.

All `SET` lines within a single `BEGIN` -> `END` block have to refer to the
same chart.

If more charts need to be updated, each chart should have its own
`BEGIN` -> `SET` -> `END` block.

If, for any reason, a plugin has issued a `BEGIN` but wants to cancel it,
it can issue a `FLUSH`. The `FLUSH` command will instruct netdata to ignore
all the values collected since the last `BEGIN` command.

If a plugin does not behave properly (outputs invalid lines, or does not
follow these guidelines), will be disabled by netdata.

### collected values

netdata will collect any **signed** value in the 64bit range:
`-9.223.372.036.854.775.808` to `+9.223.372.036.854.775.807`
