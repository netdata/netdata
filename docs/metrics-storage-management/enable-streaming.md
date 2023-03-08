# How metrics streaming works

Each node running Netdata can stream the metrics it collects, in real time, to another node. Streaming allows you to
replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series database
(TSDB).

When one node streams metrics to another, the node receiving metrics can visualize them on the dashboard, run health checks to 
[trigger alarms](https://github.com/netdata/netdata/blob/master/docs/monitor/view-active-alarms.md) and 
[send notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md), and
[export](https://github.com/netdata/netdata/blob/master/docs/export/external-databases.md) all metrics to an external TSDB. When Netdata streams metrics to another
Netdata, the receiving one is able to perform everything a Netdata instance is capable of.

Streaming lets you decide exactly how you want to store and maintain metrics data. While we believe Netdata's
[distributed architecture](https://github.com/netdata/netdata/blob/master/docs/store/distributed-data-architecture.md) is 
ideal for speed and scale, streaming provides centralization options and high data availability.

This document will get you started quickly with streaming. More advanced concepts and suggested production deployments
can be found in the [streaming and replication reference](https://github.com/netdata/netdata/blob/master/streaming/README.md).

## Streaming basics

There are three types of nodes in Netdata's streaming ecosystem.

- **Parent**: A node, running Netdata, that receives streamed metric data.
- **Child**: A node, running Netdata, that streams metric data to one or more parent.
- **Proxy**: A node, running Netdata, that receives metric data from a child and "forwards" them on to a
  separate parent node.

Netdata uses API keys, which are just random GUIDs, to authorize the communication between child and parent nodes. We
recommend using `uuidgen` for generating API keys, which can then be used across any number of streaming connections.
Or, you can generate unique API keys for each parent-child relationship.

Once the parent node authorizes the child's API key, the child can start streaming metrics.

It's important to note that the streaming connection uses TCP, UDP, or Unix sockets, _not HTTP_. To proxy streaming
metrics, you need to use a proxy that tunnels [OSI layer 4-7
traffic](https://en.wikipedia.org/wiki/OSI_model#Layer_4:_Transport_Layer) without interfering with it, such as
[SOCKS](https://en.wikipedia.org/wiki/SOCKS) or Nginx's 
[TCP/UDP load balancing](https://docs.nginx.com/nginx/admin-guide/load-balancer/tcp-udp-load-balancer/).

## Supported streaming configurations

Netdata supports any combination of parent, child, and proxy nodes that you can imagine. Any node can act as both a
parent, child, or proxy at the same time, sending or receiving streaming metrics from any number of other nodes.

Here are a few example streaming configurations:

- **Headless collector**: 
  - Child `A`, _without_ a database or web dashboard, streams metrics to parent `B`.
  - `A` metrics are only available via the local Agent dashboard for `B`.
  - `B` generates alarms for `A`.
- **Replication**: 
  - Child `A`, _with_ a database and web dashboard, streams metrics to parent `B`. 
  - `A` metrics are available on both local Agent dashboards, and can be stored with the same or different metrics
    retention policies.
  - Both `A` and `B` generate alarms.
- **Proxy**:
  - Child `A`, _with or without_ a database, sends metrics to proxy `C`, also _with or without_ a database. `C` sends
    metrics to parent `B`.
  - Any node with a database can generate alarms.

### A basic auto-scaling setup

If your nodes are ephemeral, a Netdata parent with persistent storage outside your production infrastructure can be used to
store all the metrics from the Netdata children running on the ephemeral nodes.

![A diagram of an auto-scaling setup with Netdata](https://user-images.githubusercontent.com/1153921/84290043-0c1c1600-aaf8-11ea-9757-dd8dd8a8ec6c.png)

### Archiving to a time-series database

The parent Netdata node can also archive metrics, for all its child nodes, to an external time-series database.

Check the Netdata [exporting documentation](https://github.com/netdata/netdata/blob/master/docs/export/external-databases.md) for configuring this.

This is how such a solution will work:

![Diagram showing an example configuration for archiving to a time-series
database](https://user-images.githubusercontent.com/1153921/84291308-c2ccc600-aaf9-11ea-98a9-89ccbf3a62dd.png)

### An advanced setup

Netdata also supports `proxies` with and without a local database, and data retention can be different between all nodes.

This means a setup like the following is also possible:

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23629551/bb1fd9c2-02c0-11e7-90f5-cab5a3ed4c53.png"/>
</p>

## Enable streaming between nodes

The simplest streaming configuration is **replication**, in which a child node streams its metrics in real time to a
parent node, and both nodes retain metrics in their own databases.

To configure replication, you need two nodes, each running Netdata. First you'll first enable streaming on your parent
node, then enable streaming on your child node. When you're finished, you'll be able to see the child node's metrics in
the parent node's dashboard, quickly switch between the two dashboards, and be able to serve 
[alarm notifications](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md) from either or both nodes.

### Enable streaming on the parent node

First, log onto the node that will act as the parent.

Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to identify
itself while initiating a streaming connection. Copy that into a separate text file for later use.

> Find out how to [install `uuidgen`](https://command-not-found.com/uuidgen) on your node if you don't already have it.

Next, open `stream.conf` using [`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files)
from within the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata
sudo ./edit-config stream.conf
```

Scroll down to the section beginning with `[API_KEY]`. Paste the API key you generated earlier between the brackets, so
that it looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
```

Set `enabled` to `yes`, and `default memory mode` to `dbengine`. Leave all the other settings as their defaults. A
simplified version of the configuration, minus the commented lines, looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
    enabled = yes
    default memory mode = dbengine
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

### Enable streaming on the child node

Connect to your child node with SSH.

Open `stream.conf` again. Scroll down to the `[stream]` section and set `enabled` to `yes`. Paste the IP address of your
parent node at the end of the `destination` line, and paste the API key generated on the parent node onto the `api key`
line.

Leave all the other settings as their defaults. A simplified version of the configuration, minus the commented lines,
looks like the following:

```conf
[stream]
    enabled = yes 
    destination = 203.0.113.0
    api key = 11111111-2222-3333-4444-555555555555
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system.

### Enable TLS/SSL on streaming (optional)

While encrypting the connection between your parent and child nodes is recommended for security, it's not required to
get started. If you're not interested in encryption, skip ahead to [view streamed
metrics](#view-streamed-metrics-in-netdatas-dashboard).

In this example, we'll use self-signed certificates. 

On the **parent** node, use OpenSSL to create the key and certificate, then use `chown` to make the new files readable
by the `netdata` user.

```bash
sudo openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -keyout /etc/netdata/ssl/key.pem -out /etc/netdata/ssl/cert.pem
sudo chown netdata:netdata /etc/netdata/ssl/cert.pem /etc/netdata/ssl/key.pem
```

Next, enforce TLS/SSL on the web server. Open `netdata.conf`, scroll down to the `[web]` section, and look for the `bind
to` setting. Add `^SSL=force` to turn on TLS/SSL. See the [web server
reference](https://github.com/netdata/netdata/blob/master/web/server/README.md#enabling-tls-support) for other TLS/SSL options.

```conf
[web]
    bind to = *=dashboard|registry|badges|management|streaming|netdata.conf^SSL=force
```

Next, connect to the **child** node and open `stream.conf`. Add `:SSL` to the end of the existing `destination` setting
to connect to the parent using TLS/SSL. Uncomment the `ssl skip certificate verification` line to allow the use of
self-signed certificates.

```conf
[stream]
    enabled = yes
    destination = 203.0.113.0:SSL
    ssl skip certificate verification = yes
    api key = 11111111-2222-3333-4444-555555555555
```

Restart both the parent and child nodes with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for your system, to stream encrypted metrics using TLS/SSL.

### View streamed metrics in Netdata Cloud

In Netdata Cloud you should now be able to see a new parent showing up in the Home tab under "Nodes by data replication".
The replication factor for the child node has now increased to 2, meaning that its data is now highly available.

You don't need to do anything else, as the cloud will automatically prefer to fetch data about the child from the parent
and switch to querying the child only when the parent is unavailable, or for some reason doesn't have the requested 
data (e.g. the connection between parent and the child is broken). 

### View streamed metrics in Netdata's dashboard

At this point, the child node is streaming its metrics in real time to its parent. Open the local Agent dashboard for
the parent by navigating to `http://PARENT-NODE:19999` in your browser, replacing `PARENT-NODE` with its IP address or
hostname.

This dashboard shows parent metrics. To see child metrics, open the left-hand sidebar with the hamburger icon
![Hamburger icon](https://raw.githubusercontent.com/netdata/netdata-ui/master/src/components/icon/assets/hamburger.svg)
in the top panel. Both nodes appear under the **Replicated Nodes** menu. Click on either of the links to switch between
separate parent and child dashboards.

![Switching between parent and child dashboards](https://user-images.githubusercontent.com/1153921/110043346-761ec000-7d04-11eb-8e58-77670ba39161.gif)

The child dashboard is also available directly at `http://PARENT-NODE:19999/host/CHILD-HOSTNAME`, which in this example
is `http://203.0.113.0:19999/host/netdata-child`.

