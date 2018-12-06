# Streaming and replication

Each netdata is able to replicate/mirror its database to another netdata, by streaming collected
metrics, in real-time to it. This is quite different to [data archiving to third party time-series
databases](../backends).

When a netdata streams metrics to another netdata, the receiving one is able to perform everything
a netdata performs:

- visualize them with a dashboard
- run health checks that trigger alarms and send alarm notifications
- archive metrics to a backend time-series database

## Supported configurations

### netdata without a database or web API (headless collector)

Local netdata (`slave`), **without any database or alarms**, collects metrics and sends them to
another netdata (`master`).

The user can take the full functionality of the `slave` netdata at
http://master.ip:19999/host/slave.hostname/. Alarms for the `slave` are served by the `master`.

In this mode the `slave` is just a plain data collector.
It runs with... **5MB** of RAM (yes, you read correct), spawns all external plugins, but instead
of maintaining a local database and accepting dashboard requests, it streams all metrics to the
`master`.

The same `master` can collect data for any number of `slaves`.

### database replication

Local netdata (`slave`), **with a local database (and possibly alarms)**, collects metrics and
sends them to another netdata (`master`).

The user can use all the functions **at both** http://slave.ip:19999/ and
http://master.ip:19999/host/slave.hostname/.

The `slave` and the `master` may have different data retention policies for the same metrics.

Alarms for the `slave` are triggered by **both** the `slave` and the `master` (and actually
each can have different alarms configurations or have alarms disabled).

### netdata proxies

Local netdata (`slave`), with or without a database, collects metrics and sends them to another
netdata (`proxy`), which may or may not maintain a database, which forwards them to another
netdata (`master`).

Alarms for the slave can be triggered by any of the involved hosts that maintains a database.

Any number of daisy chaining netdata servers are supported, each with or without a database and
with or without alarms for the `slave` metrics.

### mix and match with backends

All nodes that maintain a database can also send their data to a backend database.
This allows quite complex setups.

Example:

1. netdata `A`, `B` do not maintain a database and stream metrics to netdata `C`(live streaming functionality, i.e. this PR)
2. netdata `C` maintains a database for `A`, `B`, `C` and archives all metrics to `graphite` with 10 second detail (backends functionality)
3. netdata `C` also streams data for `A`, `B`, `C` to netdata `D`, which also collects data from `E`, `F` and `G` from another DMZ (live streaming functionality, i.e. this PR)
4. netdata `D` is just a proxy, without a database, that streams all data to a remote site at netdata `H`
5. netdata `H` maintains a database for `A`, `B`, `C`, `D`, `E`, `F`, `G`, `H` and sends all data to `opentsdb` with 5 seconds detail (backends functionality)
6. alarms are triggered by `H` for all hosts
7. users can use all the netdata that maintain a database to view metrics (i.e. at `H` all hosts can be viewed).

## Configuration

These are options that affect the operation of netdata in this area:

```
[global]
    memory mode = none | ram | save | map
```

`[global].memory mode = none` disables the database at this host. This also disables health
monitoring (there cannot be health monitoring without a database).

```
[web]
    mode = none | static-threaded | single-threaded | multi-threaded
    accept a streaming request every seconds = 0 
```

`[web].mode = none` disables the API (netdata will not listen to any ports).
This also disables the registry (there cannot be a registry without an API).

`accept a streaming request every seconds` can be used to set a limit on how often a master Netdata server will accept streaming requests from the slaves. 0 sets no limit, 1 means maximum once every second. If this is set, you may see error log entries "... too busy to accept new streaming request. Will be allowed in X secs". 

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

A new file is introduced: [stream.conf](stream.conf) (to edit it on your system run
`/etc/netdata/edit-config stream.conf`). This file holds streaming configuration for both the
sending and the receiving netdata.

API keys are used to authorize the communication of a pair of sending-receiving netdata.
Once the communication is authorized, the sending netdata can push metrics for any number of hosts.

You can generate an API key with the command `uuidgen`. API keys are just random GUIDs.
You can use the same API key on all your netdata, or use a different API key for any pair of
sending-receiving netdata.

##### options for the sending node

This is the section for the sending netdata. On the receiving node, `[stream].enabled` can be `no`.
If it is `yes`, the receiving node will also stream the metrics to another node (i.e. it will be
a `proxy`).

```
[stream]
    enabled = yes | no
    destination = IP:PORT ...
    api key = XXXXXXXXXXX
```

This is an overview of how these options can be combined:

target | memory<br/>mode | web<br/>mode | stream<br/>enabled | backend | alarms | dashboard
-------|:-----------:|:---:|:------:|:-------:|:---------:|:----:
headless collector|`none`|`none`|`yes`|only for `data source = as collected`|not possible|no
headless proxy|`none`|not `none`|`yes`|only for `data source = as collected`|not possible|no
proxy with db|not `none`|not `none`|`yes`|possible|possible|yes
central netdata|not `none`|not `none`|`no`|possible|possible|yes

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
# replace MACHINE_GUID with the slave /var/lib/netdata/registry/netdata.public.unique.id
[MACHINE_GUID]
    enabled = yes
    history = 3600
    memory mode = save
    health enabled = yes
    allow from = *
```

The above is the receiver configuration of a single host, at the receiver end. `MACHINE_GUID` is
the unique id the netdata generating the metrics (i.e. the netdata that originally collects
them `/var/lib/netdata/registry/netdata.unique.id`). So, metrics for netdata `A` that pass through
any number of other netdata, will have the same `MACHINE_GUID`.

##### allow from

`allow from` settings are [netdata simple patterns](../libnetdata/simple_pattern): string matches
that use `*` as wildcard (any number of times) and a `!` prefix for a negative match.
So: `allow from = !10.1.2.3 10.*` will allow all IPs in `10.*` except `10.1.2.3`. The order is
important: left to right, the first positive or negative match is used.

`allow from` is available in netdata v1.9+

##### tracing

When a `slave` is trying to push metrics to a `master` or `proxy`, it logs entries like these:

```
2017-02-25 01:57:44: netdata: ERROR: Failed to connect to '10.11.12.1', port '19999' (errno 111, Connection refused)
2017-02-25 01:57:44: netdata: ERROR: STREAM costa-pc [send to 10.11.12.1:19999]: failed to connect
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: initializing communication...
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: waiting response from remote netdata...
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [send to 10.11.12.1:19999]: established communication - sending metrics...
2017-02-25 01:58:14: netdata: ERROR: STREAM costa-pc [send]: discarding 1900 bytes of metrics already in the buffer.
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [send]: ready - sending metrics...
```

The receiving end (`proxy` or `master`) logs entries like these:

```
2017-02-25 01:58:04: netdata: INFO : STREAM [receive from [10.11.12.11]:33554]: new client connection.
2017-02-25 01:58:04: netdata: INFO : STREAM costa-pc [10.11.12.11]:33554: receive thread created (task id 7698)
2017-02-25 01:58:14: netdata: INFO : Host 'costa-pc' with guid '12345678-b5a6-11e6-8a50-00508db7e9c9' initialized, os: linux, update every: 1, memory mode: ram, history entries: 3600, streaming: disabled, health: enabled, cache_dir: '/var/cache/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9', varlib_dir: '/var/lib/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9', health_log: '/var/lib/netdata/12345678-b5a6-11e6-8a50-00508db7e9c9/health/health-log.db', alarms default handler: '/usr/libexec/netdata/plugins.d/alarm-notify.sh', alarms default recipient: 'root'
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [receive from [10.11.12.11]:33554]: initializing communication...
2017-02-25 01:58:14: netdata: INFO : STREAM costa-pc [receive from [10.11.12.11]:33554]: receiving metrics...
```

For netdata v1.9+, streaming can also be monitored via `access.log`.


## Viewing remote host dashboards, using mirrored databases

On any receiving netdata, that maintains remote databases and has its web server enabled,
`my-netdata` menu will include a list of the mirrored databases.

![image](https://cloud.githubusercontent.com/assets/2662304/24080824/24cd2d3c-0caf-11e7-909d-a8dd1dbb95d7.png)

Selecting any of these, the server will offer a dashboard using the mirrored metrics.


## Monitoring ephemeral nodes

Auto-scaling is probably the most trendy service deployment strategy these days.

Auto-scaling detects the need for additional resources and boots VMs on demand, based on a template. Soon after they start running the applications, a load balancer starts distributing traffic to them, allowing the service to grow horizontally to the scale needed to handle the load. When demands falls, auto-scaling starts shutting down VMs that are no longer needed.

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627426/65a9074a-02b9-11e7-9664-cd8f258a00af.png"/>
</p>

What a fantastic feature for controlling infrastructure costs! Pay only for what you need for the time you need it!

In auto-scaling, all servers are ephemeral, they live for just a few hours. Every VM is a brand new instance of the application, that was automatically created based on a template.

So, how can we monitor them? How can we be sure that everything is working as expected on all of them?

### The netdata way

We recently made a significant improvement at the core of netdata to support monitoring such setups.

Following the netdata way of monitoring, we wanted:

1. **real-time performance monitoring**, collecting **_thousands of metrics per server per second_**, visualized in interactive, automatically created dashboards.
2. **real-time alarms**, for all nodes.
3. **zero configuration**, all ephemeral servers should have exactly the same configuration, and nothing should be configured at any system for each of the ephemeral nodes. We shouldn't care if 10 or 100 servers are spawned to handle the load.
4. **self-cleanup**, so that nothing needs to be done for cleaning up the monitoring infrastructure from the hundreds of nodes that may have been monitored through time.

### How it works

All monitoring solutions, including netdata, work like this:

1. `collect metrics`, from the system and the running applications
2. `store metrics`, in a time-series database
3. `examine metrics` periodically, for triggering alarms and sending alarm notifications
4. `visualize metrics`, so that users can see what exactly is happening

netdata used to be self-contained, so that all these functions were handled entirely by each server. The changes we made, allow each netdata to be configured independently for each function. So, each netdata can now act as:

- a `self contained system`, much like it used to be.
- a `data collector`, that collects metrics from a host and pushes them to another netdata (with or without a local database and alarms).
- a `proxy`, that receives metrics from other hosts and pushes them immediately to other netdata servers. netdata proxies can also be `store and forward proxies` meaning that they are able to maintain a local database for all metrics passing through them (with or without alarms).
- a `time-series database` node, where data are kept, alarms are run and queries are served to visualise the metrics.

### Configuring an auto-scaling setup

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627468/96daf7ba-02b9-11e7-95ac-1f767dd8dab8.png"/>
</p>

You need a netdata `master`. This node should not be ephemeral. It will be the node where all ephemeral nodes (let's call them `slaves`) will be sending their metrics.

The master will need to authorize the slaves for accepting their metrics. This is done with an API key.

#### API keys

API keys are just random GUIDs. Use the Linux command `uuidgen` to generate one. You can use the same API key for all your `slaves`, or you can configure one API for each of them. This is entirely your decision.

We suggest to use the same API key for each ephemeral node template you have, so that all replicas of the same ephemeral node will have exactly the same configuration.

I will use this API_KEY: `11111111-2222-3333-4444-555555555555`. Replace it with your own.

#### Configuring the `master`

On the master, edit `/etc/netdata/stream.conf` (to edit it on your system run `/etc/netdata/edit-config stream.conf`) and set these:

```bash
[11111111-2222-3333-4444-555555555555]
	# enable/disable this API key
    enabled = yes
    
    # one hour of data for each of the slaves
    default history = 3600
    
    # do not save slave metrics on disk
    default memory = ram
    
    # alarms checks, only while the slave is connected
    health enabled by default = auto
```
*`stream.conf` on master, to enable receiving metrics from slaves using the API key.*

If you used many API keys, you can add one such section for each API key.

When done, restart netdata on the `master` node. It is now ready to receive metrics.

#### Configuring the `slaves`

On each of the slaves, edit `/etc/netdata/stream.conf` (to edit it on your system run `/etc/netdata/edit-config stream.conf`) and set these:

```bash
[stream]
    # stream metrics to another netdata
    enabled = yes
    
    # the IP and PORT of the master
    destination = 10.11.12.13:19999
	
	# the API key to use
    api key = 11111111-2222-3333-4444-555555555555
```
*`stream.conf` on slaves, to enable pushing metrics to master at `10.11.12.13:19999`.*

Using just the above configuration, the `slaves` will be pushing their metrics to the `master` netdata, but they will still maintain a local database of the metrics and run health checks. To disable them, edit `/etc/netdata/netdata.conf` and set:

```bash
[global]
    # disable the local database
	memory mode = none

[health]
    # disable health checks
    enabled = no
```
*`netdata.conf` configuration on slaves, to disable the local database and health checks.*

Keep in mind that setting `memory mode = none` will also force `[health].enabled = no` (health checks require access to a local database). But you can keep the database and disable health checks if you need to. You are however sending all the metrics to the master server, which can handle the health checking (`[health].enabled = yes`)

#### netdata unique id

The file `/var/lib/netdata/registry/netdata.public.unique.id` contains a random GUID that **uniquely identifies each netdata**. This file is automatically generated, by netdata, the first time it is started and remains unaltaired forever.

> If you are building an image to be used for automated provisioning of autoscaled VMs, it important to delete that file from the image, so that each instance of your image will generate its own.

#### Troubleshooting metrics streaming

Both the sender and the receiver of metrics log information at `/var/log/netdata/error.log`.


On both master and slave do this:

```
tail -f /var/log/netdata/error.log | grep STREAM
```

If the slave manages to connect to the master you will see something like (on the master):

```
2017-03-09 09:38:52: netdata: INFO : STREAM [receive from [10.11.12.86]:38564]: new client connection.
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [10.11.12.86]:38564: receive thread created (task id 27721)
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: client willing to stream metrics for host 'xxx' with machine_guid '1234567-1976-11e6-ae19-7cdd9077342a': update every = 1, history = 3600, memory mode = ram, health auto
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: initializing communication...
2017-03-09 09:38:52: netdata: INFO : STREAM xxx [receive from [10.11.12.86]:38564]: receiving metrics...
```

and something like this on the slave:

```
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: connecting...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: initializing communication...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: waiting response from remote netdata...
2017-03-09 09:38:28: netdata: INFO : STREAM xxx [send to box:19999]: established communication - sending metrics...
```

### Archiving to a time-series database

The `master` netdata node can also archive metrics, for all `slaves`, to a time-series database. At the time of this writing, netdata supports:

- graphite
- opentsdb
- prometheus
- json document DBs
- all the compatibles to the above (e.g. kairosdb, influxdb, etc)

Check the netdata [backends documentation](../backends) for configuring this.

This is how such a solution will work:

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627295/e3569adc-02b8-11e7-9d55-4014bf98c1b3.png"/>
</p>

### An advanced setup

netdata also supports `proxies` with and without a local database, and data retention can be different between all nodes.

This means a setup like the following is also possible:

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23629551/bb1fd9c2-02c0-11e7-90f5-cab5a3ed4c53.png"/>
</p>


## proxies

A proxy is a netdata that is receiving metrics from a netdata, and streams them to another netdata.

netdata proxies may or may not maintain a database for the metrics passing through them.
When they maintain a database, they can also run health checks (alarms and notifications)
for the remote host that is streaming the metrics.

To configure a proxy, configure it as a receiving and a sending netdata at the same time,
using [stream.conf](stream.conf).

The sending side of a netdata proxy, connects and disconnects to the final destination of the
metrics, following the same pattern of the receiving side.

For a practical example see [Monitoring ephemeral nodes](#monitoring-ephemeral-nodes).
