# 13. Alerts and Notifications Architecture

Understanding how alerts work internally helps in troubleshooting, optimization, and extending the system appropriately. This chapter explains the architecture of alert evaluation, notification dispatch, and configuration layering in Netdata. Read this chapter when you need to understand the flow from metric collection to notification delivery or when debugging complex alerting scenarios.

The architecture described here applies to Netdata Agent with local evaluation. When using Netdata Parents or Cloud features, additional components come into play, but the core flow remains similar.

## 13.1 Alert Evaluation Architecture

Alert evaluation is the process of checking metric values against configured conditions and determining alert status. This process happens entirely on the Agent or Parent node that owns the metrics.

### Metric Collection Pipeline

Metrics flow from collectors through the Netdata database and into the health engine. Collectors gather raw data from system APIs, application endpoints, or external services. This raw data is formatted as dimension values and stored in the round-robin database.

The database maintains per-second resolution for recent time windows and aggregated data for longer periods. Alert lookups query this database to retrieve historical values for comparison against thresholds.

The health engine runs on a configurable interval, typically every second. Each iteration evaluates all enabled alerts against recent metric values. This synchronous evaluation ensures consistent state across all alerts for a given time point.

### The Alert State Machine

Every alert instance exists in one of several states defined by the health state machine.

The `UNINITIALIZED` state indicates that the alert has not yet received enough data to evaluate. Most alerts require a warm-up period to accumulate historical values. During this period, the alert cannot determine whether conditions are normal.

The `CLEAR` state indicates that conditions are within acceptable parameters. No alert has fired, and no action is required. This is the normal operating state.

The `WARNING` state indicates that a condition has exceeded the warning threshold. Some systems treat warnings as advisory, while others require response. The context determines the appropriate response.

The `CRITICAL` state indicates that a condition has exceeded the critical threshold. Critical conditions typically require immediate attention and may involve service degradation or data loss risk.

The `UNDEFINED` state indicates that the alert encountered an error during evaluation. Missing metrics, invalid expressions, or configuration errors can cause this state. This state should not be confused with normal operation; it indicates a problem with the alert configuration itself.

The `REMOVED` state indicates that the alert has been deleted or the configuration has been removed. This state exists for historical record-keeping; it does not indicate an active problem.

State transitions occur when evaluation finds conditions that match the transition criteria. From CLEAR, conditions exceeding warning thresholds transition to WARNING. From WARNING, conditions exceeding critical thresholds transition to CRITICAL. From any state, conditions returning to acceptable ranges transition toward CLEAR.

The transition toward CLEAR follows the reverse path. CRITICAL to WARNING to CLEAR, or WARNING directly to CLEAR depending on the hysteresis configuration.

### Evaluation Scope

Alert evaluation occurs within a scope defined by the alert configuration. The `on` line specifies which chart the alert applies to. The `lookup` line defines which dimensions and time windows to evaluate. The `every` line defines how frequently to re-evaluate.

Single-host alerts apply only to the local host. The health engine does not evaluate alerts against metrics from other nodes. This isolation is intentional; it keeps alert evaluation local and avoids the complexity of distributed state.

Template alerts apply to all charts matching a context. A template defined for the `system.cpu` context applies to the system.cpu chart on every host. Templates enable consistent monitoring without explicit per-host configuration.

### Evaluation Frequency Control

The `every` line controls evaluation frequency. An alert with `every 1m` is evaluated once per minute; an alert with `every 5s` is evaluated five times per second. More frequent evaluation provides faster detection of transient problems but increases computational cost.

Balance between responsiveness and resource usage. For alerts on rapidly changing metrics, frequent evaluation catches brief anomalies. For stable metrics, less frequent evaluation saves resources without sacrificing detection capability.

The health engine processes alerts in order defined by configuration. Heavy alert configurations may require attention to evaluation timing, but for most deployments, the health engine has ample capacity to process all alerts within each evaluation interval.

## 13.2 Alert Configuration Layers

Netdata supports multiple configuration layers for health alerts. Understanding precedence rules helps in making modifications that take effect as intended.

### Stock Configuration Layer

Stock alerts are distributed with Netdata and reside in `/usr/lib/netdata/conf.d/health.d/`. These files are installed by the Netdata package and updated with each release. Modifying stock files is not recommended because changes are overwritten during upgrades.

Stock configurations define the default alert set. They are evaluated last in precedence, meaning custom configurations override stock configurations for the same alert.

Stock alerts should be treated as reference implementations. Copy stock alerts to the custom layer before modifying them. This preserves the original for comparison and ensures that upgrade migration can identify necessary adjustments.

### Custom Configuration Layer

Custom alerts reside in `/etc/netdata/health.d/`. Files in this directory take precedence over stock configurations for the same alert names. This layer is preserved during upgrades and should contain all site-specific alert modifications.

The custom layer supports complete replacement of stock alerts. An alert defined in `/etc/netdata/health.d/` with the same name as a stock alert replaces the stock definition entirely. This includes all lines of the alert definition, not just the lines being modified.

For selective modification of stock alerts without full replacement, use configuration overrides in dedicated files. Define only the lines being changed and inherit the remaining configuration from the stock layer.

### Cloud Configuration Layer

Netdata Cloud can define alerts through the Alerts Configuration Manager. These Cloud-defined alerts exist independently of local configuration files and take precedence over both stock and custom layers.

Cloud alerts are stored remotely and synchronized to Agents on demand. They appear in the local health configuration but originate from Cloud. This layer exists to support the Cloud UI while maintaining Agent autonomy.

Cloud-defined alerts can be modified through the Cloud UI. Changes are synchronized to connected Agents without requiring local file access. This enables centralized alert management across multiple nodes.

### Configuration File Merging

At startup and after configuration changes, Netdata merges all configuration layers into a single effective configuration. The merge applies precedence rules to resolve conflicts.

When evaluating the effective configuration, Netdata treats stock alerts as defaults and custom alerts as overrides. Cloud alerts as highest priority add or override further. The result is a single coherent configuration applied uniformly across all evaluation cycles.

Use `netdatacli health configuration` to view the effective merged configuration. This command helps diagnose discrepancies between intended and actual alert behavior.

## 13.3 Alert Lifecycle and State Transitions

Understanding alert lifecycle helps in troubleshooting and in designing effective alert strategies.

### Initial Evaluation

When an alert definition is first loaded, it enters the UNINITIALIZED state. This state persists until sufficient historical data exists to evaluate conditions. The required data volume varies by alert and lookup window.

During UNINITIALIZED, no notifications are sent. The alert exists but has not yet established a baseline for comparison. This warm-up period prevents false positives from triggering on startup conditions.

When enough data accumulates, the alert transitions to CLEAR. The first CLEAR evaluation establishes the baseline state. From this point, subsequent evaluations track changes from baseline.

### State Transition Logic

Each evaluation applies transition logic defined in the alert configuration. The fundamental logic compares calculated values against warning and critical thresholds.

When transitioning from CLEAR to WARNING, the alert fires with WARNING status. This firing generates events and potentially notifications, depending on repeat interval configuration.

When transitioning from WARNING to CRITICAL, the alert fires again with CRITICAL status. The severity escalation signals increased urgency and may trigger different notification channels.

When transitioning toward CLEAR, the alert fires with CLEAR status. This firing indicates resolution and may trigger resolution notifications if configured.

### Repeat Interval and Notification Suppression

Alerts do not fire on every evaluation cycle for sustained conditions. The repeat interval defines how frequently notifications are sent for ongoing alert states.

A repeat interval of `1h` means that at most one notification per hour is sent for an ongoing alert condition. This prevents notification storms while maintaining awareness of unresolved problems.

Repeat intervals are configured per alert using the `repeat` line. Different alerts may have different repeat intervals based on their urgency and expected resolution timeframes.

### Alert Removal and Recovery

When an alert is deleted from configuration, it enters the REMOVED state. This state is recorded for historical purposes but does not trigger new notifications.

Resolution is distinct from removal. A CLEAR firing indicates that conditions have returned to normal. The alert continues evaluating and will fire again if conditions deteriorate.

Long-running alerts that transition to CLEAR should trigger resolution notifications. These notifications close the loop on incident tracking and provide confidence that problems have been addressed.

## 13.4 Notification Dispatch Architecture

When an alert fires, the notification system handles delivery to configured destinations. The dispatch architecture supports multiple notification methods and provides reliability features for delivery.

### Notification Queue

Alert events enter a notification queue for asynchronous delivery. This queuing prevents alert evaluation from blocking on slow notification destinations. The queue has configurable size limits that protect system resources.

Queue overflow causes event loss. When the queue fills, new events are discarded rather than blocking alert evaluation. This backpressure mechanism ensures that alert evaluation continues even when notification delivery is degraded.

Monitor queue utilization to detect notification delivery problems. A persistently full queue indicates delivery failures that require investigation.

### Notification Methods

Netdata supports multiple notification methods including email, Slack, PagerDuty, Discord, Telegram, and others. Each method has a dedicated handler that formats and delivers notifications.

Configuration files in `/etc/netdata/` define notification settings. The `health_alarm_notify.conf` file configures method-specific settings like API keys, channel identifiers, and recipient mappings.

Notification methods are invoked asynchronously. Alert evaluation continues while notifications are delivered in the background. This parallelism ensures fast evaluation cycles regardless of notification delivery time.

### Delivery Reliability

Notification delivery is best-effort with configurable retry behavior. Failed delivery attempts are logged for troubleshooting. Persistent failures may require manual intervention.

Critical notification delivery should use redundant paths. Configure multiple notification methods for critical alerts to ensure at least one path succeeds.

### Escalation and Routing

Notification routing determines which recipients receive which alerts. Routing rules are defined in notification method configurations and can filter by alert name, chart, host, or severity.

Escalation policies route unacknowledged alerts to secondary recipients after timeout periods. This ensures that critical alerts eventually reach someone even if the primary responder is unavailable.

Configure escalation explicitly for all critical alerts. The default behavior routes to configured recipients but does not escalate on silence.

## 13.5 Scaling Alerting in Complex Topologies

Large deployments face different scaling challenges than single-node installations. Netdata supports various topologies with different alerting characteristics.

### Agent-Only Topologies

In agent-only topologies, every node runs alert evaluation locally. This model is simple and scales horizontally by adding more nodes. Each node evaluates alerts independently for its own metrics.

Alert volume scales with node count. A thousand-node deployment generates alerts from a thousand independent evaluation cycles. This volume requires thoughtful alert design to avoid overwhelming operators.

Use aggregate views in Netdata Cloud to monitor across nodes. Aggregated views show the collective alert status without requiring individual node examination.

### Parent-Child Topologies

Parent-based topologies centralize some alert evaluation on parent nodes. Child nodes stream metrics to parents; parents evaluate alerts for received metrics.

This topology reduces evaluation overhead on child nodes. Children focus on collection and streaming; parents handle aggregation and evaluation. The tradeoff is increased parent resource requirements.

Design parent-based alerting to respect organizational boundaries. Alerts should fire on the node that owns the affected service or component. Centralizing all evaluation on parents creates single points of failure and obscures ownership.

### Cloud Integration

Netdata Cloud integration adds remote components to the alerting architecture. Cloud receives alert events from Agents and Parents, providing unified views across infrastructure.

Cloud-based alerts can be defined and managed remotely through the Cloud UI. These Cloud-defined alerts are synchronized to connected Agents and take precedence over local configurations.

Cloud integration does not change local evaluation behavior. Alerts still fire based on local metric values; Cloud receives events for display and notification purposes.

### Multi-Region Considerations

Deployments spanning multiple regions should consider alert evaluation latency. Metrics may lag between regions due to network constraints. Alert evaluation uses locally available data, which may be stale for cross-region views.

Design regional alerting to be self-contained. Each region should have alerting capable of detecting local problems without depending on cross-region data freshness.

## 13.6 Performance and Resource Considerations

Alert evaluation consumes CPU and memory resources. Understanding resource usage helps in capacity planning and optimization.

### CPU Usage

Alert evaluation is CPU-intensive. Each evaluation iterates through all enabled alerts, executing lookup queries and calculation expressions.

The evaluation interval affects CPU usage inversely. More frequent evaluation increases CPU consumption but reduces detection latency. Less frequent evaluation saves resources at the cost of detection speed.

Profile alert performance using `netdata-agent-bench` to identify heavy alerts. Optimization targets are alerts with complex expressions, frequent lookups, or expensive transformations.

### Memory Usage

Alert configuration consumes memory proportional to alert count and complexity. Each alert instance requires space for expression evaluation state, variable contexts, and historical lookups.

Complex lookups that retrieve large time windows consume more memory than simple lookups. Balance detection requirements against memory constraints.

### Evaluation Latency

The health engine should complete evaluation within its configured interval. An evaluation cycle that exceeds its interval causes evaluation backlog, which delays alert firing.

Monitor evaluation latency using `netdatacli health status`. The output shows recent evaluation timing and can identify backlogs before they impact detection.

When evaluation latency approaches the evaluation interval, reduce alert complexity or increase the interval. Chronic backlog indicates that alert count exceeds system capacity.

## 13.7 Debugging and Troubleshooting

Alert problems often manifest as alerts not firing when expected or firing incorrectly. Systematic debugging identifies root causes.

### Checking Alert Configuration

Verify that alerts are loaded with `netdatacli health configuration`. The output shows all enabled alerts and their current definitions. Compare expected and actual configurations to identify discrepancies.

Check that alerts are enabled. Disabled alerts never fire regardless of conditions. Use `netdatacli health enabled` to list enabled status.

### Examining Alert Values

Query alert values with `netdatacli health alarm values`. This command shows current variable values for firing alerts, enabling diagnosis of threshold comparisons.

Examine lookup results separately using the query API. Compare query results against alert thresholds to identify threshold configuration problems.

### Reviewing Logs

Alert evaluation logs contain detailed information about firing decisions. Check `/var/log/netdata/error.log` for evaluation diagnostics.

Filter logs by alert name to focus on specific problems. The log output includes variable values, calculation results, and state transitions.

### Testing Alert Firing

Force an alert to evaluate by temporarily lowering thresholds. This active testing confirms that the alert mechanism works correctly.

Use the health test API to simulate alert conditions without waiting for actual threshold crossings. This capability accelerates troubleshooting.

## Related Chapters

- **1. Understanding Alerts in Netdata**: Foundational concepts
- **9. APIs for Alerts and Events**: Programmatic access to alert data
- **12. Best Practices for Alerting**: Design guidance for effective alerting