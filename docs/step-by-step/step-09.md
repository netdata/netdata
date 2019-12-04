# Step 9. Long-term metrics storage

By default, Netdata stores metrics in a [database engine](/docs/database/engine/), which stores recent metrics in your
system's RAM while "spilling" historical metrics to disk. It helps you store a much larger dataset than the amount of
RAM your system has.

And it's the best of both worlds. Recent metrics load immediately onto the dashboard, but the database engine doesn't
overload your system's RAM by trying to keep _everything_ there.

On a system that's collecting 2,000 metrics every second, the database engine's default configuration will store about
two day's worth of metrics in RAM and on disk.

That's a lot of metrics. We're talking 345,600,000 individual data points. And the database engine does it with a tiny
a portion of the RAM available on most systems.

To store _even more_ metrics, you have two options. First, you can tweak the database engine's options to expand the RAM
or disk it uses. Second, you can archive metrics to a separate database backend. We'll use MongoDB and Prometheus as
examples.

## What you'll learn in this step

In this step of the Netdata guide, you'll learn how to:

-   [Tweak the database engine's settings](#tweak-the-database-engines-settings)
-   [Use the MongoDB backend](#use-the-mongodb-backend)
-   [Use the Prometheus backend](#use-the-prometheus-remote-write-backend)

Let's get started!

## Tweak the database engine's settings

Netdata comes bundled with a custom database we call the _database engine_, and it's enabled by default. If you're using
Netdata v1.18.0 or higher, your Netdata agent is already using it.

Let's look at your `netdata.conf` file again. Under the `[global]` section, you'll find three connected options.

```conf
[global]
    # memory mode = dbengine
    # page cache size = 32
    # dbengine disk space = 256
```

Don't worry that `memory mode` is uncommented. It's still on by default. What about the other two lines?

The `page cache size` option determines the amount of RAM, in MiB, that Netdata dedicates to caching Netdata metric
values themselves.

The `dbengine disk space` option determines the amount of disk space, in MiB, that Netdata dedicates to storing Netdata
metric values and all related metadata describing them.

You can uncomment and change either of these values according to the resources you want the database engine
to use. However, it's recommended you read up on [](/docs/database/engine/#memory-requirements) to ensure you don't
overwhelm your system. Out of memory errors are no fun!

```conf
[global]
    memory mode = dbengine
    page cache size = 64
    dbengine disk space = 512
```

After you've made your changes, [restart](/docs/getting-started/#starting-and-stopping-netdata) the `netdata` service.

To confirm the database engine is working, go to your Netdata dashboard and click on the **Netdata Monitoring** menu on
the right-hand side. You can find `dbengine` metrics after `queries`.

![Image of the database engine reflected in the Netdata Dashboard](https://user-images.githubusercontent.com/12263278/64781383-9c71fe00-d55a-11e9-962b-efd5558efbae.png)

## Use the MongoDB backend

Netdata supports the use of backends for archiving metrics.

The supported backends include Graphite, Opentsdb, Prometheus, AWS Kinesis Data Streams, and MongoDB.

Since Netdata collects thousands of metrics per server per second, which would easily congest any backend server when
several Netdata servers are sending data to it, Netdata allows sending metrics at a lower frequency, by resampling them.

So, although Netdata collects metrics every second, it can send to the backend servers averages or sums every X seconds
(though, it can send them per second if you need it to).

With MongoDB installed and running, you can proceed to install a requirement for the backend, [`libmongoc` 1.7.0 or
higher](http://mongoc.org/libmongoc/current/installing.html).

Next, Netdata should be re-installed from the source. The installer will detect that the required libraries are now
available.

To enable archiving to the MongoDB backend, set the options in the `backend` section of `netdata.conf` to the following:

```conf
[backend]
    enabled = yes
    type = mongodb
```

In the Netdata directory, configure the [MongoDB URI](https://docs.mongodb.com/manual/reference/connection-string/),
database name, and collection name by running:

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

[Restart](/docs/getting-started/#starting-and-stopping-netdata) Netdata.

You can confirm MongoDB is saving your metrics using
[mongotop](https://docs.mongodb.com/manual/reference/program/mongotop/#bin.mongotop). Run the following command:

```sh
mongotop --uri "mongodb://<hostname>/your_database_name"
```

## Use the Prometheus remote write backend

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

[Restart](/docs/getting-started/#starting-and-stopping-netdata) Netdata. It will now be
archiving historical metrics to your Prometheus backend!

You can check out the following great resources on how to use Netdata and Prometheus:

-   [Using Netdata with Prometheus](/docs/backends/prometheus/)
-   [Netdata, Prometheus, Grafana stack](/docs/backends/walkthrough/)

## What's next?

You're getting close to the end! In this step, you learned how to make the most of the database engine, or archive
metrics to MongoDB for long-term storage.

In the last step of this step-by-step tutorial, we'll put our sysadmin hat on and use Nginx to proxy traffic to and from
our Netdata dashboard.

[Next: Set up a proxy &rarr;](step-10.md)
