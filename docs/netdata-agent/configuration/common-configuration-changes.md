# Common configuration changes

The Netdata Agent requires no configuration upon installation to collect thousands of per-second metrics from most
systems, containers, and applications, but there are hundreds of settings to tweak if you want to exercise more control
over your monitoring platform.

This document assumes familiarity with
using [`edit-config`](/docs/netdata-agent/configuration/README.md) from the Netdata config
directory.

## Change dashboards and visualizations

The Netdata Agent's [local dashboard](/docs/dashboards-and-charts/README.md), accessible
at `http://NODE:19999` is highly configurable. If
you use [Netdata Cloud](/docs/netdata-cloud/README.md)
for infrastructure monitoring, you
will see many of these
changes reflected in those visualizations due to the way Netdata Cloud proxies metric data and metadata to your browser.

### Increase the long-term metrics retention period

Read our doc on [increasing long-term metrics storage](/src/database/CONFIGURATION.md#tiers) for details.

## Modify alerts and notifications

Netdata's health monitoring watchdog uses hundreds of pre-configured health entities, with intelligent thresholds, to
generate warning and critical alerts for most production systems and their applications without configuration. However,
each alert and notification method is completely customizable.

### Add a new alert

To create a new alert configuration file, initiate an empty file, with a filename that ends in `.conf`, in the
`health.d/` directory. The Netdata Agent loads any valid alert configuration file ending in `.conf` in that directory.
Next, edit the new file with `edit-config`. For example, with a file called `example-alert.conf`.

```bash
sudo touch health.d/example-alert.conf
sudo ./edit-config health.d/example-alert.conf
```

Or, append your new alert to an existing file by editing a relevant existing file in the `health.d/` directory.

Read more about [configuring alerts](/src/health/REFERENCE.md) to
get started, and see
the [health monitoring reference](/src/health/REFERENCE.md) for a full listing
of options available in health entities.

### Configure a specific alert

Tweak existing alerts by editing files in the `health.d/` directory. For example, edit `health.d/cpu.conf` to change how
the Agent responds to anomalies related to CPU utilization.

To see which configuration file you need to edit to configure a specific
alert, [view your active alerts](/docs/dashboards-and-charts/alerts-tab.md) in
Netdata Cloud or the local Agent dashboard and look for the **source** line. For example, it might
read `source  4@/usr/lib/netdata/conf.d/health.d/cpu.conf`.

Because the source path contains `health.d/cpu.conf`, run `sudo edit-config health.d/cpu.conf` to configure that alert.

### Disable a specific alert

Open the configuration file for that alert and set the `to` line to `silent`.

```text
template: disk_fill_rate
       on: disk.space
   lookup: max -1s at -30m unaligned of avail
     calc: ($this - $avail) / (30 * 60)
    every: 15s
       to: silent
```

### Turn of all alerts and notifications

Set `enabled` to `no` in
the [`[health]`](/src/daemon/config/README.md#health-section-options)
section of `netdata.conf`.

### Enable alert notifications

Open `health_alarm_notify.conf` for editing. First, read the [enabling notifications](/src/health/notifications/README.md) doc
for an example of the process using Slack, then
click on the link to your preferred notification method to find documentation for that specific endpoint.

## Improve node security

While the Netdata Agent is both [open and secure by design](https://www.netdata.cloud/blog/netdata-agent-dashboard/), we
recommend every user take some action to administer and secure their nodes.

Learn more about the available options in the [security design documentation](/docs/security-and-privacy-design/README.md).

## Reduce resource usage

Read
our [performance optimization guide](/docs/netdata-agent/configuration/optimize-the-netdata-agents-performance.md)
for a long list of specific changes
that can reduce the Netdata Agent's CPU/memory footprint and IO requirements.

## Organize nodes with host labels

Beginning with v1.20, Netdata accepts user-defined **host labels**. These labels are sent during streaming, exporting,
and as metadata to Netdata Cloud, and help you organize the metrics coming from complex infrastructure. Host labels are
defined in the section `[host labels]`.

For a quick introduction, read
the [host label guide](/docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md).

The following restrictions apply to host label names:

- Names cannot start with `_`, but it can be present in other parts of the name.
- Names only accept alphabet letters, numbers, dots, and dashes.

The policy for values is more flexible, but you cannot use exclamation marks (`!`), whitespaces (` `), single quotes
(`'`), double quotes (`"`), or asterisks (`*`), because they are used to compare label values in health alerts and
templates.
