# Real-time performance monitoring for ephemeral nodes

Auto-scaling is probably the most trendy service deployment strategy these days.

Auto-scaling detects the need for additional resources and boots VMs on demand, based on a template. Soon after they start running the applications, a load balancer starts distributing traffic to them, allowing the service to grow horizontally to the scale needed to handle the load. When demands falls, auto-scaling starts shutting down VMs that are no longer needed.

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627426/65a9074a-02b9-11e7-9664-cd8f258a00af.png"/>
</p>

What a fantastic feature for controlling infrastructure costs! Pay only for what you need for the time you need it!

In auto-scaling, all servers are ephemeral, they live for just a few hours. Every VM is a brand new instance of the application, that was automatically created based on a template.

So, how can we monitor them? How can we be sure that everything is working as expected on all of them?

## The netdata way

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

## Configuring an auto-scaling setup

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627468/96daf7ba-02b9-11e7-95ac-1f767dd8dab8.png"/>
</p>

You need a netdata `master`. This node should not be ephemeral. It will be the node where all ephemeral nodes (let's call them `slaves`) will be sending their metrics.

The master will need to authorize the slaves for accepting their metrics. This is done with an API key.

### API keys

API keys are just random GUIDs. Use the Linux command `uuidgen` to generate one. You can use the same API key for all your `slaves`, or you can configure one API for each of them. This is entirely your decision.

We suggest to use the same API key for each ephemeral node template you have, so that all replicas of the same ephemeral node will have exactly the same configuration.

I will use this API_KEY: `11111111-2222-3333-4444-555555555555`. Replace it with your own.

### Configuring the `master`

On the master, edit `/etc/netdata/stream.conf` and set these:

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

### Configuring the `slaves`

On each of the slaves, edit `/etc/netdata/stream.conf` and set these:

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

### netdata unique id

The file `/var/lib/netdata/registry/netdata.public.unique.id` contains a random GUID that **uniquely identifies each netdata**. This file is automatically generated, by netdata, the first time it is started and remains unaltaired forever.

> If you are building an image to be used for automated provisioning of autoscaled VMs, it important to delete that file from the image, so that each instance of your image will generate its own.

### Troubleshooting metrics streaming

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


## Archiving to a time-series database

The `master` netdata node can also archive metrics, for all `slaves`, to a time-series database. At the time of this writing, netdata supports:

- graphite
- opentsdb
- prometheus
- json document DBs
- all the compatibles to the above (e.g. kairosdb, influxdb, etc)

Check the netdata [backends documentation](https://github.com/netdata/netdata/wiki/netdata-backends) for configuring this.

This is how such a solution will work:

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23627295/e3569adc-02b8-11e7-9d55-4014bf98c1b3.png"/>
</p>

## An advanced setup

netdata also supports `proxies` with and without a local database, and data retention can be different between all nodes.

This means a setup like the following is also possible:

<p align="center">
<img src="https://cloud.githubusercontent.com/assets/2662304/23629551/bb1fd9c2-02c0-11e7-90f5-cab5a3ed4c53.png"/>
</p>
