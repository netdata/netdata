# Daemon Configuration Reference

The Netdata daemon's main configuration file is located at `/INSTALL_PREFIX/netdata/netdata.conf`. While Netdata works effectively with default settings, this file allows you to fine-tune its behavior.

You can view your current configuration, including default values, at `http://IP:19999/netdata.conf`. Access to this URL is [restricted to local IPs by default](/src/web/server/README.md#access-lists).

The configuration file uses an INI-style format with `[SECTION]` headers:

| Section                                                           | Controls                                                 |
|-------------------------------------------------------------------|----------------------------------------------------------|
| [[global]](#global-section-options)                               | [Daemon](/src/daemon/README.md)                          |
| [[db]](#db-section-options)                                       | [Database](/src/database/README.md)                      |
| [[directories]](#directories-section-options)                     | Directories used by Netdata                              |
| [[logs]](#logs-section-options)                                   | Logging                                                  |
| [[environment variables]](#environment-variables-section-options) | Environment variables                                    |
| [[sqlite]](#sqlite-section-options)                               | SQLite                                                   |
| `[ml]`                                                            | [Machine Learning](/src/ml/README.md)                    |
| [[health]](#health-section-options)                               | [Health monitoring](/src/health/README.md)               |
| `[web]`                                                           | [Web Server](/src/web/server/README.md)                  |
| `[registry]`                                                      | [Registry](/src/registry/README.md)                      |
| `[telemetry]`                                                     | Internal monitoring                                      |
| `[statsd]`                                                        | [StatsD plugin](/src/collectors/statsd.plugin/README.md) |
| [`[plugins]`](#plugins-section-options)                           | Data collection Plugins (Collectors)                     |
| [[plugin:NAME]](#per-plugin-configuration)                        | Individual [Plugins](#per-plugin-configuration)          |

> **Note**
>
> The configuration uses a simple `name = value` format. Netdata tolerates unknown options, marking them with comments when viewing the running configuration through `/netdata.conf`.

## Applying changes

After `netdata.conf` has been modified, Netdata needs to be [restarted](/docs/netdata-agent/start-stop-restart.md) for changes to apply.

## Configuration Sections

### `global` section options

|              setting               |    default     | info                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
|:----------------------------------:|:--------------:|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|              profile               | auto-detected  | Can be `iot`, `child`, `parent`, `standalone`. Depending on the profile detected, Netdata changes various internal settings (like the number of allocation arenas, the max allocation size, streaming compression levels, shared memory cleanup frequency, etc) to optimize performance and balance resources usage. Especially for `iot`, it disables machine learning based anomaly detection. See below for more information.                                |
|     process scheduling policy      |     `keep`     | See [Netdata process scheduling policy](/src/daemon/README.md#process-scheduling-policy-unix-only)                                                                                                                                                                                                                                                                                                                                                              |
|             OOM score              |      `0`       |                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| glibc malloc arena max for plugins | auto-detected  | This settings affects memory allocations performance and fragmentation. More arenas give better performance, but they introduce more fragmentation.                                                                                                                                                                                                                                                                                                             |
| glibc malloc arena max for Netdata | auto-detected  | This settings affects memory allocations performance and fragmentation. More arenas give better performance, but they introduce more fragmentation.                                                                                                                                                                                                                                                                                                             |
|              hostname              | auto-detected  | The hostname of the computer running Netdata.                                                                                                                                                                                                                                                                                                                                                                                                                   |
|         host access prefix         |     empty      | This is used in Docker environments where /proc, /sys, etc have to be accessed via another path. You may also have to set SYS_PTRACE capability on the docker for this work. Check [issue 43](https://github.com/netdata/netdata/issues/43).                                                                                                                                                                                                                    |
|              timezone              | auto-detected  | The timezone retrieved from the environment variable                                                                                                                                                                                                                                                                                                                                                                                                            |
|            run as user             |   `netdata`    | The user Netdata will run as.                                                                                                                                                                                                                                                                                                                                                                                                                                   |
|         pthread stack size         | auto-detected  |                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|           crash reports            | `all` or `off` | `all` when anonymous telemetry is enabled, or the agent is claimed or connected to Netdata Cloud (directly or via a Netdata Parent). When it is `all` Netdata reports restarts and crashes. It can also be `crashes` to report only crashes. When it is `off` nothing is reported. Each kind of event is deduplicated and reported at most once per day. [Read more at this blog post](https://www.netdata.cloud/blog/2025-03-06-monitoring-netdata-restarts/). |  

#### Profiles

The profiles are detected in this order:

1. `iot` is used when the system has 1 CPU core and/or less than 1GiB of RAM. It has the highest priority among all the profiles, so that if this is detected, it will be used instead of any of the others.
2. `parent` is detected when `stream.conf` has configuration for receiving data from child nodes and the system is not `iot`.
3. `child` is detected when `stream.conf` has configuration for sending data to a parent node, does not have configuration for receiving data from other nodes, and the system is not `iot`.
4. `standalone` is the fallback profile when none of the above are detected.

The following are the parameters affected by the profile:

|                 Feature                  |   iot   |   parent   |  child   | standalone |
|:----------------------------------------:|:-------:|:----------:|:--------:|:----------:|
|          libc allocation arenas          |    1    |     4      |    1     |     1      |
|          libc memory reclaiming          |  16KiB  |   128KiB   |  32KiB   |   64KiB    |
|   outbound streaming compression level   | fastest |  fastest   | balanced |  balanced  |
|        max batch allocation size         |  16KiB  | 2MiB (THP) |  32KiB   |   64KiB    |
|        machine learning training         |   off   |    auto    |   auto   |    auto    |
| dbengine journal files unmapping timeout |   2m    |    off     |    2m    |     2m     |

A few of these settings can be individually configured in `netdata.conf`, like the libc allocation arenas and machine learning. The rest are automatically set based on the profile.

### `db` section options

|                    setting                    |            default             | info                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|:---------------------------------------------:|:------------------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|                     mode                      |           `dbengine`           | `dbengine`: The default for long-term metrics storage with efficient RAM and disk usage. Can be extended with `dbengine page cache size` and `dbengine tier X retention size`. <br />`ram`: The round-robin database will be temporary and it will be lost when Netdata exits. <br />`alloc`: Similar to `ram`, but can significantly reduce memory usage, when combined with a low retention and does not support KSM. <br />`none`: Disables the database at this host, and disables Health monitoring entirely, as that requires a database of metrics. Not to be used together with streaming. |
|                   retention                   |             `3600`             | Used with `mode = ram/alloc`, not the default `mode = dbengine`. This number reflects the number of entries the `netdata` daemon will by default keep in memory for each chart dimension. Check [Memory Requirements](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md) for more information.                                                                                                                                                                                                                                                                          |
|                 storage tiers                 |              `3`               | The number of storage tiers you want to have in your dbengine. Check the tiering mechanism in the [dbengine's reference](/src/database/engine/README.md#tiers). You can have up to 5 tiers of data (including the _Tier 0_). This number ranges between 1 and 5.                                                                                                                                                                                                                                                                                                                                   |
|           dbengine page cache size            |            `32MiB`             | Determines the amount of RAM in MiB that is dedicated to caching for _Tier 0_ Netdata metric values.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|     dbengine tier **`N`** retention size      |             `1GiB`             | The disk space dedicated to metrics storage, per tier. Can be used in single-node environments as well. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|     dbengine tier **`N`** retention time      | `14d`, `3mo`, `1y`, `1y`, `1y` | The database retention, expressed in time. Can be used in single-node environments as well. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|                 update every                  |              `1`               | The frequency in seconds, for data collection. For more information see the [performance guide](/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md). These metrics stored as _Tier 0_ data. Explore the tiering mechanism in the [dbengine's reference](/src/database/engine/README.md#tiers).                                                                                                                                                                                                                                                                           |
| dbengine tier **`N`** update every iterations |              `60`              | The down sampling value of each tier from the previous one. For each Tier, the greater by one Tier has N (equal to 60 by default) less data points of any metric it collects. This setting can take values from `2` up to `255`. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                                                                                                                      |
|            dbengine tier back fill            |             `new`              | Specifies the strategy of recreating missing data on higher database Tiers.<br /> `new`: Sees the latest point on each Tier and save new points to it only if the exact lower Tier has available points for it's observation window (`dbengine tier N update every iterations` window). <br /> `none`: No back filling is applied. <br /> `N belongs to [1..4]`                                                                                                                                                                                                                                    |
|          memory deduplication (ksm)           |             `yes`              | When set to `yes`, Netdata will offer its in-memory round robin database and the dbengine page cache to kernel same page merging (KSM) for deduplication.                                                                                                                                                                                                                                                                                                                                                                                                                                          |
|         cleanup obsolete charts after         |              `1h`              | See [monitoring ephemeral containers](/src/collectors/cgroups.plugin/README.md#monitoring-ephemeral-containers), also sets the timeout for cleaning up obsolete dimensions                                                                                                                                                                                                                                                                                                                                                                                                                         |
|        gap when lost iterations above         |              `1`               |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
|          cleanup orphan hosts after           |              `1h`              | How long to wait until automatically removing from the DB a remote Netdata host (child) that is no longer sending data.                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
|              enable zero metrics              |              `no`              | Set to `yes` to show charts when all their metrics are zero.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |

> ### Info
>
> The multiplication of all the **enabled** tiers  `dbengine tier N update every iterations` values must be less than `65535`.

### `directories` section options

|       setting       |                              default                               | info                                                                                                                                                                               |
|:-------------------:|:------------------------------------------------------------------:|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|       config        |                           `/etc/netdata`                           | The directory configuration files are kept.                                                                                                                                        |
|    stock config     |                     `/usr/lib/netdata/conf.d`                      |                                                                                                                                                                                    |
|         log         |                         `/var/log/netdata`                         | The directory in which the [log files](/src/daemon/README.md#logging) are kept.                                                                                                    |
|         web         |                      `/usr/share/netdata/web`                      | The directory the web static files are kept.                                                                                                                                       |
|        cache        |                        `/var/cache/netdata`                        | The directory the memory database will be stored if and when Netdata exits. Netdata will re-read the database when it will start again, to continue from the same point.           |
|         lib         |                         `/var/lib/netdata`                         | Contains the Alert log and the Netdata instance GUID.                                                                                                                              |
|        home         |                        `/var/cache/netdata`                        | Contains the db files for the collected metrics.                                                                                                                                   |
|        lock         |                      `/var/lib/netdata/lock`                       | Contains the data collectors lock files.                                                                                                                                           |
|       plugins       | `"/usr/libexec/netdata/plugins.d" "/etc/netdata/custom-plugins.d"` | The directory plugin programs are kept. This setting supports multiple directories, space separated. If any directory path contains spaces, enclose it in single or double quotes. |
|    Health config    |                      `/etc/netdata/health.d`                       | The directory containing the user Alert configuration files, to override the stock configurations                                                                                  |
| stock Health config |                 `/usr/lib/netdata/conf.d/health.d`                 | Contains the stock Alert configuration files for each collector                                                                                                                    |
|      registry       |              `/opt/netdata/var/lib/netdata/registry`               | Contains the [registry](/src/registry/README.md) database and GUID that uniquely identifies each Netdata Agent                                                                     |

### `logs` section options

There are additional configuration options for the logs. For more info, see [Netdata Logging](/src/libnetdata/log/README.md).

|             setting              |            default            | info                                                                                                                                                                                                                                                                                  |
|:--------------------------------:|:-----------------------------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|           debug flags            |     `0x0000000000000000`      | Bitmap of debug options to enable. For more information check [Tracing Options](/src/daemon/README.md#debugging).                                                                                                                                                                     |
|              debug               | `/var/log/netdata/debug.log`  | The filename to save debug information. This file will not be created if debugging is not enabled. You can also set it to `syslog` to send the debug messages to syslog, or `off` to disable this log. For more information check [Tracing Options](/src/daemon/README.md#debugging). |
|              error               | `/var/log/netdata/error.log`  | The filename to save error messages for Netdata daemon and all plugins (`stderr` is sent here for all Netdata programs, including the plugins). You can also set it to `syslog` to send the errors to syslog, or `off` to disable this log.                                           |
|              access              | `/var/log/netdata/access.log` | The filename to save the log of web clients accessing Netdata charts. You can also set it to `syslog` to send the access log to syslog, or `off` to disable this log.                                                                                                                 |
|            collector             |           `journal`           | The filename to save the log of Netdata collectors. You can also set it to `syslog` to send the access log to syslog, or `off` to disable this log. Defaults to `Journal` if using systemd.                                                                                           |
|              Health              |           `journal`           | The filename to save the log of Netdata Health collectors. You can also set it to `syslog` to send the access log to syslog, or `off` to disable this log. Defaults to `Journal` if using systemd.                                                                                    |
|              daemon              |           `journal`           | The filename to save the log of Netdata daemon. You can also set it to `syslog` to send the access log to syslog, or `off` to disable this log. Defaults to `Journal` if using systemd.                                                                                               |
|             facility             |           `daemon`            | A facility keyword is used to specify the type of system that is logging the message.                                                                                                                                                                                                 |
|   logs flood protection period   |             `1m`              | Length of period during which the number of errors should not exceed the `errors to trigger flood protection`.                                                                                                                                                                        |
| logs to trigger flood protection |            `1000`             | Number of errors written to the log in `errors flood protection period` sec before flood protection is activated.                                                                                                                                                                     |
|              level               |            `info`             | Controls which log messages are logged, with error being the most important. Supported values: `info` and `error`.                                                                                                                                                                    |

### `environment variables` section options

|  setting   |      default      | info                                                       |
|:----------:|:-----------------:|:-----------------------------------------------------------|
|     TZ     | `:/etc/localtime` | Where to find the timezone                                 |
|    PATH    |  `auto-detected`  | Specifies the directories to be searched to find a command |
| PYTHONPATH |                   | Used to set a custom python path                           |

### `sqlite` section options

|      setting       |    default    | info                                                                                                                                                                             |
|:------------------:|:-------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|    auto vacuum     | `INCREMENTAL` | The [auto-vacuum status](https://www.sqlite.org/pragma.html#pragma_auto_vacuum) in the database                                                                                  |
|    synchronous     |   `NORMAL`    | The setting of the ["synchronous"](https://www.sqlite.org/pragma.html#pragma_synchronous) flag                                                                                   |
|    journal mode    |     `WAL`     | The [journal mode](https://www.sqlite.org/pragma.html#pragma_journal_mode) for databases                                                                                         |
|     temp store     |   `MEMORY`    | Used to determine where [temporary tables and indices are stored](https://www.sqlite.org/pragma.html#pragma_temp_store)                                                          |
| journal size limit |  `16777216`   | Used to set a new [limit in bytes for the database](https://www.sqlite.org/pragma.html#pragma_journal_size_limit)                                                                |
|     cache size     |    `-2000`    | Used to [suggest the maximum number of database disk pages](https://www.sqlite.org/pragma.html#pragma_cache_size) that SQLite will hold in memory at once per open database file |

### `health` section options

This section controls the general behavior of the Health monitoring capabilities of Netdata.

Specific Alerts are configured in per-collector config files under the `health.d` directory. For more info, see [health
monitoring](/src/health/README.md).

[Alert notifications](/src/health/notifications/README.md) are configured in `health_alarm_notify.conf`.

|                setting                 |                     default                      | info                                                                                                                                                                                                                                                                                                  |
|:--------------------------------------:|:------------------------------------------------:|:------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|                enabled                 |                      `yes`                       | Set to `no` to disable all Alerts and notifications                                                                                                                                                                                                                                                   |
|    in memory max Health log entries    |                       1000                       | Size of the Alert history held in RAM                                                                                                                                                                                                                                                                 |
|       script to execute on alarm       | `/usr/libexec/netdata/plugins.d/alarm-notify.sh` | The script that sends Alert notifications. Note that in versions before 1.16, the plugins.d directory may be installed in a different location in certain OSs (e.g. under `/usr/lib/netdata`).                                                                                                        |
|           run at least every           |                      `10s`                       | Controls how often all Alert conditions should be evaluated.                                                                                                                                                                                                                                          |
| postpone alarms during hibernation for |                       `1m`                       | Prevents false Alerts. May need to be increased if you get Alerts during hibernation.                                                                                                                                                                                                                 |
|          Health log retention          |                       `5d`                       | Specifies the history of Alert events (in seconds) kept in the Agent's sqlite database.                                                                                                                                                                                                               |
|             enabled alarms             |                        *                         | Defines which Alerts to load from both user and stock directories. This is a [simple pattern](/src/libnetdata/simple_pattern/README.md) list of Alert or template names. Can be used to disable specific Alerts. For example, `enabled alarms =  !oom_kill *` will load all Alerts except `oom_kill`. |

### `web` section options

Refer to the [web server documentation](/src/web/server/README.md)

### `plugins` section options

In this section you will see be a boolean (`yes`/`no`) option for each plugin (e.g., tc, cgroups, apps, proc etc.). Note that the configuration options in this section for the orchestrator plugins `python.d` and  `charts.d` control **all the modules** written for that orchestrator. For instance, setting `python.d = no` means that all Python modules under `collectors/python.d.plugin` will be disabled.

Additionally, there will be the following options:

|           setting           | default | info                                                                                                                                                                                               |
|:---------------------------:|:-------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| enable running new plugins  |  `yes`  | When set to `yes`, Netdata will enable detected plugins, even if they are not configured explicitly. Setting this to `no` will only enable plugins explicitly configured in this file with a `yes` |
| check for new plugins every |   60    | The time in seconds to check for new plugins in the plugins directory. This allows having other applications dynamically creating plugins for Netdata.                                             |
|           checks            |  `no`   | This is a debugging plugin for the internal latency                                                                                                                                                |

### `registry` section options

To understand what this section is and how it should be configured, refer to the [registry documentation](/src/registry/README.md).

## Per-plugin configuration

The configuration options for plugins appear in sections following the pattern `[plugin:NAME]`.

### Internal plugins

Most internal plugins will provide additional options. Check [Internal Plugins](/src/collectors/README.md) for more information.

Note that by default, Netdata will enable monitoring metrics for disks, memory, and network only when they are not zero. If they are constantly zero, they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard when refreshed. Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics
for all internal Netdata plugins.

### External plugins

External plugins will have only two options at `netdata.conf`:

|     setting     |                   default                    | info                                                                                                                                                                                         |
|:---------------:|:--------------------------------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|  update every   | the value of `[global].update every` setting | The frequency in seconds the plugin should collect values. For more information check the [performance guide](/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md). |
| command options |                      -                       | Additional command line options to pass to the plugin.                                                                                                                                       |

External plugins that need additional configuration may support a dedicated file in `/etc/netdata`. Check their documentation.
