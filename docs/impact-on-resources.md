# Impact of Running Netdata on Cloud Provided VMs

## Overview

Netdata provides real-time performance monitoring and troubleshooting capabilities for your systems, offering unparalleled insights with minimal resource consumption. This document highlights the baseline impact of running Netdata on an empty cloud provider (AWS, GCP, Azure, etc.) Virtual Machine (VM), which serves as a reference for understanding the performance and resource usage of Netdata in a production environment.

The resources used by Netdata depend primarily on the number of metrics collected (which may vary significantly among servers) and the features enabled (e.g., streaming to Netdata Parents, Machine Learning). However, Netdata has been designed to be significantly more efficient compared to other observability agents. Despite collecting, storing, and visualizing data in real-time with per-second granularity, Netdata is lighter and more efficient than many commercial and open-source alternatives.

## Baseline Configuration on an Empty VM

When deployed on an empty VM, Netdata collects and visualizes critical system metrics with exceptional granularity and efficiency. Below is an overview of its capabilities and resource consumption:

### Features and Capabilities

#### **Comprehensive Monitoring**

- Over **2,000 unique time-series**, including metrics for:
    - CPU, memory, disks, mount points, and filesystems.
    - Network interfaces and the entire networking stack (all protocols, firewall).
    - Containers, processes, users, user groups, and systemd units.
    - All kernel technologies utilized.
- Data is collected and visualized with 1-second granularity, ensuring no performance detail is missed.

#### **Health Alerts**

- **350 Unique Alerts** ready to run, with about **50+** actively monitoring system components for common errors, misconfigurations, and issues across all monitored components.

#### **Logs Explorer**

- **Systemd-Journal Integration:** Query and visualize system and application logs without requiring an external logs database server.

#### **Network Explorer**

- Analyze and visualize TCP and UDP network connections, including connections from individual processes and containers.

#### **Machine Learning and Unsupervised Anomaly Detection**

- **Real-Time Anomaly Detection:** 18 machine learning models trained per time-series to identify and flag outliers based on behavioral trends over the last few days.

#### **Data Retention**

- **Efficient Storage:**
    - Two weeks of high-resolution data (per-second).
    - Three months of mid-resolution data (per-minute).
    - Two years of low-resolution data (per-hour).
    - Total storage required: **3 GiB**.

### Resource Consumption

Netdata operates with an exceptionally low footprint, even under demanding conditions. On an empty VM, the resource usage is as follows:

- **CPU Usage:**
    - ~1.5% of a single core without machine learning.
    - ~3% with machine learning (ML training is spread over time, so there is a constant increase in CPU utilization without spikes).
    - ~5% with machine learning and real-time streaming to a Netdata Parent.
    - Rare spikes up to 20% during data flushes (once every 15–20 minutes).
- **Memory Usage:**
    - ~150 MB of RAM without machine learning.
    - ~200 MB with machine learning enabled.
- **Disk I/O:**
    - Reads: ~2 KiB/s without machine learning, ~9 KiB/s with machine learning.
    - Writes: ~5 KiB/s.
- **Storage:** 3 GiB total.

## Typical Netdata Resources Usage on Production Systems

In production systems with more data sources and features enabled, users can expect:

- **CPU Usage:** 5%-20% of a single core.
- **Memory Usage:** 250–350 MB RAM.
- **Disk I/O:** ~10 KiB/s reads and writes.
- **Storage:** 3 GiB total.

## Impact of Running Netdata on Cloud VMs

The baseline impact of running Netdata on an empty VM can be summarized as follows:

1. **Minimal Resource Utilization:**
    - The CPU, memory, and disk I/O impact is negligible compared to the resources typically provisioned on cloud VMs.

2. **Scalable Retention:**
    - Netdata efficiently uses disk space to retain high-resolution and long-term data without requiring additional storage or databases.

3. **Real-Time Insights:**
    - Despite its lightweight nature, Netdata provides a robust monitoring solution with granular, real-time insights and alerting capabilities.

4. **Comprehensive Monitoring Stack:**
    - From system metrics to logs and network activity, Netdata provides an all-in-one observability solution without the overhead of deploying multiple tools.

## Recommendations

To ensure optimal performance and scalability, consider the following when deploying Netdata on cloud VMs:

- **Baseline Impact:** Use the above metrics as a reference for resource planning on VMs with additional workloads.
- **Retention Policies:** Adjust retention settings to match your specific needs and storage availability.
- **Alert Fine-Tuning:** Customize alerts based on the workload and environment to reduce noise and increase actionable insights.
- **Scaling:** For environments with high workloads or many containers, consider using Netdata Parents and Netdata Cloud to aggregate and analyze data from multiple nodes.

## Independent Reviews

In December 2023, the University of Amsterdam published a study on the impact of monitoring tools for Docker-based systems, focusing on:

- The impact of monitoring tools on the energy efficiency of Docker-based systems.
- The impact of monitoring tools on the performance of Docker-based systems.

Key findings include:

- **Netdata is the most efficient agent,** requiring significantly fewer system resources than others.
- **Netdata has minimal performance impact,** allowing containers and applications to run without measurable degradation due to observability.

Full analysis here: [Twitter Link](https://twitter.com/IMalavolta/status/1734208439096676680)

## Comparisons with Other Commercial Observability Agents

| Resource                      | Dynatrace  | Datadog    | Instana   | Grafana   | Netdata    |
|-------------------------------|------------|------------|-----------|-----------|------------|
| **Resolution**                | 1-minute   | 15-sec     | 1-sec     | 1-minute  | 1-sec      |
| **CPU Usage (100% = 1 core)** | 12%        | 14%        | 6.7%      | 3.3%      | 3.6%       |
| **Memory Usage**              | 1400 MB    | 972 MB     | 588 MB    | 414 MB    | 181 MB     |
| **Disk Space**                | 2 GB       | 1.2 GB     | 0.2 GB    | -         | 3 GB       |
| **Disk Read Rate**            | -          | 0.2 KB/s   | -         | -         | 0.3 KB/s   |
| **Disk Write Rate**           | 38.6 KB/s  | 8.3 KB/s   | -         | 1.6 KB/s  | 4.8 KB/s   |
| **Egress Bandwidth**          | 11.4 GB/mo | 11.1 GB/mo | 5.4 GB/mo | 4.8 GB/mo | 0.01 GB/mo |

Note: Netdata does not stream metric samples to Netdata Cloud. Egress bandwidth for Netdata reflects only user-driven queries. When streaming metrics to a Netdata Parent, bandwidth requirements are similar to other tools, but this traffic typically remains within the local network.

## Conclusion

Netdata is designed to provide detailed observability with minimal impact on system resources. When deployed on an empty cloud VM, it delivers real-time monitoring and alerting while maintaining a small resource footprint. This baseline serves as a practical reference for assessing Netdata’s performance in more complex environments.

For additional information, please visit [Netdata Documentation](https://learn.netdata.cloud/).
