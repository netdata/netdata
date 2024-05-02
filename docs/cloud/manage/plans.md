# Netdata Subscription Plans

This page explains the differences between the Community plan and paid plans at Netdata.

## Overview

Netdata offers a **Community plan** (a free SaaS and Open Source Agent) that includes unlimited nodes, users, and metrics. This plan provides real-time, high-fidelity monitoring for applications, containers, and operating systems. We also offer paid subscriptions — **Homelab**, **Business**, and **Enterprise On-Premise** — each designed to meet varying needs for tighter and customizable integration.

### Plans

Each plan is linked to a Space, defining the capabilities and customizations available. Different Spaces can have different plans, offering flexibility based on your needs.

Netdata Cloud plans (excluding Community) involve:

- A yearly flat fee for [committed nodes](#committed-nodes)
- An on-demand metered component based on the [number of running nodes](#running-nodes-and-billing)

Billing options include monthly (pay-as-you-go) and yearly (annual prepayment). For more details, visit [Netdata Cloud Pricing](https://netdata.cloud/pricing).

## Running Nodes and Billing

Billing is based on the number of active nodes. We do not charge for offline or stale nodes. We calculate daily and running P90 figures to ensure fair billing by smoothing out sporadic spikes in node activity.

## Committed Nodes

Yearly plans require specifying a number of committed nodes, which receive a discounted rate. Usage above these committed nodes incurs charges at the standard rate.

## Plan Changes and Credit Balance

You can change your plan, billing frequency, or committed nodes at any time. For guidance, see [updating your plan](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/view-plan-billing.md#update-a-subscription-plan).

> **Note**
>
> - Changes like downgrades or cancellations keep notification configurations active for 24 hours. After that, any methods not supported by the new plan are disabled.
> - Changes may restrict user access in your Space. Review role availability under [each plan](#areas-that-change-upon-subscription).
> - Any credits are valid until the end of the following year.

## Areas That Change Upon Subscription

### Role-Based Access Model

Different plans offer different roles. For specifics, refer to [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md).

### Events Feed

The plan determines the historical data and events you can access. Details are in the [Events feed documentation](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md).

### Notification Integrations

Available notification methods depend on your plan:

- **Community**: Email and Discord
- **Homelab/Business/Enterprise On-Premise**: Includes Slack, PagerDuty, Opsgenie, etc.

For more information, visit [Centralized Cloud notifications](/docs/alerts-&-notifications/notifications/centralized-cloud-notifications).

### Alert Notification Silencing Rules

Silencing rules are available only to subscribed Spaces. More details are available in the [silencing rules documentation](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-alert-notification-silencing-rules.md).
