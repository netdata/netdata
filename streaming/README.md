# Streaming and replication reference

Each Netdata node is able to replicate/mirror its database to another Netdata node, by streaming the collected
metrics in real-time. This is quite different to 
[data archiving to third party time-series databases](https://github.com/netdata/netdata/blob/master/exporting/README.md).
The nodes that send metrics are called **child** nodes, and the nodes that receive metrics are called **parent** nodes.

There are also **proxy** nodes, which collect metrics from a child and sends it to a parent.

When one Netdata node streams metrics another, the receiving instance can use the data for all features of a typical Netdata node, for example:

-   Visualize metrics with a dashboard
-   Run health checks that trigger alarms and send alarm notifications
-   Export metrics to an external time-series database

This document contains advanced streaming options and suggested deployment options for production. 
If you haven't already done so, we suggest you first go through the 
[quick introduction to streaming](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md)
, for your first, basic parent child setup.

## Supported configurations

### Netdata without a database or web API (headless collector)

A local Netdata Agent (child), **without any database or alarms**, collects metrics and sends them to another Netdata node
(parent).
The same parent can collect data for any number of child nodes and serves alerts for each child.

The node menu shows a list of all "databases streamed to" the parent. Clicking one of those links allows the user to
view the full dashboard of the child node. The URL has the form
`http://parent-host:parent-port/host/child-host/`.


In a headless setup, the child acts as a plain data collector. It spawns all external plugins, but instead of maintaining a
local database and accepting dashboard requests, it streams all metrics to the parent. 

This setup works great to reduce the memory footprint. Depending on the enabled plugins, memory usage is between 6 MiB and 40 MiB. To reduce the memory usage as much as
possible, refer to the [performance optimization guide](https://github.com/netdata/netdata/blob/master/docs/guides/configure/performance.md).

### Database Replication

The local Netdata Agent (child), **with a local database (and possibly alarms)**, collects metrics and
sends them to another Netdata node (parent).

The user can use all the functions **at both** `http://child-ip:child-port/` and
`http://parent-host:parent-port/host/child-host/`.

The child and the parent may have different data retention policies for the same metrics.

Alerts for the child are triggered by **both** the child and the parent. 
It is possible to enable different alert configurations on the parent and the child.

In order for custom chart names on the child to work correctly, follow the form `type.name`. The parent will truncate the `type` part and substitute the original chart `type` to store the name in the database.

### Netdata proxies

The local Netdata Agent(child), with or without a database, collects metrics and sends them to another
Netdata node(**proxy**), which may or may not maintain a database, which forwards them to another
Netdata (parent).

Alerts for the child can be triggered by any of the involved hosts that maintains a database.

You can daisy-chain any number of Netdata, each with or without a database and
with or without alerts for the child metrics.

### Mix and match with exporting engine

All nodes that maintain a database can also send their data to an external database.
This allows quite complex setups.

Example:

1.  Netdata nodes `A` and `B` do not maintain a database and stream metrics to Netdata node `C`(live streaming functionality). 
2.  Netdata node `C` maintains a database for `A`, `B`, `C` and archives all metrics to `graphite` with 10 second detail (exporting functionality).
3.  Netdata node `C` also streams data for `A`, `B`, `C` to Netdata `D`, which also collects data from `E`, `F` and `G` from another DMZ (live streaming functionality).
4.  Netdata node `D` is just a proxy, without a database, that streams all data to a remote site at Netdata `H`.
5.  Netdata node `H` maintains a database for `A`, `B`, `C`, `D`, `E`, `F`, `G`, `H` and sends all data to `opentsdb` with 5 seconds detail (exporting functionality)
6.  Alerts are triggered by `H` for all hosts.
7.  Users can use all Netdata nodes that maintain a database to view metrics (i.e. at `H` all hosts can be viewed).

## Viewing remote host dashboards, using mirrored databases

On any receiving Netdata, that maintains remote databases and has its web server enabled,
The node menu will include a list of the mirrored databases.

![image](https://cloud.githubusercontent.com/assets/2662304/24080824/24cd2d3c-0caf-11e7-909d-a8dd1dbb95d7.png)

Selecting any of these, the server will offer a dashboard using the mirrored metrics.

## Monitoring ephemeral nodes

Auto-scaling is probably the most trendy service deployment strategy these days.

Auto-scaling detects the need for additional resources and boots VMs on demand, based on a template. Soon after they start running the applications, a load balancer starts distributing traffic to them, allowing the service to grow horizontally to the scale needed to handle the load. When demands falls, auto-scaling starts shutting down VMs that are no longer needed.

![Monitoring ephemeral nodes with Netdata](https://cloud.githubusercontent.com/assets/2662304/23627426/65a9074a-02b9-11e7-9664-cd8f258a00af.png)

What a fantastic feature for controlling infrastructure costs! Pay only for what you need for the time you need it!

In auto-scaling, all servers are ephemeral, they live for just a few hours. Every VM is a brand new instance of the application, that was automatically created based on a template.

So, how can we monitor them? How can we be sure that everything is working as expected on all of them?

### The Netdata way

We recently made a significant improvement at the core of Netdata to support monitoring such setups.

Following the Netdata way of monitoring, we wanted:

1.  **real-time performance monitoring**, collecting ***thousands of metrics per server per second***, visualized in interactive, automatically created dashboards.
2.  **real-time alarms**, for all nodes.
3.  **zero configuration**, all ephemeral servers should have exactly the same configuration, and nothing should be configured at any system for each of the ephemeral nodes. We shouldn't care if 10 or 100 servers are spawned to handle the load.
4.  **self-cleanup**, so that nothing needs to be done for cleaning up the monitoring infrastructure from the hundreds of nodes that may have been monitored through time.

### How it works

All monitoring solutions, including Netdata, work like this:

1.  Collect metrics from the system and the running applications
2.  Store metrics in a time-series database
3.  Examine metrics periodically, for triggering alarms and sending alarm notifications
4.  Visualize metrics so that users can see what exactly is happening

Netdata used to be self-contained, so that all these functions were handled entirely by each server. The changes we made, allow each Netdata to be configured independently for each function. So, each Netdata can now act as:

-   A self-contained system, much like it used to be.
-   A data collector that collects metrics from a host and pushes them to another Netdata (with or without a local database and alarms).
-   A proxy, which receives metrics from other hosts and pushes them immediately to other Netdata servers. Netdata proxies can also be `store and forward proxies` meaning that they are able to maintain a local database for all metrics passing through them (with or without alarms).
-   A time-series database node, where data are kept, alarms are run and queries are served to visualise the metrics.

### Configuring an auto-scaling setup

![A diagram of an auto-scaling setup with Netdata](https://user-images.githubusercontent.com/1153921/84290043-0c1c1600-aaf8-11ea-9757-dd8dd8a8ec6c.png)

You need a Netdata parent. This node should not be ephemeral. It will be the node where all ephemeral child
nodes will send their metrics.

The parent will need to authorize child nodes to receive their metrics. This is done with an API key.

#### API keys

API keys are just random GUIDs. Use the Linux command `uuidgen` to generate one. You can use the same API key for all your child nodes, or you can configure one API for each of them. This is entirely your decision.

We suggest to use the same API key for each ephemeral node template you have, so that all replicas of the same ephemeral node will have exactly the same configuration.

I will use this API_KEY: `11111111-2222-3333-4444-555555555555`. Replace it with your own.

#### Configuring the parent

To configure the parent node:

1. On the parent node, edit `stream.conf` by using the `edit-config` script: 
`/etc/netdata/edit-config stream.conf`  

2. Set the following parameters:

```bash
[11111111-2222-3333-4444-555555555555]
	# enable/disable this API key
    enabled = yes

    # one hour of data for each of the child nodes
    default history = 3600

    # do not save child metrics on disk
    default memory = ram

    # alarms checks, only while the child is connected
    health enabled by default = auto
```

_`stream.conf` on the parent, to enable receiving metrics from its child nodes using the API key._

If you used many API keys, you can add one such section for each API key.

When done, restart Netdata on the parent node. It is now ready to receive metrics.

Note that `health enabled by default = auto` will still trigger `last_collected` alarms, if a connected child does not exit gracefully. If the `netdata` process running on the child is
stopped, it will close the connection to the parent, ensuring that no `last_collected` alarms are triggered. For example, a proper container restart would first terminate
the `netdata` process, but a system power issue would leave the connection open on the parent side. In the second case, you will still receive alarms.

#### Configuring the child nodes

To configure the child node:

1. On the child node, edit `stream.conf` by using the `edit-config` script: 
`/etc/netdata/edit-config stream.conf`  

2. Set the following parameters:

```bash
[stream]
    # stream metrics to another Netdata
    enabled = yes

    # the IP and PORT of the parent
    destination = 10.11.12.13:19999

	# the API key to use
    api key = 11111111-2222-3333-4444-555555555555
```

_`stream.conf` on child nodes, to enable pushing metrics to their parent at `10.11.12.13:19999`._

Using just the above configuration, the child nodes will be pushing their metrics to the parent Netdata, but they will still maintain a local database of the metrics and run health checks. To disable them, edit `/etc/netdata/netdata.conf` and set:

```bash
[global]
    # disable the local database
	memory mode = none

[health]
    # disable health checks
    enabled = no
```

_`netdata.conf` configuration on child nodes, to disable the local database and health checks._

Keep in mind that setting `memory mode = none` will also force `[health].enabled = no` (health checks require access to a local database). But you can keep the database and disable health checks if you need to. You are however sending all the metrics to the parent node, which can handle the health checking (`[health].enabled = yes`)

#### Netdata unique ID

The file `/var/lib/netdata/registry/netdata.public.unique.id` contains a random GUID that **uniquely identifies each Netdata Agent**. This file is automatically generated, by Netdata, the first time it is started and remains unaltered forever.

> If you are building an image to be used for automated provisioning of autoscaled VMs, it important to delete that file from the image, so that each instance of your image will generate its own.

#### Troubleshooting metrics streaming

Both parent and child nodes log information at `/var/log/netdata/error.log`.

To obtain the error logs, run the following on both the parent and child nodes:

```
tail -f /var/log/netdata/error.log | grep STREAM
```

If the child manages to connect to the parent you will see something like (on the parent):

```
2017-03-09 09:38:52: netdata: INFO : STREAM [receive from [10.11.12.86]:38564]: new client connection.
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [10.11.12.86]:38564: receive thread created (task id 27721)
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: client willing to stream metrics for host 'xxx' with machine_guid '1234567-1976-11e6-ae19-7cdd9077342a': update every = 1, history = 3600, memory mode = ram, health auto
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: initializing communication...
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: receiving metrics...
```

and something like this on the child:

```
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: connecting...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: initializing communication...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: waiting response from remote netdata...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: established communication - sending metrics...
```

### Archiving to a time-series database

The parent Netdata node can also archive metrics, for all its child nodes, to a time-series database. At the time of
this writing, Netdata supports:

-   graphite
-   opentsdb
-   prometheus
-   json document DBs
-   all the compatibles to the above (e.g. kairosdb, influxdb, etc)

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

## Proxies

A proxy is a Netdata node that is receiving metrics from a Netdata node, and streams them to another Netdata node.

Netdata proxies may or may not maintain a database for the metrics passing through them.
When they maintain a database, they can also run health checks (alarms and notifications)
for the remote host that is streaming the metrics.

To configure a proxy, configure it as a receiving and a sending Netdata at the same time,
using `stream.conf`.

The sending side of a Netdata proxy, connects and disconnects to the final destination of the
metrics, following the same pattern of the receiving side.

For a practical example see [Monitoring ephemeral nodes](#monitoring-ephemeral-nodes).

## Troubleshooting streaming connections

This section describes the most common issues you might encounter when connecting parent and child nodes.

### Slow connections between parent and child

When you have a slow connection between parent and child, Netdata raises a few different errors. Most of the
errors will appear in the child's `error.log`.

```bash
netdata ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM CHILD HOSTNAME [send to PARENT IP:PARENT PORT]: too many data pending - buffer is X bytes long,
Y unsent - we have sent Z bytes in total, W on this connection. Closing connection to flush the data.
```

On the parent side, you may see various error messages, most commonly the following:

```
netdata ERROR : STREAM_PARENT[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : read failed: end of file
```

Another common problem in slow connections is the CHILD sending a partial message to the parent. In this case,
the parent will write the following in its `error.log`:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : sent command 'B' which is not known by netdata, for host 'HOSTNAME'. Disabling it.
```

In this example, `B` was part of a `BEGIN` message that was cut due to connection problems.

Slow connections can also cause problems when the parent misses a message and then receives a command related to the
missed message. For example, a parent might miss a message containing the child's charts, and then doesn't know
what to do with the `SET` message that follows. When that happens, the parent will show a message like this:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : requested a SET on chart 'CHART NAME' of host 'HOSTNAME', without a dimension. Disabling it.
```

### Child cannot connect to parent

When the child can't connect to a parent for any reason (misconfiguration, networking, firewalls, parent
down), you will see the following in the child's `error.log`.

```
ERROR : STREAM_SENDER[HOSTNAME] : Failed to connect to 'PARENT IP', port 'PARENT PORT' (errno 113, No route to host)
```

### 'Is this a Netdata node?'

This question can appear when Netdata starts the stream and receives an unexpected response. This error can appear when
the parent is using SSL and the child tries to connect using plain text. You will also see this message when
Netdata connects to another server that isn't a Netdata node. The complete error message will look like this:

```
ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM child HOSTNAME [send to PARENT HOSTNAME:PARENT PORT]: server is not replying properly (is it a netdata?).
```

### Stream charts wrong

Chart data needs to be consistent between child and parent nodes. If there are differences between chart data on
a parent and a child, such as gaps in metrics collection, it most often means your child's `memory mode`
does not match the parent's. To learn more about the different ways Netdata can store metrics, and thus keep chart
data consistent, read our [memory mode documentation](https://github.com/netdata/netdata/blob/master/database/README.md).

### Forbidding access

You may see errors about "forbidding access" for a number of reasons. It could be because of a slow connection between
the parent and child nodes, but it could also be due to other failures. Look in your parent's `error.log` for errors
that look like this: 

```
STREAM [receive from [child HOSTNAME]:child IP]: `MESSAGE`. Forbidding access."
```

`MESSAGE` will have one of the following patterns:

-   `request without KEY` : The message received is incomplete and the KEY value can be API, hostname, machine GUID.
-   `API key 'VALUE' is not valid GUID`: The UUID received from child does not have the format defined in [RFC 4122]
    (https://tools.ietf.org/html/rfc4122)
-   `machine GUID 'VALUE' is not GUID.`: This error with machine GUID is like the previous one.
-   `API key 'VALUE' is not allowed`: This stream has a wrong API key.
-   `API key 'VALUE' is not permitted from this IP`: The IP is not allowed to use STREAM with this parent.
-   `machine GUID 'VALUE' is not allowed.`: The GUID that is trying to send stream is not allowed.
-   `Machine GUID 'VALUE' is not permitted from this IP. `: The IP does not match the pattern or IP allowed to connect 
    to use stream.

### Netdata could not create a stream

The connection between parent and child is a stream. When the parent can't convert the initial connection into
a stream, it will write the following message inside `error.log`:

```
file descriptor given is not a valid stream
```

After logging this error, Netdata will close the stream.


## Configuration

There are two files responsible for configuring Netdata's streaming capabilities: `stream.conf` and `netdata.conf`.

From within your Netdata config directory (typically `/etc/netdata`), [use `edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) to
open either `stream.conf` or `netdata.conf`.

```
sudo ./edit-config stream.conf
sudo ./edit-config netdata.conf
```

### `stream.conf`

The `stream.conf` file contains three sections. The `[stream]` section is for configuring child nodes.

The `[API_KEY]` and `[MACHINE_GUID]` sections are both for configuring parent nodes, and share the same settings.
`[API_KEY]` settings affect every child node using that key, whereas `[MACHINE_GUID]` settings affect only the child
node with a matching GUID.

The file `/var/lib/netdata/registry/netdata.public.unique.id` contains a random GUID that **uniquely identifies each
node**. This file is automatically generated by Netdata the first time it is started and remains unaltered forever.

#### `[stream]` section

| Setting                                         | Default                   | Description                                                                                                                                                                                                                                                                                                                                               |
| :---------------------------------------------- | :------------------------ | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                                       | `no`                      | Whether this node streams metrics to any parent. Change to `yes` to enable streaming.                                                                                                                                                                                                                                                                     |
| [`destination`](#destination)                   | ` `                       | A space-separated list of parent nodes to attempt to stream to, with the first available parent receiving metrics, using the following format: `[PROTOCOL:]HOST[%INTERFACE][:PORT][:SSL]`. [Read more &rarr;](#destination)                                                                                                                               |
| `ssl skip certificate verification`             | `yes`                     | If you want to accept self-signed or expired certificates, set to `yes` and uncomment.                                                                                                                                                                                                                                                                    |
| `CApath`                                        | `/etc/ssl/certs/`         | The directory where known certificates are found. Defaults to OpenSSL's default path.                                                                                                                                                                                                                                                                     |
| `CAfile`                                        | `/etc/ssl/certs/cert.pem` | Add a parent node certificate to the list of known certificates in `CAPath`.                                                                                                                                                                                                                                                                              |
| `api key`                                       | ` `                       | The `API_KEY` to use as the child node.                                                                                                                                                                                                                                                                                                                   |
| `timeout seconds`                               | `60`                      | The timeout to connect and send metrics to a parent.                                                                                                                                                                                                                                                                                                      |
| `default port`                                  | `19999`                   | The port to use if `destination` does not specify one.                                                                                                                                                                                                                                                                                                    |
| [`send charts matching`](#send-charts-matching) | `*`                       | A space-separated list of [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md) to filter which charts are streamed. [Read more &rarr;](#send-charts-matching)                                                                                                                                                                                  |
| `buffer size bytes`                             | `10485760`                | The size of the buffer to use when sending metrics. The default `10485760` equals a buffer of 10MB, which is good for 60 seconds of data. Increase this if you expect latencies higher than that. The buffer is flushed on reconnect.                                                                                                                    |
| `reconnect delay seconds`                       | `5`                       | How long to wait until retrying to connect to the parent node.                                                                                                                                                                                                                                                                                            |
| `initial clock resync iterations`               | `60`                      | Sync the clock of charts for how many seconds when starting.                                                                                                                                                                                                                                                                                              |

### `[API_KEY]` and `[MACHINE_GUID]` sections

| Setting                                         | Default                   | Description                                                                                                                                                                                                                                                                                                                                               |
| :---------------------------------------------- | :------------------------ | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`                                       | `no`                      | Whether this API KEY enabled or disabled.                                                                                                                                                                                                                                                                                                                 |
| [`allow from`](#allow-from)                     | `*`                       | A space-separated list of [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md) matching the IPs of nodes that will stream metrics using this API key. [Read more &rarr;](#allow-from)                                                                                                                                                          |
| `default history`                               | `3600`                    | The default amount of child metrics history to retain when using the `save`, `map`, or `ram` memory modes.                                                                                                                                                                                                                                                |
| [`default memory mode`](#default-memory-mode)   | `ram`                     | The [database](https://github.com/netdata/netdata/blob/master/database/README.md) to use for all nodes using this `API_KEY`. Valid settings are `dbengine`, `map`, `save`, `ram`, or `none`. [Read more &rarr;](#default-memory-mode)                                                                                                                                                                   |
| `health enabled by default`                     | `auto`                    | Whether alarms and notifications should be enabled for nodes using this `API_KEY`. `auto` enables alarms when the child is connected. `yes` enables alarms always, and `no` disables alarms.                                                                                                                                                              |
| `default postpone alarms on connect seconds`    | `60`                      | Postpone alarms and notifications for a period of time after the child connects.                                                                                                                                                                                                                                                                          |
| `default proxy enabled`                         | ` `                       | Route metrics through a proxy.                                                                                                                                                                                                                                                                                                                            |
| `default proxy destination`                     | ` `                       | Space-separated list of `IP:PORT` for proxies.                                                                                                                                                                                                                                                                                                            |
| `default proxy api key`                         | ` `                       | The `API_KEY` of the proxy.                                                                                                                                                                                                                                                                                                                               |
| `default send charts matching`                  | `*`                       | See [`send charts matching`](#send-charts-matching).                                                                                                                                                                                                                                                                                                      |

#### `destination`

A space-separated list of parent nodes to attempt to stream to, with the first available parent receiving metrics, using
the following format: `[PROTOCOL:]HOST[%INTERFACE][:PORT][:SSL]`.

- `PROTOCOL`: `tcp`, `udp`, or `unix`. (only tcp and unix are supported by parent nodes)
- `HOST`: A IPv4, IPv6 IP, or a hostname, or a unix domain socket path. IPv6 IPs should be given with brackets
  `[ip:address]`.
- `INTERFACE` (IPv6 only): The network interface to use.
- `PORT`: The port number or service name (`/etc/services`) to use.
- `SSL`: To enable TLS/SSL encryption of the streaming connection.

To enable TCP streaming to a parent node at `203.0.113.0` on port `20000` and with TLS/SSL encryption: 

```conf
[stream]
    destination = tcp:203.0.113.0:20000:SSL
```

#### `send charts matching`

A space-separated list of [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md) to filter which charts are streamed.

The default is a single wildcard `*`, which streams all charts.

To send only a few charts, list them explicitly, or list a group using a wildcard. To send _only_ the `apps.cpu` chart
and charts with contexts beginning with `system.`: 

```conf
[stream]
    send charts matching = apps.cpu system.*
```

To send all but a few charts, use `!` to create a negative match. To send _all_ charts _but_ `apps.cpu`:

```conf
[stream]
    send charts matching = !apps.cpu *
```

#### `allow from`

A space-separated list of [Netdata simple patterns](https://github.com/netdata/netdata/blob/master/libnetdata/simple_pattern/README.md) matching the IPs of nodes that
will stream metrics using this API key. The order is important, left to right, as the first positive or negative match is used.

The default is `*`, which accepts all requests including the `API_KEY`.

To allow from only a specific IP address:

```conf
[API_KEY]
    allow from = 203.0.113.10
```

To allow all IPs starting with `10.*`, except `10.1.2.3`:

```conf
[API_KEY]
    allow from = !10.1.2.3 10.*
```

> If you set specific IP addresses here, and also use the `allow connections` setting in the `[web]` section of
> `netdata.conf`, be sure to add the IP address there so that it can access the API port.

#### `default memory mode`

The [database](https://github.com/netdata/netdata/blob/master/database/README.md) to use for all nodes using this `API_KEY`. Valid settings are `dbengine`, `ram`,
`save`, `map`, or `none`.

- `dbengine`: The default, recommended time-series database (TSDB) for Netdata. Stores recent metrics in memory, then
  efficiently spills them to disk for long-term storage.
- `ram`: Stores metrics _only_ in memory, which means metrics are lost when Netdata stops or restarts. Ideal for
  streaming configurations that use ephemeral nodes.
- `save`: Stores metrics in memory, but saves metrics to disk when Netdata stops or restarts, and loads historical
  metrics on start.
- `map`: Stores metrics in memory-mapped files, like swap, with constant disk write.
- `none`: No database.

When using `default memory mode = dbengine`, the parent node creates a separate instance of the TSDB to store metrics
from child nodes. The [size of _each_ instance is configurable](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md) with the `page
cache size` and `dbengine multihost disk space` settings in the `[global]` section in `netdata.conf`.

### `netdata.conf`

| Setting                                    | Default           | Description                                                                                                                                                                                                                                                                  |
| :----------------------------------------- | :---------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **`[global]` section**                     |                   |                                                                                                                                                                                                                                                                              |
| `memory mode`                              | `dbengine`        | Determines the [database type](https://github.com/netdata/netdata/blob/master/database/README.md) to be used on that node. Other options settings include `none`, `ram`, `save`, and `map`. `none` disables the database at this host. This also disables alarms and notifications, as those can't run without a database. |
| **`[web]` section**                        |                   |                                                                                                                                                                                                                                                                              |
| `mode`                                     | `static-threaded` | Determines the [web server](https://github.com/netdata/netdata/blob/master/web/server/README.md) type. The other option is `none`, which disables the dashboard, API, and registry.                                                                                                                                        |
| `accept a streaming request every seconds` | `0`               | Set a limit on how often a parent node accepts streaming requests from child nodes. `0` equals no limit. If this is set, you may see `... too busy to accept new streaming request. Will be allowed in X secs` in Netdata's `error.log`.                                     |

### Streaming compression

[![Supported version Netdata Agent release](https://img.shields.io/badge/Supported%20Netdata%20Agent-v1.33%2B-brightgreen)](https://github.com/netdata/netdata/releases/latest)

[![Supported version Netdata Agent release](https://img.shields.io/badge/Supported%20Netdata%20stream%20version-v5%2B-blue)](https://github.com/netdata/netdata/releases/latest)

#### OS dependencies
* Streaming compression is based on [lz4 v1.9.0+](https://github.com/lz4/lz4). The [lz4 v1.9.0+](https://github.com/lz4/lz4) library must be installed in your OS in order to enable streaming compression. Any lower version will disable Netdata streaming compression for compatibility purposes between the older versions of Netdata agents.

To check if your Netdata Agent supports stream compression run the following GET request in your browser or terminal:

```
curl -X GET http://localhost:19999/api/v1/info | grep 'Stream Compression'
```

**Output**
```
"buildinfo": "dbengine|Native HTTPS|Netdata Cloud|ACLK Next Generation|New Cloud Protocol Support|ACLK Legacy|TLS Host Verification|Machine Learning|Stream Compression|protobuf|JSON-C|libcrypto|libm|LWS v3.2.2|mosquitto|zlib|apps|cgroup Network Tracking|EBPF|perf|slabinfo",
```
> Note: If your OS doesn't support Netdata compression the `buildinfo` will not contain the `Stream Compression` statement.

To check if your Netdata Agent has stream compression enabled, run the following GET request in your browser or terminal: 

```
 curl -X GET http://localhost:19999/api/v1/info | grep 'stream-compression'
```
**Output**
```
"stream-compression": "enabled"
```
Note: The `stream-compression` status can be `"enabled" | "disabled" | "N/A"`.

A compressed data packet is determined and decompressed on the fly.

#### Limitations
This limitation will be withdrawn asap and is work-in-progress.

The current implementation of streaming data compression can support only a few number of dimensions in a chart with names that cannot exceed the size of 16384 bytes. In case your instance hit this limitation, the agent will deactivate compression during runtime to avoid stream corruption. This limitation can be seen in the error.log file with the sequence of the following messages: 
```
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: connecting...
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: initializing communication...
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: waiting response from remote netdata...
netdata INFO  : STREAM_SENDER[child01] : STREAM_COMPRESSION: Compressor Reset
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: established communication with a parent using protocol version 5 - ready to send metrics...
...
netdata ERROR : PLUGINSD[go.d] : STREAM_COMPRESSION: Compression Failed - Message size 27847 above compression buffer limit: 16384 (errno 9, Bad file descriptor)
netdata ERROR : PLUGINSD[go.d] : STREAM_COMPRESSION: Deactivating compression to avoid stream corruption
netdata ERROR : PLUGINSD[go.d] : STREAM_COMPRESSION child01 [send to my.parent.IP]: Restarting connection without compression
...
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: connecting...
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: initializing communication...
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: waiting response from remote netdata...
netdata INFO  : STREAM_SENDER[child01] : Stream is uncompressed! One of the agents (my.parent.IP <-> child01) does not support compression OR compression is disabled.
netdata INFO  : STREAM_SENDER[child01] : STREAM child01 [send to my.parent.IP]: established communication with a parent using protocol version 4 - ready to send metrics...
netdata INFO  : WEB_SERVER[static4] : STREAM child01 [send]: sending metrics...
```

#### How to enable stream compression
Netdata Agents are shipped with data compression enabled by default. You can also configure which streams will use compression.

With enabled stream compression, a Netdata Agent can negotiate streaming compression with other Netdata Agents. During the negotiation of streaming compression both Netdata Agents should support and enable compression in order to communicate over a compressed stream. The negotiation will result into an uncompressed stream, if one of the Netdata Agents doesn't support **or** has compression disabled.

To enable stream compression: 

1. Edit `stream.conf` by using the `edit-config` script: 
`/etc/netdata/edit-config stream.conf`. 

2. In the `[stream]` section, set `enable compression` to `yes`.
```
# This is the default stream compression flag for an agent.

[stream]
    enable compression = yes | no
```


| Parent               | Stream compression | Child                |
|----------------------|--------------------|----------------------|
| Supported & Enabled  | compressed         | Supported & Enabled  |
| (Supported & Disabled)/Not supported | uncompressed         | Supported & Enabled |
| Supported & Enabled | uncompressed         | (Supported & Disabled)/Not supported |
| (Supported & Disabled)/Not supported | uncompressed         | (Supported & Disabled)/Not supported |

In case of parents with multiple children you can select which streams will be compressed by using the same configuration under the `[API_KEY]`, `[MACHINE_GUID]` section. 

This configuration uses AND logic with the default stream compression configuration under the `[stream]` section. This means the stream compression from child to parent will be enabled only if the outcome of the AND logic operation is true (`default compression enabled` && `api key compression enabled`). So both should be enabled to get stream compression otherwise  stream compression is disabled.
```  
[API_KEY]
    enable compression = yes | no
```
Same thing applies with the `[MACHINE_GUID]` configuration.
```
[MACHINE_GUID]
    enable compression = yes | no
```

## Examples

### Basic use cases

This is an overview of how the main options can be combined:

| target|memory<br/>mode|web<br/>mode|stream<br/>enabled|exporting|alarms|dashboard|
|------|:-------------:|:----------:|:----------------:|:-----:|:----:|:-------:|
| headless collector|`none`|`none`|`yes`|only for `data source = as collected`|not possible|no|
| headless proxy|`none`|not `none`|`yes`|only for `data source = as collected`|not possible|no|
| proxy with db|not `none`|not `none`|`yes`|possible|possible|yes|
| central netdata|not `none`|not `none`|`no`|possible|possible|yes|

### Per-child settings

While the `[API_KEY]` section applies settings for any child node using that key, you can also use per-child settings
with the `[MACHINE_GUID]` section.

For example, the metrics streamed from only the child node with `MACHINE_GUID` are saved in memory, not using the
default `dbengine` as specified by the `API_KEY`, and alarms are disabled.

```conf
[API_KEY]
    enabled = yes
    default memory mode = dbengine
    health enabled by default = auto
    allow from = *

[MACHINE_GUID]
    enabled = yes
    memory mode = save
    health enabled = no
```

### Securing streaming with TLS/SSL

Netdata does not activate TLS encryption by default. To encrypt streaming connections, you first need to [enable TLS
support](https://github.com/netdata/netdata/blob/master/web/server/README.md#enabling-tls-support) on the parent. With encryption enabled on the receiving side, you
need to instruct the child to use TLS/SSL as well. On the child's `stream.conf`, configure the destination as follows:

```
[stream]
    destination = host:port:SSL
```

The word `SSL` appended to the end of the destination tells the child that connections must be encrypted.

> While Netdata uses Transport Layer Security (TLS) 1.2 to encrypt communications rather than the obsolete SSL protocol,
> it's still common practice to refer to encrypted web connections as `SSL`. Many vendors, like Nginx and even Netdata
> itself, use `SSL` in configuration files, whereas documentation will always refer to encrypted communications as `TLS`
> or `TLS/SSL`.

#### Certificate verification

When TLS/SSL is enabled on the child, the default behavior will be to not connect with the parent unless the server's
certificate can be verified via the default chain. In case you want to avoid this check, add the following to the
child's `stream.conf` file:

```
[stream]
    ssl skip certificate verification = yes
```

#### Trusted certificate

If you've enabled [certificate verification](#certificate-verification), you might see errors from the OpenSSL library
when there's a problem with checking the certificate chain (`X509_V_ERR_UNABLE_TO_GET_ISSUER_CERT_LOCALLY`). More
importantly, OpenSSL will reject self-signed certificates.

Given these known issues, you have two options. If you trust your certificate, you can set the options `CApath` and
`CAfile` to inform Netdata where your certificates, and the certificate trusted file, are stored.

For more details about these options, you can read about [verify
locations](https://www.openssl.org/docs/man1.1.1/man3/SSL_CTX_load_verify_locations.html).

Before you changed your streaming configuration, you need to copy your trusted certificate to your child system and add
the certificate to OpenSSL's list.

On most Linux distributions, the `update-ca-certificates` command searches inside the `/usr/share/ca-certificates`
directory for certificates. You should double-check by reading the `update-ca-certificate` manual (`man
update-ca-certificate`), and then change the directory in the below commands if needed.

If you have `sudo` configured on your child system, you can use that to run the following commands. If not, you'll have
to log in as `root` to complete them.

```
# mkdir /usr/share/ca-certificates/netdata
# cp parent_cert.pem /usr/share/ca-certificates/netdata/parent_cert.crt
# chown -R netdata.netdata /usr/share/ca-certificates/netdata/
```

First, you create a new directory to store your certificates for Netdata. Next, you need to change the extension on your
certificate from `.pem` to `.crt` so it's compatible with `update-ca-certificate`. Finally, you need to change
permissions so the user that runs Netdata can access the directory where you copied in your certificate.

Next, edit the file `/etc/ca-certificates.conf` and add the following line:

```
netdata/parent_cert.crt
```

Now you update the list of certificates running the following, again either as `sudo` or `root`:

```
# update-ca-certificates
```

> Some Linux distributions have different methods of updating the certificate list. For more details, please read this
> guide on [adding trusted root certificates](https://github.com/Busindre/How-to-Add-trusted-root-certificates).

Once you update your certificate list, you can set the stream parameters for Netdata to trust the parent certificate.
Open `stream.conf` for editing and change the following lines:

```
[stream]
    CApath = /etc/ssl/certs/
    CAfile = /etc/ssl/certs/parent_cert.pem
```

With this configuration, the `CApath` option tells Netdata to search for trusted certificates inside `/etc/ssl/certs`.
The `CAfile` option specifies the Netdata parent certificate is located at `/etc/ssl/certs/parent_cert.pem`. With this
configuration, you can skip using the system's entire list of certificates and use Netdata's parent certificate instead.

#### Expected behaviors

With the introduction of TLS/SSL, the parent-child communication behaves as shown in the table below, depending on the
following configurations:

-   **Parent TLS (Yes/No)**: Whether the `[web]` section in `netdata.conf` has `ssl key` and `ssl certificate`.
-   **Parent port TLS (-/force/optional)**: Depends on whether the `[web]` section `bind to` contains a `^SSL=force` or
    `^SSL=optional` directive on the port(s) used for streaming.
-   **Child TLS (Yes/No)**: Whether the destination in the child's `stream.conf` has `:SSL` at the end.
-   **Child TLS Verification (yes/no)**: Value of the child's `stream.conf` `ssl skip certificate verification`
    parameter (default is no).

| Parent TLS enabled | Parent port SSL  | Child TLS | Child SSL Ver. | Behavior                                                                                                                                 |
| :----------------- | :--------------- | :-------- | :------------- | :--------------------------------------------------------------------------------------------------------------------------------------- |
| No                 | -                | No        | no             | Legacy behavior. The parent-child stream is unencrypted.                                                                                 |
| Yes                | force            | No        | no             | The parent rejects the child connection.                                                                                                 |
| Yes                | -/optional       | No        | no             | The parent-child stream is unencrypted (expected situation for legacy child nodes and newer parent nodes)                                |
| Yes                | -/force/optional | Yes       | no             | The parent-child stream is encrypted, provided that the parent has a valid TLS/SSL certificate. Otherwise, the child refuses to connect. |
| Yes                | -/force/optional | Yes       | yes            | The parent-child stream is encrypted.                                                                                                    |

### Proxy

A proxy is a node that receives metrics from a child, then streams them onward to a parent. To configure a proxy,
configure it as a receiving and a sending Netdata at the same time.

Netdata proxies may or may not maintain a database for the metrics passing through them. When they maintain a database,
they can also run health checks (alarms and notifications) for the remote host that is streaming the metrics.

In the following example, the proxy receives metrics from a child node using the `API_KEY` of
`66666666-7777-8888-9999-000000000000`, then stores metrics using `dbengine`. It then uses the `API_KEY` of
`11111111-2222-3333-4444-555555555555` to proxy those same metrics on to a parent node at `203.0.113.0`.

```conf
[stream]
    enabled = yes 
    destination = 203.0.113.0
    api key = 11111111-2222-3333-4444-555555555555

[66666666-7777-8888-9999-000000000000]
    enabled = yes
    default memory mode = dbengine
```

### Ephemeral nodes

Netdata can help you monitor ephemeral nodes, such as containers in an auto-scaling infrastructure, by always streaming
metrics to any number of permanently-running parent nodes.

On the parent, set the following in `stream.conf`:

```conf
[11111111-2222-3333-4444-555555555555]
	# enable/disable this API key
    enabled = yes

    # one hour of data for each of the child nodes
    default history = 3600

    # do not save child metrics on disk
    default memory = ram

    # alarms checks, only while the child is connected
    health enabled by default = auto
```

On the child nodes, set the following in `stream.conf`:

```bash
[stream]
    # stream metrics to another Netdata
    enabled = yes

    # the IP and PORT of the parent
    destination = 10.11.12.13:19999

	  # the API key to use
    api key = 11111111-2222-3333-4444-555555555555
```

In addition, edit `netdata.conf` on each child node to disable the database and alarms.

```bash
[global]
    # disable the local database
	  memory mode = none

[health]
    # disable health checks
    enabled = no
```

## Troubleshooting

Both parent and child nodes log information at `/var/log/netdata/error.log`.

If the child manages to connect to the parent you will see something like (on the parent):

```
2017-03-09 09:38:52: netdata: INFO : STREAM [receive from [10.11.12.86]:38564]: new client connection.
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [10.11.12.86]:38564: receive thread created (task id 27721)
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: client willing to stream metrics for host 'xxx' with machine_guid '1234567-1976-11e6-ae19-7cdd9077342a': update every = 1, history = 3600, memory mode = ram, health auto
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: initializing communication...
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: receiving metrics...
```

and something like this on the child:

```
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: connecting...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: initializing communication...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: waiting response from remote netdata...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: established communication - sending metrics...
```

The following sections describe the most common issues you might encounter when connecting parent and child nodes.

### Slow connections between parent and child

When you have a slow connection between parent and child, Netdata raises a few different errors. Most of the
errors will appear in the child's `error.log`.

```bash
netdata ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM CHILD HOSTNAME [send to PARENT IP:PARENT PORT]: too many data pending - buffer is X bytes long,
Y unsent - we have sent Z bytes in total, W on this connection. Closing connection to flush the data.
```

On the parent side, you may see various error messages, most commonly the following:

```
netdata ERROR : STREAM_PARENT[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : read failed: end of file
```

Another common problem in slow connections is the child sending a partial message to the parent. In this case, the
parent will write the following to its `error.log`:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : sent command 'B' which is not known by netdata, for host 'HOSTNAME'. Disabling it.
```

In this example, `B` was part of a `BEGIN` message that was cut due to connection problems.

Slow connections can also cause problems when the parent misses a message and then receives a command related to the
missed message. For example, a parent might miss a message containing the child's charts, and then doesn't know
what to do with the `SET` message that follows. When that happens, the parent will show a message like this:

```
ERROR : STREAM_RECEIVER[CHILD HOSTNAME,[CHILD IP]:CHILD PORT] : requested a SET on chart 'CHART NAME' of host 'HOSTNAME', without a dimension. Disabling it.
```

### Child cannot connect to parent

When the child can't connect to a parent for any reason (misconfiguration, networking, firewalls, parent
down), you will see the following in the child's `error.log`.

```
ERROR : STREAM_SENDER[HOSTNAME] : Failed to connect to 'PARENT IP', port 'PARENT PORT' (errno 113, No route to host)
```

### 'Is this a Netdata?'

This question can appear when Netdata starts the stream and receives an unexpected response. This error can appear when
the parent is using SSL and the child tries to connect using plain text. You will also see this message when
Netdata connects to another server that isn't Netdata. The complete error message will look like this:

```
ERROR : STREAM_SENDER[CHILD HOSTNAME] : STREAM child HOSTNAME [send to PARENT HOSTNAME:PARENT PORT]: server is not replying properly (is it a netdata?).
```

### Stream charts wrong

Chart data needs to be consistent between child and parent nodes. If there are differences between chart data on
a parent and a child, such as gaps in metrics collection, it most often means your child's `memory mode`
does not match the parent's. To learn more about the different ways Netdata can store metrics, and thus keep chart
data consistent, read our [memory mode documentation](https://github.com/netdata/netdata/blob/master/database/README.md).

### Forbidding access

You may see errors about "forbidding access" for a number of reasons. It could be because of a slow connection between
the parent and child nodes, but it could also be due to other failures. Look in your parent's `error.log` for errors
that look like this: 

```
STREAM [receive from [child HOSTNAME]:child IP]: `MESSAGE`. Forbidding access."
```

`MESSAGE` will have one of the following patterns:

- `request without KEY` : The message received is incomplete and the KEY value can be API, hostname, machine GUID.
- `API key 'VALUE' is not valid GUID`: The UUID received from child does not have the format defined in [RFC
  4122](https://tools.ietf.org/html/rfc4122)
- `machine GUID 'VALUE' is not GUID.`: This error with machine GUID is like the previous one.
- `API key 'VALUE' is not allowed`: This stream has a wrong API key.
- `API key 'VALUE' is not permitted from this IP`: The IP is not allowed to use STREAM with this parent.
- `machine GUID 'VALUE' is not allowed.`: The GUID that is trying to send stream is not allowed.
- `Machine GUID 'VALUE' is not permitted from this IP. `: The IP does not match the pattern or IP allowed to connect to
  use stream.

### Netdata could not create a stream

The connection between parent and child is a stream. When the parent can't convert the initial connection into
a stream, it will write the following message inside `error.log`:

```
file descriptor given is not a valid stream
```

After logging this error, Netdata will close the stream.
