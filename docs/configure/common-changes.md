<!--
title: "Common configuration changes"
description: "See the most popular configuration changes to make to the Netdata Agent, including longer metrics retention, reduce sampling, and more."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/common-changes.md
-->

# Common configuration changes

The Netdata Agent requires no configuration upon installation to collect thousands of per-second metrics from most
systems, containers, and applications, but there are hundreds of settings to tweak if you want to exercise more control
over your monitoring platform.

This document assumes familiarity with using [`edit-config`](/docs/configure/nodes.md) from the Netdata config
directory.

## Change dashboards and visualizations

The Netdata Agent's [local dashboard](/web/gui/README.md), accessible at `http://NODE:19999` is highly configurable. If
you use Netdata Cloud for [infrastructure monitoring](/docs/quickstart/infrastructure.md), you will see many of these
changes reflected in those visualizations due to the way Netdata Cloud proxies metric data and metadata to your browser.

### Increase the long-term metrics retention period

Increase the values for the `page cache size` and `dbengine multihost disk space` settings in the [`[global]`
section](/daemon/config/README.md#global-section-options) of `netdata.conf`.

```conf
[global]
    page cache size = 128   # 128 MiB of memory for metrics storage
    dbengine multihost disk space = 4096   # 4GiB of disk space for metrics storage
```

Read our doc on [increasing long-term metrics storage](/docs/store/change-metrics-storage.md) for details, including a
[calculator](/docs/store/change-metrics-storage.md#calculate-the-system-resources-RAM-disk-space-needed-to-store-metrics)
to help you determine the exact settings for your desired retention period.

### Reduce the data collection frequency

Change `update every` in the [`[global]` section](/daemon/config/README.md#global-section-options) of `netdata.conf` so
that it is greater than `1`. An `update every` of `5` means the Netdata Agent enforces a _minimum_ collection frequency
of 5 seconds.

```conf
[global]
    update every = 5
```

Every collector and plugin has its own `update every` setting, which you can also change in the `go.d.conf`,
`python.d.conf`, `node.d.conf`, or `charts.d.conf` files, or in individual collector configuration files. If the `update
every` for an individual collector is less than the global, the Netdata Agent uses the global setting. See the [enable
or configure a collector](/docs/collect/enable-configure.md) doc for details.

### Disable a collector or plugin

Turn off entire plugins in the [`[plugins]` section](/daemon/config/README.md#plugins-section-options) of
`netdata.conf`.

To disable specific collectors, open `go.d.conf`, `python.d.conf`, `node.d.conf`, or `charts.d.conf` and find the line
for that specific module. Uncomment the line and change its value to `no`.

## Modify alarms and notifications

Netdata's health monitoring watchdog uses hundreds of preconfigured health entities, with intelligent thresholds, to
generate warning and critical alarms for most production systems and their applications without configuration. However,
each alarm and notification method is completely customizable.

### Add a new alarm

To create a new alarm configuration file, initiate an empty file, with a filename that ends in `.conf`, in the
`health.d/` directory. The Netdata Agent loads any valid alarm configuration file ending in `.conf` in that directory.
Next, edit the new file with `edit-config`. For example, with a file called `example-alarm.conf`.

```bash
sudo touch health.d/example-alarm.conf
sudo ./edit-config health.d/example-alarm.conf
```

Or, append your new alarm to an existing file by editing a relevant existing file in the `health.d/` directory.

Read more about [configuring alarms](/docs/monitor/configure-alarms.md) to get started, and see the [health monitoring
reference](/health/REFERENCE.md) for a full listing of options available in health entities.

### Configure a specific alarm

Tweak existing alarms by editing files in the `health.d/` directory. For example, edit `health.d/cpu.conf` to change how
the Agent responds to anomalies related to CPU utilization.

To see which configuration file you need to edit to configure a specific alarm, [view your active
alarms](/docs/monitor/view-active-alarms.md) in Netdata Cloud or the local Agent dashboard and look for the **source**
line. For example, it might read `source  4@/usr/lib/netdata/conf.d/health.d/cpu.conf`. 

Because the source path contains `health.d/cpu.conf`, run `sudo edit-config health.d/cpu.conf` to configure that alarm.

### Disable a specific alarm

Open the configuration file for that alarm and set the `to` line to `silent`.

```conf
template: disk_fill_rate
       on: disk.space
   lookup: max -1s at -30m unaligned of avail
     calc: ($this - $avail) / (30 * 60)
    every: 15s
       to: silent
```

### Turn of all alarms and notifications

Set `enabled` to `no` in the [`[health]` section](/daemon/config/README.md#health-section-options) section of
`netdata.conf`.

### Enable alarm notifications

Open `health_alarm_notify.conf` for editing. First, read the [enabling
notifications](/docs/monitor/enable-notifications.md#netdata-agent) doc for an example of the process using Slack, then
click on the link to your preferred notification method to find documentation for that specific endpoint.

## Improve node security

While the Netdata Agent is both [open and secure by design](https://www.netdata.cloud/blog/netdata-agent-dashboard/), we
recommend every user take some action to administer and secure their nodes.

Learn more about a few of the following changes in the [node security doc](/docs/configure/secure-nodes.md).

### Disable the local Agent dashboard (`http://NODE:19999`)

If you use Netdata Cloud to visualize metrics, stream metrics to a parent node, or otherwise don't need the local Agent
dashboard, disabling it reduces the Agent's resource utilization and improves security.

Change the `mode` setting to `none` in the [`[web]` section](/web/server/README.md#configuration) of `netdata.conf`.

```conf
[web]
    mode = none
```

### Use access lists to restrict access to specific assets

Allow access from only specific IP addresses, ranges of IP addresses, or hostnames using [access
lists](/web/server/README.md#access-lists) and [simple patterns](/libnetdata/simple_pattern/README.md).

See a quickstart to access lists in the [node security
doc](/docs/configure/secure-nodes.md#restrict-access-to-the-local-dashboard).

### Stop sending anonymous statistics to Google Analytics

Create a file called `.opt-out-from-anonymous-statistics` inside of your Netdata config directory to immediately stop
the statistics script.

```bash
sudo touch .opt-out-from-anonymous-statistics
```

Learn more about [why we collect anonymous statistics](/docs/anonymous-statistics.md).

### Change the IP address/port Netdata listens to

Change the `default port` setting in the `[web]` section to a port other than `19999`.

```conf
[web]
   default port = 39999
```

Use the `bind to` setting to the ports other assets, such as the [running `netdata.conf`
configuration](/docs/configure/nodes.md#see-an-agents-running-configuration), API, or streaming requests listen to.

## Reduce resource usage

Read our [performance optimization guide](/docs/guides/configure/performance.md) for a long list of specific changes
that can reduce the Netdata Agent's CPU/memory footprint and IO requirements.

## Organize nodes with host labels

Beginning with v1.20, Netdata accepts user-defined **host labels**. These labels are sent during streaming, exporting,
and as metadata to Netdata Cloud, and help you organize the metrics coming from complex infrastructure. Host labels are
defined in the section `[host labels]`. 

For a quick introduction, read the [host label guide](/docs/guides/using-host-labels.md).

The following restrictions apply to host label names: 
 
-   Names cannot start with `_`, but it can be present in other parts of the name.
-   Names only accept alphabet letters, numbers, dots, and dashes.

The policy for values is more flexible, but you can not use exclamation marks (`!`), whitespaces (` `), single quotes
(`'`), double quotes (`"`), or asterisks (`*`), because they are used to compare label values in health alarms and
templates.

## What's next?

If you haven't already, learn how to [secure your nodes](/docs/configure/secure-nodes.md).

As mentioned at the top, there are plenty of other 

You can also take what you've learned about node configuration to tweak the Agent's behavior or enable new features:

- [Enable new collectors](/docs/collect/enable-configure.md) or tweak their behavior.
- [Configure existing health alarms](/docs/monitor/configure-alarms.md) or create new ones.
- [Enable notifications](/docs/monitor/enable-notifications.md) to receive updates about the health of your
  infrastructure.
- Change [the long-term metrics retention period](/docs/store/change-metrics-storage.md) using the database engine.

### Related reference documentation

- [Netdata Agent · Daemon](/health/README.md)
- [Netdata Agent · Daemon configuration](/daemon/config/README.md)
- [Netdata Agent · Web server](/web/server/README.md)
- [Netdata Agent · Local Agent dashboard](/web/gui/README.md)
- [Netdata Agent · Health monitoring](/health/REFERENCE.md)
- [Netdata Agent · Notifications](/health/notifications/README.md)
- [Netdata Agent · Simple patterns](/libnetdata/simple_pattern/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fcommon-changes&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
