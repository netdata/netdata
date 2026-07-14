# Live Tab

The **Live tab** is the Netdata Cloud interface for selecting and executing [Netdata Functions](/docs/top-monitoring-netdata-functions.md) on monitored nodes. Functions provide current operational details that complement metrics and charts.

You can also execute a Function from the [Nodes tab](/docs/dashboards-and-charts/nodes-tab.md) by selecting the `f(x)` control for a node.

## Execute a Function

1. Open the **Live** tab.
2. Select a Function from the Functions bar.
3. Select the node where the Function should run.
4. Apply any filters offered by that Function.
5. Review the returned visualization and data table.

Available Functions and filters depend on the selected node and its enabled collectors.

## Review Results

A Function can return:

| Element           | Purpose                                                                   |
|:------------------|:--------------------------------------------------------------------------|
| **Visualization** | Summarizes the result when the Function supplies a visual representation. |
| **Data table**    | Shows the detailed rows returned by the Function.                         |

Use the controls in the upper-right corner to:

- Refresh the result manually while the view is paused.
- Select an update interval for repeated execution.

Some Function results may contain sensitive process, query, log, or network information. Follow your organization's handling rules before copying or sharing them.

## Troubleshooting

If a Function is missing or cannot execute, check the [Function availability troubleshooting steps](/docs/top-monitoring-netdata-functions.md#troubleshooting-availability). The target node must be online, the component providing the Function must be running, and the current user must have the required access.
