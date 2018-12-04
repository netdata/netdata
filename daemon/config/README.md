# Configuration guide

<details><summary>Configuration files are placed in `/etc/netdata`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

Under that directory you will see the following:
 - `netdata.conf` is [the main configuration file](DAEMON.md#daemon-configuration) 
 - `edit-config` is [the sh script](#edit-config) that you can use to easily and safely edit the configuration
 - Other directories, initially empty, where your custom configurations for alarms and collector plugins/modules will be copied from the stock configuration, if and when you customize them using `edit-config`. 
 - `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains the stock configurations for everything not included in `netdata.conf`:
    - `health_alarm_notify.conf` is where you configure how and to who Netdata will send [alarm notifications](../health/notifications/#netdata-alarm-notifications). 
    - `health.d` is the directory that contains the alarm triggers for [health monitoring](../../health/#health-monitoring). It contains one .conf file per collector. 
    - The [modular plugin orchestrators](../../collectors/plugins.d/#external-plugins-overview) have:
        -  One config file each, mainly to turn their modules on and off: `python.d.conf` for [python](../../collectors/python.d.plugin/#pythondplugin), `node.d.conf` for [nodejs](../../collectors/node.d.plugin/#nodedplugin) and `charts.d.conf` for [bash](../../collectors/charts.d.plugin/#chartsdplugin) modules.
        - One directory each, where the module-specific configuration files can be found.
    - `stream.conf` is where you configure [streaming and replication](../../streaming/#streaming-and-replication)
    -  `stats.d` is a directory under which you can add .conf files to add [synthetic charts](../../collectors/statsd.plugin/#synthetic-statsd-charts).
    - Individual collector plugin config files, such as `fping.conf` for the [fping plugin](../../collectors/fping.plugin/) and `apps_groups.conf` for the [apps plugin](collectors/apps.plugin/) 

So there are many configuration files to control every aspect of Netdata's behavior. It can be overwhelming at first, but you won't have to deal with most of them anyway. The following HOWTO will guide you on how to customize your netdata, based on what you want to do. 

## Common customizations

I want to... | Where to look |
:-- | :-- |
Increase the metrics retention period | [netdata.conf [global]](DAEMON.md#global-section-options) : `history` |
Reduce the data collection frequency | [netdata.conf [global]](DAEMON.md#global-section-options) : `update every` |
Change the IP address/port netdata listens to | [netdata.conf [global]](DAEMON.md#web-section-options) : `bind to`, `default port` |
Modify the netdata web server settings | [netdata.conf [web]](DAEMON.md#web-section-options) : `
Make netdata never save metrics to disk<br>Make netdata always save metrics to disk | [netdata.conf [global]](DAEMON.md#global-section-options) : `memory mode` |
Move some netdata directories elsewhere | [netdata.conf [global]](DAEMON.md#global-section-options) |
Prevent netdata from getting immediately killed when my server runs out of memory | netdata.conf [global] : [OOM score](DAEMON.md#oom-score)


## edit-config script
```
USAGE:
  /etc/netdata/edit-config FILENAME

  Copy and edit the stock config file named: FILENAME
  if FILENAME is already copied, it will be edited as-is.

  The EDITOR shell variable is used to define the editor to be used.

  Stock config files at: <path>
  User  config files at: <path>

  Available files in '/usr/lib/netdata/conf.d' to copy and edit:

<list of files>
```

## Netdata simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic to use, write and understand.

So, netdata supports [simple patterns](../../libnetdata/simple_pattern/). 
