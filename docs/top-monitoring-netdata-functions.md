# Netdata Functions

Netdata Functions are on-demand routines exposed by collectors and other Agent components. They return live troubleshooting information or perform a supported action on the node where they run.

Unlike charts, which visualize stored time-series metrics, a Function executes when requested and returns its current result. Depending on the Function, that result can include processes, database queries, network connections, logs, service state, streaming status, or other operational details.

## When to Use Functions

Use Functions when charts show that something changed and you need current, high-cardinality detail to investigate it. Common workflows include:

- Finding the processes responsible for CPU, memory, or I/O usage.
- Inspecting expensive or currently running database queries.
- Reviewing network connections, service state, or logs.
- Examining the health and topology of a streaming deployment.
- Retrieving collector-specific details that do not belong in time-series charts.

The Functions available on a node depend on its operating system, enabled collectors, Agent version, and the data sources those collectors can access. The dashboard and API report the set currently available on the selected node; this page intentionally does not maintain a duplicate inventory.

## Access Functions

You can execute Functions through:

- The [Live tab](/docs/dashboards-and-charts/live-tab.md) in Netdata Cloud.
- The `f(x)` control for a node in the [Nodes tab](/docs/dashboards-and-charts/nodes-tab.md).
- A Netdata Agent or Parent API.
- [Netdata MCP](/docs/netdata-ai/mcp/README.md), which can call supported Functions during an investigation.

A Function runs on the selected node. When the node streams to a Parent, its Function definitions propagate through the streaming path so an authorized request can be routed to that node. The target node must be online and reachable through the active streaming path.

## Access and Sensitive Data

Functions can expose data that is more sensitive than ordinary metrics. Process command lines, database queries, logs, and network connections can contain credentials, personal data, or internal infrastructure details.

Each Function declares its required access level. Depending on that declaration and the access path, execution may require an authenticated user in the same Netdata Cloud Space, an additional permission, or direct-Agent authentication. Direct Agent and Parent access also respects configured network ACLs and [bearer token protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md).

Before sharing Function output, review it for secrets and identifying information.

## Explore Functions by Domain

Start with the domain pages below when you want a stable entry point instead of a mutable inventory:

- [Processes](/docs/functions/processes.md)
- [Database Queries](/docs/functions/databases.md)
- [SQL collector Functions](/src/go/plugin/go.d/collector/sql/integrations/sql_databases_generic.md)
- [systemd journal Functions](/src/collectors/systemd-journal.plugin/README.md)
- [Windows Events Functions](/src/collectors/windows-events.plugin/README.md)
- [macOS Logs Functions](/src/collectors/macos-logs.plugin/README.md)
- [Live tab](/docs/dashboards-and-charts/live-tab.md)
- [Nodes tab](/docs/dashboards-and-charts/nodes-tab.md)
- [Netdata MCP](/docs/netdata-ai/mcp/README.md)

## Function Guides

- [Processes](/docs/functions/processes.md): attribute current resource usage to individual processes and application groups.
- [Database Queries](/docs/functions/databases.md): investigate query performance, running queries, deadlocks, and errors across supported database collectors.
- Collector and integration documentation describes any additional Functions supplied by that integration.

## Troubleshooting Availability

If a Function does not appear or cannot execute:

1. Confirm that the target node is online.
2. Confirm that the collector or plugin providing the Function is running.
3. Check whether the Function is supported on that operating system and Agent version.
4. Verify the current user's permissions and the Agent's access-control settings.
5. For a Child node, verify that the streaming path to its Parent is active.
