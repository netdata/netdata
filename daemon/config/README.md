# Configuration guide

No configuration is required to run netdata, but you fill find plenty of options to tweak, so that you can adapt it to your particular needs.

<details markdown="1"><summary>Configuration files are placed in `/etc/netdata`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

Under that directory you will see the following:
 - `netdata.conf` is [the main configuration file](DAEMON.md#daemon-configuration) 
 - `edit-config` is an sh script that you can use to easily and safely edit the configuration. Just run it to see its usage.
 - Other directories, initially empty, where your custom configurations for alarms and collector plugins/modules will be copied from the stock configuration, if and when you customize them using `edit-config`. 
 - `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains the stock configurations for everything not included in `netdata.conf`:
    - `health_alarm_notify.conf` is where you configure how and to who Netdata will send [alarm notifications](../../health/notifications/#netdata-alarm-notifications). 
    - `health.d` is the directory that contains the alarm triggers for [health monitoring](../../health/#health-monitoring). It contains one .conf file per collector. 
    - The [modular plugin orchestrators](../../collectors/plugins.d/#external-plugins-overview) have:
        -  One config file each, mainly to turn their modules on and off: `python.d.conf` for [python](../../collectors/python.d.plugin/#pythondplugin), `node.d.conf` for [nodejs](../../collectors/node.d.plugin/#nodedplugin) and `charts.d.conf` for [bash](../../collectors/charts.d.plugin/#chartsdplugin) modules.
        - One directory each, where the module-specific configuration files can be found.
    - `stream.conf` is where you configure [streaming and replication](../../streaming/#streaming-and-replication)
    -  `stats.d` is a directory under which you can add .conf files to add [synthetic charts](../../collectors/statsd.plugin/#synthetic-statsd-charts).
    - Individual collector plugin config files, such as `fping.conf` for the [fping plugin](../../collectors/fping.plugin/) and `apps_groups.conf` for the [apps plugin](../../collectors/apps.plugin/) 

So there are many configuration files to control every aspect of Netdata's behavior. It can be overwhelming at first, but you won't have to deal with any of them, unless you have specific things you need to change. The following HOWTO will guide you on how to customize your netdata, based on what you want to do. 

## Common customizations

I want to... 

<details markdown="1"><summary>Increase the metrics retention period
</summary>Increase `history` in [netdata.conf [global]](DAEMON.md#global-section-options). Just ensure you understand [how much memory will be required](../../database).</details>
<details markdown="1"><summary>Reduce the data collection frequency</summary>Increase `update every` in [netdata.conf [global]]
(DAEMON.md#global-section-options). This is another way to increase your metrics retention period, but at a lower resolution than the default 1s.</details>
<details markdown="1"><summary>Change the IP address/port netdata listens to</summary>The settings are under netdata.conf [web]. Look at the [web server documentation](../../web/server/#binding-netdata-to-multiple-ports) for more info.</details>
<details markdown="1"><summary>Modify the netdata web server settings</summary>[netdata.conf [web]](DAEMON.md#web-section-options) : `</details>
<details markdown="1"><summary>Change when netdata saves metrics to disk</summary> [netdata.conf [global]](DAEMON.md#global-section-options) : `memory mode`</details>
<details markdown="1"><summary>Move some netdata directories elsewhere</summary>[netdata.conf [global]](DAEMON.md#global-section-options)</details>
#### Prevent netdata from getting immediately killed when my server runs out of memory

netdata.conf [global] : [OOM score](../#oom-score)


## How netdata configuration works

The configuration files are `name = value` dictionaries with `[sections]`. Write whatever you like there as long as it follows this simple format.

Netdata loads this dictionary and then when the code needs a value from it, it just looks up the `name` in the dictionary at the proper `section`. In all places, in the code, there are both the `names` and their `default values`, so if something is not found in the configuration file, the default is used. The lookup is made using B-Trees and hashes (no string comparisons), so they are super fast. Also the `names` of the settings can be `my super duper setting that once set to yes, will turn the world upside down = no` - so goodbye to most of the documentation involved.

Next, netdata can generate a valid configuration for the user to edit. No need to remember anything. Just get the configuration from the server (`/netdata.conf` on your netdata server), edit it and save it.

Last, what about options you believe you have set, but you misspelled?When you get the configuration file from the server, there will be a comment above all `name = value` pairs the server does not use. So you know that whatever you wrote there, is not used.

## Netdata simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic to use, write and understand.

So, netdata supports [simple patterns](../../libnetdata/simple_pattern/). 
