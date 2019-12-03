# Health quickstart

In this getting started guide, you'll learn the basics of editing health configuration files. With this knowledge, you
will be able to customize how and when Netdata triggers alarms based on the health and performance of your system or
infrastructure.

To learn about more advanced health configurations, visit the [health reference guide](REFERENCE.md).

## What's in this getting started guide

-   [Locate health configuration files](#locate-health-configuration-files)
-   [Edit existing health configuration files](#edit-existing-health-configuration-files)
-   [Write a new health entity](#write-a-new-health-entity)
-   [Reload health configuration](#reload-health-configuration)

## Locate health configuration files

By default, Netdata will put health configuration files in `/usr/lib/netdata/conf.d/health.d`.

However, you can double-check the location of these files by navigating to `http://HOST:19999/netdata.conf` in your
browser and looking for the `stock health configuration directory` option. The value here will show the correct path for
your installation.

```conf
[health]
 ...
 # stock health configuration directory = /usr/lib/netdata/conf.d/health.d
```

Navigate to the health configuration directory to see all the available files.

```bash
cd /usr/lib/netdata/conf.d/health.d/
ls
adaptec_raid.conf entropy.conf memory.conf squid.conf
am2320.conf fping.conf mongodb.conf stiebeleltron.conf
apache.conf fronius.conf mysql.conf swap.conf
...
```

## Edit existing health configuration files

You should use `edit-config` to edit Netdata's health configuration files.

For example, to edit the `cpu.conf` health configuration file, you would run:

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config health.d/cpu.conf
```

> You may need to use `sudo` or another method of elevating your privileges.

`edit-config` will open a text editor for you to make your changes. Once you've saved and closed the editor,
`edit-config` will copy your edited file into `/etc/netdata/health.d/`, and it will now override the default in
`/usr/lib/netdata/conf.d/health.d/`.

Each health configuration file contains one or more health entities, which always begin with an `alarm:` or `template:`
line. You can edit these entities based on your needs. To make any changes live, be sure to [reload your health
configuration](#reload-health-configuration).

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

-   `alarm`: The name for your new entity. The name can be anything, but the only symbols allowed are `.` and `_`.
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
killall -USR2 netdata
```

## What's next?

To learn about all of Netdata's health configuration options, view the [reference guide](REFERENCE.md).

Or, get guided insights into specific health configurations with our [health tutorials](README.md#tutorials).

Finally, move on to Netdata's [notification system](notifications/README.md) to learn more about how Netdata can let you
know when the health of your systems or apps goes awry.
