<!--
title: "Metrics storage"
sidebar_label: "Metrics storage"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md"
sidebar_position: "1100"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata agent"
learn_docs_purpose: "Explain how the Agent can manage/retain the metrics it collects, where it stores them, how it stores them and deletion policies (innovations like the tiering mechanism)"
-->

Upon collection, collected metrics need to be either forwarded, exported, or just stored for further treatment. The
Agent is capable of storing metrics both short and long-term, with or without the usage of non-volatile storage.

## Agent database modes and their cases

The Agent support different modes of managing metrics to cover any deployment case.

```conf
[db]
  # dbengine (default), ram, save (the default if dbengine not available), map (swap like), none, alloc
  mode = dbengine
```

In a nutshell you can use:

1. `[db].mode = dbengine` [default mode]: Is the custom, tailored-made time series database. It works like a traditional
   database, and it's optimized for the Agent's operations. You can use this mode in any case that you want to store
   Agent related metrics to a designated area of non-volatile devices. These metrics can be either collected metrics on
   the spot or streamed/replicated metrics of other Agents.

2. `[db].mode = ram`: data are purely in memory as such they are never saved on disks. This mode uses `mmap()` and
   supports [KSM](#ksm). You can use this mode in cases where there are no non-volatile devices in your system or you
   don't want to use them. You can only make use of the last `[db].retention = <time_points>`. The impact on ram may
   vary, it depends on the metrics that are collected.

3. `[db].mode = save`, data are only in RAM while Netdata runs. On restart Agent flushes them into `[directories].cache`
   as unique files of any chart. On Agent's reboot it also loads the data from this directory. The metric retention
   equal to last the `[db].retention = <time_points>`. It uses `mmap()` and supports [KSM](#ksm). This is a legacy
   configuratio, you can use it in cases where you may want to back up partial chucks of metrics.

4. `[db].mode = map`, data are in memory mapped files. This works like the swap. When Netdata writes data on its memory,
   the Linux kernel marks the related memory pages as dirty and automatically starts updating them on disk during
   periodical `sync` flushes. Unfortunately we cannot control how frequently this works. The Linux kernel uses exactly
   the same algorithm it uses for its swap memory. This mode uses `mmap()` but does not support [KSM](#ksm). _Keep in
   mind though, this option will have a constant write on your disk._

5. `[db].mode = alloc`, like `ram` but it uses `calloc()` and does not support [KSM](#ksm). This mode is the fallback
   for all others except `none`.

6. `[db].mode = none`, without a database (collected metrics can only be streamed to another Netdata).

## Netdata's dbengine

In the `dbengine`'s implementation, the amount of historical metrics stored is based on the amount of disk space you
allocate and the effective compression ratio, not just on the fixed number of metrics collected. It allocates a certain
amount of ram ( subject to `[db].dbengine page cache size MB`) and stores them into the allocated space for each tier of
data collected.

### Tiering

Tiering is a mechanism of providing multiple tiers of data with different granularity of metrics (the frequency they are
collected and stored, i.e. their resolution), significantly affecting retention. Every Tier down samples the exact
lower tier (lower tiers have greater resolution). You can have up to 5 Tiers [0. . 4] of data (including the Tier 0,
which has the highest resolution)

Lowering the granularity from per second to every two seconds, will double their retention and half the CPU requirements
of the Netdata Agent, without affecting disk space or memory requirements.

The `dbengine` is capable of retaining metrics for years. To further understand the `dbengine` tiering mechanism let's
explore the following configuration.

```conf
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enables Tier 1 and Tier 2, Tier 0 is always enabled in dbengine mode
    storage tiers = 3
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 1100
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 330

    # Tier 2, per hour data for a year
    dbengine tier 2 multihost disk space MB = 67
```

For 2000 metrics, collected every second and retained for a week, Tier 0 needs: 1 byte x 2000 metrics x 3600 secs per
hour x 24 hours per day x 7 days per week = 1100MB.

By setting `dbengine multihost disk space MB` to `1100`, this node will start maintaining about a week of data. But pay
attention to the number of metrics. If you have more than 2000 metrics on a node, or you need more that a week of high
resolution metrics, you may need to adjust this setting accordingly.

Tier 1 is by default sampling the data every **60 points of Tier 0**. In our case, Tier 0 is per second, if we want to
transform this information in terms of time then the Tier 1 "resolution" is per minute.

Tier 1 needs four times more storage per point compared to Tier 0. So, for 2000 metrics, with per minute resolution,
retained for a month, Tier 1 needs: 4 bytes x 2000 metrics x 60 minutes per hour x 24 hours per day x 30 days per month
= 330MB.

Tier 2 is by default sampling data every 3600 points of Tier 0 (60 of Tier 1, which is the previous exact Tier). Again
in term of "time" (Tier 0 is per second), then Tier 2 is per hour.

The storage requirements are the same to Tier 1.

For 2000 metrics, with per hour resolution, retained for a year, Tier 2 needs: 4 bytes x 2000 metrics x 24 hours per day
x 365 days per year = 67MB.

:::caution

Every storage disk space option contains the keyword `multihost`, this is due to the fact that this space is allocated 
for any data this Agent stores, including data of other Agent's that are streamed to it.

:::

### Metric size


Every Tier down samples the exact lower tier (lower tiers have greater resolution). You can have up to 5
Tiers **[0. . 4]** of data (including the Tier 0, which has the highest resolution)

Tier 0 is the default that was always available in `dbengine` mode. Tier 1 is the first level of aggregation, Tier 2 is
the second, and so on.

Metrics on all tiers except of the _Tier 0_ also store the following five additional values for every point for accurate
representation:

1. The `sum` of the points aggregated
2. The `min` of the points aggregated
3. The `max` of the points aggregated
4. The `count` of the points aggregated (could be constant, but it may not be due to gaps in data collection)
5. The `anomaly_count` of the points aggregated (how many of the aggregated points found anomalous)

Among `min`, `max` and `sum`, the correct value is chosen based on the user query. `average` is calculated on the fly at
query time.


### Storage files

With the DB engine mode the metric data are stored in database files. These files are organized in pairs, the datafiles
and their corresponding journalfiles, e.g.:

```sh
datafile-1-0000000001.ndf
journalfile-1-0000000001.njf
datafile-1-0000000002.ndf
journalfile-1-0000000002.njf
datafile-1-0000000003.ndf
journalfile-1-0000000003.njf
...
```

They are located under their host's cache directory in the directory `./dbengine` (e.g. for localhost the default
location is `/var/cache/netdata/dbengine/*`). The higher numbered filenames contain more recent metric data. The user
can safely delete some pairs of files when Netdata is stopped to manually free up some space.

_Users should_ **back up** _their `./dbengine` folders if they consider this data to be important._

### Related Concepts

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [Registry](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/registry.md)
- [Metrics streaming/replication](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
- [Metrics exporting](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-exporting.md)
- [Metrics collection](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md)


## Related Tasks

- [Claim existing Agent deployments](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
