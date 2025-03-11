# Extreme Cardinality Protection in Netdata

Netdata’s tiered storage is designed to efficiently retain metric data and metadata for long periods. However, when extreme cardinality occurs—often unintentionally through misconfigurations or inadvertent practices (e.g., spawning many short-lived docker containers or using unbounded label values)—the long-term retention of metadata can lead to excessive resource consumption.

To protect Netdata from extreme cardinality, Netdata has an automated protection. This document explains **why** this protection is needed, **how** it works, **how** to configure it, and **how** to verify its operation.

## Why Extreme Cardinality Protection is Needed

Extreme cardinality refers to the explosion in the number of unique time series generated when metrics are combined with a wide range of labels or dimensions. In modern observability platforms like Netdata, metrics aren’t just simple numeric values—they come with metadata (labels, tags, dimensions) that help contextualize the data. When these labels are overly dynamic or unbounded (for example, when using unique identifiers such as session IDs, user IDs, or ephemeral container names), combined with a very long retention, like the one provided by Netdata, the system ends up tracking an enormous number of unique series.

Despite the fact that Netdata performs better than most other observability solution, extreme cardinality has a few implications:

-   **Resource Consumption:** The system needs to remember and index vast amounts of metadata, increasing its memory footprint.
-   **Performance Degradation:** When performing long-term queries (days, weeks, months), the system needs to query a vast number of time-series leading to slower query responses.
-   **Operational Complexity:** High cardinality makes it harder to manage, visualize, and analyze data. Dashboards can become cluttered.
-   **Scalability Challenges:** As the number of time series grows, the resources required for maintaining aggregation points (Netdata parents) increase too.

## Definition of Metrics Ephemerality

Metrics ephemerality is the percentage of metrics that is no longer actively collected (old) compared to the total metrics available (sum of currently collected metrics and old metrics).

- **Old Metrics** = The number of unique time-series that were once collected, but not currently.
- **Current Metrics** = The number of unique time-series actively being collected.

High Ephemerality (close to 100%): The system frequently generates new unique metrics for a short period, indicating a high turnover in metrics.
Low Ephemerality (close to 0%): The system maintains a stable set of metrics over time, with little change in the total number of unique series.

## How The Netdata Protection Works

The mechanism kicks in during tier0 (high-resolution) database rotations (i.e., when the oldest tier0 samples are deleted) and proceeds as follows:

1. **Counting Instances with Zero Tier0 Retention:**
    - For each context (e.g., containers, disks, network interfaces, etc), Netdata counts the number of instances that have **ZERO** retention in tier0.

2. **Threshold Verification:**
    - If the number of instances with zero tier0 retention is **greater than or equal to 1000** (the default threshold) **and** these instances make up more than **50%** (the default Ephemerality threshold) of the total instances in that context, further action is taken.

3. **Forceful Clearing in Long-Term Storage:**
    - The system forcefully clears the retention of the excess time-series. This action automatically triggers the deletion of the associated metadata. So, Netdata "forgets" them. Their samples are still on disk, but they are no longer accessible.

4. **Retention Rules:**
    - **Protected Data:**
        - Metrics that are actively collected (and thus present in tier0) are never deleted.
        - A context with fewer than 1000 instances (as presented in the Netdata dashboards at the NIDL bar of the charts) is considered safe and is not modified.
    - **Clean-up Trigger:**
        - Only metrics that have lost their tier0 retention in a context that meets the thresholds (≥1000 instances and >50% ephemerality) will have their long-term retention cleared.

## Configuration

You can control the protection mechanism via the following settings in the `netdata.conf` file under the `[db]` section:

```ini
[db]
    extreme cardinality protection = yes
    extreme cardinality keep instances = 1000
    extreme cardinality min ephemerality = 50
```

-   **extreme cardinality keep instances:**  
    The minimum number of instances per context that should be kept. The default value is **1000**.

-   **extreme cardinality min ephemerality:**  
    The minimum percentage (in percent) of instances in a context that have zero tier0 retention to trigger the cleanup. The default value is **50%**.


**Recommendations:**

-   If you have samples in tier0, you also have their corresponding long-term data and metadata. Ensure that tier0 retention is configured properly.
-   If you expect to have more than 1000 instances per context per node (for example, more than 1000 containers, disks, network interfaces, database tables, etc.), adjust these settings to suit your specific environment.

## How to Check Its Work

When the protection mechanism is activated, Netdata logs a detailed message. The log entry includes:

-   The host name.
-   The context affected.
-   The number of metrics and instances that had their retention forcefully cleared.
-   The time range for which the non-tier0 retention was deleted.

### Example Log Message

```
EXTREME CARDINALITY PROTECTION: on host '<HOST>', for context '<CONTEXT>': forcefully cleared the retention of <METRICS_COUNT> metrics and <INSTANCES_COUNT> instances, having non-tier0 retention from <START_TIME> to <END_TIME>.
```

This log message is tagged with the following message ID for easy identification:

```
MESSAGE_ID=d1f59606dd4d41e3b217a0cfcae8e632
```

### Verification Steps

1.  **Using System Logs:**

    You can use `journalctl` (or your system’s log viewer) to search for the message ID:

```
journalctl --namespace=netdata MESSAGE_ID=d1f59606dd4d41e3b217a0cfcae8e632
``` 

2.  **Netdata Logs Dashboard:**

    Navigate to the Netdata Logs dashboard. On the right side under `MESSAGE_ID`, select **"Netdata extreme cardinality"** to filter only those messages.

## Summary

The extreme cardinality protection mechanism in Netdata is designed to automatically safeguard your system against the potential issues caused by excessive metric metadata retention. It does so by:

-   Automatically counting instances without tier0 retention.
-   Checking against configurable thresholds.
-   Forcefully clearing long-term retention (and metadata) when thresholds are exceeded.

By properly configuring tier0 and adjusting the `extreme cardinality` settings in `netdata.conf`, you can ensure that your system remains both efficient and protected, even when extreme cardinality issues occur.
