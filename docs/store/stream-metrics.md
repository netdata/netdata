<!--
title: "Stream metrics between nodes"
description: ""
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/store/stream-metrics.md
-->

# Stream metrics between nodes

Each node running the Netdata Agent can stream the metrics it collects, in real time, to another node. Streaming allows
you to replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series
database (TSDB).

The node that receives metrics (a **parent**) can visualize metrics from the sending nodes (a **child**), run health
checks to [trigger alarms](/docs/monitor/view-active-alarms.md) and [send
notifications](/docs/monitor/enable-notifications), and [export](/docs/export/external-databases.md) all metrics to an
external TSDB.

Streaming lets you decide exactly how you want to store and maintain metrics data. While we believe Netdata's
[distributed architecture](/docs/store/distributed-data-architecture.md) is ideal for speed and scale, streaming
provides centralization options for those who want to maintain only a single TSDB instance.

## Streaming quickstart (`parent-child`)

The simplest streaming configuration is **replication**, in which a child node streams its metrics in real time to a
parent node, and both nodes retain metrics in their own databases.

See the [streaming reference doc](/streaming/README.md#supported-streaming-configurations) for details about other
possible configurations.

### Enable streaming on the parent node

First, log onto the node that will act as the parent.

Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to identify
itself while initiating a streaming connection. Copy that into a separate text file for later use.

Next, open `stream.conf` using [`edit-config`](/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files)
from within the [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata
sudo ./edit-config stream.conf
```

Scroll down to the section beginning with `[API_KEY]`. Paste the API key you generated earlier between the brackets, so
that it looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
```

Set `enabled` to `yes`. Leave all the other settings as their defaults. A simplified version of the configuration, minus
the commented lines, looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
    enabled = yes
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](/docs/configure/start-stop-restart.md) for your system.

### Enable streaming on the child node

Log into the node that will act as the child.

Open `stream.conf` again. Scroll down to the section beginning with `[stream]`. Set `enabled` to `yes`. Paste the IP
address of your parent node at the end of the `destination` line, and paste the API key generated on the parent node
onto the `api key` line.

Leave all the other settings as their defaults. A simplified version of the configuration, minus the commented lines,
looks like the following:

```conf
[stream]
    enabled = yes 
    destination = 203.0.113.0
    api key = 11111111-2222-3333-4444-555555555555
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](/docs/configure/start-stop-restart.md) for your system.

## View streamed metrics in Netdata's dashboards

At this point, the child node is streaming its metrics in real time to its parent. Open the local Agent dashboard for
the parent by navigating to `http://PARENT-NODE:19999` in your browser, replacing `PARENT-NODE` with its IP address or
hostname.

This dashboard shows parent metrics. To see child metrics, open the left-hand sidebar with the hamburger icon in the top
navigation. Both nodes appear under the **Replicated Nodes** menu. Click on either of the links to switch between
separate parent and child dashboards.

![Switching between
](https://user-images.githubusercontent.com/1153921/110043346-761ec000-7d04-11eb-8e58-77670ba39161.gif)

The child dashboard is also available directly at `http://PARENT-NODE:19999/host/CHILD-HOSTNAME`, which in this example
is `http://203.0.113.0:19999/host/netdata-child`.

## What's next?

Now that you have a basic streaming setup with replication, you may want to tweak the configuration to eliminate the
child database, disable the child dashboard, or enable SSL on the streaming connection between the parent and child.

See the [streaming reference](/streaming/README.md) for details on all the possible configurations and options.

### Related reference documentation

- [Netdata Agent · Streaming and replication](/streaming/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fstore%2Fstream-metrics&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)