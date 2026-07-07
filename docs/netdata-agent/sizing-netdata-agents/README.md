# Resource utilization

Netdata is designed to automatically adjust its resource consumption based on the specific workload.

## Minimum system requirements

A standalone Netdata Agent has a small footprint and runs comfortably on a minimal system. The table below shows Netdata's measured resource usage — follow the links for how each figure is derived.

| Resource       | Netdata's footprint                                                                                                                                                                                                             |
|:---------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **CPU**        | [1%-5% of a single core](/docs/netdata-agent/sizing-netdata-agents/cpu-requirements.md) with default settings; up to [5%-20% in production](/docs/impact-on-resources.md#typical-netdata-resources-usage-on-production-systems) |
| **RAM**        | [100-200 MB](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md) on an empty system; [250-350 MB in typical production](/docs/impact-on-resources.md#typical-netdata-resources-usage-on-production-systems)          |
| **Disk**       | [~4 GiB by default](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md#default-disk-footprint) (3 GiB metrics plus metadata), configurable per tier                                                   |
| **Privileges** | Root on Linux, or Administrator on Windows, required for installation                                                                                                                                                           |

For multi-node setups that centralize metrics on a Netdata Parent, resource needs scale with the number of Children and retention — see [Parent Configuration Best Practices](/docs/observability-centralization-points/best-practices.md).

## What affects resource usage

This table shows the specific system resources affected by different Netdata features:

|                 Feature | CPU | RAM | Disk I/O | Disk Space | Network Traffic |
|------------------------:|:---:|:---:|:--------:|:----------:|:---------------:|
|       Collected metrics |  ✓  |  ✓  |    ✓     |     ✓      |        -        |
|        Sample frequency |  ✓  |  -  |    ✓     |     ✓      |        -        |
| Database mode and tiers |  -  |  ✓  |    ✓     |     ✓      |        -        |
|        Machine learning |  ✓  |  ✓  |    -     |     -      |        -        |
|               Streaming |  ✓  |  ✓  |    -     |     -      |        ✓        |

1. **Collected metrics**

    - **Impact**: More metrics mean higher CPU, RAM, disk I/O, and disk space usage.
    - **Optimization**: To reduce resource consumption, consider lowering the number of collected metrics by disabling unnecessary data collectors.

2. **Sample frequency**

    - **Impact**: Netdata collects most metrics with 1-second granularity. This high frequency impacts CPU usage.
    - **Optimization**: Lowering the sampling frequency (e.g., 1-second to 2-second intervals) can halve CPU usage. Balance the need for detailed data with resource efficiency.

3. **Database Mode**

    - **Impact**: The default database mode, `dbengine`, compresses data and writes it to disk.
    - **Optimization**: In a Parent-Child setup, switch the Child's database mode to `ram`. This eliminates disk I/O for the Child.

4. **Database Tiers**

    - **Impact**: The number of database tiers directly affects memory consumption. More tiers mean higher memory usage.
    - **Optimization**: The default number of tiers is 3. Choose the appropriate number of tiers based on data retention requirements.

5. **Machine Learning**

    - **Impact**: Machine learning model training is CPU-intensive, affecting overall CPU usage.
    - **Optimization**: Consider disabling machine learning for less critical metrics or adjusting model training frequency.

6. **Streaming Compression**

    - **Impact**: Compression algorithm choice affects CPU usage and network traffic.
    - **Optimization**: Select an algorithm that balances CPU efficiency with network bandwidth requirements (e.g., zstd for a good balance).

## Minimizing the resources used by Netdata Agents

To optimize resource utilization, consider using a **Parent-Child** setup.

This approach involves centralizing the collection and processing of metrics on Parent nodes while running lightweight Children Agents on edge devices.

## Maximizing the scale of Parent Agents

Parents dynamically adjust their resource usage based on the volume of metrics received. However, for optimal query performance, you may need to dedicate more RAM.

Check [RAM Requirements](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md) for more information.

## Netdata's performance and scalability optimization techniques

1. **Minimal Disk I/O**

   Netdata directly writes metric data to disk, bypassing system caches and reducing I/O overhead. Additionally, its optimized data structures minimize disk space and memory usage through efficient compression and timestamping.

2. **Compact Storage Engine**

   Netdata uses a custom 32-bit floating-point format tailored for efficient storage of time-series data, along with an anomaly bit. This, combined with a fixed-step database design, enables efficient storage and retrieval of data. Timestamp optimization further reduces storage overhead by storing timestamps at regular intervals.

   For per-tier on-disk sample sizes, see [Disk Requirements & Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

3. **Intelligent Query Engine**

   Netdata prioritizes interactive queries over background tasks like machine learning and replication, ensuring optimal user experience, especially under heavy load.

4. **Efficient Label Storage**

   Netdata uses pointers to reference shared label key-value pairs, minimizing memory usage, especially in highly dynamic environments.

5. **Scalable Streaming Protocol**

   Netdata's streaming protocol enables the creation of distributed monitoring setups, where Children offload data processing to Parents, optimizing resource utilization.
