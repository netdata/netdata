<!--
title: "Health quickstart"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/QUICKSTART.md
-->

# Health quickstart

In this quickstart guide, you'll learn the basics of editing health configuration files. With this knowledge, you
will be able to customize how and when Netdata triggers alarms based on the health and performance of your system or
infrastructure.

To learn about more advanced health configurations, visit the [health reference guide](/health/REFERENCE.md).

## Edit health configuration files

You should [use `edit-config`](/docs/configure/nodes.md) to edit Netdata's health configuration files. `edit-config`
will open your system's default terminal editor for you to make your changes. Once you've saved and closed the editor,
`edit-config` will copy your edited file into `/etc/netdata/health.d/`, which will override the stock file in
`/usr/lib/netdata/conf.d/health.d/` and ensure your customizations are persistent between updates.

For example, to edit the `cpu.conf` health configuration file, you would run:

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config health.d/cpu.conf
```

Each health configuration file contains one or more health entities, which always begin with an `alarm:` or `template:`
line. You can edit these entities based on your needs. To make any changes live, be sure to [reload your health
configuration](#reload-health-configuration).

## Reference Netdata's stock health configuration files

While you should always [use `edit-config`](#edit-health-configuration-files), you might also want to view the stock
health configuration files Netdata ships with. Stock files can be useful as reference material, or to determine which
file you should edit with `edit-config`.

By default, Netdata will put health configuration files in `/usr/lib/netdata/conf.d/health.d`.  However, you can
double-check the location of these files by navigating to `http://NODE:19999/netdata.conf`, replacing `NODE` with the IP
address or hostname for your Agent dashboard, looking for the `stock health configuration directory` option. The value
here will show the correct path for your installation.

```conf
[health]
 ...
 # stock health configuration directory = /usr/lib/netdata/conf.d/health.d
```

Navigate to the health configuration directory to see all the available files and open them for reading.

```bash
cd /usr/lib/netdata/conf.d/health.d/
ls
adaptec_raid.conf entropy.conf memory.conf squid.conf
am2320.conf fping.conf mongodb.conf
apache.conf mysql.conf swap.conf
...
```

> ⚠️ If you edit configuration files in your stock health configuration directory, Netdata will overwrite them during
> any updates. Please use `edit-config` as described in the [section above](#edit-health-configuration-files).

## Write a new health entity

While tuning existing alarms may work in some cases, you may need to write entirely new health entities based on how
your systems and applications work.

To write a new health entity, let's create a new file inside of the `health.d/` directory. We'll name our file
`example.conf` for now.

```bash
./edit-config health.d/example.conf
```

As an example, let's build a health entity that triggers an alarm your system's RAM usage goes above 80%. Copy and paste
the following into the editor:

```yaml
 alarm: ram_usage
 on: system.ram
lookup: average -1m percentage of used
 units: %
 every: 1m
 warn: $this > 80
 crit: $this > 90
 info: The percentage of RAM used by the system.
```

Let's look into each of the lines to see how they create a working health entity.

-   `alarm`: The name for your new entity. The name needs to follow these requirements:
     -   Any alphabet letter or number.
     -   The symbols `.` and `_`.
     -   Cannot be `chart name`, `dimension name`, `family name`, or `chart variable names`.  
-   `on`: Which chart the entity listens to.
-   `lookup`: Which metrics the alarm monitors, the duration of time to monitor, and how to process the metrics into a
    usable format.
    -   `average`: Calculate the average of all the metrics collected.
    -   `-1m`: Use metrics from 1 minute ago until now to calculate that average.
    -   `percentage`: Clarify that we're calculating a percentage of RAM usage.
    -   `of used`: Specify which dimension (`used`) on the `system.ram` chart you want to monitor with this entity.
-   `units`: Use percentages rather than absolute units.
-   `every`: How often to perform the `lookup` calculation to decide whether or not to trigger this alarm.
-   `warn`/`crit`: The value at which Netdata should trigger a warning or critical alarm.
-   `info`: A description of the alarm, which will appear in the dashboard and notifications.

Let's put all these lines into a human-readable format.

This health entity, named **ram_usage**, watches at the **system.ram** chart. It looks up the last **1 minute** of
metrics from the **used** dimension and calculates the **average** of all those metrics in a **percentage** format,
using a **% unit**. The entity performs this lookup **every minute**. If the average RAM usage percentage over the last
1 minute is **more than 80%**, the entity triggers a warning alarm. If the usage is **more than 90%**, the entity
triggers a critical alarm.

Now that you've written a new health entity, you need to reload it to see it live on the dashboard.

## Reload health configuration

To make any changes to your health configuration live, you must reload Netdata's health monitoring system. To do that
without restarting all of Netdata, run the following:

```bash
netdatacli reload-health
```

If you receive an error like `command not found`, this means that `netdatacli` is not installed in your `$PATH`. In that 
 case, you can reload only the health component by sending a `SIGUSR2` to Netdata:

```bash
killall -USR2 netdata
```
## What's next?

To learn about all of Netdata's health configuration options, view the [reference guide](/health/REFERENCE.md) and
[daemon configuration](/daemon/config/README.md#health-section-options) for additional options available in the
`[health]` section of `netdata.conf`.

Or, get guided insights into specific health configurations with our [health guides](/health/README.md#guides).

Finally, move on to Netdata's [notification system](/health/notifications/README.md) to learn more about how Netdata can
let you know when the health of your systems or apps goes awry.


