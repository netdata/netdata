# Daemon configuration


<details markdown="1"><summary>The daemon configuration file is read from `/etc/netdata/netdata.conf`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

This config file **is not needed by default**. Netdata works fine out of the box without it. But it does allow you to adapt the general behavior of Netdata, in great detail. You can find all these settings, with their default values, by accessing the URL `https://netdata.server.hostname:19999/netdata.conf`. For example check the configuration file of [netdata.firehol.org](http://netdata.firehol.org/netdata.conf). HTTP access to this file is limited by default to private IPs, via the [web server access lists](../../web/server/#access-lists).

`netdata.conf` has sections stated with `[section]`. You will see the following sections:

1. `[global]` to [configure](#global-section-options) the [netdata daemon](../).
2. `[web]` to [configure the web server](../../web/server).
3. `[plugins]` to [configure](#plugins-section-options) which [collectors](../../collectors) to use and PATH settings.
4. `[health]` to [configure](#health-section-options) general settings for [health monitoring](../../health)
5. `[registry]` for the [netdata registry](../../registry). 
6. `[backend]` to set up [streaming and replication](../../streaming) options.
7. `[statsd]` for the general settings of the [stats.d.plugin](../../collectors/statsd.plugin). 
8. `[plugin:NAME]` sections for each collector plugin, under the comment [Per plugin configuration](#per-plugin-configuration).
9. `[CHART_NAME]` sections for each chart defined, under the comment [Per chart configuration](#per-chart-configuration).

The configuration file is a `name = value` dictionary. Netdata will not complain if you set options unknown to it. When you check the running configuration by accessing the URL `/netdata.conf` on your netdata server, netdata will add a comment on settings it does not currently use.

## Applying changes

After `netdata.conf` has been modified, netdata needs to be restarted for changes to apply:

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

setting | default | info
:------:|:-------:|:----
process scheduling policy | `keep` |  See [netdata process scheduling policy](../#netdata-process-scheduling-policy)
OOM score | `1000` |  See [OOM score](../#oom-score)
glibc malloc arena max for plugins | `1` |  See [Virtual memory](../#virtual-memory).
glibc malloc arena max for netdata | `1` |  See [Virtual memory](../#virtual-memory).
hostname | auto-detected | The hostname of the computer running netdata. 
history | `3996` | The number of entries the netdata daemon will by default keep in memory for each chart dimension. This setting can also be configured per chart. Check [Memory Requirements](../../database/#database) for more information. 
update every | `1` | The frequency in seconds, for data collection. For more information see [Performance](../../docs/Performance.md#performance). 
config directory | `/etc/netdata` | The directory configuration files are kept. 
stock config directory | `/usr/lib/netdata/conf.d` |  
log directory | `/var/log/netdata` | The directory in which the [log files](../#log-files) are kept. 
web files directory | `/usr/share/netdata/web` | The directory the web static files are kept. 
cache directory | `/var/cache/netdata` | The directory the memory database will be stored if and when netdata exits. Netdata will re-read the database when it will start again, to continue from the same point. 
lib directory | `/var/lib/netdata` | Contains the alarm log and the netdata instance guid.
home directory | `/var/cache/netdata` | Contains the db files for the collected metrics
plugins directory | `"/usr/libexec/netdata/plugins.d" "/etc/netdata/custom-plugins.d"` | The directory plugin programs are kept. This setting supports multiple directories, space separated. If any directory path contains spaces, enclose it in single or double quotes. 
memory mode | `save` | When set to `save` netdata will save its round robin database on exit and load it on startup. When set to `map` the cache files will be updated in real time (check `man mmap` - do not set this on systems with heavy load or slow disks - the disks will continuously sync the in-memory database of netdata). When set to `ram` the round robin database will be temporary and it will be lost when netdata exits. `none` disables the database at this host. This also disables health monitoring (there cannot be health monitoring without a database). host access prefix | | This is used in docker environments where /proc, /sys, etc have to be accessed via another path. You may also have to set SYS_PTRACE capability on the docker for this work. Check [issue 43](https://github.com/netdata/netdata/issues/43). 
memory deduplication (ksm) | `yes` | When set to `yes`, netdata will offer its in-memory round robin database to kernel same page merging (KSM) for deduplication. For more information check [Memory Deduplication - Kernel Same Page Merging - KSM](../../database/#ksm) 
TZ environment variable | `:/etc/localtime` | Where to find the timezone 
timezone | auto-detected | The timezone retrieved from the environment variable 
debug flags | `0x0000000000000000` | Bitmap of debug options to enable. For more information check [Tracing Options](../#debugging). 
debug log | `/var/log/netdata/debug.log` | The filename to save debug information. This file will not be created is debugging is not enabled. You can also set it to `syslog` to send the debug messages to syslog, or `none` to disable this log. For more information check [Tracing Options](../#debugging). 
error log | `/var/log/netdata/error.log` | The filename to save error messages for netdata daemon and all plugins (`stderr` is sent here for all netdata programs, including the plugins). You can also set it to `syslog` to send the errors to syslog, or `none` to disable this log. 
access log | `/var/log/netdata/access.log` | The filename to save the log of web clients accessing netdata charts. You can also set it to `syslog` to send the access log to syslog, or `none` to disable this log. 
errors flood protection period | `1200` | UNUSED - Length of period (in sec) during which the number of errors should not exceed the `errors to trigger flood protection`.
errors to trigger flood protection | `200` |  UNUSED - Number of errors written to the log in `errors flood protection period` sec before flood protection is activated.
run as user | `netdata` | The user netdata will run as. 
pthread stack size | auto-detected |  
cleanup obsolete charts after seconds | `3600` |  See [monitoring ephemeral containers](../../collectors/cgroups.plugin/#monitoring-ephemeral-containers)
gap when lost iterations above | `1` |  
cleanup orphan hosts after seconds | `3600` |  How long to wait until automatically removing from the DB a remote netdata host (slave) that is no longer sending data.
delete obsolete charts files | `yes` |  See [monitoring ephemeral containers](../../collectors/cgroups.plugin/#monitoring-ephemeral-containers)
delete orphan hosts files | `yes` |  Set to `no` to disable non-responsive host removal.

### [web] section options

Refer to the [web server documentation](../../web/server)

### [plugins] section options

In this section you will see be a boolean (`yes`/`no`) option for each plugin (e.g. tc, cgroups, apps, proc etc.). Note that the configuration options in this section for the orchestrator plugins `python.d`, `charts.d` and `node.d` control **all the modules** written for that orchestrator. For instance, setting `python.d = no` means that all Python modules under `collectors/python.d.plugin` will be disabled.  

Additionally, there will be the following options:

setting | default | info
:------:|:-------:|:----
PATH environment variable | `auto-detected` | 
PYTHONPATH environment variable | | Used to set a custom python path
enable running new plugins | `yes` | When set to `yes`, netdata will enable detected plugins, even if they are not configured explicitly. Setting this to `no` will only enable plugins explicitly configirued in this file with a `yes` 
check for new plugins every | 60 | The time in seconds to check for new plugins in the plugins directory. This allows having other applications dynamically creating plugins for netdata.
checks | `no` | This is a debugging plugin for the internal latency 

### [health] section options

This section controls the general behavior of the health monitoring capabilities of Netdata. 

Specific alarms are configured in per-collector config files under the `health.d` directory. For more info, see [health monitoring](../../health/#health-monitoring). 

[Alarm notifications](../../health/notifications/#netdata-alarm-notifications) are configured in `health_alarm_notify.conf`. 

setting | default | info
:------:|:-------:|:----
enabled | `yes` | Set to `no` to disable all alarms and notifications
in memory max health log entries | 1000 | Size of the alarm history held in RAM
script to execute on alarm | `/usr/libexec/netdata/plugins.d/alarm-notify.sh` | The script that sends alarm notifications. 
stock health configuration directory | `/usr/lib/netdata/conf.d/health.d` | Contains the stock alarm configuration files for each collector
health configuration directory | `/etc/netdata/health.d` | The directory containing the user alarm configuration files, to override the stock configurations
run at least every seconds | `10` | Controls how often all alarm conditions should be evaluated. 
postpone alarms during hibernation for seconds | `60` | Prevents false alarms. May need to be increased if you get alarms during hibernation.
rotate log every lines | 2000 | Controls the number of alarm log entries stored in `<lib directory>/health-log.db`, where <lib directory> is the one configured in the [[global] section](#global-section-options)

### [registry] section options

To understand what this section is and how it should be configured, please refer to the [registry documentation](../../registry). 

### [backend]

Refer to the [streaming and replication](../../streaming) documentation.

### Per plugin configuration

The configuration options for plugins appear in sections following the pattern `[plugin:NAME]`.

#### Internal plugins

Most internal plugins will provide additional options. Check [Internal Plugins](../../collectors/) for more information.

#### External plugins

External plugins will have only 2 options at `netdata.conf`:

setting | default | info
:------:|:-------:|:----
update every|the value of `[global].update every` setting|The frequency in seconds the plugin should collect values. For more information check [Performance](../../docs/Performance.md#performance).
command options|*empty*|Additional command line options to pass to the plugin. 

External plugins that need additional configuration may support a dedicated file in `/etc/netdata`. Check their documentation.

### Per chart configuration

In this section you will a separate subsection for each chart shown on the dashboard. You can control all aspects of a specific chart here. You can understand what each option does by reading [how charts are defined](../../collectors/plugins.d/#chart). If you don't know how to find the name of a chart, you can learn about it [here](../../docs/Charts.md).
