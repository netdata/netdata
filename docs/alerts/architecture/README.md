# Chapter 13: Alert Architecture

Understanding how alerts work under the hood helps with troubleshooting, optimization, and designing effective notification strategies. This chapter covers the evaluation pipeline, state machine, and how alerts interact with the broader Netdata ecosystem.

:::note

Alert evaluation is local to each Agent. Netdata Cloud receives state changes but does not re-evaluate or modify alert logic. All evaluation happens on the node that owns the metrics.

:::

## 13.0.1 The Alert Pipeline

Alerts progress through several stages from metric collection to notification dispatch.

| Stage | Description |
|-------|-------------|
| **Collection** | Collectors gather raw metrics from system APIs and applications |
| **Storage** | Metrics stored in the round-robin database |
| **Evaluation** | Health engine checks conditions against configured thresholds |
| **State Management** | Alert status transitions through defined states |
| **Notification** | Dispatch alerts to configured recipients |

## 13.0.2 Configuration Layers

Alert configuration exists in multiple layers with defined precedence.

| Layer | Location | Purpose |
|-------|----------|---------|
| **Stock** | `/usr/lib/netdata/conf.d/health.d/` | Built-in alerts from packages |
| **Custom** | `/etc/netdata/health.d/` | User-defined alerts |
| **Cloud** | Netdata Cloud | Cloud-pushed alerts via UI |

Custom alerts override stock alerts with the same name. Cloud alerts are stored separately and synchronized to nodes.

## 13.0.3 Alert Lifecycle

Each alert instance moves through a defined lifecycle from creation to removal.

| State | Description |
|-------|-------------|
| **UNINITIALIZED** | Insufficient data to evaluate |
| **CLEAR** | Conditions acceptable |
| **WARNING** | Exceeded warning threshold |
| **CRITICAL** | Exceeded critical threshold |
| **UNDEFINED** | Evaluation error |
| **REMOVED** | Alert deleted |

Status transitions trigger notifications and update the alert history.

## 13.0.4 Scaling Topologies

Alert behavior varies depending on deployment topology.

| Topology | Description |
|----------|-------------|
| **Standalone** | Single Agent, local evaluation only |
| **Parent-Child** | Parent aggregates metrics, evaluates alerts |
| **Cloud-connected** | Cloud receives state changes, provides global view |

## 13.0.5 What's Included

| Section | Description |
|---------|-------------|
| **13.1 Evaluation Architecture** | How alerts are evaluated against metric data |
| **13.2 Configuration Layers** | Stock, custom, and cloud alert precedence |
| **13.3 Alert Lifecycle** | States and transitions in alert lifecycle |
| **13.4 Scaling Topologies** | Standalone, parent-child, and cloud models |
| **13.5 Notification Dispatch** | How notifications are routed to recipients |

## 13.0.6 Related Sections

- **Chapter 3**: Alert configuration syntax reference
- **Chapter 9**: API endpoints for alert management
- **Chapter 5**: Receiving notifications
- **Chapter 10**: Troubleshooting alert behavior