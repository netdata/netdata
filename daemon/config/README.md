# Configuration guide

<details><summary>Configuration files are placed in `/etc/netdata`.</summary>
Depending on your installation method, Netdata will have been installed either directly under `/`, or under `/opt/netdata`. The paths mentioned here and in the documentation in general assume that your installation is under `/`. If it is not, you will find the exact same paths under `/opt/netdata` as well. (i.e. `/etc/netdata` will be `/opt/netdata/etc/netdata`).</details>

Under that directory you will see the following:
 - `netdata.conf` is the main configuration file 
 - `edit-config` is the sh script that you can use to easily and safely edit the configuration
 - `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains the stock configurations for everything not included in `netdata.conf`:
    - `health_alarm_notify.conf` is where you configure how and to who Netdata will send [alarm notifications](../health/notifications/#netdata-alarm-notifications). 
    - The alarm triggers for [health monitoring](../../health/#health-monitoring) are under the `health.d` directory, with one file per collector. 
    - Individual collector plugin config files, such as `fping.conf` for the [fping plugin](../../collectors/fping.plugin/) and `apps_groups.conf` for the [apps plugin](collectors/apps.plugin/) 
    - `stream.conf` is where you configure [streaming and replication](../../streaming/#streaming-and-replication)
    -  A directory for `stats.d`, where you can configure [synthetic charts](../../collectors/statsd.plugin/#synthetic-statsd-charts).
    - The [modular plugin orchestrators](../../collectors/plugins.d/#external-plugins-overview) have:
        -  One config file each, mainly to turn their modules on and off: `python.d.conf` for [python](../../collectors/python.d.plugin/#pythondplugin), `node.d.conf` for [nodejs](../../collectors/node.d.plugin/#nodedplugin) and `charts.d.conf` for [bash](../../collectors/charts.d.plugin/#chartsdplugin) 
        - One directory each, where module-specific configuration files can be found.
 - 5 directories, initially empty, where your custom configurations for alarms and collector plugins/modules will be copied, when you customize them using `edit-config`. 


## Netdata simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic to use, write and understand.

So, netdata supports [simple patterns](../../libnetdata/simple_pattern/). 
