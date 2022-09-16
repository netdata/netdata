<!--
title: "Netdata Longer Metrics Retention"
description: ""
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/longer-metrics-storage.md
-->

# Netdata Longer Metrics Retention

Metrics retention affects 3 parameters on the operation of a Netdata Agent:

1. The disk space required to store the metrics.
2. The memory the Netdata Agent will require to have that retention available for queries.
3. The CPU resources that will be required to query longer time-frames.

As retention increases, the resources required to support that retention increase too.

Since Netdata Agents usually run at the edge, inside production systems, Netdata Agent **parents** should be considered. When having a **parent - child** setup, the child (the Netdata Agent running on a production system) delegates all its functions, including longer metrics retention and querying, to the parent node that can dedicate more resources to this task. A single Netdata Agent parent can centralize multiple children Netdata Agents (dozens, hundreds, or even thousands depending on its available resources). 


## Ephemerality of metrics

The ephemerality of metrics plays an important role in retention. In environments where metrics stop being collected and new metrics are constantly being generated, we are interested about 2 parameters:

1. The **expected concurrent number of metrics** as an average for the lifetime of the database.
   This affects mainly the storage requirements.

2. The **expected total number of unique metrics** for the lifetime of the database.
   This affects mainly the memory requirements for having all these metrics indexed and available to be queried.

## Granularity of metrics

The granularity of metrics (the frequency they are collected and stored, i.e. their resolution) is significantly affecting retention.

Lowering the granularity from per second to every two seconds, will double their retention and half the CPU requirements of the Netdata Agent, without affecting disk space or memory requirements.

## Which database mode to use

Netdata Agents support multiple database modes.

The default mode `[db].mode = dbengine` has been designed to scale for longer retentions.

The other available database modes are designed to minimize resource utilization and should usually be considered on **parent - child** setups at the children side.

So,

* On a single node setup, use `[db].mode = dbengine` to increase retention.
* On a **parent - child** setup, use `[db].mode = dbengine` on the parent to increase retention and a more resource efficient mode (like `save`, `ram` or `none`) for the child to minimize resources utilization.

To use `dbengine`, set this in `netdata.conf` (it is the default):

```
[db]
    mode = dbengine
```

## Tiering

`dbengine` supports tiering. Tiering allows having up to 3 versions of the data:

1. Tier 0 is the high resolution data.
2. Tier 1 is the first tier that samples data every 60 data collections of Tier 0.
3. Tier 2 is the second tier that samples data every 3600 data collections of Tier 0 (60 of Tier 1).

To enable tiering set `[db].storage tiers` in `netdata.conf` (the default is 1, to enable only Tier 0):

```
[db]
    mode = dbengine
    storage tiers = 3
```

## Disk space requirements

Netdata Agents require about 1 bytes on disk per database point on Tier 0 and 4 times more on higher tiers (Tier 1 and 2). They require 4 times more storage per point compared to Tier 0, because for every point higher tiers store `min`, `max`, `sum`, `count` and `anomaly rate` (the values are 5, but they require 4 times the storage because `count` and `anomaly rate` are 16-bit integers). The `average` is calculated on the fly at query time using `sum / count`.

### Tier 0 - per second for a week

For 2000 metrics, collected every second and retained for a week, Tier 0 needs: 1 byte x 2000 metrics x 3600 secs per hour x 24 hours per day x 7 days per week = 1100MB.

The setting to control this is in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0
    storage tiers = 1
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 1100
```

By setting it to `1100` and restarting the Netdata Agent, this node will start maintaining about a week of data. But pay attention to the number of metrics. If you have more than 2000 metrics on a node, or you need more that a week of high resolution metrics, you may need to adjust this setting accordingly.

### Tier 1 - per minute for a month

Tier 1 is by default sampling the data every 60 points of Tier 0. If Tier 0 is per second, then Tier 1 is per minute.

Tier 1 needs 4 times more storage per point compared to Tier 0. So, for 2000 metrics, with per minute resolution, retained for a month, Tier 1 needs: 4 bytes x 2000 metrics x 60 minutes per hour x 24 hours per day x 30 days per month = 330MB.

Do this in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0 and Tier 1
    storage tiers = 2
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 1100
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 330
```

Once `netdata.conf` is edited, the Netdata Agent needs to be restarted for the changes to take effect.

### Tier 2 - per hour for a year

Tier 2 is by default sampling data every 3600 points of Tier 0 (60 of Tier 1). If Tier 0 is per second, then Tier 2 is per hour.

The storage requirements are the same to Tier 1.

For 2000 metrics, with per hour resolution, retained for a year, Tier 2 needs: 4 bytes x 2000 metrics x 24 hours per day x 365 days per year = 67MB.

Do this in `netdata.conf`:

```
[db]
    mode = dbengine
    
    # per second data collection
    update every = 1
    
    # enable only Tier 0 and Tier 1
    storage tiers = 3
    
    # Tier 0, per second data for a week
    dbengine multihost disk space MB = 1100
    
    # Tier 1, per minute data for a month
    dbengine tier 1 multihost disk space MB = 330

    # Tier 2, per hour data for a year
    dbengine tier 2 multihost disk space MB = 67
```

Once `netdata.conf` is edited, the Netdata Agent needs to be restarted for the changes to take effect.



