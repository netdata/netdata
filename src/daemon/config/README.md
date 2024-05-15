<!--
title: "Daemon configuration"
description: "The Netdata Agent's daemon is installed preconfigured to collect thousands of metrics every second, but is highly configurable for real-world workloads."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/daemon/config/README.md"
sidebar_label: "Daemon"
learn_status: "Published"
learn_rel_path: "Configuration"
learn_doc_purpose: "Explain the daemon options, the log files, the process scheduling, virtual memory, explain how the netdata.conf is used and backlink to the netdata.conf file reference"
-->

# Daemon configuration

<details>
<summary>The daemon configuration file is read from /etc/netdata/netdata.conf.</summary>

Depending on your installation method, Netdata will have been installed either directly under `/`, or
under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is
under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will
be `/opt/netdata/etc/netdata`).

</details>

This config file **is not needed by default**. Netdata works fine out of the box without it. But it does allow you to
adapt the general behavior of Netdata, in great detail. You can find all these settings, with their default values, by
accessing the URL `https://netdata.server.hostname:19999/netdata.conf`. For example check the configuration file
of [netdata.firehol.org](http://netdata.firehol.org/netdata.conf). HTTP access to this file is limited by default to
[private IPs](https://en.wikipedia.org/wiki/Private_network), via
the [web server access lists](https://github.com/netdata/netdata/blob/master/src/web/server/README.md#access-lists).

`netdata.conf` has sections stated with `[section]`. You will see the following sections:

1. `[global]` to [configure](#global-section-options) the [Netdata daemon](https://github.com/netdata/netdata/blob/master/src/daemon/README.md).
2. `[db]` to [configure](#db-section-options) the database of Netdata.
3. `[directories]` to [configure](#directories-section-options) the directories used by Netdata.
4. `[logs]` to [configure](#logs-section-options) the Netdata logging.
5. `[environment variables]` to [configure](#environment-variables-section-options) the environment variables used
   Netdata.
6. `[sqlite]` to [configure](#sqlite-section-options) the [Netdata daemon](https://github.com/netdata/netdata/blob/master/src/daemon/README.md) SQLite settings.
7. `[ml]` to configure settings for [machine learning](https://github.com/netdata/netdata/blob/master/src/ml/README.md).
8. `[health]` to [configure](#health-section-options) general settings for [health monitoring](https://github.com/netdata/netdata/blob/master/src/health/README.md).
9. `[web]` to [configure the web server](https://github.com/netdata/netdata/blob/master/src/web/server/README.md).
10. `[registry]` for the [Netdata registry](https://github.com/netdata/netdata/blob/master/src/registry/README.md).
11. `[global statistics]` for the [Netdata registry](https://github.com/netdata/netdata/blob/master/src/registry/README.md).
12. `[statsd]` for the general settings of the [stats.d.plugin](https://github.com/netdata/netdata/blob/master/src/collectors/statsd.plugin/README.md).
13. `[plugins]` to [configure](#plugins-section-options) which [collectors](https://github.com/netdata/netdata/blob/master/src/collectors/README.md) to use and PATH
    settings.
14. `[plugin:NAME]` sections for each collector plugin, under the
    comment [Per plugin configuration](#per-plugin-configuration).

The configuration file is a `name = value` dictionary. Netdata will not complain if you set options unknown to it. When
you check the running configuration by accessing the URL `/netdata.conf` on your Netdata server, Netdata will add a
comment on settings it does not currently use.

## Applying changes

After `netdata.conf` has been modified, Netdata needs to be [restarted](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for
changes to apply:

```bash
sudo systemctl restart netdata
```

If the above does not work, try the following:

```bash
sudo killall netdata; sleep 10; sudo netdata
```

Please note that your data history will be lost if you have modified `history` parameter in section `[global]`.

## Sections

### [global] section options

|              setting               |    default    | info                                                                                                                                                                                                                                         |
|:----------------------------------:|:-------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|     process scheduling policy      |    `keep`     | See [Netdata process scheduling policy](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#netdata-process-scheduling-policy)                                                                                                   |
|             OOM score              |      `0`      |                                                                                                                                                                                                                                              |
| glibc malloc arena max for plugins |      `1`      | See [Virtual memory](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#virtual-memory).                                                                                                                                        |
| glibc malloc arena max for Netdata |      `1`      | See [Virtual memory](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#virtual-memory).                                                                                                                                        |
|              hostname              | auto-detected | The hostname of the computer running Netdata.                                                                                                                                                                                                |
|         host access prefix         |     empty     | This is used in docker environments where /proc, /sys, etc have to be accessed via another path. You may also have to set SYS_PTRACE capability on the docker for this work. Check [issue 43](https://github.com/netdata/netdata/issues/43). |
|              timezone              | auto-detected | The timezone retrieved from the environment variable                                                                                                                                                                                         |
|            run as user             |   `netdata`   | The user Netdata will run as.                                                                                                                                                                                                                |
|         pthread stack size         | auto-detected |                                                                                                                                                                                                                                              |

### [db] section options

|                    setting                    |  default   | info                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|:---------------------------------------------:|:----------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|                     mode                      | `dbengine` | `dbengine`: The default for long-term metrics storage with efficient RAM and disk usage. Can be extended with `dbengine page cache size MB` and `dbengine disk space MB`. <br />`ram`: The round-robin database will be temporary and it will be lost when Netdata exits. <br />`alloc`: Similar to `ram`, but can significantly reduce memory usage, when combined with a low retention and does not support KSM. <br />`none`: Disables the database at this host, and disables health monitoring entirely, as that requires a database of metrics. Not to be used together with streaming. |
|                   retention                   |   `3600`   | Used with `mode = ram/alloc`, not the default `mode = dbengine`. This number reflects the number of entries the `netdata` daemon will by default keep in memory for each chart dimension. Check [Memory Requirements](https://github.com/netdata/netdata/blob/master/src/database/README.md) for more information.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
|                 storage tiers                 |    `3`     | The number of storage tiers you want to have in your dbengine. Check the tiering mechanism in the [dbengine's reference](https://github.com/netdata/netdata/blob/master/src/database/engine/README.md#tiering). You can have up to 5 tiers of data (including the _Tier 0_). This number ranges between 1 and 5.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|          dbengine page cache size MB          |    `32`    | Determines the amount of RAM in MiB that is dedicated to caching for _Tier 0_ Netdata metric values.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|   dbengine tier **`N`** page cache size MB    |    `32`    | Determines the amount of RAM in MiB that is dedicated for caching Netdata metric values of the **`N`** tier. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
|            dbengine disk space MB             |   `256`    | Determines the amount of disk space in MiB that is dedicated to storing _Tier 0_ Netdata metric values and all related metadata describing them. This option is available **only for legacy configuration** (`Agent v1.23.2 and prior`).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
|       dbengine multihost disk space MB        |   `256`    | Same functionality as `dbengine disk space MB`, but includes support for storing metrics streamed to a parent node by its children. Can be used in single-node environments as well. This setting is only for _Tier 0_ metrics.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| dbengine tier **`N`** multihost disk space MB |   `256`    | Same functionality as `dbengine multihost disk space MB`, but stores metrics of the **`N`** tier (both parent node and its children). Can be used in single-node environments as well. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|                 update every                  |    `1`     | The frequency in seconds, for data collection. For more information see the [performance guide](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md). These metrics stored as _Tier 0_ data. Explore the tiering mechanism in the [dbengine's reference](https://github.com/netdata/netdata/blob/master/src/database/engine/README.md#tiering).                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| dbengine tier **`N`** update every iterations |    `60`    | The down sampling value of each tier from the previous one. For each Tier, the greater by one Tier has N (equal to 60 by default) less data points of any metric it collects. This setting can take values from `2` up to `255`. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
|        dbengine tier **`N`** back fill        |   `New`    | Specifies the strategy of recreating missing data on each Tier from the exact lower Tier. <br /> `New`: Sees the latest point on each Tier and save new points to it only if the exact lower Tier has available points for it's observation window (`dbengine tier N update every iterations` window). <br /> `none`: No back filling is applied. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                       |
|          memory deduplication (ksm)           |   `yes`    | When set to `yes`, Netdata will offer its in-memory round robin database and the dbengine page cache to kernel same page merging (KSM) for deduplication. For more information check [Memory Deduplication - Kernel Same Page Merging - KSM](https://github.com/netdata/netdata/blob/master/src/database/README.md#ksm)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|      cleanup obsolete charts after secs       |   `3600`   | See [monitoring ephemeral containers](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/README.md#monitoring-ephemeral-containers), also sets the timeout for cleaning up obsolete dimensions                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|        gap when lost iterations above         |    `1`     |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
|        cleanup orphan hosts after secs        |   `3600`   | How long to wait until automatically removing from the DB a remote Netdata host (child) that is no longer sending data.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
|              enable zero metrics              |    `no`    | Set to `yes` to show charts when all their metrics are zero.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

> ### Info
>
>The multiplication of all the **enabled** tiers  `dbengine tier N update every iterations` values  must be less than `65535`.

### [directories] section options

|       setting       |                              default                               | info                                                                                                                                                                               |
|:-------------------:|:------------------------------------------------------------------:|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|       config        |                           `/etc/netdata`                           | The directory configuration files are kept.                                                                                                                                        |
|    stock config     |                     `/usr/lib/netdata/conf.d`                      |                                                                                                                                                                                    |
|         log         |                         `/var/log/netdata`                         | The directory in which the [log files](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#log-files) are kept.                                                        |
|         web         |                      `/usr/share/netdata/web`                      | The directory the web static files are kept.                                                                                                                                       |
|        cache        |                        `/var/cache/netdata`                        | The directory the memory database will be stored if and when Netdata exits. Netdata will re-read the database when it will start again, to continue from the same point.           |
|         lib         |                         `/var/lib/netdata`                         | Contains the alert log and the Netdata instance GUID.                                                                                                                              |
|        home         |                        `/var/cache/netdata`                        | Contains the db files for the collected metrics.                                                                                                                                   |
|        lock         |                      `/var/lib/netdata/lock`                       | Contains the data collectors lock files.                                                                                                                                           |
|       plugins       | `"/usr/libexec/netdata/plugins.d" "/etc/netdata/custom-plugins.d"` | The directory plugin programs are kept. This setting supports multiple directories, space separated. If any directory path contains spaces, enclose it in single or double quotes. |
|    health config    |                      `/etc/netdata/health.d`                       | The directory containing the user alert configuration files, to override the stock configurations                                                                                  |
| stock health config |                 `/usr/lib/netdata/conf.d/health.d`                 | Contains the stock alert configuration files for each collector                                                                                                                    |
|      registry       |              `/opt/netdata/var/lib/netdata/registry`               | Contains the [registry](https://github.com/netdata/netdata/blob/master/src/registry/README.md) database and GUID that uniquely identifies each Netdata Agent                           |

### [logs] section options

|              setting               |            default            | info                                                                                                                                                                                                                                                                                                                             |
|:----------------------------------:|:-----------------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|            debug flags             |     `0x0000000000000000`      | Bitmap of debug options to enable. For more information check [Tracing Options](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#debugging).                                                                                                                                                                      |
|               debug                | `/var/log/netdata/debug.log`  | The filename to save debug information. This file will not be created if debugging is not enabled. You can also set it to `syslog` to send the debug messages to syslog, or `none` to disable this log. For more information check [Tracing Options](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#debugging). |
|               error                | `/var/log/netdata/error.log`  | The filename to save error messages for Netdata daemon and all plugins (`stderr` is sent here for all Netdata programs, including the plugins). You can also set it to `syslog` to send the errors to syslog, or `none` to disable this log.                                                                                     |
|               access               | `/var/log/netdata/access.log` | The filename to save the log of web clients accessing Netdata charts. You can also set it to `syslog` to send the access log to syslog, or `none` to disable this log.                                                                                                                                                           |
|              facility              |           `daemon`            | A facility keyword is used to specify the type of system that is logging the message.                                                                                                                                                                                                                                            |
|   errors flood protection period   |            `1200`             | Length of period (in sec) during which the number of errors should not exceed the `errors to trigger flood protection`.                                                                                                                                                                                                          |
| errors to trigger flood protection |             `200`             | Number of errors written to the log in `errors flood protection period` sec before flood protection is activated.                                                                                                                                                                                                                |
|           severity level           |            `info`             | Controls which log messages are logged, with error being the most important. Supported values: `info` and `error`.                                                                                                                                                                                                               |

### [environment variables] section options

|  setting   |      default      | info                                                       |
|:----------:|:-----------------:|:-----------------------------------------------------------|
|     TZ     | `:/etc/localtime` | Where to find the timezone                                 |
|    PATH    |  `auto-detected`  | Specifies the directories to be searched to find a command |
| PYTHONPATH |                   | Used to set a custom python path                           |

### [sqlite] section options

|      setting       |    default    | info                                                                                                                                                                             |
|:------------------:|:-------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|    auto vacuum     | `INCREMENTAL` | The [auto-vacuum status](https://www.sqlite.org/pragma.html#pragma_auto_vacuum) in the database                                                                                  |
|    synchronous     |   `NORMAL`    | The setting of the ["synchronous"](https://www.sqlite.org/pragma.html#pragma_synchronous) flag                                                                                   |
|    journal mode    |     `WAL`     | The [journal mode](https://www.sqlite.org/pragma.html#pragma_journal_mode) for databases                                                                                         |
|     temp store     |   `MEMORY`    | Used to determine where [temporary tables and indices are stored](https://www.sqlite.org/pragma.html#pragma_temp_store)                                                          |
| journal size limit |  `16777216`   | Used to set a new [limit in bytes for the database](https://www.sqlite.org/pragma.html#pragma_journal_size_limit)                                                                |
|     cache size     |    `-2000`    | Used to [suggest the maximum number of database disk pages](https://www.sqlite.org/pragma.html#pragma_cache_size) that SQLite will hold in memory at once per open database file |

### [health] section options

This section controls the general behavior of the health monitoring capabilities of Netdata.

Specific alerts are configured in per-collector config files under the `health.d` directory. For more info, see [health
monitoring](https://github.com/netdata/netdata/blob/master/src/health/README.md).

[Alert notifications](https://github.com/netdata/netdata/blob/master/src/health/notifications/README.md) are configured in `health_alarm_notify.conf`.

|                    setting                     |                     default                      | info                                                                                                                                                                                                                                                                                                                                            |
|:----------------------------------------------:|:------------------------------------------------:|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|                    enabled                     |                      `yes`                       | Set to `no` to disable all alerts and notifications                                                                                                                                                                                                                                                                                             |
|        in memory max health log entries        |                       1000                       | Size of the alert history held in RAM                                                                                                                                                                                                                                                                                                           |
|           script to execute on alarm           | `/usr/libexec/netdata/plugins.d/alarm-notify.sh` | The script that sends alert notifications. Note that in versions before 1.16, the plugins.d directory may be installed in a different location in certain OSs (e.g. under `/usr/lib/netdata`).                                                                                                                                                  |
|           run at least every seconds           |                       `10`                       | Controls how often all alert conditions should be evaluated.                                                                                                                                                                                                                                                                                    |
| postpone alarms during hibernation for seconds |                       `60`                       | Prevents false alerts. May need to be increased if you get alerts during hibernation.                                                                                                                                                                                                                                                           |
|               health log history               |                     `432000`                     | Specifies the history of alert events (in seconds) kept in the agent's sqlite database.                                                                                                                                                                                                                                                         |
|                 enabled alarms                 |                        *                         | Defines which alerts to load from both user and stock directories. This is a [simple pattern](https://github.com/netdata/netdata/blob/master/src/libnetdata/simple_pattern/README.md) list of alert or template names. Can be used to disable specific alerts. For example, `enabled alarms =  !oom_kill *` will load all alerts except `oom_kill`. |

### [web] section options

Refer to the [web server documentation](https://github.com/netdata/netdata/blob/master/src/web/server/README.md)

### [plugins] section options

In this section you will see be a boolean (`yes`/`no`) option for each plugin (e.g. tc, cgroups, apps, proc etc.). Note
that the configuration options in this section for the orchestrator plugins `python.d` and  `charts.d` control **all the
modules** written for that orchestrator. For instance, setting `python.d = no` means that all Python modules
under `collectors/python.d.plugin` will be disabled.

Additionally, there will be the following options:

|             setting             |     default     | info                                                                                                                                                                                               |
|:-------------------------------:|:---------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|   enable running new plugins    |      `yes`      | When set to `yes`, Netdata will enable detected plugins, even if they are not configured explicitly. Setting this to `no` will only enable plugins explicitly configured in this file with a `yes` |
|   check for new plugins every   |       60        | The time in seconds to check for new plugins in the plugins directory. This allows having other applications dynamically creating plugins for Netdata.                                             |
|             checks              |      `no`       | This is a debugging plugin for the internal latency                                                                                                                                                |

### [registry] section options

To understand what this section is and how it should be configured, please refer to
the [registry documentation](https://github.com/netdata/netdata/blob/master/src/registry/README.md).

## Per-plugin configuration

The configuration options for plugins appear in sections following the pattern `[plugin:NAME]`.

### Internal plugins

Most internal plugins will provide additional options. Check [Internal Plugins](https://github.com/netdata/netdata/blob/master/src/collectors/README.md) for more
information.

Please note, that by default Netdata will enable monitoring metrics for disks, memory, and network only when they are
not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started,
will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them
to appear though). Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You
can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics
for all internal Netdata plugins.

### External plugins

External plugins will have only 2 options at `netdata.conf`:

|     setting     |                   default                    | info                                                                                                                                                                                                |
|:---------------:|:--------------------------------------------:|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|  update every   | the value of `[global].update every` setting | The frequency in seconds the plugin should collect values. For more information check the [performance guide](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md). |
| command options |                      -                       | Additional command line options to pass to the plugin.                                                                                                                                              |           

External plugins that need additional configuration may support a dedicated file in `/etc/netdata`. Check their
documentation.

