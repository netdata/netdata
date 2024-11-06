# RAM Utilization

Using the default [Database Tier configuration](/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md), Netdata needs about 16KiB per unique metric collected, independently of the data collection frequency.

## Children

Netdata by default should need 100MB to 200MB of RAM, depending on the number of metrics being collected.

This number can be lowered by limiting the number of database tier or switching database modes. For more information, check [Disk Requirements and Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

## Parents

|Description|Scope|RAM Required|Notes|
|:---|:---:|:---:|:---|
|metrics with retention|time-series in the db|1 KiB|Metadata and indexes.
|metrics currently collected|time-series collected|20 KiB|16 KiB for db + 4 KiB for collection structures.
|metrics with machine learning models|time-series collected|5 KiB|The trained models per dimension.
|nodes with retention|nodes in the db|10 KiB|Metadata and indexes.
|nodes currently received|nodes collected|512 KiB|Structures and reception buffers.
|nodes currently sent|nodes collected|512 KiB|Structures and dispatch buffers.

These numbers vary depending on name length, the number of dimensions per instance and per context, the number and length of the labels added, the number of machine learning models maintained and similar parameters. For most use cases, they represent the worst case scenario, so you may find out Netdata actually needs less than that.

Each metric currently being collected needs (1 index + 20 collection + 5 ml) = 26 KiB.  When it stops being collected it needs 1 KiB (index).

Each node currently being collected needs (10 index + 512 reception + 512 dispatch) = 1034 KiB. When it stops being collected it needs 10 KiB (index).

### Example

A Netdata Parents cluster (2 nodes) has 1 million currently collected metrics from 500 nodes, and 10 million archived metrics from 5000 nodes:

|Description|Entries|RAM per Entry|Total RAM|
|:---|:---:|:---:|---:|
|metrics with retention|11 million|1 KiB|10742 MiB|
|metrics currently collected|1 million|20 KiB|19531 MiB|
|metrics with machine learning models|1 million|5 KiB|4883 MiB|
|nodes with retention|5500|10 KiB|52 MiB|
|nodes currently received|500|512 KiB|256 MiB|
|nodes currently sent|500|512 KiB|256 MiB|
|**Memory required per node**|||**35.7 GiB**|

On highly volatile environments (like Kubernetes clusters), the database retention can significantly affect memory usage. Usually reducing retention on higher database tiers helps reducing memory usage.

## Database Size

Netdata supports memory ballooning to automatically adjust its database memory size based on the number of time-series concurrently being collected.

The general formula, with the default configuration of database tiers, is:

```text
memory = UNIQUE_METRICS x 16KiB + CONFIGURED_CACHES
```

The default `CONFIGURED_CACHES` is 32MiB.

For **one million concurrently collected time-series** (independently of their data collection frequency), **the required memory is 16 GiB**. In detail:

```text
UNIQUE_METRICS = 1000000
CONFIGURED_CACHES = 32MiB

(UNIQUE_METRICS * 16KiB / 1024 in MiB) + CONFIGURED_CACHES =
( 1000000       * 16KiB / 1024 in MiB) + 32 MiB            =
15657 MiB =
about 16 GiB
```

## Parents that also act as `systemd-journal` Logs centralization points

Logs usually require significantly more disk space and I/O bandwidth than metrics. For optimal performance, we recommend to store metrics and logs on separate, independent disks.

Netdata uses direct-I/O for its database, in order to not pollute the system caches with its own data.

To optimize disk I/O, Netdata maintains its own private caches. The default settings of these caches are automatically adjusted to the minimum required size for acceptable metrics query performance.

`systemd-journal` on the other hand, relies on operating system caches for improving the query performance of logs. When the system lacks free memory, querying logs leads to increased disk I/O.

If you are experiencing slow responses and increased disk reads when metrics queries run, we suggest dedicating some more RAM to Netdata.

We frequently see that the following strategy gives the best results:

1. Start the Netdata Parent, send all the load you expect it to have and let it stabilize for a few hours. Netdata will now use the minimum memory it believes is required for smooth operation.
2. Check the available system memory.
3. Set the page cache in `netdata.conf` to use 1/3 of the available memory.

This will allow Netdata queries to have more caches, while leaving plenty of available memory of logs and the operating system.
