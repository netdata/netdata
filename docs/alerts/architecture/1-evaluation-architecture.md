# 13.1 Alert Evaluation Architecture

Alert evaluation is the process of checking metric values against configured conditions and determining alert status. This process happens entirely on the Agent or Parent node that owns the metrics.

:::note

Alert evaluation is local to each Agent. Netdata Cloud receives state changes but does not re-evaluate alerts.

:::

## 13.1.1 Metric Collection Pipeline

Metrics flow from collectors through the Netdata database and into the health engine. Collectors gather raw data from system APIs, application endpoints, or external services. This raw data is formatted as dimension values and stored in the round-robin database.

| Pipeline Stage | Description |
|---------------|-------------|
| **Collectors** | Gather raw data from APIs, applications |
| **Round-robin database** | Store time-series data |
| **Health engine** | Evaluate alerts against stored data |
| **State machine** | Track alert status transitions |

The database maintains per-second resolution for recent time windows and aggregated data for longer periods. Alert lookups query this database to retrieve historical values for comparison against thresholds.

The health engine runs on a configurable interval, typically every second. Each iteration evaluates all enabled alerts against recent metric values.

## 13.1.2 The Alert State Machine

Every alert instance exists in one of several states defined by the health state machine.

| State | Description | Action Required |
|-------|-------------|----------------|
| **UNINITIALIZED** | Insufficient data to evaluate | Wait for warm-up |
| **CLEAR** | Conditions acceptable | No action needed |
| **WARNING** | Exceeded warning threshold | Investigate |
| **CRITICAL** | Exceeded critical threshold | Immediate response |
| **UNDEFINED** | Evaluation error | Check configuration |
| **REMOVED** | Alert deleted | Remove from monitoring |

The `UNINITIALIZED` state indicates that the alert has not yet received enough data to evaluate. Most alerts require a warm-up period to accumulate historical values.

The `CLEAR` state indicates that conditions are within acceptable parameters. No alert has fired, and no action is required.

The `WARNING` state indicates that a condition has exceeded the warning threshold.

The `CRITICAL` state indicates that a condition has exceeded the critical threshold.

The `UNDEFINED` state indicates that the alert encountered an error during evaluation.

The `REMOVED` state indicates that the alert has been deleted or the configuration has been removed.

## 13.1.3 Evaluation Scope

Alert evaluation occurs within a scope defined by the alert configuration. The `on` line specifies which chart the alert applies to. The `lookup` line defines which dimensions and time windows to evaluate.

| Scope Type | Description |
|------------|-------------|
| **Single-host** | Applies only to local host |
| **Template** | Applies to all charts matching context |
| **Multi-instance** | Scoped to specific dimensions |

Single-host alerts apply only to the local host. Template alerts apply to all charts matching a context.

## 13.1.4 Evaluation Frequency Control

The `every` line controls evaluation frequency. Balance between responsiveness and resource usage.

| Alert Type | Recommended Frequency |
|------------|----------------------|
| **Rapidly changing metrics** | Every 1-5 seconds |
| **Stable metrics** | Every 30-60 seconds |
| **Slow-changing metrics** | Every 5-15 minutes |

For alerts on rapidly changing metrics, frequent evaluation catches brief anomalies; for stable metrics, less frequent evaluation saves resources.

## Related Sections

- [13.2 Configuration Layers](4-configuration-layers.md) - How configurations are merged
- [13.4 Notification Dispatch](3-notification-dispatch.md) - How notifications are delivered
- [13.5 Scaling Topologies](5-scaling-topologies.md) - Behavior in distributed setups