# Live View

Live View is the home for live, on-demand insights from monitored nodes. Each available item is a Function exposed by a collector or another Agent component.

Unlike charts, which visualize stored time-series metrics, a Function executes when requested and returns current operational details or performs a supported action on the selected node.

## Explore Live View

- [Use the Live tab](/docs/dashboards-and-charts/live-tab.md) to select a Function, target node, filters, and update interval.
- [Processes](/docs/functions/processes.md) to attribute current resource usage to individual processes and application groups.
- [Database Queries](/docs/functions/databases.md) to investigate query performance, running queries, deadlocks, and errors across supported database collectors.

The Functions available on a node depend on its operating system, enabled collectors, Agent version, and accessible data sources. Collector and integration documentation describes any additional Functions that it provides.

## Other Access Paths

Other access paths include the [node dropdown](/docs/dashboards-and-charts/nodes-tab.md), a Netdata Agent or Parent API, and [Netdata MCP](/docs/netdata-ai/mcp/README.md).
