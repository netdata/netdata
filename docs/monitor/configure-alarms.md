<!--
title: "Configure health alarms"
description: "Netdata's health monitoring watchdog is incredibly adaptable to your infrastructure's unique needs, with configurable health alarms."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/monitor/configure-alarms.md
-->

# Configure health alarms

Netdata's health watchdog is highly configurable, with support for dynamic thresholds, hysteresis, alarm templates, and
more. You can tweak any of the existing alarms based on your infrastructure's topology or specific monitoring needs, or
create new entities.

You can use health alarms in conjunction with any of Netdata's [collectors](/docs/collect/how-collectors-work.md) (see
the [supported collector list](/collectors/COLLECTORS.md)) to monitor the health of your systems, containers, and
applications in real time.

While you can see active alarms both on the local dashboard and Netdata Cloud, all health alarms are configured _per
node_ via individual Netdata Agents. If you want to deploy a new alarm across your
[infrastructure](/docs/quickstart/infrastructure.md), you must configure each node with the same health configuration
files.

## Edit health configuration files

All of Netdata's [health configuration files](/health/REFERENCE.md#health-configuration-files) are in Netdata's config
directory, inside the `health.d/` directory. Use Netdata's `edit-config` script to make changes to any of these files.

For example, to edit the `cpu.conf` health configuration file, run:

```bash
sudo ./edit-config health.d/cpu.conf
```

Each health configuration file contains one or more health _entities_, which always begin with `alarm:` or `template:`.
For example, here is the first health entity in `health.d/cpu.conf`:

```yaml
template: 10min_cpu_usage
      on: system.cpu
      os: linux
   hosts: *
  lookup: average -10m unaligned of user,system,softirq,irq,guest
   units: %
   every: 1m
    warn: $this > (($status >= $WARNING)  ? (75) : (85))
    crit: $this > (($status == $CRITICAL) ? (85) : (95))
   delay: down 15m multiplier 1.5 max 1h
    info: average cpu utilization for the last 10 minutes (excluding iowait, nice and steal)
      to: sysadmin
```

To tune this alarm to trigger warning and critical alarms at a lower CPU utilization, change the `warn` and `crit` lines
to the values of your choosing. For example:

```yaml
    warn: $this > (($status >= $WARNING)  ? (60) : (75))
    crit: $this > (($status == $CRITICAL) ? (75) : (85))
```

Save the file and [reload Netdata's health configuration](#reload-health-configuration) to make your changes live.

### Silence an individual alarm

Many Netdata users don't need all the default alarms enabled. Instead of disabling any given alarm, or even _all_
alarms, you can silence individual alarms by changing one line in a given health entity. 

To silence any single alarm, change the `to:` line to `silent`.

```yaml
      to: silent
```

## Write a new health entity

While tuning existing alarms may work in some cases, you may need to write entirely new health entities based on how
your systems and applications work.

Read Netdata's [health reference](/health/REFERENCE.md#health-entity-reference) for a full listing of the format,
syntax, and functionality of health entities.

To write a new health entity, use `edit-config` to create a new file inside of the `health.d/` directory.

```bash
sudo ./edit-config health.d/example.conf
```

For example, here is a health entity that triggers an alarm when a node's RAM usage rises above 80%:

```yaml
 alarm: ram_usage
    on: system.ram
lookup: average -1m percentage of used
 units: %
 every: 1m
  warn: $this > 80
  crit: $this > 90
  info: The percentage of RAM being used by the system.
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
-   `warn`/`crit`: The value at which Netdata should trigger a warning or critical alarm. This example uses simple
    syntax, but most pre-configured health entities use
    [hysteresis](/health/REFERENCE.md#special-usage-of-the-conditional-operator) to avoid superfluous notifications.
-   `info`: A description of the alarm, which will appear in the dashboard and notifications.

In human-readable format: 

> This health entity, named **ram_usage**, watches the **system.ram** chart. It looks up the last **1 minute** of
> metrics from the **used** dimension and calculates the **average** of all those metrics in a **percentage** format,
> using a **% unit**. The entity performs this lookup **every minute**. 
> 
> If the average RAM usage percentage over the last 1 minute is **more than 80%**, the entity triggers a warning alarm.
> If the usage is **more than 90%**, the entity triggers a critical alarm.

When you finish writing this new health entity, [reload Netdata's health configuration](#reload-health-configuration) to
see it live on the local dashboard or Netdata Cloud.

## Reload health configuration

To make any changes to your health configuration live, you must reload Netdata's health monitoring system. To do that
without restarting all of Netdata, run `netdatacli reload-health` or `killall -USR2 netdata`.

## What's next?

With your health entities configured properly, it's time to [enable
notifications](/docs/monitor/enable-notifications.md) to get notified whenever a node reaches a warning or critical
state.

To build complex, dynamic alarms, read our guide on [dimension templates](/docs/guides/monitor/dimension-templates.md).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fmonitor%2Fview-active-alarms&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
