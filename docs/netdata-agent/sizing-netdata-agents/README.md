# Sizing Netdata Agents

Netdata automatically adjusts its resource utilization based on its workload.

The resource needs of Netdata's features are as follows:

|                 Feature | CPU | RAM | Disk I/O | Disk Space | Retention | Bandwidth |
|------------------------:|:---:|:---:|:--------:|:----------:|:---------:|:---------:|
|       Collected metrics |  X  |  X  |    X     |     X      |     X     |     -     |
|        Sample frequency |  X  |  -  |    X     |     X      |     X     |     -     |
| Database mode and tiers |  -  |  X  |    X     |     X      |     X     |     -     |
|        Machine learning |  X  |  X  |    -     |     -      |     -     |     -     |
|               Streaming |  X  |  X  |    -     |     -      |     -     |     X     |

1. **Collected metrics**

   The number of collected metrics affects almost every aspect of the resources utilization.

   This is the first point to consider restricting when you need to lower the resources used by Netdata.

2. **Sample frequency**

   By default Netdata collects most metrics with 1-second granularity.

   Lowering the data collection frequency from every-second to every-2-seconds, will make Netdata use half the CPU resources. CPU utilization is analogous to the data collection frequency.

3. **Database Mode and Tiers**

   By default Netdata stores metrics in 3 database tiers. They are updated in parallel during data collection, and depending on the query duration, Netdata may consult one or more tiers to optimize the resources required to satisfy it.

   The number of database tiers affects the memory requirements of Netdata. Going from 3 tiers to 1 tier, will make Netdata use half the memory.

4. **Machine Learning**

   By default Netdata trains multiple machine learning models for every metric collected, to learn its behavior and detect anomalies.

   Machine Learning is a CPU intensive process and affects the overall CPU utilization of Netdata.

5. **Streaming Compression**

   When using Netdata in Parent-Child configurations to create Metrics Centralization Points, the compression algorithm used greatly affects CPU utilization and bandwidth consumption.

   Netdata supports multiple streaming compression algorithms, allowing the optimization of either CPU utilization or Network Bandwidth. The default algorithm `zstd` provides the best balance among the two.

## Minimizing the resources used by Netdata Agents

We suggest to configure Netdata Parents for centralizing metric samples, and disabling most of the features on Netdata Children.

This will provide minimal resource utilization at the edge, while all the features of Netdata are available through the Netdata Parents.

## Maximizing the scale of Netdata Parents

Netdata Parents automatically size their resource utilization based on the workload they receive. The only possible option for improving query performance is to dedicate more RAM to them.

Check [RAM Requirements](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md) for more information.

## Innovations Netdata has for optimal performance and scalability

The following are some of the innovations the open-source Netdata Agent has, that contribute to its excellent performance, and scalability.

1. **Minimal disk I/O**

   When Netdata saves data on-disk, it stores them at their final place, eliminating the need to reorganize this data.

   Netdata is organizing its data structures in such a way that samples are committed on disk as evenly as possible across time, without affecting its memory requirements.

   Furthermore, Netdata Agents use direct I/O for saving and loading metric samples. This prevents Netdata from polluting system caches with metric data. Netdata maintains its own caches for this data.

2. **4 bytes per sample uncompressed**

   To achieve optimal memory and disk footprint, Netdata uses a custom 32-bit floating point number. This floating point number is used to store the collected samples, together with their anomaly bit.

   The database of Netdata is fixed-step, so it has predefined slots for every sample, allowing Netdata to store timestamps once every several hundreds of samples, minimizing both its memory requirements and the disk footprint.

   The final disk footprint of Netdata varies due to compression efficiency. It is usually about 0.6 bytes per sample for the high-resolution tier (per-second), 6 bytes per sample for the mid-resolution tier (per-minute) and 18 bytes per sample for the low-resolution tier (per-hour).

3. **Query priorities**

   Alerting, Machine Learning, Streaming and Replication rely on metric queries.

   When multiple queries are running in parallel, Netdata assigns priorities to all of them, favoring interactive queries over background tasks.

   This means that queries do not compete equally for resources. Machine learning or replication may slow down when interactive queries are running and the system starves for resources.

4. **A pointer per label**

   Apart from metric samples, metric labels and their cardinality is the biggest memory consumer, especially in highly ephemeral environments, like kubernetes.

   Netdata uses a single pointer for any label key-value pair that is reused. Keys and values are also deduplicated, providing the best possible memory footprint for metric labels.

5. **Streaming Protocol**

   The streaming protocol of Netdata allows minimizing the resources consumed on production systems by delegating features of to other Netdata Agents (Parents), without compromising monitoring fidelity or responsiveness, enabling the creation of a highly distributed observability platform.
