# Example Framework Demo collector

## Overview

Demonstrates the IBM.D framework capabilities with test metrics and dynamic instances.
Collects sample metrics to showcase framework features including precision handling,
instance management, and chart lifecycle.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per Example Framework Demo instance


These metrics refer to the entire monitored Example Framework Demo instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| example.test_absolute | value | seconds |
| example.test_incremental | counter | count/s |



### Per item

These metrics refer to individual item instances.

Labels:

| Label | Description |
|:------|:------------|
| slot | Slot identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| example.item_percentage | percentage | percentage |
| example.item_counter | counter | count/s |


## Configuration

### File

The configuration file name for this integration is `ibm.d/example.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/example.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| Endpoint | Connection endpoint (for demonstration) | `dummy://localhost` | no | - | - |
| ConnectTimeout | Connection timeout in seconds | `5` | no | 1 | 300 |
| CollectItems | Whether to collect item metrics | `true` | no | - | - |
| MaxItems | Maximum number of items to collect | `10` | no | 1 | 1000 |

### Examples

#### Basic configuration

Example Framework Demo monitoring with default settings.

<details>
<summary>Config</summary>

```yaml
jobs:
  - name: local
    endpoint: dummy://localhost
```

</details>

## Troubleshooting

### Debug Mode

To troubleshoot issues with the `example` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m example
```

## Getting Logs

If you're encountering problems with the `example` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep example
```

### For non-systemd systems

```bash
sudo grep example /var/log/netdata/error.log
sudo grep example /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep example
```
