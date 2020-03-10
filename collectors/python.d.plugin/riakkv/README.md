<!--
---
title: "RiakKV monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/riakkv/README.md
---
-->

# RiakKV monitoring with Netdata

Collects database stats from `/stats` endpoint.

## Requirements

-   An accessible `/stats` endpoint. See [the Riak KV configuration reference documentation](https://docs.riak.com/riak/kv/2.2.3/configuring/reference/#client-interfaces)
    for how to enable this.

The following charts are included, which are mostly derived from the metrics
listed
[here](https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#riak-metrics-to-graph).

1.  **Throughput** in operations/s

-   **KV operations**
    -   gets
    -   puts

-   **Data type updates**
    -   counters
    -   sets
    -   maps

-   **Search queries**
    -   queries

-   **Search documents**
    -   indexed

-   **Strong consistency operations**
    -   gets
    -   puts

2.  **Latency** in milliseconds

-   **KV latency** of the past minute
    -   get (mean, median, 95th / 99th / 100th percentile)
    -   put (mean, median, 95th / 99th / 100th percentile)

-   **Data type latency** of the past minute
    -   counter_merge (mean, median, 95th / 99th / 100th percentile)
    -   set_merge (mean, median, 95th / 99th / 100th percentile)
    -   map_merge (mean, median, 95th / 99th / 100th percentile)

-   **Search latency** of the past minute
    -   query (median, min, max, 95th / 99th percentile)
    -   index (median, min, max, 95th / 99th percentile)

-   **Strong consistency latency** of the past minute
    -   get (mean, median, 95th / 99th / 100th percentile)
    -   put (mean, median, 95th / 99th / 100th percentile)

3.  **Erlang VM metrics**

-   **System counters**
    -   processes

-   **Memory allocation** in MB
    -   processes.allocated
    -   processes.used

4.  **General load / health metrics**

-   **Siblings encountered in KV operations** during the past minute
    -   get (mean, median, 95th / 99th / 100th percentile)

-   **Object size in KV operations** during the past minute in KB
    -   get (mean, median, 95th / 99th / 100th percentile)

-   **Message queue length** in unprocessed messages
    -   vnodeq_size (mean, median, 95th / 99th / 100th percentile)

-   **Index operations** encountered by Search
    -   errors

-   **Protocol buffer connections**
    -   active

-   **Repair operations coordinated by this node**
    -   read

-   **Active finite state machines by kind**
    -   get
    -   put
    -   secondary_index
    -   list_keys

-   **Rejected finite state machines**
    -   get
    -   put

-   **Number of writes to Search failed due to bad data format by reason**
    -   bad_entry
    -   extract_fail

## Configuration

Edit the `python.d/riakkv.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/riakkv.conf
```

The module needs to be passed the full URL to Riak's stats endpoint.
For example:

```yaml
myriak:
  url: http://myriak.example.com:8098/stats
```

With no explicit configuration given, the module will attempt to connect to
`http://localhost:8098/stats`.

The default update frequency for the plugin is set to 2 seconds as Riak
internally updates the metrics every second. If we were to update the metrics
every second, the resulting graph would contain odd jitter.
