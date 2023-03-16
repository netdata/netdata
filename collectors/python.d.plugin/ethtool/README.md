<!--
title: "Network monitoring interface via ethtool for Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/ethtool/README.md
sidebar_label: "Network interfaces"
-->

# Network with ethtool monitoring with Netdata

Monitors performance metrics (bandwidth, number of packets discarded etc.) using `ethtool` cli tool.

> **Warning**: this collector does not work when the Netdata Agent is [running in a container](https://learn.netdata.cloud/docs/agent/packaging/docker).


## Requirements and Notes

-   You must have the `ethtool` tool installed and your network interface must support the tool. Read more about [ethtool](https://mymellanox.force.com/mellanoxcommunity/s/article/understanding-mlx5-ethtool-counters).
-   Make sure `netdata` user can execute `/usr/bin/ethtool` or wherever your binary is.
-   `poll_seconds` is how often in seconds the tool is polled for as an integer.

## Charts

It produces the following charts:

-   Network Bandwidth Utilization for Rx and TX in `Gb/s`
-   Network Bandwidth Utilization for Rx and TX in `percentage`
-   Packet loss for Rx and Tx in `absolute`

## Configuration

Edit the `python.d/ethtool.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ethtool.conf
```

Sample:

```yaml
loop_mode    : yes
poll_seconds : 1
dev_filter : "eno" # Only network devices containing eno in their name will be measured
```


