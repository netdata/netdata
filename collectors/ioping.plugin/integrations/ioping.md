<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/ioping.plugin/README.md"
meta_yaml: "https://github.com/netdata/netdata/edit/master/collectors/ioping.plugin/metadata.yaml"
sidebar_label: "IOPing"
learn_status: "Published"
learn_rel_path: "Data Collection/Synthetic Checks"
most_popular: False
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE COLLECTOR'S metadata.yaml FILE"
endmeta-->

# IOPing


<img src="https://netdata.cloud/img/syslog.png" width="150"/>


Plugin: ioping.plugin
Module: ioping.plugin

<img src="https://img.shields.io/badge/maintained%20by-Netdata-%2300ab44" />

## Overview

Monitor IOPing metrics for efficient disk I/O latency tracking. Keep track of read/write speeds, latency, and error rates for optimized disk operations.

Plugin uses `ioping` command.

This collector is supported on all platforms.

This collector supports collecting metrics from multiple instances of this integration, including remote instances.


### Default Behavior

#### Auto-Detection

This integration doesn't support auto-detection.

#### Limits

The default configuration for this integration does not impose any limits on data collection.

#### Performance Impact

The default configuration for this integration is not expected to impose a significant performance impact on the system.


## Metrics

Metrics grouped by *scope*.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.



### Per disk



This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:------|:----------|:----|
| ioping.latency | latency | microseconds |



## Alerts


The following alerts are available:

| Alert name  | On metric | Description |
|:------------|:----------|:------------|
| [ ioping_disk_latency ](https://github.com/netdata/netdata/blob/master/health/health.d/ioping.conf) | ioping.latency | average I/O latency over the last 10 seconds |


## Setup

### Prerequisites

#### Install ioping

You can install the command by passing the argument `install` to the plugin (`/usr/libexec/netdata/plugins.d/ioping.plugin install`).



### Configuration

#### File

The configuration file name for this integration is `ioping.conf`.


You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ioping.conf
```
#### Options



<details><summary>Config options</summary>

| Name | Description | Default | Required |
|:----|:-----------|:-------|:--------:|
| update_every | Data collection frequency. | 1s | no |
| destination | The directory/file/device to ioping. |  | yes |
| request_size | The request size in bytes to ioping the destination (symbolic modifiers are supported) | 4k | no |
| ioping_opts | Options passed to `ioping` commands. | -T 1000000 | no |

</details>

#### Examples

##### Basic Configuration

This example has the minimum configuration necessary to have the plugin running.

<details><summary>Config</summary>

```yaml
destination="/dev/sda"

```
</details>

