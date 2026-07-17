# Processes Function

The `processes` Function provides a live snapshot of resource usage and process relationships on the selected node. It is supplied by [`apps.plugin`](/src/collectors/apps.plugin/README.md) and complements the aggregated application charts created by that plugin.

Use it to move from a chart showing high resource use to the individual processes contributing to that activity.

## Access Requirements

The Function exposes sensitive process information. Executing it requires an authenticated identity with signed, same-Space, and sensitive-data access.

Command-line arguments require the additional **View Agent Config** permission, unless the Agent operator has explicitly enabled command-line exposure. Arguments can contain credentials or other secrets, so review them before sharing Function output.

## Investigate a Resource Problem

1. Open the Function from the [Live tab](/docs/dashboards-and-charts/live-tab.md).
2. Select the affected node.
3. Sort by the resource implicated by the charts, such as CPU, memory, reads, writes, file descriptors, or threads.
4. Filter or group by application category, user, group, parent process, or another available dimension.
5. Compare the individual processes with the corresponding `apps.plugin` charts to understand how they contribute to the aggregate.

The result is a current snapshot, not process history. Use charts and event or log data when you need to reconstruct earlier behavior.

## Result Fields

The exact columns depend on the operating system, kernel capabilities, permissions, and Agent configuration. The Function groups fields into these areas:

| Area                    | Examples                                             | What it helps answer                                               |
|:------------------------|:-----------------------------------------------------|:-------------------------------------------------------------------|
| Identity and ownership  | PID, parent PID, command, user, group                | Which process is this, and who owns it?                            |
| Application grouping    | Category and process relationships                   | Which charted application group contains it?                       |
| CPU and scheduling      | User/system CPU, context switches, faults            | Which processes are consuming CPU or creating scheduler pressure?  |
| Memory                  | Resident, virtual, swap, shared, proportional memory | Which processes are consuming or sharing memory?                   |
| Storage I/O             | Read/write throughput and operations                 | Which processes contribute to disk activity?                       |
| Descriptors and threads | Files, pipes, sockets, handles, threads              | Is a process approaching a resource limit or growing unexpectedly? |
| Runtime state           | Uptime and process state                             | Did a process restart, stall, or remain active unexpectedly?       |

Use the column descriptions returned with the Function as the runtime source of truth. They stay aligned with the Agent version actually producing the result.

## Platform and Configuration Differences

- Linux exposes the broadest set of process and file-descriptor details.
- Windows, FreeBSD, and macOS return the fields supported by their native process APIs.
- Some fields require elevated operating-system access and may be absent even when the Function itself is available.
- Linux proportional set size (PSS) sampling is **off by default**. Operators can enable it with the `apps.plugin` `--pss` interval option. Sampling `smaps` adds overhead, so choose an interval appropriate for the node.
- Short-lived processes can exit between collection and display. PID reuse also means a PID alone is not a durable process identity.

## Common Workflows

### Find a Heavy Process

Sort by the affected resource, then inspect the leading processes and their application categories. Check parent-child relationships before attributing usage to a single executable.

### Investigate Growth

Repeat the Function at a controlled interval and watch memory, descriptors, sockets, or thread counts. Confirm suspected growth with charts or application telemetry before concluding that it is a leak.

### Check Restarts

Sort or filter by uptime to find recently started processes. Correlate the result with service logs, deployment events, and alerts.

### Review Unexpected Activity

Filter by user, command, parent, network-related descriptors, or application category. The Function is an investigation aid, not a replacement for an audit log or endpoint security product.

## Related Documentation

- [Live View](/docs/top-monitoring-netdata-functions.md)
- [Apps plugin](/src/collectors/apps.plugin/README.md)
