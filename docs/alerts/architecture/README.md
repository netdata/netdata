# 12: Alerts and Notifications Architecture

Understanding how alerts work under the hood helps with troubleshooting, optimization, and designing effective notification strategies. This chapter covers the evaluation pipeline, state machine, and how alerts interact with the broader Netdata ecosystem.

:::note

Alert evaluation is local to each Agent. Netdata Cloud receives state changes but does not re-evaluate or modify alert logic. All evaluation happens on the node that owns the metrics.

:::

## What's Included

This chapter covers five deep-dive areas:

- **[12.1 Evaluation Architecture](/docs/alerts/architecture/1-evaluation-architecture.md)** — How alerts check metrics against thresholds
- **[12.2 Alert Lifecycle](/docs/alerts/architecture/2-alert-lifecycle.md)** — States and transitions (UNINITIALIZED → CLEAR → WARNING → CRITICAL)
- **[12.3 Notification Dispatch](/docs/alerts/architecture/3-notification-dispatch.md)** — Queues, methods, and delivery reliability
- **[12.4 Configuration Layers](/docs/alerts/architecture/4-configuration-layers.md)** — Stock, custom, and Cloud-based configs
- **[12.5 Scaling Topologies](/docs/alerts/architecture/5-scaling-topologies.md)** — Agent-only, parent-child, and multi-region setups

## Related Sections

- **[3. Alert Configuration Syntax](/docs/alerts/alert-configuration-syntax/README.md)** - Alert configuration syntax reference
- **[9. APIs for Alerts and Events](/docs/alerts/apis-alerts-events/README.md)** - API endpoints for alert management
- **[5. Receiving Notifications](/docs/alerts/receiving-notifications/README.md)** - Notification configuration
- **[7. Troubleshooting Alerts](/docs/alerts/troubleshooting-alerts/README.md)** - Diagnose and fix alert behavior