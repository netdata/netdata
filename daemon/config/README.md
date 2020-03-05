# Daemon configuration

<details markdown="1"><summary>The daemon configuration file is read from `/etc/netdata/netdata.conf`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

This config file **is not needed by default**. Netdata works fine out of the box without it. But it does allow you to adapt the general behavior of Netdata, in great detail. You can find all these settings, with their default values, by accessing the URL `https://netdata.server.hostname:19999/netdata.conf`. For example check the configuration file of [netdata.firehol.org](http://netdata.firehol.org/netdata.conf). HTTP access to this file is limited by default to private IPs, via the [web server access lists](../../web/server/#access-lists).

`netdata.conf` has sections stated with `[section]`. You will see the following sections:

1.  `[global]` to [configure](#global-section-options) the [Netdata daemon](../).
2.  `[web]` to [configure the web server](../../web/server).
3.  `[plugins]` to [configure](#plugins-section-options) which [collectors](../../collectors) to use and PATH settings.
4.  `[health]` to [configure](#health-section-options) general settings for [health monitoring](../../health)
5.  `[registry]` for the [Netdata registry](../../registry).
6.  `[backend]` to set up [streaming and replication](../../streaming) options.
7.  `[statsd]` for the general settings of the [stats.d.plugin](../../collectors/statsd.plugin).
8.  `[plugin:NAME]` sections for each collector plugin, under the comment [Per plugin configuration](#per-plugin-configuration).
9.  `[CHART_NAME]` sections for each chart defined, under the comment [Per chart configuration](#per-chart-configuration).

The configuration file is a `name = value` dictionary. Netdata will not complain if you set options unknown to it. When you check the running configuration by accessing the URL `/netdata.conf` on your Netdata server, Netdata will add a comment on settings it does not currently use.

## Applying changes

After `netdata.conf` has been modified, Netdata needs to be restarted for changes to apply:

```bash
sudo service netdata restart
```

If the above does not work, try the following:

```bash
sudo killall netdata; sleep 10; sudo netdata
```

Please note that your data history will be lost if you have modified `history` parameter in section `[global]`.

## Sections

### [global] section options

| setting|default|info|||
|:-----:|:-----:|:---|---|---|
| process scheduling policy|`keep`|See [Netdata process scheduling policy](../#netdata-process-scheduling-policy)|||
| OOM score|`1000`|See [OOM score](../#oom-score)|||
| glibc malloc arena max for plugins|`1`|See [Virtual memory](../#virtual-memory).|||
| glibc malloc arena max for Netdata|`1`|See [Virtual memory](../#virtual-memory).|||
| hostname|auto-detected|The hostname of the computer running Netdata.|||
| history|`3996`| Used with `memory mode = save/map/ram/alloc`, not the default `memory mode = dbengine`. This number reflects the number of entries the `netdata` daemon will by default keep in memory for each chart dimension. This setting can also be configured per chart. Check [Memory Requirements](../../database/README.md#database) for more information. |||
| update every|`1`|The frequency in seconds, for data collection. For more information see [Performance](../../docs/Performance.md#performance).|||
| config directory|`/etc/netdata`|The directory configuration files are kept.|||
| stock config directory|`/usr/lib/netdata/conf.d`||||
| log directory|`/var/log/netdata`|The directory in which the [log files](../#log-files) are kept.|||
| web files directory|`/usr/share/netdata/web`|The directory the web static files are kept.|||
| cache directory|`/var/cache/netdata`|The directory the memory database will be stored if and when Netdata exits. Netdata will re-read the database when it will start again, to continue from the same point.|||
| lib directory|`/var/lib/netdata`|Contains the alarm log and the Netdata instance guid.|||
| home directory|`/var/cache/netdata`|Contains the db files for the collected metrics|||
| plugins directory|`"/usr/libexec/netdata/plugins.d" "/etc/netdata/custom-plugins.d"`|The directory plugin programs are kept. This setting supports multiple directories, space separated. If any directory path contains spaces, enclose it in single or double quotes.|||
| memory mode         | `dbengine` | `dbengine`: The default for long-term metrics storage with efficient RAM and disk usage. Can be extended with `page cache size` and `dbengine disk space`. <br />`save`: Netdata will save its round robin database on exit and load it on startup. <br />`map`: Cache files will be updated in real-time. Not ideal for systems with high load or slow disks (check `man mmap`). <br />`ram`: The round-robin database will be temporary and it will be lost when Netdata exits. <br />`none`: Disables the database at this host, and disables health monitoring entirely, as that requires a database of metrics. |
| page cache size     | 32         | Determines the amount of RAM in MiB that is dedicated to caching Netdata metric values. |||
| dbengine disk space | 256        | Determines the amount of disk space in MiB that is dedicated to storing Netdata metric values and all related metadata describing them |||
| host access prefix||This is used in docker environments where /proc, /sys, etc have to be accessed via another path. You may also have to set SYS_PTRACE capability on the docker for this work. Check [issue 43](https://github.com/netdata/netdata/issues/43).|
| memory deduplication (ksm)|`yes`|When set to `yes`, Netdata will offer its in-memory round robin database to kernel same page merging (KSM) for deduplication. For more information check [Memory Deduplication - Kernel Same Page Merging - KSM](../../database/#ksm)|||
| TZ environment variable|`:/etc/localtime`|Where to find the timezone|||
| timezone|auto-detected|The timezone retrieved from the environment variable|||
| debug flags|`0x0000000000000000`|Bitmap of debug options to enable. For more information check [Tracing Options](../#debugging).|||
| debug log|`/var/log/netdata/debug.log`|The filename to save debug information. This file will not be created if debugging is not enabled. You can also set it to `syslog` to send the debug messages to syslog, or `none` to disable this log. For more information check [Tracing Options](../#debugging).|||
| error log|`/var/log/netdata/error.log`|The filename to save error messages for Netdata daemon and all plugins (`stderr` is sent here for all Netdata programs, including the plugins). You can also set it to `syslog` to send the errors to syslog, or `none` to disable this log.|||
| access log|`/var/log/netdata/access.log`|The filename to save the log of web clients accessing Netdata charts. You can also set it to `syslog` to send the access log to syslog, or `none` to disable this log.|||
| errors flood protection period|`1200`|UNUSED - Length of period (in sec) during which the number of errors should not exceed the `errors to trigger flood protection`.|||
| errors to trigger flood protection|`200`|UNUSED - Number of errors written to the log in `errors flood protection period` sec before flood protection is activated.|||
| run as user|`netdata`|The user Netdata will run as.|||
| pthread stack size|auto-detected||||
| cleanup obsolete charts after seconds|`3600`|See [monitoring ephemeral containers](../../collectors/cgroups.plugin/#monitoring-ephemeral-containers), also sets the timeout for cleaning up obsolete dimensions|||
| gap when lost iterations above|`1`||||
| cleanup orphan hosts after seconds|`3600`|How long to wait until automatically removing from the DB a remote Netdata host (slave) that is no longer sending data.|||
| delete obsolete charts files|`yes`|See [monitoring ephemeral containers](../../collectors/cgroups.plugin/#monitoring-ephemeral-containers), also affects the deletion of files for obsolete dimensions|||
| delete orphan hosts files|`yes`|Set to `no` to disable non-responsive host removal.|||
| enable zero metrics|`no`|Set to `yes` to show charts when all their metrics are zero.|||

### [web] section options

Refer to the [web server documentation](../../web/server)

### [plugins] section options

In this section you will see be a boolean (`yes`/`no`) option for each plugin (e.g. tc, cgroups, apps, proc etc.). Note that the configuration options in this section for the orchestrator plugins `python.d`, `charts.d` and `node.d` control **all the modules** written for that orchestrator. For instance, setting `python.d = no` means that all Python modules under `collectors/python.d.plugin` will be disabled.

Additionally, there will be the following options:

| setting|default|info|
|:-----:|:-----:|:---|
| PATH environment variable|`auto-detected`||
| PYTHONPATH environment variable||Used to set a custom python path|
| enable running new plugins|`yes`|When set to `yes`, Netdata will enable detected plugins, even if they are not configured explicitly. Setting this to `no` will only enable plugins explicitly configirued in this file with a `yes`|
| check for new plugins every|60|The time in seconds to check for new plugins in the plugins directory. This allows having other applications dynamically creating plugins for Netdata.|
| checks|`no`|This is a debugging plugin for the internal latency|

### [health] section options

This section controls the general behavior of the health monitoring capabilities of Netdata.

Specific alarms are configured in per-collector config files under the `health.d` directory. For more info, see [health monitoring](../../health/#health-monitoring).

[Alarm notifications](../../health/notifications/#netdata-alarm-notifications) are configured in `health_alarm_notify.conf`.

| setting|default|info|
|:-----:|:-----:|:---|
| enabled|`yes`|Set to `no` to disable all alarms and notifications|
| in memory max health log entries|1000|Size of the alarm history held in RAM|
| script to execute on alarm|`/usr/libexec/netdata/plugins.d/alarm-notify.sh`|The script that sends alarm notifications. Note that in versions before 1.16, the plugins.d directory may be installed in a different location in certain OSs (e.g. under `/usr/lib/netdata`).|
| stock health configuration directory|`/usr/lib/netdata/conf.d/health.d`|Contains the stock alarm configuration files for each collector|
| health configuration directory|`/etc/netdata/health.d`|The directory containing the user alarm configuration files, to override the stock configurations|
| run at least every seconds|`10`|Controls how often all alarm conditions should be evaluated.|
| postpone alarms during hibernation for seconds|`60`|Prevents false alarms. May need to be increased if you get alarms during hibernation.|
| rotate log every lines|2000|Controls the number of alarm log entries stored in `<lib directory>/health-log.db`, where `<lib directory>` is the one configured in the [\[global\] section](#global-section-options)|

### [registry] section options

To understand what this section is and how it should be configured, please refer to the [registry documentation](../../registry).

### [backend]

Refer to the [streaming and replication](../../streaming) documentation.

## Per-plugin configuration

The configuration options for plugins appear in sections following the pattern `[plugin:NAME]`.

### Internal plugins

Most internal plugins will provide additional options. Check [Internal Plugins](../../collectors/) for more information.

Please note, that by default Netdata will enable monitoring metrics for disks, memory, and network only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though). Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all internal Netdata plugins.

### External plugins

External plugins will have only 2 options at `netdata.conf`:

| setting | default | info |
| :-----:|:-----:|:---|
| update every | the value of `[global].update every` setting|The frequency in seconds the plugin should collect values. For more information check [Performance](../../docs/Performance.md#performance).|
| command options | _empty_ | Additional command line options to pass to the plugin.|

External plugins that need additional configuration may support a dedicated file in `/etc/netdata`. Check their documentation.

## Per-chart configuration

In this area of `netdata.conf` you can find configuration options for individual charts. They appear in sections
following the pattern `[NAME]`.

Using the settings and values under these sections, you can control all aspects of a specific chart. You can change its
title, make it appear higher in Netdata's [menu](../../web/gui/README.md#menus), tweak its dimensions, and much more.

To find the name of a given chart, and thus the name of its section in `netdata.conf`, look at the top-left corner of a
chart:

![Finding the unique ID of a
chart](https://user-images.githubusercontent.com/1153921/67443082-43b16e80-f5b8-11e9-8d33-d6ee052c6678.png)

Every per-chart configuration section has several common settings, which are listed in the table just below. Beneath
that is information about lines that begin with `dim`, which affect a chart's dimensions.

| Setting           | Function                                                                                                                                                                                                                                                        |
| :---------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `history`         | Override the `history` setting in the [[global] options](#global-section-options) for this particular chart. Should be less than or equal to the global `history` setting.                                                                                      |
| `enabled`         | A boolean (`yes` or `no`) that explicitly enables or disables the chart in question.                                                                                                                                                                            |
| `cache directory` | The directory where cache files for this plugin, if needed, are stored.                                                                                                                                                                                         |
| `chart type`      | Defines what type of chart to display. It can be `line`, `area`, or `stacked`. If empty or missing, `line` will be used.                                                                                                                                        |
| `type`            | Uniquely identify which [menu](../../web/gui/README.md#menus) on the Netdata dashboard this chart should appear under. Some examples include `system` (**System**), `disk` (**Disks**), `net` (**Network Interfaces**), and `netdata` (**Netdata Monitoring**). |
| `family`          | Change the chart's [family](../../web/README.md#families) from its default. For example, you could force a disk space chart to collect metrics for family `sdb` instead of family `sda`.                                                                        |
| `units`           | Text for the label of the vertical axis of the chart. This means all dimensions should have the same unit of measurement.                                                                                                                                       |
| `context`         | Change the default [context](../../web/README.md#contexts) of the chart. Changing this setting will affect what metrics and metrics the chart displays, and which alarms are attached to it.                                                                    |
| `priority`        | Define where the chart should appear on the Netdata dashboard. Lower values equal higher priority, so a priority of `1` will place the chart highest, while a priority of `9999999` would place the chart at the bottom of the Netdata dashboard.               |
| `name`            | The name of the chart that appears in the top-left corner, after the chart's title. You can also use this name when writing [health entities](../../health/REFERENCE.md#health-entity-reference).                                                               |
| `title`           | The text that appears above the chart in the Netdata dashboard.                                                                                                                                                                                                 |

### Dimension settings

You may notice some settings that begin with `dim` beneath the ones defined in the table above. These settings determine
which dimensions appear on the given chart and how Netdata calculates them.

Each dimension setting has the following structure: `dim [DIMENSION ID] [OPTION] = [VALUE]`. The available options are `name`, `algorithm`, `multipler`, and `divisor`.

| Setting      | Function                                                                                                                                                                                                                                                      |
| :----------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `name`       | The name of the dimension as it will appear in the legend of the chart. If left empty, or is missing, Netdata will use the `[DIMENSION ID]` instead.                                                                                                          |
| `algorithm`  | Can be `absolute`, `incremental`, `percentage-of-absolute-row`, or `percentage-of-incremental-row`. If this setting is empty, invalid, or missing, Netdata will use `absolute`. See the list beneath this table for descriptions of what each algorithm does. |
| `multiplier` | An integer value by which to multiply the collected value. If empty or missing, Netdata will use `1`. This setting is often used with the value `1024` to convert metabytes to kilobytes, kilobytes to bytes, and so on.                                      |
| `divisor`    | An integer value by which to divide the collected value. If empty or missing, Netdata will use `1`. This setting is often used with the value `1024` to convert bytes to kilobytes, kilobytes to megabytes, and so on.                                        |

Here are the options for the `algorithm` setting:

-   `absolute`: The value is drawn as-is (interpolated to second boundary).
-   `incremental`: To be used when the value always increases over time, such as the I/O on a disk. Netdata takes the
    difference between the current metric and the past metric to calculate a per-second figure.
-   `percentage-of-absolute-row`: The % of this value compared to the total of all dimensions.
-   `percentage-of-incremental-row`: The % of this value compared to the incremental total of all dimensions.

For example, the `system.io` chart has the following default settings:

```conf
        # dim in name = in
        # dim in algorithm = incremental
        # dim in multiplier = 1
        # dim in divisor = 1
        # dim out name = out
        # dim out algorithm = incremental
        # dim out multiplier = -1
        # dim out divisor = 1
```

These `dim` settings produce two dimensions, `in` and `out`, both of which use the `incremental` algorithm. By
multiplying the value of `out` by -1, Netdata creates the negative values seen in the following area chart:

![The system.io chart on a MacOS
laptop](https://user-images.githubusercontent.com/1153921/69286708-2cfb3900-0bb1-11ea-9fcd-dd8fbb2adf11.png)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdaemon%2Fconfig%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
