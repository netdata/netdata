<!--
title: "Change how long Netdata stores metrics"
description: "With a single configuration change, the Netdata Agent can store days, weeks, or months of metrics at its famous per-second granularity."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/store/change-metrics-storage.md"
sidebar_label: "Change how long Netdata stores metrics"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Configuration"
-->

# Change how long Netdata stores metrics

The Netdata Agent uses a custom made time-series database (TSDB), named the 
[`dbengine`](https://github.com/netdata/netdata/blob/master/database/engine/README.md), to store metrics.

To increase or decrease the metric retention time, you just [configure](#configure-metric-retention) 
the number of storage tiers and the space allocated to each one. The effect of these two parameters 
on the maximum retention and the memory used by Netdata is described in detail, below. 

## Calculate the system resources (RAM, disk space) needed to store metrics

### Disk space allocated to each tier

3 tiers are enabled by default in Netdata, with the following configuration:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # number of tiers used (1 to 5, 3 being default)
    storage tiers = 3
    
    # Tier 0, per second data
    dbengine multihost disk space MB = 256
    
    # Tier 1, per minute data
    dbengine tier 1 multihost disk space MB = 128
    dbengine tier 1 update every iterations = 60
    
    # Tier 2, per hour data
    dbengine tier 2 multihost disk space MB = 64
    dbengine tier 2 update every iterations = 60
```

The default "update every iterations" of 60 means that if a metric is collected per second in Tier 0, then
we will have a data point every minute in tier 1 and every minute in tier 2. 

Up to 5 tiers are supported. You may add, or remove tiers and/or modify these multipliers, as long as the 
product of all the "update every iterations" does not exceed 65535 (number of points for each tier0 point).

e.g. If you simply add a fourth tier by setting `storage tiers = 4` and defining the disk space for the new tier, 
the product of the "update every iterations" will be 60 * 60 * 60 = 216,000, which is > 65535. So you'd need to reduce  
the `update every iterations` of the tiers, to stay under the limit.

The exact retention that can be achieved by each tier depends on the number of metrics collected. The more 
the metrics, the smaller the retention that will fit in a given size. The general rule is that Netdata needs 
about **1 byte per data point on disk for tier 0**, and **4 bytes per data point on disk for tier 1 and above**.

So, for 1000 metrics collected per second and 256 MB for tier 0, Netdata will store about:

```
256MB on disk / 1 byte per point / 1000 metrics => 256k points per metric / 86400 seconds per day = about 3 days
```

At tier 1 (per minute):

```
128MB on disk / 4 bytes per point / 1000 metrics => 32k points per metric / (24 hours * 60 minutes) = about 22 days
```

At tier 2 (per hour):

```
64MB on disk / 4 bytes per point / 1000 metrics => 16k points per metric / 24 hours per day = about 2 years 
```

Of course double the metrics, half the retention. There are more factors that affect retention. The number 
of ephemeral metrics (i.e. metrics that are collected for part of the time). The number of metrics that are 
usually constant over time (affecting compression efficiency). The number of restarts a Netdata Agents gets 
through time (because it has to break pages prematurely, increasing the metadata overhead). But the actual 
numbers should not deviate significantly from the above. 


### Memory for concurrently collected metrics

DBENGINE memory is related to the number of metrics concurrently being collected, the retention of the metrics 
on disk in relation with the queries running, and the number of metrics for which retention is maintained.

The precise analysis of how much memory will be used is described in 
[dbengine memory requirements](https://github.com/netdata/netdata/blob/master/database/engine/README.md#memory-requirements).

The quick rule of thumb for a high level estimation is

```
memory in KiB = METRICS x (TIERS - 1) x 4KiB x 2 + 32768 KiB
```

So, for 2000 metrics (dimensions) in 3 storage tiers:

```
memory for 2k metrics = 2000 x (3 - 1) x 4 KiB x 2 + 32768 KiB = 64 MiB
```

## Configure metric retention

Once you have decided how to size each tier, open `netdata.conf` with
[`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) 
and make your changes in the `[db]` subsection. 

Save the file and restart the Agent with `sudo systemctl restart netdata`, or
the [appropriate method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) 
for your system, to change the database engine's size.


