# Using the Live tab

The **Live** tab is the Netdata Cloud interface for selecting and executing Functions on monitored nodes. See [Live View](/docs/top-monitoring-netdata-functions.md) for the feature overview and task-specific guides.

You can also open the same view from a node dropdown by selecting **Live, on-demand infrastructure insights** (the `f(x)` icon).

## Execute a Function

1. Open the **Live** tab.
2. Select a Function using the **Function** selector.
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

If a Function does not appear or cannot execute:

1. Confirm that the target node is online.
2. Confirm that the collector or plugin providing the Function is running.
3. Check whether the Function is supported on that operating system and Agent version.
4. Verify the current user's permissions and the Agent's access-control settings.
5. For a Child node, verify that the streaming path to its Parent is active.
