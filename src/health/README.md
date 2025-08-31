# Alerts and Notifications in Netdata

## Introduction to Alerting

Netdata provides a **distributed, real-time health monitoring framework** that evaluates conditions against your metrics and executes actions on state transitions. You can configure notifications as one of these actions.

Unlike traditional monitoring systems, Netdata evaluates alerts simultaneously at multiple levels - on the edge (Agents), at aggregation points (Parents), and deduplicates them in Netdata Cloud. This allows your teams to implement different alerting strategies at different infrastructure levels.

### Understanding Alerts in Netdata

Netdata alerts function as **component-level watchdogs**. You attach them to specific components/instances (network interfaces, database instances, web servers, containers, processes) where they evaluate metrics at configurable intervals.

To simplify your configuration, you can define alert templates once and apply them to all matching components. The system matches instances by host labels, instance labels, and names, allowing you to define the same alert multiple times with different matching criteria.

Each alert provides a name, value, unit, and status - making them easy to display in dashboards and send as meaningful notifications regardless of your infrastructure's complexity.

### Where Your Alerts Run

Your alerts evaluate at the edge. Every Netdata Agent and Parent runs alerts on the metrics it processes and stores (enabled by default, but you can disable alerting at any level). When you stream metrics to a Parent, the Parent evaluates its own alerts on those metrics independently of the child's alerts. Each Agent maintains its own alert configuration and evaluates alerts autonomously. Metric streaming doesn't propagate alert configurations or transitions to Parents.

```
┌─────────┐     Metrics      ┌──────────┐     Metrics      ┌──────────┐
│  Child  │ ───────────────> │ Parent 1 │ ───────────────> │ Parent 2 │
│  Agent  │     of child     │  Agent   │    of child +    │  Agent   │
└────┬────┘                  └────┬─────┘      Parent 1    └────┬─────┘
     │                            │                             │
     │ Evaluates alerts on        │ Evaluates alerts on         │ Evaluates alerts on
     │ local metrics              │ child + local metrics       │ all streamed + local
     │                            │                             │
     ▼                            ▼                             ▼
  Alerts                        Alerts                        Alerts
```

### Alert Actions and Notifications

Your Netdata Agents treat notifications as **actions** triggered by alert status transitions. Agents can dispatch notifications or perform automation tasks like scaling services, restarting processes, or rotating logs. Actions are shell scripts or executable programs that receive all alert transition metadata from Netdata.

When you claim Agents to Netdata Cloud, they send their alert configurations and transitions to Cloud, which deduplicates them (merging multiple transitions from different Agents for the same host). Netdata Cloud triggers notifications centrally through its integrations (Slack, Microsoft Teams, Amazon SNS, PagerDuty, OpsGenie).

Netdata Cloud's intelligent deduplication works by:
- **Consolidating multiple Agents** reporting the same alert
- **Prioritizing highest severity**: CRITICAL > WARNING > CLEAR
- **Creating unique keys**: Alert name + Instance + Node

Your Agents and Netdata Cloud trigger actions independently using their own configurations and integrations.

This design enables you to:
1. **Maintain team independence**: Different teams run their own Parents with custom alerts
2. **Implement edge intelligence**: Critical alerts trigger automations directly on nodes
3. **Scale naturally**: Alert evaluation distributes with your infrastructure
4. **Mix strategies**: Combine edge, regional, and central alerting

### Quick Example

```yaml
Web Server (Child):
  - Alert: system CPU > 80% triggers scale out
  - Alert: process X memory > 90% restarts process X

DevOps Parent:
  - Alert: Response time > 500ms across all web servers
  - Alert: Error rate > 1% for any service

SRE Parent:
  - Alert: Anomaly detection on traffic patterns
  - Alert: Capacity planning thresholds

Netdata Cloud:
  - Receives all alert transitions
  - Deduplicates overlapping alerts
  - Shows CRITICAL if any instance reports CRITICAL
  - Provides unified view for incident response
```

Each level operates independently while Netdata Cloud provides a coherent, deduplicated view of your entire infrastructure's health (when all agents connect directly to Cloud).

## Managing Alert Configuration

You configure Netdata alerts in 3 layers:

1. **Stock Alerts**: Netdata provides hundreds of alert definitions in `/usr/lib/netdata/conf.d/health.d` to detect common issues. Don't edit these directly - updates will overwrite your changes.
2. **Your Custom Alerts**: Create your own definitions in `/etc/netdata/health.d`.
3. **Dynamic UI Configuration**: Use Netdata dashboards to edit, add, enable, or disable alerts on any node through the streaming transport.

## Managing Notification Configuration

You can configure notifications for any infrastructure node at 3 levels:

| Level              | What It Evaluates           | Where Notifications Come From | Use Case                          | Documentation                                                                                                               |
|--------------------|-----------------------------|--------------------|-----------------------------------|-----------------------------------------------------------------------------------------------------------------------------|
| **Netdata Agent**  | Local Metrics               | Netdata Agent      | Edge automation                   | [Agent integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications)  |
| **Netdata Parent** | Local and Children Metrics  | Netdata Parent     | Edge automation                   | [Agent integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications)  |
| **Netdata Cloud**  | Receives Transitions        | Netdata Cloud      | Web-hooks, role/room based | [Cloud integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/centralized-cloud-notifications) |

:::note
When using Parents and Cloud with default settings, you may receive duplicate email notifications. Agents send emails by default when an MTA exists on their systems. Disable email notifications on Agents and Parents when using Cloud by setting `SEND_EMAIL="NO"` in `/etc/netdata/health_alarm_notify.conf` [using `edit-config`](/docs/netdata-agent/configuration/README.md).
:::

### Best Practices for Large Deployments

#### Central Alerting Strategy

When you:
- Don't need edge automation (no scripts reacting to alerts)
- Use highly available Parents for all nodes
- Use Netdata Cloud (at least for Parents)

Follow these steps:
1. Disable health monitoring on child nodes
2. Share the same alert configuration across Parents (use git repo or CI/CD)
3. Disable Parent notifications (`SEND_EMAIL="NO"` in `/etc/netdata/health_alarm_notify.conf`)
4. Keep only Cloud notifications

This emulates traditional monitoring tools where you configure alerts centrally and dispatch notifications centrally.

#### Edge Flexible Alerting Strategy

When you:
- Need edge automation (scale out, restart processes)
- Use Parents
- Use Cloud for all nodes

Follow these steps:
- Disable stock alerts on children (`enable stock health configuration` to `no` in `/etc/netdata/netdata.conf` `[health]` section)
- Configure only automation-required alerts on children
- Keep stock alerts on Parents but disable notifications (`SEND_EMAIL="NO"`)
- Keep only Cloud notifications

This enables edge automation on children while maintaining central alerting control and deduplicated Cloud notifications.

## Set Up Alerts via Netdata Cloud

1. Connect your nodes to [Netdata Cloud](https://app.netdata.cloud/)
2. Navigate to: `Space → Notifications`
3. Choose your integration (Slack, Amazon SNS, Splunk)
4. Configure alert severity filters

[View all Cloud integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/centralized-cloud-notifications)

## Set Up Alerts via Netdata Agent

1. Open notification config:
   ```bash
   sudo ./edit-config health_alarm_notify.conf
   ```

2. Enable your method (example: email):
   ```ini
   SEND_EMAIL="YES"
   DEFAULT_RECIPIENT_EMAIL="you@example.com"
   ```

3. Verify your system can send mail (sendmail, SMTP relay)

4. Restart the agent:
   ```bash
   sudo systemctl restart netdata
   ```

[View all Agent integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications)

## Core Alerting Concepts

Netdata supports two alert types:
- **Alarms**: Attach to specific instances (specific network interface, database instance)
- **Templates**: Apply to all matching instances (all network interfaces, all databases)

### Alert Lifecycle and States

Your alerts produce more than threshold checks. Each generates:
- **A value**: Combines metrics or other alerts using time-series lookups and expressions
- **A unit**: Makes alerts meaningful ("seconds", "%", "requests/s")
- **A name**: Identifies the alert

This enables sophisticated alerts like:
- `out of disk space time: 450 seconds` - Predicts when disk fills based on current rate
- `3xx redirects: 12.5 percent` - Calculates redirects as percentage of total
- `response time vs yesterday: 150%` - Compares current to historical baseline

### Alert States

Your alerts exist in one of these states:

| State             | Description                                       | Trigger                                                                  |
|-------------------|---------------------------------------------------|--------------------------------------------------------------------------|
| **CLEAR**         | Normal - conditions exist but not triggered | Warning and critical conditions evaluate to zero                         |
| **WARNING**       | Warning threshold exceeded                        | Warning condition evaluates to non-zero                                  |
| **CRITICAL**      | Critical threshold exceeded                       | Critical condition evaluates to non-zero                                 |
| **UNDEFINED**     | Cannot evaluate                                   | No conditions defined, or value is NaN/Inf                               |
| **UNINITIALIZED** | Never evaluated                                   | Alert just created                                                      |
| **REMOVED**       | Alert deleted                                     | Child disconnected, agent exit, or health reload                         |

Alerts transition freely between states based on:
- **Calculated value** (including NaN, Inf, or valid numbers)
- **Warning/critical conditions** (evaluation results)
- **External events** (disconnections, reloads, exits)

Key behaviors:
- Alerts jump directly from CLEAR to CRITICAL (no WARNING required)
- WARNING and CRITICAL evaluate independently
- Alerts return to appropriate state when data becomes available
- CRITICAL takes precedence when both conditions are true

### Alert Evaluation Process

#### 1. Calculate Value

Your alerts perform complex calculations:

```
     lookup              calc             warn,crit           status
   ┌──────────┐       ┌──────────┐       ┌──────────┐      ┌───────────┐
   │ Database │       │Expression│       │ Warning  │      │  Execute  │
   │  Query   │──────>│Processor │──────>│ Critical │ ───> │ Action on │
   │(optional)│ $this │(optional)│ $this │  Checks  │      │Transition │
   └──────────┘       └──────────┘       └──────────┘      └───────────┘
```

Examples:
```yaml
# Simple threshold
calc: $used
# Result: $this = latest value of dimension 'used'

# Time-series lookup
lookup: average -1h of used
# Result: $this = average of 'used' over last hour

# Combined calculation
lookup: average -1h of used
calc: $this * 100 / $total
# Result: $this = percentage of hourly average vs total

# Baseline comparison
lookup: average -1h of used
calc: $this * 100 / $average_yesterday
# Result: $this = percentage vs yesterday's average
```

#### 2. Evaluate Conditions

After calculating, check conditions:

```yaml
# Simple conditions
warn: $this > 80
crit: $this > 90

# Flapping prevention
warn: ($status >= $WARNING) ? ($this > 50) : ($this > 80)
crit: ($status == $CRITICAL) ? ($this > 70) : ($this > 90)

# Complex conditions
warn: $this > 80 AND $rate > 10
crit: $this > 90 OR $failures > 5
```

#### 3. Determine State

Each condition evaluates to:
- NaN or Inf → UNDEFINED
- Non-zero → RAISED
- Zero → CLEAR

Final status:
- Critical RAISED → **CRITICAL** (priority)
- Warning RAISED → **WARNING**
- Either CLEAR → **CLEAR**
- Both missing/UNDEFINED → **UNDEFINED**

### Evaluation Timing

Alert evaluation runs independently from data collection:

```
Data Collection         Alert Evaluation
     │                        │
     ▼ every 1s               ▼ configurable interval
  [Metrics] ──────────> [Alert Engine]
                              │
                              ▼
                        Query metrics,
                        Calculate values,
                        Check conditions
```

- **Default interval**: Query window duration (with lookup) or manual setting required
- **Configurable**: Use `every` for custom intervals
- **Constrained**: Cannot evaluate faster than data collection frequency

### Anti-Flapping Mechanisms

Netdata prevents alert flapping through:

#### 1. Hysteresis
```yaml
warn: ($status < $WARNING) ? ($this > 80) : ($this > 50)
```
Triggers at 80, clears at 50, preventing flapping between 50-80.

#### 2. Dynamic Delays
Alerts transition immediately in dashboards but notifications use exponential backoff.

#### 3. Duration Requirements
```yaml
lookup: average -10m of used
warn: $this > 80
```
Requires 10 minutes of data before triggering.

### Multi-Stage Alerts

Create dependent alerts:

```yaml
# Stage 1: Baseline
template: requests_average_yesterday
      on: web_log.requests
  lookup: average -1h at -1d
   every: 10s

# Stage 2: Current
template: requests_average_now
      on: web_log.requests
  lookup: average -1h
   every: 10s

# Stage 3: Compare
template: web_requests_vs_yesterday
      on: web_log.requests
    calc: $requests_average_now * 100 / $requests_average_yesterday
   units: %
    warn: $this > 150 || $this < 75
    crit: $this > 200 || $this < 50
```

### Available Variables

Variables resolve in order (first match wins):

#### 1. Built-in Variables

| Variable            | Description               | Value              |
|---------------------|---------------------------|--------------------|
| `$this`             | Current calculated value  | Result from lookup/calc |
| `$after`            | Query start timestamp     | Unix timestamp     |
| `$before`           | Query end timestamp       | Unix timestamp     |
| `$now`              | Current time              | Unix timestamp     |
| `$last_collected_t` | Last collection time      | Unix timestamp     |
| `$update_every`     | Collection frequency      | Seconds            |
| `$status`           | Current status code       | -2 to 3            |
| `$REMOVED`          | Status constant           | -2                 |
| `$UNINITIALIZED`    | Status constant           | -1                 |
| `$UNDEFINED`        | Status constant           | 0                  |
| `$CLEAR`            | Status constant           | 1                  |
| `$WARNING`          | Status constant           | 2                  |
| `$CRITICAL`         | Status constant           | 3                  |

#### 2. Dimension Values

| Syntax                             | Description                      | Example          |
|------------------------------------|----------------------------------|------------------|
| `$dimension_name`                  | Last normalized value            | `$used`          |
| `$dimension_name_raw`              | Last raw collected value         | `$used_raw`      |
| `$dimension_name_last_collected_t` | Collection timestamp             | `$used_last_collected_t` |

```yaml
template: disk_usage_percent
      on: disk.space
    calc: $used * 100 / ($used + $available)
   units: %
```

#### 3. Chart Variables
```yaml
calc: $used > $threshold  # If chart defines 'threshold'
```

#### 4. Host Variables
```yaml
warn: $connections > $max_connections * 0.8  # If host defines 'max_connections'
```

#### 5. Other Alerts
```yaml
# Alert 1
template: cpu_baseline
    calc: $system + $user

# Alert 2
template: cpu_check
    calc: $system
    warn: $this > $cpu_baseline * 1.5
```

#### 6. Cross-Context References
```yaml
template: disk_io_vs_iops
      on: disk.io
    calc: $reads / ${disk.iops.reads}
   units: bytes per operation
```

### Variable Resolution and Label Scoring

When alerts reference variables matching multiple instances, Netdata uses label similarity scoring:

1. **Collect candidates** with matching names
2. **Score by labels** - count common labels
3. **Select best match** - highest label overlap

Example: Alert on `disk.io` (labels: `device=sda`, `mount=/data`) references `${disk.iops.reads}`:
- `disk.iops` for sda (labels match) → Score: 2
- `disk.iops` for sdb (no match) → Score: 0
Result: Uses sda's value

### Missing Data Handling

During lookups with missing data:
- **All values NULL**: `$this` becomes `NaN`
- **Some values exist**: Ignores NULL, continues calculation
- **Dimension doesn't exist**: `$this` becomes `NaN`

This handles intermittent collection, dynamic dimensions, and partial outages.

### Evaluation Frequency

Determine frequency by:

1. **With lookup**: Defaults to window duration
   ```yaml
   lookup: average -5m  # Evaluates every 5 minutes
   ```

2. **Without lookup**: Set explicitly
   ```yaml
   every: 10s
   calc: $system + $user
   ```

3. **Custom interval**: Override default
   ```yaml
   lookup: average -1m
   every: 10s  # Check every 10s despite 1m window
   ```

Constraints:
- Cannot exceed data collection frequency
- High frequency impacts performance
- Use larger intervals with `unaligned` for efficiency

## Troubleshooting Your Alerts

### Netdata Assistant

The [Netdata Assistant](https://learn.netdata.cloud/docs/machine-learning-and-anomaly-detection/ai-powered-troubleshooting-assistant) provides AI-powered troubleshooting when alerts trigger:

1. Click the alert in your dashboard
2. Press the Assistant button
3. Receive customized troubleshooting tips

The Assistant window follows you through dashboards for easy reference while investigating.

### Community Resources

Visit our [Alerts Troubleshooting space](https://community.netdata.cloud/c/alerts/28) for complex issues. Get help through [GitHub](https://github.com/netdata/netdata) or [Discord](https://discord.gg/kUk3nCmbtx). Share your solutions to help others.

### Customizing Alerts

Tune alerts for your environment by adjusting thresholds, writing custom conditions, silencing alerts, and using statistical functions.

- [Customize alerts](/src/health/REFERENCE.md)
- [Silence or disable alerts](/src/health/REFERENCE.md#how-to-disable-or-silence-alerts)

## Related Documentation

- [All notification methods](/docs/alerts-and-notifications/notifications/README.md)
- [Supported collectors](/src/collectors/COLLECTORS.md)
- [Full alert reference](/src/health/REFERENCE.md)
