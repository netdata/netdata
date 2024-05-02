# Netdata Subscription Plans

This page explains the Netdata subscription plan structure.

## Overview

Netdata offers a **Community plan**, a free SaaS and Open Source Agent, while also it offers paid subscriptions — **Homelab**, **Business**, and **Enterprise On-Premise** — providing key business features and unlimited access to your dashboards.

For more info visit the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page.

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

Please refer to the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page for more information on what each plan provides.
