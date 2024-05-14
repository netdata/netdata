<!--
title: "Exporting reference"
description: "With the exporting engine, you can archive your Netdata metrics to multiple external databases for long-term storage or further analysis."
sidebar_label: "Export"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/exporting/README.md"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
learn_doc_purpose: "Explain the exporting engine options and all of our the exporting connectors options"
-->

# Exporting reference

Welcome to the exporting engine reference guide. This guide contains comprehensive information about enabling,
configuring, and monitoring Netdata's exporting engine, which allows you to send metrics to external time-series
databases.

For a quick introduction to the exporting engine's features, read our doc on [exporting metrics to time-series
databases](https://github.com/netdata/netdata/blob/master/docs/exporting-metrics/README.md), or jump in to [enabling a connector](https://github.com/netdata/netdata/blob/master/docs/exporting-metrics/enable-an-exporting-connector.md).

The exporting engine has a modular structure and supports metric exporting via multiple exporting connector instances at
the same time. You can have different update intervals and filters configured for every exporting connector instance. 

When you enable the exporting engine and a connector, the Netdata Agent exports metrics _beginning from the time you
restart its process_, not the entire [database of long-term metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md).

Since Netdata collects thousands of metrics per server per second, which would easily congest any database server when
several Netdata servers are sending data to it, Netdata allows sending metrics at a lower frequency, by resampling them.

So, although Netdata collects metrics every second, it can send to the external database servers averages or sums every
X seconds (though, it can send them per second if you need it to).

## Features

### Integration

The exporting engine uses a number of connectors to send Netdata metrics to external time-series databases. See our
[list of supported databases](https://github.com/netdata/netdata/blob/master/docs/exporting-metrics/README.md#supported-databases) for information on which
connector to enable and configure for your database of choice.

-   [**AWS Kinesis Data Streams**](https://github.com/netdata/netdata/blob/master/src/exporting/aws_kinesis/README.md): Metrics are sent to the service in `JSON`
    format.
-   [**Google Cloud Pub/Sub Service**](https://github.com/netdata/netdata/blob/master/src/exporting/pubsub/README.md): Metrics are sent to the service in `JSON`
    format.
-   [**Graphite**](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md): A plaintext interface. Metrics are sent to the database server as
    `prefix.hostname.chart.dimension`. `prefix` is configured below, `hostname` is the hostname of the machine (can
    also be configured). Learn more in our guide to [export and visualize Netdata metrics in
    Graphite](https://github.com/netdata/netdata/blob/master/src/exporting/graphite/README.md).
-   [**JSON** document databases](https://github.com/netdata/netdata/blob/master/src/exporting/json/README.md)
-   [**OpenTSDB**](https://github.com/netdata/netdata/blob/master/src/exporting/opentsdb/README.md): Use a plaintext or HTTP interfaces. Metrics are sent to
    OpenTSDB as `prefix.chart.dimension` with tag `host=hostname`.
-   [**MongoDB**](https://github.com/netdata/netdata/blob/master/src/exporting/mongodb/README.md): Metrics are sent to the database in `JSON` format.
-   [**Prometheus**](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/README.md): Use an existing Prometheus installation to scrape metrics
    from node using the Netdata API.
-   [**Prometheus remote write**](https://github.com/netdata/netdata/blob/master/src/exporting/prometheus/remote_write/README.md). A binary snappy-compressed protocol
    buffer encoding over HTTP. Supports many [storage
    providers](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).
-   [**TimescaleDB**](https://github.com/netdata/netdata/blob/master/src/exporting/TIMESCALE.md): Use a community-built connector that takes JSON streams from a
    Netdata client and writes them to a TimescaleDB table.

### Chart filtering

Netdata can filter metrics, to send only a subset of the collected metrics. You can use the
configuration file

```txt
[prometheus:exporter]
    send charts matching = system.*
```

or the URL parameter `filter` in the `allmetrics` API call.

```txt
http://localhost:19999/api/v1/allmetrics?format=shell&filter=system.*
```

### Operation modes

Netdata supports three modes of operation for all exporting connectors:

-   `as-collected` sends to external databases the metrics as they are collected, in the units they are collected.
    So, counters are sent as counters and gauges are sent as gauges, much like all data collectors do. For example,
    to calculate CPU utilization in this format, you need to know how to convert kernel ticks to percentage.

-   `average` sends to external databases normalized metrics from the Netdata database. In this mode, all metrics
    are sent as gauges, in the units Netdata uses. This abstracts data collection and simplifies visualization, but
    you will not be able to copy and paste queries from other sources to convert units. For example, CPU utilization
    percentage is calculated by Netdata, so Netdata will convert ticks to percentage and send the average percentage
    to the external database.

-   `sum` or `volume`: the sum of the interpolated values shown on the Netdata graphs is sent to the external
    database. So, if Netdata is configured to send data to the database every 10 seconds, the sum of the 10 values
    shown on the Netdata charts will be used.

Time-series databases suggest to collect the raw values (`as-collected`). If you plan to invest on building your
monitoring around a time-series database and you already know (or you will invest in learning) how to convert units
and normalize the metrics in Grafana or other visualization tools, we suggest to use `as-collected`.

If, on the other hand, you just need long term archiving of Netdata metrics and you plan to mainly work with
Netdata, we suggest to use `average`. It decouples visualization from data collection, so it will generally be a lot
simpler. Furthermore, if you use `average`, the charts shown in the external service will match exactly what you
see in Netdata, which is not necessarily true for the other modes of operation.

### Independent operation

This code is smart enough, not to slow down Netdata, independently of the speed of the external database server. 

> ❗ You should keep in mind though that many exporting connector instances can consume a lot of CPU resources if they
> run their batches at the same time. You can set different update intervals for every exporting connector instance,
> but even in that case they can occasionally synchronize their batches for a moment.

## Configuration

Here are the configuration blocks for every supported connector. Your current `exporting.conf` file may look a little
different. 

You can configure each connector individually using the available [options](#options). The
`[graphite:my_graphite_instance]` block contains examples of some of these additional options in action.

```conf
[exporting:global]
    enabled = yes
    send configured labels = no
    send automatic labels = no
    update every = 10

[prometheus:exporter]
    send names instead of ids = yes
    send configured labels = yes
    send automatic labels = no
    send charts matching = *
    send hosts matching = localhost *
    prefix = netdata

[graphite:my_graphite_instance]
    enabled = yes
    destination = localhost:2003
    data source = average
    prefix = Netdata
    hostname = my-name
    update every = 10
    buffer on failures = 10
    timeout ms = 20000
    send charts matching = *
    send hosts matching = localhost *
    send names instead of ids = yes
    send configured labels = yes
    send automatic labels = yes

[prometheus_remote_write:my_prometheus_remote_write_instance]
    enabled = yes
    destination = localhost
    remote write URL path = /receive

[kinesis:my_kinesis_instance]
    enabled = yes
    destination = us-east-1
    stream name = netdata
    aws_access_key_id = my_access_key_id
    aws_secret_access_key = my_aws_secret_access_key

[pubsub:my_pubsub_instance]
    enabled = yes
    destination = pubsub.googleapis.com
    credentials file = /etc/netdata/pubsub_credentials.json
    project id = my_project
    topic id = my_topic

[mongodb:my_mongodb_instance]
    enabled = yes
    destination = localhost
    database = my_database
    collection = my_collection

[json:my_json_instance]
    enabled = yes
    destination = localhost:5448

[opentsdb:my_opentsdb_plaintext_instance]
    enabled = yes
    destination = localhost:4242

[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
    username = my_username
    password = my_password

[opentsdb:https:my_opentsdb_https_instance]
    enabled = yes
    destination = localhost:8082
```

### Sections

-   `[exporting:global]` is a section where you can set your defaults for all exporting connectors
-   `[prometheus:exporter]` defines settings for Prometheus exporter API queries (e.g.:
    `http://NODE:19999/api/v1/allmetrics?format=prometheus&help=yes&source=as-collected`).
-   `[<type>:<name>]` keeps settings for a particular exporting connector instance, where:
  -   `type` selects the exporting connector type: graphite | opentsdb:telnet | opentsdb:http |
      prometheus_remote_write | json | kinesis | pubsub | mongodb. For graphite, opentsdb,
      json, and prometheus_remote_write connectors you can also use `:http` or `:https` modifiers
      (e.g.: `opentsdb:https`).
  -   `name` can be arbitrary instance name you chose.

### Options

Configure individual connectors and override any global settings with the following options.

-   `enabled = yes | no`, enables or disables an exporting connector instance

-   `destination = host1 host2 host3 ...`, accepts **a space separated list** of hostnames, IPs (IPv4 and IPv6) and
     ports to connect to. Netdata will use the **first available** to send the metrics.

     The format of each item in this list, is: `[PROTOCOL:]IP[:PORT]`.

     `PROTOCOL` can be `udp` or `tcp`. `tcp` is the default and only supported by the current exporting engine.

     `IP` can be `XX.XX.XX.XX` (IPv4), or `[XX:XX...XX:XX]` (IPv6). For IPv6 you can to enclose the IP in `[]` to
     separate it from the port.

     `PORT` can be a number of a service name. If omitted, the default port for the exporting connector will be used
     (graphite = 2003, opentsdb = 4242).

     Example IPv4:

```conf
   destination = 10.11.14.2:4242 10.11.14.3:4242 10.11.14.4:4242
```

   Example IPv6 and IPv4 together:

```conf
   destination = [ffff:...:0001]:2003 10.11.12.1:2003
```

   When multiple servers are defined, Netdata will try the next one when the previous one fails.

   Netdata also ships `nc-exporting.sh`, a script that can be used as a fallback exporting connector to save the
   metrics to disk and push them to the time-series database when it becomes available again. It can also be used to
   monitor / trace / debug the metrics Netdata generates.

   For the Kinesis exporting connector `destination` should be set to an AWS region (for example, `us-east-1`).

   For the MongoDB exporting connector `destination` should be set to a
   [MongoDB URI](https://docs.mongodb.com/manual/reference/connection-string/).

   For the Pub/Sub exporting connector `destination` can be set to a specific service endpoint.

-   `data source = as collected`, or `data source = average`, or `data source = sum`, selects the kind of data that will
     be sent to the external database.

-   `hostname = my-name`, is the hostname to be used for sending data to the external database server. By default this
    is `[global].hostname`.

-   `prefix = Netdata`, is the prefix to add to all metrics.

-   `update every = 10`, is the number of seconds between sending data to the external database. Netdata will add some
    randomness to this number, to prevent stressing the external server when many Netdata servers send data to the same
    database. This randomness does not affect the quality of the data, only the time they are sent.

-   `buffer on failures = 10`, is the number of iterations (each iteration is `update every` seconds) to buffer data,
    when the external database server is not available. If the server fails to receive the data after that many
    failures, data loss on the connector instance is expected (Netdata will also log it).

-   `timeout ms = 20000`, is the timeout in milliseconds to wait for the external database server to process the data.
    By default this is `2 * update_every * 1000`.

-   `send hosts matching = localhost *` includes one or more space separated patterns, using `*` as wildcard (any number
    of times within each pattern). The patterns are checked against the hostname (the localhost is always checked as
    `localhost`), allowing us to filter which hosts will be sent to the external database when this Netdata is a central
    Netdata aggregating multiple hosts. A pattern starting with `!` gives a negative match. So to match all hosts named
    `*db*` except hosts containing `*child*`, use `!*child* *db*` (so, the order is important: the first
    pattern matching the hostname will be used - positive or negative).

-   `send charts matching = *` includes one or more space separated patterns, using `*` as wildcard (any number of times
    within each pattern). The patterns are checked against both chart id and chart name. A pattern starting with `!`
    gives a negative match. So to match all charts named `apps.*` except charts ending in `*reads`, use `!*reads
    apps.*` (so, the order is important: the first pattern matching the chart id or the chart name will be used -
    positive or negative). There is also a URL parameter `filter` that can be used while querying `allmetrics`. The URL
    parameter has a higher priority than the configuration option.

-   `send names instead of ids = yes | no` controls the metric names Netdata should send to the external database.
    Netdata supports names and IDs for charts and dimensions. Usually IDs are unique identifiers as read by the system
    and names are human friendly labels (also unique). Most charts and metrics have the same ID and name, but in several
    cases they are different: disks with device-mapper, interrupts, QoS classes, statsd synthetic charts, etc.

-   `send configured labels = yes | no` controls if host labels defined in the `[host labels]` section in `netdata.conf`
    should be sent to the external database

-   `send automatic labels = yes | no` controls if automatically created labels, like `_os_name` or `_architecture`
    should be sent to the external database

## HTTPS

Netdata can send metrics to external databases using the TLS/SSL protocol. Unfortunately, some of
them does not support encrypted connections, so you will have to configure a reverse proxy to enable
HTTPS communication between Netdata and an external database. You can set up a reverse proxy with
[Nginx](https://github.com/netdata/netdata/blob/master/docs/Running-behind-nginx.md).

## Exporting engine monitoring

Netdata creates five charts in the dashboard, under the **Netdata Monitoring** section, to help you monitor the health
and performance of the exporting engine itself:

1.  **Buffered metrics**, the number of metrics Netdata added to the buffer for dispatching them to the
    external database server.

2.  **Exporting data size**, the amount of data (in KB) Netdata added the buffer.

3.  **Exporting operations**, the number of operations performed by Netdata.

4.  **Exporting thread CPU usage**, the CPU resources consumed by the Netdata thread, that is responsible for sending
    the metrics to the external database server.

![image](https://cloud.githubusercontent.com/assets/2662304/20463536/eb196084-af3d-11e6-8ee5-ddbd3b4d8449.png)

## Exporting engine alerts

Netdata adds 3 alerts:

1.  `exporting_last_buffering`, number of seconds since the last successful buffering of exported data
2.  `exporting_metrics_sent`, percentage of metrics sent to the external database server
3.  `exporting_metrics_lost`, number of metrics lost due to repeating failures to contact the external database server

![image](https://cloud.githubusercontent.com/assets/2662304/20463779/a46ed1c2-af43-11e6-91a5-07ca4533cac3.png)


