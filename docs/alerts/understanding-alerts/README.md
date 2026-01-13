# 1. Understanding Alerts in Netdata

In Netdata, an **alert** (also called a "health check") is a rule that continuously monitors one or more metrics and assigns a **status** based on whether conditions are met.

| Core Concept | Description |
|--------------|-------------|
| **Where alerts are evaluated** | Locally on each Netdata Agent, using locally stored metrics data |
| **How alerts work** | Each alert inspects recent metrics data (for example, "average CPU over the last 5 minutes") and decides whether the system is in a healthy state (`CLEAR`), needs attention (`WARNING`), or has a problem (`CRITICAL`) |
| **What happens on status change** | When an alert's status changes, that transition becomes an **alert event** visible in Netdata Cloud's Events Feed, and may trigger notifications depending on your configuration |

## Where Alerts Run

- **Agents** evaluate alerts against their own collected metrics
- **Parents** (in streaming setups or when collecting on behalf of a virtual node) can evaluate alerts on metrics received from child nodes
- **Netdata Cloud** receives alert events from Agents and presents them in a unified view, but does **not** re-evaluate the rules itself. When multiple Agents represent a given node, their reported statuses are consolidated for that node.

## Where Alert Definitions Come From

- **Configuration files** on each Agent, including stock alerts shipped with Netdata, plus your custom rules
- **Netdata Cloud** (alerts created via the Alerts Configuration Manager UI, which are pushed to specific Agents at runtime)

:::tip

Because Netdata evaluates alerts locally on each Agent, you can still see alerts in the Agent dashboard and receive notifications even when there's no Cloud connectivity. Cloud provides a unified view of all alerts across your infrastructure.

:::

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **[1.1 What is a Netdata Alert?](1-what-is-a-netdata-alert.md)** | Status values, how alerts relate to charts and contexts |
| **[1.2 Alert Types: `alarm` vs `template`](2-alert-types-alarm-vs-template.md)** | Chart-specific vs context-based rules |
| **[1.3 Where Alerts Live](3-where-alerts-live.md)** | File paths, stock vs custom, Cloud integration |

## What's Next

- **[Chapter 2: Creating Alerts](../creating-alerts-pages/index.md)** - Learn to create and edit alerts
