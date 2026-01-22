# 12: Alerts and Notifications Architecture

Understanding how alerts work under the hood helps with troubleshooting, optimization, and designing effective notification strategies. This chapter covers the evaluation pipeline, state machine, and how alerts interact with the broader Netdata ecosystem.

:::note

Alert evaluation is local to each Agent. Netdata Cloud receives state changes but does not re-evaluate or modify alert logic. All evaluation happens on the node that owns the metrics.

:::

## 12.0.1 The Alert Pipeline {#the-alert-pipeline}

Alerts progress through several stages from metric collection to notification dispatch.

| Stage | Description |
|-------|-------------|
| **Collection** | Collectors gather raw metrics from system APIs and applications |
| **Storage** | Metrics stored in the round-robin database |
| **Evaluation** | Health engine checks conditions against configured thresholds |
| **State Management** | Alert status transitions through defined states |
| **Notification** | Dispatch alerts to configured recipients |

## 12.0.2 Configuration Layers {#configuration-layers}

Alert configuration exists in multiple layers with defined precedence.

| Layer | Location | Purpose |
|-------|----------|---------|
| **Stock** | `/usr/lib/netdata/conf.d/health.d/` | Built-in alerts from packages |
| **Custom** | `/etc/netdata/health.d/` | User-defined alerts |
| **Cloud** | Netdata Cloud | Cloud-pushed alerts via UI |

Custom alerts override stock alerts with the same name. Cloud alerts are stored separately and synchronized to nodes.

## 12.0.3 Alert Lifecycle

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

## 12.0.4 Scaling Topologies

Alert behavior varies depending on deployment topology.

| Topology | Description |
|----------|-------------|
| **Standalone** | Single Agent, local evaluation only |
| **Parent-Child** | Parent aggregates metrics, evaluates alerts |
| **Cloud-connected** | Cloud receives state changes, provides global view |

## What's Included

This chapter breaks down the architecture into four key areas:

- **The Alert Pipeline** — From metric collection to notification dispatch
- **Configuration Layers** — How alert configs layer and override each other
- **Alert Lifecycle** — States and transitions an alert goes through
- **Scaling Topologies** — Alert behavior in standalone, parent-child, and cloud topologies

## Related Sections

- **[3. Alert Configuration Syntax](/docs/alerts/alert-configuration-syntax/README.md)** - Alert configuration syntax reference
- **[9. APIs for Alerts and Events](/docs/alerts/apis-alerts-events/README.md)** - API endpoints for alert management
- **[5. Receiving Notifications](/docs/alerts/receiving-notifications/README.md)** - Notification configuration
- **[7. Troubleshooting Alerts](/docs/alerts/troubleshooting-alerts/README.md)** - Diagnose and fix alert behavior