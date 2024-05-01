# Netdata Subscription Plans

This page will guide you through the differences between the Community, Homelab, Business and Enterprise On-Premise plans.

At Netdata, we believe in providing free and unrestricted access to high-quality monitoring solutions, and our commitment to this principle will not change. We offer our free SaaS - what we call **Community plan** - and Open Source Agent, which features unlimited nodes and users, unlimited metrics, and retention, providing real-time, high-fidelity, out-of-the-box infrastructure monitoring for packaged applications, containers, and operating systems.

We also provide paid subscriptions that are designed to provide additional features and capabilities for businesses and labs that need tighter and customizable integration of the free monitoring solution to their processes.

These are divided into three different plans: **Homelab**, **Business**, and **Enterprise On-Premise**. Each plan offers a different set of features and capabilities to meet the needs of businesses with different sizes and monitoring requirements.

## Plans

The plan is an attribute that is directly attached to your Space and dictates what capabilities and customizations you have available on that Space.

If you have different Spaces you will have different subscription plans on them. This gives you flexibility to chose what is more adequate for your needs on each of your Spaces.

Netdata Cloud plans, with the exception of Community, work as subscriptions and overall consist of two pricing components:

- A flat fee component, that is applied on yearly subscriptions for the [committed nodes](#committed-nodes)
- An on-demand metered component, that is related to your usage of Netdata which is directly linked to the [number of nodes you have running](#running-nodes-and-billing)

Netdata provides two billing frequency options:

- Monthly - Pay as you go, where we charge the on-demand component every month
- Yearly - Annual prepayment, where we charge upfront the flat fee and have the on-demand metered component for any overages that might occur above the [committed nodes](#committed-nodes)

For more details on the plans and subscription conditions please check <https://netdata.cloud/pricing>.

## Running nodes and billing

The only dynamic variable we consider for billing is the number of concurrently running nodes or Agents. We only charge you for your active running nodes, so we don't count:

- offline nodes
- stale nodes, nodes that are available to query through a Netdata parent Agent but are not actively connecting metrics at the moment

To ensure we don't overcharge you due to sporadic spikes throughout a month or even at a certain point in a day we are:

- Calculate a daily P90 figure for your running nodes. To achieve that, we take a daily snapshot of your running nodes, and using the node state change events (live, offline) we guarantee that a daily P90 figure is calculated to remove any daily spikes
- On top of the above, we do a running P90 calculation from the start to the end of your billing cycle. Even if you have a yearly billing frequency we keep a monthly subscription linked to that to identify any potential overage over your [committed nodes](#committed-nodes).

## Committed nodes

When you subscribe to a Yearly plan you will need to specify the number of nodes that you will commit to. On these nodes, a discounted price is applied.

For a given month, if your usage is over these committed nodes we will charge the original cost per node for the nodes above the committed number. That's what we call overage.

## Plan changes and credit balance

It is ok to change your mind. We allow to change your plan, billing frequency or adjust the committed nodes on yearly plans, at any time.

To achieve this you can check our documentation on [updating your plan](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/view-plan-billing.md#update-a-subscription-plan).

> **Note**
>
> - On a downgrade (going to a new plan with less benefits) or cancellation of an active subscription, please note that you will have all your notification methods configurations active **for a period of 24 hours**.
>
>   After that, any notification methods unavailable in your new plan at that time will be automatically disabled. You can always re-enable them once you move to a paid plan that includes them.
>
> - Also note that a downgrade or a cancellation may affect users in your Space. Please check what roles are available on [each of the plans](#areas-that-change-upon-subscription). Users with unavailable roles on the new plan will immediately have restricted access to the Space.
>
> - Any credit given to you will be available to use on future paid subscriptions with us. It will be available until the **end of the following year**.

## Areas that change upon subscription

### Role-Based Access model

Depending on the plan associated to your Space you will have different roles available, please check the documentation under [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access.md) for more info.

### Events feed

The plan you have subscribed on your Space will determine the amount of historical data you will be able to query and the events that will be available. For more details check the documentation related to the [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md).

### Notification integrations

The plan on your Space will determine what type of notifications methods will be available to you:

- **Community** - Email and Discord
- **Homelab/Business/Enterprise On-Premise** - Unlimited, this includes Slack, PagerDuty, Opsgenie etc.

For more details check the documentation under [Centralized Cloud notifications](/docs/alerts-&-notifications/notifications/centralized-cloud-notifications).

### Alert notification silencing rules

The silencing rules feature is only available for Spaces on a subscription, for more details check the [related documentation](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-alert-notification-silencing-rules.md).
