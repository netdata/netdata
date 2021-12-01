<!--
title: "Stop notifications for individual alarms"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/stop-notifications-alarms.md
-->

# Stop notifications for individual alarms

In this short tutorial, you'll learn how to stop notifications for individual alarms in Netdata's health
monitoring system. We also refer to this process as _silencing_ the alarm.

Why silence alarms? We designed Netdata's pre-configured alarms  for production systems, so they might not be 
relevant if you run Netdata on your laptop or a small virtual server. If they're not helpful, they can be a distraction
to real issues with health and performance.

Silencing individual alarms is an excellent solution for situations where you're not interested in seeing a specific
alarm but don't want to disable a [notification system](/health/notifications/README.md) entirely. 

## Find the alarm configuration file

To silence an alarm, you need to know where to find its configuration file.

Let's use the `system.cpu` chart as an example. It's the first chart you'll see on most Netdata dashboards.

To figure out which file you need to edit, open up Netdata's dashboard and, click the **Alarms** button at the top
of the dashboard, followed by clicking on the **All** tab.

In this example, we're looking for the `system - cpu` entity, which, when opened, looks like this:

![The system - cpu alarm
entity](https://user-images.githubusercontent.com/1153921/67034648-ebb4cc80-f0cc-11e9-9d49-1023629924f5.png)

In the `source` row, you see that this chart is getting its configuration from
`4@/usr/lib/netdata/conf.d/health.d/cpu.conf`. The relevant part of begins at `health.d`: `health.d/cpu.conf`. That's
the file you need to edit if you want to silence this alarm.

For more information about editing or referencing health configuration files on your system, see the [health
quickstart](/health/QUICKSTART.md#edit-health-configuration-files).

## Edit the file to enable silencing

To edit `health.d/cpu.conf`, use `edit-config` from inside of your Netdata configuration directory.

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config health.d/cpu.conf
```

> You may need to use `sudo` or another method of elevating your privileges.

The beginning of the file looks like this:

```yaml
template: 10min_cpu_usage
 on: system.cpu
 os: linux
 hosts: *
 lookup: average -10m unaligned of user,system,softirq,irq,guest
 units: %
 every: 1m
 warn: $this > (($status >= $WARNING) ? (75) : (85))
 crit: $this > (($status == $CRITICAL) ? (85) : (95))
 delay: down 15m multiplier 1.5 max 1h
 info: average cpu utilization for the last 10 minutes (excluding iowait, nice and steal)
 to: sysadmin
```

To silence this alarm, change `sysadmin` to `silent`.

```yaml
 to: silent
```

Use one of the available [methods](/health/QUICKSTART.md#reload-health-configuration) to reload your health configuration 
 and ensure you get no more notifications about that alarm**.

You can add `to: silent` to any alarm you'd rather not bother you with notifications.

## What's next?

You should now know the fundamentals behind silencing any individual alarm in Netdata.

To learn about _all_ of Netdata's health configuration possibilities, visit the [health reference
guide](/health/REFERENCE.md), or check out other [tutorials on health monitoring](/health/README.md#tutorials).

Or, take better control over how you get notified about alarms via the [notification
system](/health/notifications/README.md).

You can also use Netdata's [Health Management API](/web/api/health/README.md#health-management-api) to control health
checks and notifications while Netdata runs. With this API, you can disable health checks during a maintenance window or
backup process, for example.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fstop-notifications-alarms%2F&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
