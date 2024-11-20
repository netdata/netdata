# Bandwidth Requirements

## Production Systems: Standalone Netdata

Standalone Netdata may use network bandwidth under the following conditions:

1. You configured data collection jobs that are fetching data from remote systems. There are no such jobs enabled by default.
2. You use the dashboard of the Netdata.
3. [Netdata Cloud communication](#netdata-cloud-communication) (see below).

## Metrics Centralization Points: Between Netdata Children & Parents

Netdata supports multiple compression algorithms for streaming communication. Netdata Children offer all their compression algorithms when connecting to a Netdata Parent, and the Netdata Parent decides which one to use based on algorithm availability and user configuration.

| Algorithm |                                                              Best for                                                               |
|:---------:|:-----------------------------------------------------------------------------------------------------------------------------------:|
|  `zstd`   |                      The best balance between CPU utilization and compression efficiency. This is the default.                      |
|   `lz4`   |                   The fastest of the algorithms. Use this when CPU utilization is more important than bandwidth.                    |
|  `gzip`   | The best compression efficiency, at the expense of CPU utilization. Use this when bandwidth is more important than CPU utilization. |
| `brotli`  |                                  The most CPU intensive algorithm, providing the best compression.                                  |

The expected bandwidth consumption using `zstd` for 1 million samples per second is 84 Mbps, or 10.5 MiB/s.

The order compression algorithms is selected is configured in `stream.conf`, per `[API KEY]`, like this:

```text
    compression algorithms order = zstd lz4 brotli gzip
```

The first available algorithm on both the Netdata Child and the Netdata Parent, from left to right, is chosen.

Compression can also be disabled in `stream.conf` at either Netdata Children or Netdata Parents.

## Netdata Cloud Communication

When Netdata Agents connect to Netdata Cloud, they communicate metadata of the metrics being collected, but they do not stream the samples collected for each metric.

The information transferred to Netdata Cloud is:

1. Information and **metadata about the system itself**, like its hostname, architecture, virtualization technologies used and generally labels associated with the system.
2. Information about the **running data collection plugins, modules and jobs**.
3. Information about the **metrics available and their retention**.
4. Information about the **configured alerts and their transitions**.

This is not a constant stream of information. Netdata Agents update Netdata Cloud only about status changes on all the above (e.g., an alert being triggered, or a metric stopped being collected). So, there is an initial handshake and exchange of information when Netdata starts, and then there only updates when required.

Of course, when you view Netdata Cloud dashboards that need to query the database a Netdata Agent maintains, this query is forwarded to an Agent that can satisfy it. This means that Netdata Cloud receives metric samples only when a user is accessing a dashboard and the samples transferred are usually aggregations to allow rendering the dashboards.  
