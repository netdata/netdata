# Step 9. Long-term metrics storage

By default, Netdata stores metrics in a custom database we call the [database engine](../../database/engine/), which
stores recent metrics in your system's RAM and "spills" historical metrics to disk. By using both RAM and disk, the
database engine helps you store a much larger dataset than the amount of RAM your system has.

On a system that's collecting 2,000 metrics every second, the database engine's default configuration will store about
two day's worth of metrics in RAM and on disk.

That's a lot of metrics. We're talking 345,600,000 individual data points. And the database engine does it with a tiny
a portion of the RAM available on most systems.

To store _even more_ metrics, you have two options. First, you can tweak the database engine's options to expand the RAM
or disk it uses. Second, you can archive metrics to a different backend. For that, we'll use MongoDB and Prometheus as
examples.

## What you'll learn in this step

In this step of the Netdata guide, you'll learn how to:

-   [Tweak the database engine's settings](#tweak-the-database-engines-settings)
-   [Archive metrics to a backend](#archive-metrics-to-a-backend)
    -   [Use the MongoDB backend](#archive-metrics-via-the-mongodb-backend)
    -   [Use the Prometheus backend](#archive-metrics-via-the-prometheus-remote-write-backend)

Let's get started!

## Tweak the database engine's settings

If you're using Netdata v1.18.0 or higher, and you haven't changed your `memory mode` settings before following this
tutorial, your Netdata agent is already using the database engine.

Let's look at your `netdata.conf` file again. Under the `[global]` section, you'll find three connected options.

```conf
[global]
    # memory mode = dbengine
    # page cache size = 32
    # dbengine disk space = 256
```

The `memory mode` option is set, by default, to `dbengine`. `page cache size` determines the amount of RAM, in MiB, that
the database engine dedicates to caching the metrics it's collecting. `dbengine disk space` determines the amount of
disk space, in MiB, that the database engine will use to store these metrics once they've been "spilled" to disk..

You can uncomment and change either `page cache size` or `dbengine disk space` based on how much RAM and disk you want
the database engine to use. The higher those values, the more metrics Netdata will store. If you change them to 64 and
512, respectively, the database engine should store about four day's worth of data on a system collecting 2,000 metrics
every second.

> Before you make changes, we recommended you read up on the [database
> engine's](../../database/engine/README.md#memory-requirements) to ensure you don't overwhelm your system. Out of
> memory errors are no fun!

```conf
[global]
    memory mode = dbengine
    page cache size = 64
    dbengine disk space = 512
```

After you've made your changes, [restart Netdata](../getting-started.md#start-stop-and-restart-netdata).

To confirm the database engine is working, go to your Netdata dashboard and click on the **Netdata Monitoring** menu on
the right-hand side. You can find `dbengine` metrics after `queries`.

![Image of the database engine reflected in the Netdata
Dashboard](https://user-images.githubusercontent.com/12263278/64781383-9c71fe00-d55a-11e9-962b-efd5558efbae.png)

## Archive metrics to a backend

You can archive all the metrics collected by Netdata to what we call **backends**. The supported backends include
Graphite, OpenTSDB, Prometheus, AWS Kinesis Data Streams, MongoDB, and the list is always growing.

As we said in [step 1](step-01.md), we have only complimentary systems, not competitors! We're happy to support these
archiving methods and are always working to improve them.

A lot of Netdata users archive their metrics to one of these backends for long-term storage or further analysis. Since
Netdata collects so many metrics every second, they can quickly overload small devices or even big servers that are
aggregating metrics streaming in from other Netdata agents.

We even support resampling metrics during archiving. With resampling enabled, Netdata will archive only the average or
sum of every X seconds of metrics. This reduces the sheer amount of data, albeit with a little less accuracy.

How you archive metrics, or if you archive metrics at all, is entirely up to you! But let's cover two easy archiving
methods, MongoDB and Prometheus remote write, to get you started.

> Currently, Netdata can only use a single backend at a time. We are currently working on a new archiving solution,
> which we call "exporters," that simplifies the configuration process and allows you to archive to multiple backends.
> We'll update this tutorial as soon as exporters are enabled.

### Archive metrics via the MongoDB backend

Let's begin with installing MongoDB its dependencies.

```bash
sudo apt-get install mongodb  # Debian/Ubuntu
sudo dnf install mongodb      # Fedora
sudo yum install mongodb      # CentOS
```

Next, install the one essential dependency: v1.7.0 or higher of
[libmongoc](http://mongoc.org/libmongoc/current/installing.html). 

```bash
sudo apt-get install libmongoc-1.0-0  # Debian/Ubuntu
sudo dnf install mongo-c-driver       # Fedora
sudo yum install mongo-c-driver       # CentOS
```

Let's create a new database and connection to 
**FINISH THIS**

Next, Netdata needs to be reinstalled in order to detect that the required libraries to make this backend connection
exist. Since you most likely installed Netdata using the one-line installer script, all you have to do is run that
script again. Don't worry—any configuration changes you made along the way will be retained!

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

To enable archiving to the MongoDB backend, set the options in the `backend` section of `netdata.conf` to the following:

```conf
[backend]
    enabled = yes
    type = mongodb
```

In your Netdata config directory, configure the [MongoDB
URI](https://docs.mongodb.com/manual/reference/connection-string/), database name, and collection name by running:

```sh
./edit-config mongodb.conf
```

You will proceed to fill up the MongoDB connection details:

```yaml
# URI
uri = mongodb://<hostname>

# database name
database = your_database_name

# collection name
collection = your_collection_name
```

[Restart](../getting-started.md#start-stop-and-restart-netdata) Netdata.

You can confirm MongoDB is saving your metrics using
[mongotop](https://docs.mongodb.com/manual/reference/program/mongotop/#bin.mongotop). Run the following command:

```sh
mongotop --uri "mongodb://<hostname>/your_database_name"
```

### Archive metrics via the Prometheus remote write backend

If you don't want to use the MongoDB backend, you can try the Prometheus remote write API.

To use this option with [storage
providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage),
[protobuf](https://developers.google.com/protocol-buffers/) and [snappy](https://github.com/google/snappy), install the
libraries first. Next, Netdata should be re-installed from the source. The installer will detect that the required
libraries and utilities are now available.

In the `[backend]` section of `netdata.conf`, enable and add configuration for the remote write API:

```conf
[backend]
    enabled = yes
    type = prometheus_remote_write
    remote write URL path = /receive
```

[Restart](../getting-started.md#start-stop-and-restart-netdata) Netdata. It will now be archiving historical metrics to
your Prometheus backend!

You can check out the following great resources on how to use Netdata and Prometheus:

-   [Using Netdata with Prometheus](../../backends/prometheus/)
-   [Netdata, Prometheus, Grafana stack](../../backends/WALKTHROUGH.md)

## What's next?

You're getting close to the end! In this step, you learned how to make the most of the database engine, or archive
metrics to MongoDB for long-term storage.

In the last step of this step-by-step tutorial, we'll put our sysadmin hat on and use Nginx to proxy traffic to and from
our Netdata dashboard.

[Next: Set up a proxy &rarr;](step-10.md)
