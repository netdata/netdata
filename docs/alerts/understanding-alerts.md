# 1. Understanding Alerts in Netdata

In Netdata, an **alert** (also called a "health check") is a rule that continuously monitors one or more metrics and assigns a **status** based on whether conditions are met.

| Core Concept | Description |
|--------------|-------------|
| **Where alerts are evaluated** | Locally on each Netdata Agent or Parent node, not centrally in Netdata Cloud |
| **How alerts work** | Each alert inspects recent metric data (for example, "average CPU over the last 5 minutes") and decides whether the system is in a healthy state (`CLEAR`), needs attention (`WARNING`), or has a problem (`CRITICAL`) |
| **What happens on status change** | When an alert's status changes, that transition becomes an **alert event** visible in the Agent dashboard, APIs, and Netdata Cloud's Events Feed |

## Where Alerts Run

- **Agents** evaluate alerts against their own collected metrics
- **Parents** (in streaming setups) can evaluate alerts on metrics received from child nodes
- **Netdata Cloud** receives alert events from Agents/Parents and presents them in a unified view, but does **not** re-evaluate the rules itself

## Where Alert Definitions Come From

- **Configuration files** on each Agent/Parent (stock alerts shipped with Netdata plus your custom rules)
- **Netdata Cloud** (alerts created via the Alerts Configuration Manager UI, stored in Cloud and applied at runtime on the nodes)

:::tip

This distributed, edge-first model means alerts continue to work on each node even without Cloud connectivity, giving you flexibility in where you define and manage your alert rules.

:::

## What's Next

- **1.1 What is a Netdata Alert?** Status values, how alerts relate to charts and contexts
- **1.2 Alert Types: `alarm` vs `template`** Chart-specific vs context-based rules
- **1.3 Where Alerts Live (Files, Agent, Cloud)** File paths, stock vs custom, Cloud integration