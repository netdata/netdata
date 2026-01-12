# 1.1 What is a Netdata Alert?

A **Netdata alert** is a rule that continuously monitors one or more metrics from charts and assigns a **status** based on whether configured conditions are met. Alerts are the core of Netdata's health monitoring system, acting as component-level watchdogs that evaluate metrics at regular intervals.

Each alert instance is uniquely identified by the combination of **node**, **alert name**, and **chart instance**. This means you cannot currently have a single alert instance that uses metrics data from multiple chart instances at once.

## How Alerts Work

At each evaluation cycle (typically every 10 seconds, configurable per alert via the `every` line):

1. The **health engine** running on the Agent reads metrics data from the relevant chart(s)
2. It applies the alert's logic (for example, "average CPU over the last 5 minutes > 80%")
3. It assigns a **status** to that alert based on the result
4. If the status changes (for example, from `CLEAR` to `WARNING`), this transition becomes an **alert event**

Alert events are:
- Visible in the local Agent dashboard
- Queryable via Agent APIs (`/api/v1/alerts`, `/api/v2/alerts`)
- Sent to Netdata Cloud and displayed in the Events Tab
- Used to trigger notifications (email, Slack, PagerDuty, etc.)

## Alert Statuses

Every alert has one of six possible statuses at any given time:

| Status | Meaning |
|--------|---------|
| **UNINITIALIZED** | The alert has just been created but has not yet collected enough data to evaluate |
| **UNDEFINED** | The alert could not be evaluated (missing data, division by zero, NaN/Inf result) |
| **CLEAR** | The alert can be evaluated and neither warning nor critical thresholds are triggered; the metric is in a normal, healthy state |
| **WARNING** | The warning expression evaluated to true; something may require attention |
| **CRITICAL** | The critical expression evaluated to true; if both warning and critical conditions are met, CRITICAL takes precedence |
| **REMOVED** | The alert has been deleted (configuration reload, child node disconnected, obsolete chart instance, or the device/service is no longer there) |

### Status Lifecycle

Alerts typically flow through this lifecycle:
```
UNINITIALIZED → UNDEFINED / CLEAR → WARNING → CRITICAL
```

- New alerts start in `UNINITIALIZED` until enough data is collected
- Once data is available, they move to `CLEAR` (healthy) or `UNDEFINED` (cannot evaluate)
- From `CLEAR`, they can transition to `WARNING` or `CRITICAL` when conditions are met
- They return to `CLEAR` when conditions are no longer met
- `REMOVED` appears when an associated chart instance is no longer getting new data (obsolete device, service, or configuration reload)

## How Alerts Relate to Charts and Contexts

Netdata alerts inspect **metrics from charts**. Each chart represents a specific metric or set of related dimensions (for example, CPU usage, disk I/O, network traffic).

There are two ways alerts can be attached to charts:

### 1. Chart-Specific Alerts (Alarms)

- An **alarm** is attached to a **single instance** of a chart
- Example: "Alert if `eth0` network interface bandwidth > 90%"
- Use when you need a rule for one specific chart

### 2. Context-Based Alerts (Templates)

- A **template** applies the same alert logic to **all charts that match a context**
- Example: "Alert if _any_ network interface bandwidth > 90%"
- Use when you want the same rule to monitor all instances of a type (all disks, all network interfaces, all containers, etc.)

**The difference:**
- **Alarm** = one rule → one chart → one alert instance
- **Template** = one rule → all matching charts → **multiple alert instances** (one per chart)

Both types inspect metric values from charts in the same way; the difference is in **scope** (specific vs. all matching instances).

:::note

Detailed syntax for defining alarms vs templates is covered in **1.2 Alert Types: `alarm` vs `template`** and **Chapter 3: Alert Configuration Syntax**.

:::

## Where Alert Evaluation Happens

As established in the chapter introduction:
- Alerts are **evaluated locally** on each Netdata Agent
- The health engine runs **on the Agent** that has the metrics, not centrally in Netdata Cloud
- Cloud receives alert events and presents them in a unified view, but does not re-evaluate the rules

This distributed model means:
- Alert logic runs at the edge, close to the data, with minimal latency
- Each **Agent** independently decides alert status based on the metrics it holds for a given **node**
- You can still see alerts in the Agent dashboard and receive notifications even without Cloud connectivity (though Cloud won't receive or display them)

For a given alert definition applied to a given chart instance, there is one **alert instance** per **node instance** (the combination of Claim ID and Node ID representing a specific node). Cloud aggregates the status for an alert from multiple node instances by taking the highest severity status (`CLEAR` < `WARNING` < `CRITICAL`) across them.

## Key Takeaways

- A Netdata alert is a **rule that monitors metrics from charts** and assigns a status
- Alerts have **six possible statuses**: `UNINITIALIZED`, `UNDEFINED`, `CLEAR`, `WARNING`, `CRITICAL`, `REMOVED`
- Status transitions become **alert events** visible locally and in Netdata Cloud
- Alerts can be **chart-specific (alarms)** or **context-based (templates)**
- Alert evaluation happens **on the Agent**, not in Cloud
- Each template rule creates **multiple alert instances** (one per chart instance)

## What's Next

- **1.2 Alert Types: `alarm` vs `template`** Detailed explanation of chart-specific vs context-based rules
- **1.3 Where Alerts Live (Files, Agent, Cloud)** File paths, stock vs custom alerts, Cloud integration
