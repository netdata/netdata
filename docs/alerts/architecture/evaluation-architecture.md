# 13.1 Alert Evaluation Architecture

Alert evaluation is the process of checking metric values against configured conditions and determining alert status. This process happens entirely on the Agent or Parent node that owns the metrics.

## Metric Collection Pipeline

Metrics flow from collectors through the Netdata database and into the health engine. Collectors gather raw data from system APIs, application endpoints, or external services. This raw data is formatted as dimension values and stored in the round-robin database.

The database maintains per-second resolution for recent time windows and aggregated data for longer periods. Alert lookups query this database to retrieve historical values for comparison against thresholds.

The health engine runs on a configurable interval, typically every second. Each iteration evaluates all enabled alerts against recent metric values.

## The Alert State Machine

Every alert instance exists in one of several states defined by the health state machine.

The `UNINITIALIZED` state indicates that the alert has not yet received enough data to evaluate. Most alerts require a warm-up period to accumulate historical values.

The `CLEAR` state indicates that conditions are within acceptable parameters. No alert has fired, and no action is required.

The `WARNING` state indicates that a condition has exceeded the warning threshold.

The `CRITICAL` state indicates that a condition has exceeded the critical threshold.

The `UNDEFINED` state indicates that the alert encountered an error during evaluation.

The `REMOVED` state indicates that the alert has been deleted or the configuration has been removed.

## Evaluation Scope

Alert evaluation occurs within a scope defined by the alert configuration. The `on` line specifies which chart the alert applies to. The `lookup` line defines which dimensions and time windows to evaluate.

Single-host alerts apply only to the local host. Template alerts apply to all charts matching a context.

## Evaluation Frequency Control

The `every` line controls evaluation frequency. Balance between responsiveness and resource usage. For alerts on rapidly changing metrics, frequent evaluation catches brief anomalies; for stable metrics, less frequent evaluation saves resources.