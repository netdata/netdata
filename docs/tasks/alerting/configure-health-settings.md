<!--
title: "Configure health settings"
sidebar_label: "Configure health settings"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/alerting/configure-health-settings.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "alerting"
learn_docs_purpose: "Instructions on how to write a health entity, notification systems/methods"
-->

Netdata's health watchdog is highly configurable, with support for dynamic thresholds, hysteresis, alert templates, and
more. You can tweak any of the existing alerts based on your infrastructure's topology or specific monitoring needs, or
create new entities.

You can use health alerts in conjunction with any of
Netdata's [collectors](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md)
(see the [supported collector list](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md)) to monitor
the health of your systems, containers, and applications in real time.

While you can see active alerts both on the local dashboard and the Cloud, all health alerts are configured _per
node_ via individual Agents. If you want to deploy a new alert across your infrastructure, you must configure each node
with the same health configuration files.

:::tip
To make any changes to your health configuration live, you must reload Netdata's health monitoring system. To do that
without restarting all of Netdata, run `netdatacli reload-health` or `killall -USR2 netdata`.
:::

## Prerequisites

- A node with the Agent installed, and terminal access to that node

## Edit health configuration files

All of Netdata's health configuration files are in Netdata's config
directory, inside the `health.d/` directory. You can edit them by following
the [Configure the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
Task.

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
      warn:
        $this > (($status >= $WARNING)  ? (75): (85))
      crit:
        $this > (($status == $CRITICAL) ? (85): (95))
    delay: down 15m multiplier 1.5 max 1h
      info: average cpu utilization for the last 10 minutes (excluding iowait, nice and steal)
        to: sysadmin
```

To tune this alert to trigger warning and critical alerts at a lower CPU utilization thresholds, change the `warn`
and `crit` lines to the values of your choosing. For example:

```yaml
    warn:
      $this > (($status >= $WARNING)  ? (60): (75))
    crit:
      $this > (($status == $CRITICAL) ? (75): (85))
```

:::info
<details><summary>Explanation of the conditional operator</summary>

Some alerts might use the conditional operator to determine in which state the alert is. Let's break down this block
of code:

```
warn: $this > (($status >= $WARNING)  ? (75) : (85))
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

In the above:

If the alert is currently a warning, then the threshold for being considered a warning is 75, otherwise it's 85.
If the alert is currently critical, then the threshold for being considered critical is 85, otherwise it's 95.

Which in turn, results in the following behavior:

While the value is rising, it will trigger a warning when it exceeds 85, and a critical alert when it exceeds 95.
While the value is falling, it will return to a warning state when it goes below 85, and a normal state when it goes
below 75.

If the value is fluctuating between 80 and 90, then it will trigger a warning the first time it goes above 85
and will remain a warning until it goes below 75 (or goes above 85). If the value is fluctuating between 90 and 100,
then it will trigger a critical alert first time it goes above 95 and will remain a critical alert until it goes below
85 - at which point it will return to being a warning.
</details>
:::

## Write a new health entity

While tuning existing alerts may work in some cases, you may need to write entirely new health entities based on how
your systems, containers, and applications work.

Read Netdata's [health reference](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) for a full
listing of the format, syntax, and functionality of health entities.

To write a new health entity into a new file, edit it by following
the [Configure the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
Task.

As an example, let's create a `ram-usage.conf` file.

```bash
sudo touch health.d/ram-usage.conf
sudo ./edit-config health.d/ram-usage.conf
```

Here is a health entity that triggers a warning alert when a node's RAM usage rises above 80%, and a critical alert
above 90%:

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

- `alarm`: The name for your new entity. The name needs to follow these requirements:
    - Any alphabet letter or number.
    - The symbols `.` and `_`.
    - Cannot be `chart name`, `dimension name`, `family name`, or `chart variable names`.
- `on`: Which chart the entity listens to.
- `lookup`: Which metrics the alert monitors, the duration of time to monitor, and how to process the metrics into a
  usable format.
    - `average`: Calculate the average of all the metrics collected.
    - `-1m`: Use metrics from 1 minute ago until now to calculate that average.
    - `percentage`: Clarify that we're calculating a percentage of RAM usage.
    - `of used`: Specify which dimension (`used`) on the `system.ram` chart you want to monitor with this entity.
- `units`: Use percentages rather than absolute units.
- `every`: How often to perform the `lookup` calculation to decide whether to trigger this alert.
- `warn`/`crit`: The value at which Netdata should trigger a warning or critical alert. This example uses simple
  syntax, but most pre-configured health entities use
  [hysteresis](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) to avoid superfluous notifications.
- `info`: A description of the alert, which will appear in the dashboard and notifications.

In human-readable format:

> This health entity, named **ram_usage**, watches the **system.ram** chart. It looks up the last **1 minute** of
> metrics from the **used** dimension and calculates the **average** of all those metrics in a **percentage** format,
> using a **% unit**. The entity performs this lookup **every minute**.
>
> If the average RAM usage percentage over the last 1 minute is **more than 80%**, the entity triggers a warning alert.
> If the usage is **more than 90%**, the entity triggers a critical alert.

When you finish writing this new health entity, make sure to reload Netdata's health configuration to see it live on the
local dashboard or on the Cloud.
