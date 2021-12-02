<!--
title: "Metrics long term archiving"
custom_edit_url: https://github.com/netdata/netdata/edit/master/backends/README.md
-->

# Metrics long term archiving

> ⚠️ The backends system is now deprecated in favor of the [exporting engine](/exporting/README.md).

Netdata supports backends for archiving the metrics, or providing long term dashboards, using Grafana or other tools,
like this:

![image](https://cloud.githubusercontent.com/assets/2662304/20649711/29f182ba-b4ce-11e6-97c8-ab2c0ab59833.png)

Since Netdata collects thousands of metrics per server per second, which would easily congest any backend server when
several Netdata servers are sending data to it, Netdata allows sending metrics at a lower frequency, by resampling them.

So, although Netdata collects metrics every second, it can send to the backend servers averages or sums every X seconds
(though, it can send them per second if you need it to).

## features

1.  Supported backends

    -   **graphite** (`plaintext interface`, used by **Graphite**, **InfluxDB**, **KairosDB**, **Blueflood**,
        **ElasticSearch** via logstash tcp input and the graphite codec, etc)

        metrics are sent to the backend server as `prefix.hostname.chart.dimension`. `prefix` is configured below,
        `hostname` is the hostname of the machine (can also be configured).

    -   **opentsdb** (`telnet or HTTP interfaces`, used by **OpenTSDB**, **InfluxDB**, **KairosDB**, etc)

        metrics are sent to opentsdb as `prefix.chart.dimension` with tag `host=hostname`.

    -   **json** document DBs

        metrics are sent to a document db, `JSON` formatted.

    -   **prometheus** is described at [prometheus page](/backends/prometheus/README.md) since it pulls data from
        Netdata.

    -   **prometheus remote write** (a binary snappy-compressed protocol buffer encoding over HTTP used by
        **Elasticsearch**, **Gnocchi**, **Graphite**, **InfluxDB**, **Kafka**, **OpenTSDB**, **PostgreSQL/TimescaleDB**,
        **Splunk**, **VictoriaMetrics**, and a lot of other [storage
        providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage))

        metrics are labeled in the format, which is used by Netdata for the [plaintext prometheus
        protocol](/backends/prometheus/README.md). Notes on using the remote write backend are [here](/backends/prometheus/remote_write/README.md).

    -   **TimescaleDB** via [community-built connector](/backends/TIMESCALE.md) that takes JSON streams from a Netdata
        client and writes them to a TimescaleDB table.

    -   **AWS Kinesis Data Streams**

        metrics are sent to the service in `JSON` format.

    -   **MongoDB**

        metrics are sent to the database in `JSON` format.

2.  Only one backend may be active at a time.

3.  Netdata can filter metrics (at the chart level), to send only a subset of the collected metrics.

4.  Netdata supports three modes of operation for all backends:

    -   `as-collected` sends to backends the metrics as they are collected, in the units they are collected. So,
        counters are sent as counters and gauges are sent as gauges, much like all data collectors do. For example, to
        calculate CPU utilization in this format, you need to know how to convert kernel ticks to percentage.

    -   `average` sends to backends normalized metrics from the Netdata database. In this mode, all metrics are sent as
        gauges, in the units Netdata uses. This abstracts data collection and simplifies visualization, but you will not
        be able to copy and paste queries from other sources to convert units. For example, CPU utilization percentage
        is calculated by Netdata, so Netdata will convert ticks to percentage and send the average percentage to the
        backend.

    -   `sum` or `volume`: the sum of the interpolated values shown on the Netdata graphs is sent to the backend. So, if
        Netdata is configured to send data to the backend every 10 seconds, the sum of the 10 values shown on the
        Netdata charts will be used.

    Time-series databases suggest to collect the raw values (`as-collected`). If you plan to invest on building your
    monitoring around a time-series database and you already know (or you will invest in learning) how to convert units
    and normalize the metrics in Grafana or other visualization tools, we suggest to use `as-collected`.

    If, on the other hand, you just need long term archiving of Netdata metrics and you plan to mainly work with
    Netdata, we suggest to use `average`. It decouples visualization from data collection, so it will generally be a lot
    simpler. Furthermore, if you use `average`, the charts shown in the back-end will match exactly what you see in
    Netdata, which is not necessarily true for the other modes of operation.

5.  This code is smart enough, not to slow down Netdata, independently of the speed of the backend server.

## configuration

In `/etc/netdata/netdata.conf` you should have something like this (if not download the latest version of `netdata.conf`
from your Netdata):

```conf
[backend]
    enabled = yes | no
    type = graphite | opentsdb:telnet | opentsdb:http | opentsdb:https | prometheus_remote_write | json | kinesis | mongodb
    host tags = list of TAG=VALUE
    destination = space separated list of [PROTOCOL:]HOST[:PORT] - the first working will be used, or a region for kinesis
    data source = average | sum | as collected
    prefix = Netdata
    hostname = my-name
    update every = 10
    buffer on failures = 10
    timeout ms = 20000
    send charts matching = *
    send hosts matching = localhost *
    send names instead of ids = yes
```

-   `enabled = yes | no`, enables or disables sending data to a backend

-   `type = graphite | opentsdb:telnet | opentsdb:http | opentsdb:https | json | kinesis | mongodb`, selects the backend
    type

-   `destination = host1 host2 host3 ...`, accepts **a space separated list** of hostnames, IPs (IPv4 and IPv6) and
     ports to connect to. Netdata will use the **first available** to send the metrics.

     The format of each item in this list, is: `[PROTOCOL:]IP[:PORT]`.

     `PROTOCOL` can be `udp` or `tcp`. `tcp` is the default and only supported by the current backends.

     `IP` can be `XX.XX.XX.XX` (IPv4), or `[XX:XX...XX:XX]` (IPv6). For IPv6 you can to enclose the IP in `[]` to
     separate it from the port.

     `PORT` can be a number of a service name. If omitted, the default port for the backend will be used
     (graphite = 2003, opentsdb = 4242).

     Example IPv4:

```conf
   destination = 10.11.14.2:4242 10.11.14.3:4242 10.11.14.4:4242
```

   Example IPv6 and IPv4 together:

```conf
   destination = [ffff:...:0001]:2003 10.11.12.1:2003
```

   When multiple servers are defined, Netdata will try the next one when the first one fails. This allows you to
   load-balance different servers: give your backend servers in different order on each Netdata.

   Netdata also ships `nc-backend.sh`, a script that can be used as a fallback backend to save the
   metrics to disk and push them to the time-series database when it becomes available again. It can also be used to
   monitor / trace / debug the metrics Netdata generates.

   For kinesis backend `destination` should be set to an AWS region (for example, `us-east-1`).

   The MongoDB backend doesn't use the `destination` option for its configuration. It uses the `mongodb.conf`
   [configuration file](/backends/mongodb/README.md) instead.

-   `data source = as collected`, or `data source = average`, or `data source = sum`, selects the kind of data that will
     be sent to the backend.

-   `hostname = my-name`, is the hostname to be used for sending data to the backend server. By default this is
     `[global].hostname`.

-   `prefix = Netdata`, is the prefix to add to all metrics.

-   `update every = 10`, is the number of seconds between sending data to the backend. Netdata will add some randomness
     to this number, to prevent stressing the backend server when many Netdata servers send data to the same backend.
     This randomness does not affect the quality of the data, only the time they are sent.

-   `buffer on failures = 10`, is the number of iterations (each iteration is `[backend].update every` seconds) to
     buffer data, when the backend is not available. If the backend fails to receive the data after that many failures,
     data loss on the backend is expected (Netdata will also log it).

-   `timeout ms = 20000`, is the timeout in milliseconds to wait for the backend server to process the data. By default
     this is `2 * update_every * 1000`.

-   `send hosts matching = localhost *` includes one or more space separated patterns, using `*` as wildcard (any number
     of times within each pattern). The patterns are checked against the hostname (the localhost is always checked as
     `localhost`), allowing us to filter which hosts will be sent to the backend when this Netdata is a central Netdata
     aggregating multiple hosts. A pattern starting with `!` gives a negative match. So to match all hosts named `*db*`
     except hosts containing `*child*`, use `!*child* *db*` (so, the order is important: the first pattern
     matching the hostname will be used - positive or negative).

-   `send charts matching = *` includes one or more space separated patterns, using `*` as wildcard (any number of times
     within each pattern). The patterns are checked against both chart id and chart name. A pattern starting with `!`
     gives a negative match. So to match all charts named `apps.*` except charts ending in `*reads`, use `!*reads
     apps.*` (so, the order is important: the first pattern matching the chart id or the chart name will be used -
     positive or negative).

-   `send names instead of ids = yes | no` controls the metric names Netdata should send to backend. Netdata supports
     names and IDs for charts and dimensions. Usually IDs are unique identifiers as read by the system and names are
     human friendly labels (also unique). Most charts and metrics have the same ID and name, but in several cases they
     are different: disks with device-mapper, interrupts, QoS classes, statsd synthetic charts, etc.

-   `host tags = list of TAG=VALUE` defines tags that should be appended on all metrics for the given host. These are
     currently only sent to graphite, json, opentsdb and prometheus. Please use the appropriate format for each
     time-series db. For example opentsdb likes them like `TAG1=VALUE1 TAG2=VALUE2`, but prometheus like `tag1="value1",
     tag2="value2"`. Host tags are mirrored with database replication (streaming of metrics between Netdata servers).

     Starting from Netdata v1.20 the host tags are parsed in accordance with a configured backend type and stored as
     host labels so that they can be reused in API responses and exporting connectors. The parsing is supported for
     graphite, json, opentsdb, and prometheus (default) backend types. You can check how the host tags were parsed using
     the /api/v1/info API call.

## monitoring operation

Netdata provides 5 charts:

1.  **Buffered metrics**, the number of metrics Netdata added to the buffer for dispatching them to the
    backend server.

2.  **Buffered data size**, the amount of data (in KB) Netdata added the buffer.

3.  ~~**Backend latency**, the time the backend server needed to process the data Netdata sent. If there was a
    re-connection involved, this includes the connection time.~~ (this chart has been removed, because it only measures
    the time Netdata needs to give the data to the O/S - since the backend servers do not ack the reception, Netdata
    does not have any means to measure this properly).

4.  **Backend operations**, the number of operations performed by Netdata.

5.  **Backend thread CPU usage**, the CPU resources consumed by the Netdata thread, that is responsible for sending the
    metrics to the backend server.

![image](https://cloud.githubusercontent.com/assets/2662304/20463536/eb196084-af3d-11e6-8ee5-ddbd3b4d8449.png)

## alarms

Netdata adds 4 alarms:

1.  `backend_last_buffering`, number of seconds since the last successful buffering of backend data
2.  `backend_metrics_sent`, percentage of metrics sent to the backend server
3.  `backend_metrics_lost`, number of metrics lost due to repeating failures to contact the backend server
4.  ~~`backend_slow`, the percentage of time between iterations needed by the backend time to process the data sent by
    Netdata~~ (this was misleading and has been removed).

![image](https://cloud.githubusercontent.com/assets/2662304/20463779/a46ed1c2-af43-11e6-91a5-07ca4533cac3.png)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fbackends%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
