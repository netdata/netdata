# Netdata Model Context Protocol (MCP) Overview

## Introduction

The Netdata Model Context Protocol (MCP) provides an interface for Large Language Models (LLMs) to access monitoring data from Netdata. This protocol enables LLMs to function as SRE/DevOps assistants, answering infrastructure questions by accessing relevant metrics, alerts, and system information.

## Server Info

This MCP server provides data for the following:

-   Type: [Single Agent | Parent | Cloud]
-   Available Nodes: {{NUMBER_OF_NODES}}
-   Available Time-series: {{NUMBER_OF_TIMESERIES}}
-   Available Metric Contexts: {{NUMBER_OF_CONTEXTS}}
-   Available Context Categories: {{NUMBER_OF_CATEGORIES}}
-   Metrics Retention:
    - per-second tier: {{TIER0_RETENTION}}
    - per-minute tier: {{TIER1_RETENTION}} (min, max, average aggregations are same with per-second)
    - per-hour tier: {{TIER2_RETENTION}} (min, max, average aggregations are same with per-second)
    - tiers are automatically selected during queries, for performance

## How to run this API

`./mcp.sh ENDPOINT [OPTIONS]`

## Core Endpoints

### Help

-   `hello` (or nothing): Returns this overview document
-   `help`: Returns detailed help on specific endpoints (e.g., `./mcp.sh metrics-query help`)

#### Metrics Scope

All metrics endpoints can be given a scope, to slice the underlying dataset and provide focused responses. A scope can include:

- `scope_nodes`: a comma separated list of node ids.
- `scope_contexts`: a comma separated list of metric contexts.
- `after`, `before`: unix epoch timestamps in seconds.

When any of the above is not set, the scope is only limited by the data available. To improve query performance and reduce delays, always set the scope to whatever makes sense.

#### Metrics Structure

1. Nodes have host labels
2. Nodes have metric instances (e.g. network interfaces, database servers, db tables, etc)
3. Metric instances have labels, dimensions and alerts
4. Each dimension of a single instance is a unique time-series
5. Metric instances follow contexts (templates)
6. Typically instances of the same context, have the same instance labels keys and dimension names
7. Contexts appear as "charts" on the dashboard. They can be single-node or multi-node, single-instance or multi-instance.
8. Charts can be sliced and diced by node, instance, dimension, instance labels and any combination of them.

#### Metrics Discovery

- `context-categories`: Returns available metric categories under the given scope (first-level navigation)
- `contexts`: Returns contexts/charts within a category under a given scope (second-level navigation)
- `context-sources`: Returns available nodes, instances, dimensions and labels for a context under the given scope.
- `context-search`: Performs full text search on all metrics metadata (instances, dimensions, labels keys and values, chart titles, context units) and returns matching contexts under the given scope.

#### Metrics Queries

- `context-query`: Queries time-series data of a context under a given scope. The result includes 3 time-series for each dimension queried: a) collected data, b) anomaly-rate of the collected data, c) flags indicating lost data collections or partial data.

#### Scoring Queries

- `score-metrics`: Scores all time-series under a scope, to find outliers, correlations or similarities, using various algorithms. The query can be based on either the collected data, or their anomaly rate.
- `host-level-anomalies`: Returns the anomaly rate per node (i.e. each point in time is the % of time-series collected found anomalous at the same time). Spikes in this are a strong indication of a wide spread host-level anomaly. Spikes across hosts, indicate a wide spread infrastructure level anomaly.

#### Metrics Health (alerts and transitions)

- `alerts-active`: returns the currently raised alerts
- `alerts-runnning`: returns the configured alerts
- `alerts-transitions`: returns the past transitions of alerts

### Logs Endpoints

Logs are queried per node. Multi-node logs queries are only possible on logs centralization servers.

#### Logs Structure

- linux hosts follow systemd-journald schema
- windows hosts follow windows-events schema

#### Logs Discovery

- `logs-find`: Given a hostname, it finds the nodes that can respond to logs queries for it. Usually this returns the node itself (if it is logs capable), or logs centralization servers that have logs for this node. The response includes the retention.

#### Logs Queries

-   `logs-query`: Retrieves logs from specified nodes and time ranges. Can return raw data, a histogram on any field. A drill down on multiple fields. Supports full text search.

### Nodes Endpoints

#### Node Queries

- `nodes-info`: Given a node id, it returns detailed information about the node.
- `node-instances`: Returns all the replicas of a node (child and parent instances of a node), including their streaming path.

### Functions (Live-Only Data)

Functions can be only be queried per node.

-   `processes`: Returns the process tree of a node (all running PIDs)
-   `connections`: Returns current network connections of node (TCP and UDP)
-   `mount-points`: Returns current mount points information
-   `containers`: Returns current cgroups containers/VMs information
-   `network-interfaces`: Returns current network interfaces status

## Examples

It is strongly recommented to use the `help` of each endpoint before executing it for the first time.

### Find all database servers

1. `./mcp.sh context-categories` to get all the metrics categories, and check the result for well known database server names, like mysql, postgresql, redis, etc.

### Find the workload of all database servers

1. `./mcp.sh context-categories` to find the top level metrics categories and find common database server names in them.
2. `./mcp.sh endpoint=contexts categories A,B,C,D` where A,B,C,D are the database categories identified as databases. This returns a list of contexts. Look for contexts that indicate workload.
3. `./mcp.sh endpoint=context-query scope_contexts C1 after -600 before 0 points 10`, where C1 is one of the contexts identified above (repeat for all contexts identified). This queries the last 10 minutes of data with per-dimension aggregation, averaging per minute over time. The result includes also anomaly rates.

### Check overall infrastructure health

1. `./mcp.sh alerts-active` returns the currently raised alerts.
2. `./mcp.sh host-level-anomalies after -3600 before 0 points 60 aggregation max` returns the max % of metrics that were anomalous per node, over the last hour. Look for spikes in the result.

### Find the root cause of an issue or find related metrics

Assuming the time-frame (T0 to T1) on an issue or something of interest, has been identified:

1. `./mcp.sh score-metrics after T0 before T1`, returns an ordered list of metrics that changed behavior between T0 and T1. The bigger the change, the closer to the top the metric will be. This query can be customized to use anomaly rates (find outliers) or the collected data (find correlations). Check its help for more information.

## Best Practices

Identify groups of nodes (e.g., database servers, web servers), or types of metrics (workload, errors, utilization) by first discovering relevant metric `contexts` (via `context-categories` -> `contexts`) and then querying a representative context (using `metrics-latest` without node filters) to see which nodes report that data.

For further assistance or examples, use the help system for any specific endpoint.

To identify congestions or bottlenecks on Linux system resources, use pressure stall metrics, instead of load average or utilization metrics.

Common context categories:

- `system`: system resources (cpu, pressure, load average, entropy, etc)
- `mem`: system memory (ram, swap, page faults, hugepages, numa, ecc, fragmentation, slab, etc)
- `disk`: disks and mount points
- `net`: network interfaces
- `ip` and `ipv6`: networking stack
- `cgroup`: containers and VMs (as viewed from the host)
- `systemd`: cgroups metrics for systemd units
- `apps`, `users`, `groups`: aggregations of the process tree

Get the full list from the `context-categories` endpoint.
