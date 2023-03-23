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

The total memory Netdata uses is heavily influenced by the memory consumed by the DBENGINE.
The DBENGINE memory is related to the number of metrics concurrently being collected, the retention of the metrics 
on disk in relation with the queries running, and the number of metrics for which retention is maintained.

The precise analysis of how much memory will be used by the DBENGINE itself is described in 
[DBENGINE memory requirements](https://github.com/netdata/netdata/blob/master/database/engine/README.md#memory-requirements).

In addition to the DBENGINE, Netdata uses memory for contexts, metric labels (e.g. in a Kubernetes setup), 
other Netdata structures/processes (e.g. Health) and system overhead.

The quick rule of thumb, for a high level estimation is

```
DBENGINE memory in MiB = METRICS x (TIERS - 1) x 8 / 1024 MiB
Total Netdata memory in MiB = Metric ephemerality factor x DBENGINE memory in MiB + "dbengine page cache size MB" from netdata.conf
```

You can get the currently collected **METRICS** from the "dbengine metrics" chart of the Netdata dashboard. You just need to divide the 
value of the "collected" dimension with the number of tiers. For example, at the specific point highlighted in the chart below, 608k metrics 
were being collected across all 3 tiers, which means that `METRICS = 608k / 3 = 203667`. 

<img width="988" alt="image" src="https://user-images.githubusercontent.com/43294513/225335899-a9216ba7-a09e-469e-89f6-4690aada69a4.png" />


The **ephemerality factor** is usually between 3 or 4 and depends on how frequently the identifiers of the collected metrics change, increasing their
cardinality. The more ephemeral the infrastructure, the more short-lived metrics you have, increasing the ephemerality factor. If the metric cardinality is 
extremely high due for example to a lot of extremely short lived containers (hundreds started every minute), the ephemerality factor can be much higher than 4.
In such cases, we recommend splitting the load across multiple Netdata parents, until we can provide a way to lower the metric cardinality, 
by aggregating similar metrics.

#### Small agent RAM usage

For 2000 metrics (dimensions) in 3 storage tiers and the default cache size:

```
DBENGINE memory for 2k metrics = 2000 x (3 - 1) x 8 / 1024 MiB = 32 MiB
dbengine page cache size MB = 32 MiB 
Total Netdata memory in MiB = 3*32 + 32 = 128 MiB (low ephemerality)
```

#### Large parent RAM usage

The Netdata parent in our production infrastructure at the time of writing:
 - Collects 206k metrics per second, most from children streaming data
 - The metrics include moderately ephemeral Kubernetes containers, leading to an ephemerality factor of about 4
 - 3 tiers are used for retention
 - The `dbengine page cache size MB` in `netdata.conf` is configured to be 4GB

The rule of thumb calculation for this set up gives us
```
DBENGINE memory = 206,000 x 16 / 1024 MiB = 3,217 MiB = about 3 GiB
Extra cache = 4 GiB
Metric ephemerality factor = 4
Estimated total Netdata memory = 3 * 4 + 4 = 16 GiB
```

The actual measurement during a low usage time was the following:

Purpose|RAM|Note
:--- | ---: | :--- 
DBENGINE usage | 5.9 GiB | Out of 7GB max 
Cardinality/ephemerality related memory (k8s contexts, labels, strings) | 3.4 GiB
Buffer for queries | 0 GiB | Out of 0.5 GiB max, when heavily queried
Other | 0.5 GiB | 
System overhead | 4.4 GiB | Calculated by subtracting all of the above from the total 
**Total Netdata memory usage** | 14.2 GiB | 

All the figures above except for the system memory management overhead were retrieved from Netdata itself. 
The overhead can't be directly calculated, so we subtracted all the other figures from the total Netdata memory usage to get it. 
This overhead is usually around 50% of the memory actually useable by Netdata, but could range from 20% in small 
setups, all the way to 100% in some edge cases. 

## Configure metric retention

Once you have decided how to size each tier, open `netdata.conf` with
[`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) 
and make your changes in the `[db]` subsection. 

Save the file and restart the Agent with `sudo systemctl restart netdata`, or
the [appropriate method](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) 
for your system, to change the database engine's size.

