<!--
title: "Streaming and replication"
description: "Replicate and mirror Netdata's metrics through real-time streaming from child to parent nodes. Then combine, correlate, and export."
custom_edit_url: https://github.com/netdata/netdata/edit/master/streaming/README.md
-->

# Streaming and replication

Each Netdata is able to replicate/mirror its database to another Netdata, by streaming collected
metrics, in real-time to it. This is quite different to [data archiving to third party time-series
databases](/exporting/README.md).

When Netdata streams metrics to another Netdata, the receiving one is able to perform everything a Netdata instance is
capable of:

-   Visualize metrics with a dashboard
-   Run health checks that trigger alarms and send alarm notifications
-   Export metrics to a external time-series database

The nodes that send metrics are called **child** nodes, and the nodes that receive metrics are called **parent** nodes.
There are also **proxies**, which collects metrics from a child and sends it to a parent.

## Supported configurations

### Netdata without a database or web API (headless collector)

Local Netdata (child), **without any database or alarms**, collects metrics and sends them to another Netdata
(parent).

The node menu shows a list of all "databases streamed to" the parent. Clicking one of those links allows the user to
view the full dashboard of the child node. The URL has the form
`http://parent-host:parent-port/host/child-host/`.

Alarms for the child are served by the parent.

In this mode the child is just a plain data collector. It spawns all external plugins, but instead of maintaining a
local database and accepting dashboard requests, it streams all metrics to the parent. The memory footprint is reduced
significantly, to between 6 MiB and 40 MiB, depending on the enabled plugins. To reduce the memory usage as much as
possible, refer to [running Netdata in embedded devices](/docs/Performance.md#running-netdata-in-embedded-devices).

The same parent can collect data for any number of child nodes.

### Database Replication

Local Netdata (child), **with a local database (and possibly alarms)**, collects metrics and
sends them to another Netdata (parent).

The user can use all the functions **at both** `http://child-ip:child-port/` and
`http://parent-host:parent-port/host/child-host/`.

The child and the parent may have different data retention policies for the same metrics.

Alarms for the child are triggered by **both** the child and the parent (and actually
each can have different alarms configurations or have alarms disabled).

Take a note, that custom chart names, configured on the child, should be in the form `type.name` to work correctly. The parent will truncate the `type` part and substitute the original chart `type` to store the name in the database.

### Netdata proxies

Local Netdata (child), with or without a database, collects metrics and sends them to another
Netdata (**proxy**), which may or may not maintain a database, which forwards them to another
Netdata (parent).

Alarms for the child can be triggered by any of the involved hosts that maintains a database.

Any number of daisy chaining Netdata servers are supported, each with or without a database and
with or without alarms for the child metrics.

### mix and match with backends

All nodes that maintain a database can also send their data to a backend database.
This allows quite complex setups.

Example:

1.  Netdata `A`, `B` do not maintain a database and stream metrics to Netdata `C`(live streaming functionality, i.e. this PR)
2.  Netdata `C` maintains a database for `A`, `B`, `C` and archives all metrics to `graphite` with 10 second detail (backends functionality)
3.  Netdata `C` also streams data for `A`, `B`, `C` to Netdata `D`, which also collects data from `E`, `F` and `G` from another DMZ (live streaming functionality, i.e. this PR)
4.  Netdata `D` is just a proxy, without a database, that streams all data to a remote site at Netdata `H`
5.  Netdata `H` maintains a database for `A`, `B`, `C`, `D`, `E`, `F`, `G`, `H` and sends all data to `opentsdb` with 5 seconds detail (backends functionality)
6.  alarms are triggered by `H` for all hosts
7.  users can use all the Netdata that maintain a database to view metrics (i.e. at `H` all hosts can be viewed).

## Configuration

These are options that affect the operation of Netdata in this area:

```
[global]
    memory mode = none | ram | save | map | dbengine
```

`[global].memory mode = none` disables the database at this host. This also disables health
monitoring (there cannot be health monitoring without a database).

```
[web]
    mode = none | static-threaded
    accept a streaming request every seconds = 0
```

`[web].mode = none` disables the API (Netdata will not listen to any ports).
This also disables the registry (there cannot be a registry without an API).

`accept a streaming request every seconds` can be used to set a limit on how often a parent node will accept streaming
requests from its child nodes. 0 sets no limit, 1 means maximum once every second. If this is set, you may see error log
entries "... too busy to accept new streaming request. Will be allowed in X secs".

```
[backend]
    enabled = yes | no
    type = graphite | opentsdb
    destination = IP:PORT ...
    update every = 10
```

`[backend]` configures data archiving to a backend (it archives all databases maintained on
this host).

### streaming configuration

A new file is introduced: `stream.conf` (to edit it on your system run
`/etc/netdata/edit-config stream.conf`). This file holds streaming configuration for both the
sending and the receiving Netdata.

API keys are used to authorize the communication of a pair of sending-receiving Netdata.
Once the communication is authorized, the sending Netdata can push metrics for any number of hosts.

You can generate an API key with the command `uuidgen`. API keys are just random GUIDs.
You can use the same API key on all your Netdata, or use a different API key for any pair of
sending-receiving Netdata.

##### options for the sending node

This is the section for the sending Netdata. On the receiving node, `[stream].enabled` can be `no`.
If it is `yes`, the receiving node will also stream the metrics to another node (i.e. it will be
a proxy).

```
[stream]
    enabled = yes | no
    destination = IP:PORT[:SSL] ...
    api key = XXXXXXXXXXX
```

This is an overview of how these options can be combined:

| target|memory<br/>mode|web<br/>mode|stream<br/>enabled|backend|alarms|dashboard|
|------|:-------------:|:----------:|:----------------:|:-----:|:----:|:-------:|
| headless collector|`none`|`none`|`yes`|only for `data source = as collected`|not possible|no|
| headless proxy|`none`|not `none`|`yes`|only for `data source = as collected`|not possible|no|
| proxy with db|not `none`|not `none`|`yes`|possible|possible|yes|
| central netdata|not `none`|not `none`|`no`|possible|possible|yes|

For the options to encrypt the data stream between the child and the parent, refer to [securing the communication](#securing-streaming-communications)

##### options for the receiving node

`stream.conf` looks like this:

```sh
# replace API_KEY with your uuidgen generated GUID
[API_KEY]
    enabled = yes
    default history = 3600
    default memory mode = save
    health enabled by default = auto
    allow from = *
```

You can add many such sections, one for each API key. The above are used as default values for
all hosts pushed with this API key.

You can also add sections like this:

```sh
# replace MACHINE_GUID with the child /var/lib/netdata/registry/netdata.public.unique.id
[MACHINE_GUID]
    enabled = yes
    history = 3600
    memory mode = save
    health enabled = yes
    allow from = *
```

The above is the parent configuration of a single host, at the parent end. `MACHINE_GUID` is
the unique id the Netdata generating the metrics (i.e. the Netdata that originally collects
them `/var/lib/netdata/registry/netdata.unique.id`). So, metrics for Netdata `A` that pass through
any number of other Netdata, will have the same `MACHINE_GUID`.

You can also use `default memory mode = dbengine` for an API key or `memory mode = dbengine` for
 a single host. The additional `page cache size` and `dbengine multihost disk space` configuration options
 are inherited from the global Netdata configuration.

##### allow from

`allow from` settings are [Netdata simple patterns](/libnetdata/simple_pattern/README.md): string matches
that use `*` as wildcard (any number of times) and a `!` prefix for a negative match.
So: `allow from = !10.1.2.3 10.*` will allow all IPs in `10.*` except `10.1.2.3`. The order is
important: left to right, the first positive or negative match is used.

`allow from` is available in Netdata v1.9+

##### tracing

When a child is trying to push metrics to a parent or proxy, it logs entries like these:

```
2017-02-25 01:57:44: netdata: ERROR: Failed to connect to '10.11.12.1', port '19999' (errno 111, Connection refused)
2017-02-25 01:57:44: netdata: ERROR: STREAM costa-pc [send to 10.11.12.1:19999]: failed to connect
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: initializing communication...
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: waiting response from remote netdata...
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: established communication - sending metrics...
2017-02-25 01:58:14: netdata: ERROR: STREAM costa-pc [send]: discarding 1900 bytes of metrics already in the buffer.
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [send]: ready - sending metrics...
```

The receiving end (proxy or parent) logs entries like these:

```
2017-02-25 01:58:04: netdata: INFO : STREAM [receive from [10.11.12.11]:33554]: new client connection.
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [10.11.12.11]:33554: receive thread created (task id 7698)
2017-02-25 01:58:14: netdata: INFO : Host 'costa-pc' with guid '12345678-b5a6-11e6-8a50-00508db7e9c9' initialized, os: linux, update every: 1, memory mode: ram, history entries: 3600, streaming: disabled, health: enabled, cache_dir: '/var/cache/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9', varlib_dir: '/var/lib/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9', health_log: '/var/lib/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9/health/health-log.db', alarms default handler: '/usr/libexec/netdata/plugins.d/alarm-notify.sh', alarms default recipient: 'root'
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [receive from [10.11.12.11]:33554]: initializing communication...
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [receive from [10.11.12.11]:33554]: receiving metrics...
```

For Netdata v1.9+, streaming can also be monitored via `access.log`.

### Securing streaming communications

Netdata does not activate TLS encryption by default. To encrypt streaming connections, you first need to [enable TLS support](/web/server/README.md#enabling-tls-support) on the parent. With encryption enabled on the receiving side, you need to instruct the child to use TLS/SSL as well. On the child's `stream.conf`, configure the destination as follows:

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

When TLS/SSL is enabled on the child, the default behavior will be to not connect with the parent unless the server's certificate can be verified via the default chain. In case you want to avoid this check, add the following to the child's `stream.conf` file:

```
[stream]
    ssl skip certificate verification = yes
```

#### Trusted certificate

If you've enabled [certificate verification](#certificate-verification), you might see errors from the OpenSSL library when there's a problem with checking the certificate chain (`X509_V_ERR_UNABLE_TO_GET_ISSUER_CERT_LOCALLY`). More importantly, OpenSSL will reject self-signed certificates.

Given these known issues, you have two options. If you trust your certificate, you can set the options `CApath` and `CAfile` to inform Netdata where your certificates, and the certificate trusted file, are stored.

For more details about these options, you can read about [verify locations](https://www.openssl.org/docs/man1.1.1/man3/SSL_CTX_load_verify_locations.html).

Before you changed your streaming configuration, you need to copy your trusted certificate to your child system and add the certificate to OpenSSL's list.

On most Linux distributions, the `update-ca-certificates` command searches inside the `/usr/share/ca-certificates` directory for certificates. You should double-check by reading the `update-ca-certificate` manual (`man update-ca-certificate`), and then change the directory in the below commands if needed.

If you have `sudo` configured on your child system, you can use that to run the following commands. If not, you'll have to log in as `root` to complete them.

```
# mkdir /usr/share/ca-certificates/netdata
# cp parent_cert.pem /usr/share/ca-certificates/netdata/parent_cert.crt
# chown -R netdata.netdata /usr/share/ca-certificates/netdata/
```

First, you create a new directory to store your certificates for Netdata. Next, you need to change the extension on your certificate from `.pem` to `.crt` so it's compatible with `update-ca-certificate`. Finally, you need to change permissions so the user that runs Netdata can access the directory where you copied in your certificate.

Next, edit the file `/etc/ca-certificates.conf` and add the following line:

```
netdata/parent_cert.crt
```

Now you update the list of certificates running the following, again either as `sudo` or `root`:

```
# update-ca-certificates
```

> Some Linux distributions have different methods of updating the certificate list. For more details, please read this
> guide on [addding trusted root certificates](https://github.com/Busindre/How-to-Add-trusted-root-certificates).

Once you update your certificate list, you can set the stream parameters for Netdata to trust the parent certificate. Open `stream.conf` for editing and change the following lines:

```
[stream]
    CApath = /etc/ssl/certs/
    CAfile = /etc/ssl/certs/parent_cert.pem
```

With this configuration, the `CApath` option tells Netdata to search for trusted certificates inside `/etc/ssl/certs`. The `CAfile` option specifies the Netdata parent certificate is located at `/etc/ssl/certs/parent_cert.pem`. With this configuration, you can skip using the system's entire list of certificates and use Netdata's parent certificate instead.

#### Expected behaviors

With the introduction of TLS/SSL, the parent-child communication behaves as shown in the table below, depending on the following configurations:

-   **Parent TLS (Yes/No)**: Whether the `[web]` section in `netdata.conf` has `ssl key` and `ssl certificate`.
-   **Parent port TLS (-/force/optional)**: Depends on whether the `[web]` section `bind to` contains a `^SSL=force` or `^SSL=optional` directive on the port(s) used for streaming.
-   **Child TLS (Yes/No)**: Whether the destination in the child's `stream.conf` has `:SSL` at the end.
-   **Child TLS Verification (yes/no)**: Value of the child's `stream.conf` `ssl skip certificate verification` parameter (default is no).

| Parent TLS enabled|Parent port SSL|Child TLS|Child SSL Ver.|Behavior|
|:----------------:|:-------------:|:-------:|:------------:|:-------|
| No|-|No|no|Legacy behavior. The parent-child stream is unencrypted.|
| Yes|force|No|no|The parent rejects the child connection.|
| Yes|-/optional|No|no|The parent-child stream is unencrypted (expected situation for legacy child nodes and newer parent nodes)|
| Yes|-/force/optional|Yes|no|The parent-child stream is encrypted, provided that the parent has a valid TLS/SSL certificate. Otherwise, the child refuses to connect.|
| Yes|-/force/optional|Yes|yes|The parent-child stream is encrypted.|

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

On the parent, edit `/etc/netdata/stream.conf` (to edit it on your system run `/etc/netdata/edit-config stream.conf`) and set these:

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

On each of the child nodes, edit `/etc/netdata/stream.conf` (to edit it on your system run `/etc/netdata/edit-config stream.conf`) and set these:

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

#### Netdata unique id

The file `/var/lib/netdata/registry/netdata.public.unique.id` contains a random GUID that **uniquely identifies each Netdata**. This file is automatically generated, by Netdata, the first time it is started and remains unaltered forever.

> If you are building an image to be used for automated provisioning of autoscaled VMs, it important to delete that file from the image, so that each instance of your image will generate its own.

#### Troubleshooting metrics streaming

Both parent and child nodes log information at `/var/log/netdata/error.log`.

Run the following on both the parent and child nodes:

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

Check the Netdata [exporting documentation](/docs/export/external-databases.md) for configuring this.

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

A proxy is a Netdata instance that is receiving metrics from a Netdata, and streams them to another Netdata.

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

Slow connections can also cause problems when the parent misses a message and then recieves a command related to the
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
data consistent, read our [memory mode documentation](/database/README.md).

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fstreaming%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
