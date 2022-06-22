<!--
title: "Step 9. Long-term metrics storage"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/step-by-step/step-09.md
-->

# Step 9. Long-term metrics storage

By default, Netdata stores metrics in a custom database we call the [database engine](/database/engine/README.md), which
stores recent metrics in your system's RAM and "spills" historical metrics to disk. By using both RAM and disk, the
database engine helps you store a much larger dataset than the amount of RAM your system has.

On a system that's collecting approximately 2,000 metrics every second, the database engine's default configuration will
store about two days worth of metrics in RAM and on disk.

That's a lot of metrics. We're talking 345,600,000 individual data points. And the database engine does it with a tiny a
footprint in the RAM available of any system.

To increase the data retention of your collected metrics you could do that with two ways:

1. Increase the data retention for the node in question (if the nodes has enough resources)
2. Set up a parent Netdata Agent to act as a centralized database for multiple Agents.
3. Archive metrics to an external database

The third option is just an archive of your data, you will be able to see them only by querying and creating custom
dashboards.

## What you'll learn in this step

In this step of the Netdata guide, you'll learn how to:

- [Configure the Agent to retain its data for a greater period](#Configure-the-Agent-to-retain-its-data-for-a-greater-period)
- [Set up a parent node to act as a centralize database for multiple Agents](#Set-up-a-parent-node-to-act-as-a-centralize-database-for-multiple-Agents)
- [Archive metrics to an external database](#archive-metrics-to-an-external-database)

Let's get started!

## Configure the Agent to retain its data for a greater period

> If you're using Netdata v1.18.0 or higher, and you haven't changed your `memory mode` settings before following this guide, your Netdata agent is already using the database engine.

Let's look at your `netdata.conf` file again. Under the `[global]` section, you'll find the following entries.

```conf
[global]
    . . . 
    # page cache size = 32
    # dbengine mulithost disk space = 256
    . . .
    # memory mode = dbengine
    . . .
```

The `memory mode` option is set by default to `dbengine`. The `page cache size` parameter determines the amount of RAM,
in MiB that the database engine dedicates for caching the metrics it collects. The `dbengine multihost disk space`
parameter determines the amount of disk space (in MiB) Netdata can allocate from the host for **any data** (metrics) it
will store in it.

[**See our database engine calculator**](/docs/store/change-metrics-storage.md) to help you correctly
set `dbengine multihost disk space` based on your needs. The calculator gives an approximately estimation based on how
many child nodes you have, how many metrics your Agent collects, and more.

These values above are the defaults, to change the data retention you mostly care about the
`dbengine multihost disk space`. The higher this value, the more metrics Netdata will store. For example a value of
524 (in MiB) the database engine should store about two days worth of data on a system collecting 2,000 metrics every
second.

```conf
[global]
    . . . 
    # page cache size = 32
    # dbengine mulithost disk space = 524
    . . .
    # memory mode = dbengine
    . . .
```

After you've made your changes, restart Netdata using `sudo systemctl restart netdata`, or
the [appropriate method](/docs/configure/start-stop-restart.md) for your system.

Done!

## Set up a parent node to act as a centralize database for multiple Agents

> For this how-to you will need to have installed the Agent in two different hosts that can talk to each other

### Intermediate steps of this guide

What will cover in this guide:

1. Configure the Agents in a parent-child architecture (enable streaming between nodes)
2. Configure the parent Agent to retain its data for a greater period.

#### Configure the Agents in a parent-child architecture

> We recommend you to follow the [Enable streaming between node](/docs/metrics-storage-management/enable-streaming.md)
> guide to explore all the options of this process, otherwise stick to the following instructions for the least
> actions you need to do.

##### Enable streaming on the parent node

First, log onto the node that will act as the parent.

Run `uuidgen` to create a new API key, which is a randomly-generated machine GUID the Netdata Agent uses to identify
itself while initiating a streaming connection. Copy that into a separate text file for later use.

> Find out how to [install `uuidgen`](https://command-not-found.com/uuidgen) on your node if you don't already have it.

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

Set `enabled` to `yes`, and `default memory mode` to `dbengine`. Leave all the other settings as their defaults. A
simplified version of the configuration, minus the commented lines, looks like the following:

```conf
[11111111-2222-3333-4444-555555555555]
    enabled = yes
    default memory mode = dbengine
```

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or
the [appropriate method](/docs/configure/start-stop-restart.md) for your system.

##### Enable streaming on the child node

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

Save the file and close it, then restart Netdata with `sudo systemctl restart netdata`, or
the [appropriate method](/docs/configure/start-stop-restart.md) for your system.

#### Configure the parent Agent to retain its data for a greater period.

Here we won't introduce new actions here, you need to follow
the [Configure the Agent to retain its data for a greater period](#Configure-the-Agent-to-retain-its-data-for-a-greater-period)
guide we presented above, but we will focus on some aspects of it

In this guide we will highlight this: _`dbengine multihost disk space` parameter determines the amount of disk space (in
MiB) Netdata can allocate from the host for **any data** (metrics)_ sentence, which needs a bit more explanation. A
parent now will need to balance it's available space for:

1. The metrics it collects (its own database)
2. The metrics every child collects

That means that you need to take into consideration and all the metrics your children collect and include this info to
your calculations in the [**database engine calculator**](/docs/store/change-metrics-storage.md)

## Archive metrics to an external database

You can archive all the metrics collected by Netdata to **external databases**. The supported databases and services
include Graphite, OpenTSDB, Prometheus, AWS Kinesis Data Streams, Google Cloud Pub/Sub, MongoDB, and the list is always
growing.

As we said in [step 1](/docs/guides/step-by-step/step-01.md), we have only complimentary systems, not competitors! We're
happy to support these archiving methods and are always working to improve them.

A lot of Netdata users archive their metrics to one of these databases for long-term storage or further analysis. Since
Netdata collects so many metrics every second, they can quickly overload small devices or even big servers that are
aggregating metrics streaming in from other Netdata agents.

We even support resampling metrics during archiving. With resampling enabled, Netdata will archive only the average or
sum of every X seconds of metrics. This reduces the sheer amount of data, albeit with a little less accuracy.

How you archive metrics, or if you archive metrics at all, is entirely up to you! But let's cover two easy archiving
methods, MongoDB and Prometheus remote write, to get you started.

### Archive metrics via the MongoDB exporting connector

Begin by installing MongoDB its dependencies via the correct package manager for your system.

```bash
sudo apt-get install mongodb  # Debian/Ubuntu
sudo dnf install mongodb      # Fedora
sudo yum install mongodb      # CentOS
```

Next, install the one essential dependency: v1.7.0 or higher of
[libmongoc](http://mongoc.org/libmongoc/current/installing.html).

```bash
sudo apt-get install libmongoc-1.0-0 libmongoc-dev    # Debian/Ubuntu
sudo dnf install mongo-c-driver mongo-c-driver-devel  # Fedora
sudo yum install mongo-c-driver mongo-c-driver-devel  # CentOS
```

Next, create a new MongoDB database and collection to store all these archived metrics. Use the `mongo` command to start
the MongoDB shell, and then execute the following command:

```mongodb
use netdata
db.createCollection("netdata_metrics")
```

Next, Netdata needs to be [reinstalled](/packaging/installer/REINSTALL.md) in order to detect that the required
libraries to make this exporting connection exist. Since you most likely installed Netdata using the one-line installer
script, all you have to do is run that script again. Don't worry—any configuration changes you made along the way will
be retained!

Now, from your Netdata config directory, initialize and edit a `exporting.conf` file to tell Netdata where to find the
database you just created.

```sh
./edit-config exporting.conf
```

Add the following section to the file:

```conf
[mongodb:my_mongo_instance]
    enabled = yes
    destination = mongodb://localhost
    database = netdata
    collection = netdata_metrics
```

Restart Netdata using `sudo systemctl restart netdata`, or
the [appropriate method](/docs/configure/start-stop-restart.md) for your system, to enable the MongoDB exporting
connector. Click on the
**Netdata Monitoring** menu and check out the **exporting my mongo instance** sub-menu. You should start seeing these
charts fill up with data about the exporting process!

![image](https://user-images.githubusercontent.com/1153921/70443852-25171200-1a56-11ea-8be3-494544b1c295.png)

If you'd like to try connecting Netdata to another database, such as Prometheus or OpenTSDB, read
our [exporting documentation](/exporting/README.md).

## What's next?

You're getting close to the end! In this step, you learned how to make the most of the database engine, or archive
metrics to MongoDB for long-term storage.

In the last step of this step-by-step guide, we'll put our sysadmin hat on and use Nginx to proxy traffic to and from
our Netdata dashboard.

[Next: Set up a proxy &rarr;](/docs/guides/step-by-step/step-10.md)


