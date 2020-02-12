# Configuration guide

No configuration is required to run Netdata, but you will find plenty of options to tweak, so that you can adapt it to
your particular needs.

<details markdown="1"><summary>Configuration files are placed in `/etc/netdata`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

Under that directory you will see the following:

-   `netdata.conf` is [the main configuration file](../daemon/config/README.md#daemon-configuration) 
-   `edit-config` is an sh script that you can use to easily and safely edit the configuration. Just run it to see its
    usage.
-   Other directories, initially empty, where your custom configurations for alarms and collector plugins/modules will
    be copied from the stock configuration, if and when you customize them using `edit-config`. 
-   `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains the stock configurations for
    everything not included in `netdata.conf`:
    -   `health_alarm_notify.conf` is where you configure how and to who Netdata will send [alarm
        notifications](../health/notifications/README.md#netdata-alarm-notifications). 
    -   `health.d` is the directory that contains the alarm triggers for [health
        monitoring](../health/README.md#health-monitoring). It contains one .conf file per collector. 
    -   The [modular plugin orchestrators](../collectors/plugins.d/README.md#external-plugins-overview) have:
        -   One config file each, mainly to turn their modules on and off: `python.d.conf` for
            [python](../collectors/python.d.plugin/README.md#pythondplugin), `node.d.conf` for
            [nodejs](../collectors/node.d.plugin/README.md#nodedplugin) and `charts.d.conf` for
            [bash](../collectors/charts.d.plugin/README.md#chartsdplugin) modules.
        -   One directory each, where the module-specific configuration files can be found.
    -   `stream.conf` is where you configure [streaming and
        replication](../streaming/README.md#streaming-and-replication)
    -   `stats.d` is a directory under which you can add .conf files to add [synthetic
        charts](../collectors/statsd.plugin/README.md#synthetic-statsd-charts).
    -   Individual collector plugin config files, such as `fping.conf` for the [fping
        plugin](../collectors/fping.plugin/) and `apps_groups.conf` for the [apps plugin](../collectors/apps.plugin/) 

So there are many configuration files to control every aspect of Netdata's behavior. It can be overwhelming at first,
but you won't have to deal with any of them, unless you have specific things you need to change. The following HOWTO
will guide you on how to customize your Netdata, based on what you want to do. 

## How to

### Persist my configuration

In <http://localhost:19999/netdata.conf>, you will see the following two parameters:

```bash
	# config directory = /etc/netdata
	# stock config directory = /usr/lib/netdata/conf.d
```

To persist your configurations, don't edit the files under the `stock config directory` directly. Use the `sudo [config
directory]/edit-config` command, or copy the stock config file to its proper place under the `config directory` and edit
it there. 

### Change what I see

#### Increase the long-term metrics retention period

Increase the values for the `page cache size` and `dbengine disk space` settings in the [`[global]`
section](../daemon/config/README.md#global-section-options) of `netdata.conf`. Read our tutorial on [increasing
long-term metrics storage](tutorials/longer-metrics-storage.md) and the [memory requirements for the database
engine](../database/engine/README.md#memory-requirements).

#### Reduce the data collection frequency

Increase `update every` in [netdata.conf \[global\]](../daemon/config/README.md#global-section-options). This is another
way to increase your metrics retention period, but at a lower resolution than the default 1s.

#### Modify how a chart is displayed

In `netdata.conf` under `# Per chart configuration` you will find several [\[CHART_NAME\]
sections](../daemon/config/README.md#per-chart-configuration), where you can control all aspects of a specific chart. 

#### Disable a collector

Entire plugins can be turned off from the [netdata.conf \[plugins\]](../daemon/config/README.md#plugins-section-options)
section. To disable specific modules of a plugin orchestrator, you need to edit one of the following:

-   `python.d.conf` for [python](../collectors/python.d.plugin/README.md)
-   `node.d.conf` for [nodejs](../collectors/node.d.plugin/README.md)
-   `charts.d.conf` for [bash](../collectors/charts.d.plugin/README.md)

#### Show charts with zero metrics

By default, Netdata will enable monitoring metrics for disks, memory, and network only when they are not zero. If they
are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected
and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear
though). Use `yes` instead of `auto` in plugin configuration sections to enable these charts permanently. You can also
set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all
internal Netdata plugins.

### Modify alarms and notifications

#### Add a new alarm

You can add a new alarm definition either by editing an existing stock alarm config file under `health.d` (e.g.
`/etc/netdata/edit-config health.d/load.conf`), or by adding a new `.conf` file under `/etc/netdata/health.d`. The
documentation on how to define an alarm is in [health monitoring](../health/README.md). It is
suggested to look at some of the stock alarm definitions, so you can ensure you understand how the various options work.

#### Turn off all alarms and notifications

Just set `enabled = no` in the [netdata.conf \[health\]](../daemon/config/README.md#health-section-options) section

#### Modify or disable a specific alarm

The `health.d` directory that contains the alarm triggers for [health monitoring](../health/README.md). It has
one .conf file per collector. You can easily find the .conf file you will need to modify, by looking for the "source"
line on the table that appears on the right side of an alarm on the Netdata gui. 

For example, if you click on Alarms and go to the tab 'All', the default Netdata installation will show you at the top
the configured alarm for `10 min cpu usage` (it's the name of the badge). Looking at the table on the right side, you
will see a row that says: `source  4@/usr/lib/netdata/conf.d/health.d/cpu.conf`. This way, you know that you will need
to run `/etc/netdata/edit-config health.d/cpu.conf` and look for alarm at line 4 of the conf file. 

As stated at the top of the .conf file, **you can disable an alarm notification by setting the 'to' line to: silent**.
To modify how the alarm gets triggered, we suggest that you go through the guide on [health
monitoring](../health/README.md#health-monitoring).

#### Receive notifications using my preferred method

You only need to configure `health_alarm_notify.conf`. To learn how to do it, read first [alarm
notifications](../health/notifications/README.md#netdata-alarm-notifications) and then open the submenu `Supported
Notifications` under `Alarm notifications` in the documentation to find the specific page on your preferred notification
method. 

### Make security-related customizations

#### Change the Netdata web server access lists

You have several options under the [netdata.conf \[web\]](../web/server/README.md#access-lists) section. 

#### Stop sending info to registry.my-netdata.io

You will need to configure the `[registry]` section in `netdata.conf`. First read the [registry
documentation](../registry/). In it, are instructions on how to [run your own
registry](../registry/README.md#run-your-own-registry).

#### Change the IP address/port Netdata listens to

The settings are under the `[web]` section. Look at the [web server
documentation](../web/server/README.md#binding-netdata-to-multiple-ports) for more info.

### System resource usage

#### Reduce the resources Netdata uses

The page on [Netdata performance](Performance.md) has an excellent guide on how to reduce the Netdata cpu/disk/RAM
utilization to levels suitable even for the weakest [IoT devices](netdata-for-IoT.md).

#### Change when Netdata saves metrics to disk

[netdata.conf \[global\]](../daemon/config/README.md#global-section-options): `memory mode`

#### Prevent Netdata from getting immediately killed when my server runs out of memory

You can change the Netdata [OOM score](../daemon/README.md#oom-score) in `[global]`. 

### Other

#### Move Netdata directories

The various directory paths are in [netdata.conf \[global\]](../daemon/config/README.md#global-section-options).

## How Netdata configuration works

The configuration files are `name = value` dictionaries with `[sections]`. Write whatever you like there as long as it
follows this simple format.

Netdata loads this dictionary and then when the code needs a value from it, it just looks up the `name` in the
dictionary at the proper `section`. In all places, in the code, there are both the `names` and their `default values`,
so if something is not found in the configuration file, the default is used. The lookup is made using B-Trees and hashes
(no string comparisons), so they are super fast. Also the `names` of the settings can be `my super duper setting that
once set to yes, will turn the world upside down = no` - so goodbye to most of the documentation involved.

Next, Netdata can generate a valid configuration for the user to edit. No need to remember anything. Just get the
configuration from the server (`/netdata.conf` on your Netdata server), edit it and save it.

Last, what about options you believe you have set, but you misspelled?When you get the configuration file from the
server, there will be a comment above all `name = value` pairs the server does not use. So you know that whatever you
wrote there, is not used.

## Netdata simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic to use, write and understand.

So, Netdata supports [simple patterns](../libnetdata/simple_pattern/). 

## Netdata labels

Since version 1.20, Netdata accepts user defined labels for host. The labels are defined in the section `[host labels]`.
To define a label inside this section, some rules needs to be followed, or Netdata will reject the label. The following
restrictions are applied for label names:
 
-   Names cannot start with `_`, but it can be present in other parts of the name.
-   Names only accept alphabet letters, numbers, dots, and dashes.

The policy for values is more flexible, but you can not use exclamation marks (`!`), whitespaces (` `), single quotes
(`'`), double quotes (`"`), or asterisks (`*`), because they are used to compare label values in health alarms and
templates.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfiguration-guide&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
