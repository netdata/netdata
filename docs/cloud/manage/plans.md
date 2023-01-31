<!--
title: "Netdata Plans"
sidebar_label: "Netdata Plans"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md"
sidebar_position: "1000"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts"
learn_docs_purpose: "Explain what are Netdata Plans"
-->

At Netdata, we believe in providing free and unrestricted access to high-quality monitoring solutions, and our commitment to this principle will not change. We offer our free SaaS offering - what we call **Community plan** - and Open Source Agent, which features unlimited nodes and users, unlimited metrics, and retention, providing real-time, high-fidelity, out-of-the-box infrastructure monitoring for packaged applications, containers, and operating systems.

We also provide paid subscriptions that designed to provide additional features and capabilities for businesses that need tighter and customizable integration of the free monitoring solution to their processes. These are divided into three different plans: **Pro**, **Business**, and **Enterprise**. Each plan will offers a different set of features and capabilities to meet the needs of businesses of different sizes and with different monitoring requirements.

### Plans

The plan is an attribute that is directly attached to your space(s) and that dictates what capabilities and customizations you have on your space. If you have different spaces you can have different Netdata plans on them. This gives you flexibility to chose what is more adequate for your needs on each of your spaces.

Netdata Cloud plans, with the exception of Community, work as subscriptions and overall consist of:
* A flat fee component, that is a price per space, and
* An on-demand metered component, that is related to your usage of Netdata which directly links to the [number of nodes you have running](#running-nodes-and-billing)

Netdata provides two billing frequency options:
* Monthly - Pay as you go, where we charge both the flat fee and the on-demand component every month
* Yearly - Annual prepayment, where we charge upfront the flat fee and committed amount related to your estimated usage of Netdata (more details [here](#committed-nodes))

For more details on the plans and subscription conditions please check https://netdata.cloud/pricing.

#### Running nodes and billing

The only dynamic variable we consider for billing is the number of concurrently running nodes or agents. We only charge you for your active running nodes, so we don't count:
* offline nodes
* stale nodes, nodes that are available to query through a Netdata parent agent but are not actively connecting metrics at the moment

To ensure we don't overcharge you due to sporadic spikes throughout a month or even at a certain point in a day we are:
* Calculate a daily P90 figure for your running nodes. To achieve that, we take a daily snapshot of your running nodes, and using the node state change events (live, offline) we guarantee that a daily P90 figure is calculated to remove any daily spikes
* On top of the above, we do a running P90 calculation from the start to the end of your billing cycle. Even if you have an yearly billing frequency we keep a monthly subscription linked to that to identify any potential overage over your [committed nodes](#committed-nodes).

#### Committed nodes

When you subscribe to an Yearly plan you will need to specify the number of nodes that you will commit to. On these nodes, a discounted price of less 25% than the original cost per node of the plan is applied. This amount will be part of your annual prepayment.

```
Node plan discounted price x committed nodes x 12 months
```

If, for a given month, your usage is over these committed nodes we will charge the original cost per node for the nodes above the committed number.

#### Plan changes and credit balance

It is ok to change your mind. We allow to change your plan, billing frequency or adjust the committed nodes, on yearly plans, at any time.

To achieve this you will need to:
* Move to the Community plan, where we will cancel the current subscription and:
   * Issue a credit to you for the unused period, in case you are on an **yearly plan**
   * Charge you only for the current used period and issue a credit for the unused period related to the flat fee, in case you are on a **monthly plan**
* Select the new subscription with the change that you want

Note: This is a temporary approach and we are working on making this flow seamless and effortless to you.

<!--
 TODO: This credit balance will be available for a period of X months/years. 
 -->

> ⚠️ On a move to Community, cancellation of active subscription, please note that you will have all your notification methods configurations active for a period of 24 hours.
> After that, any of those that won't available on your space's plan at that time will be automatically disabled. You can always re-enable them once you move to the proper paid plan.

### Areas impacted by plans

##### Role-Based Access model

Depending on the plan associated to your space you will have different roles available:

| **Role** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Administrators**<p>Unrestricted access to all features and management of a Space.</p> | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| **Managers**<p>Same as admins, but can't add nodes, or manage space settings.</p> | - | - | :heavy_check_mark: |
| **Troubleshooters**<p>Same as managers, but can’t manage users or rooms.</p> | - | :heavy_check_mark: | :heavy_check_mark: |
| **Observers**<p>Read only role, restricted to specific rooms.</p> | - | - | :heavy_check_mark: |
| **Billing**<p>Access to billing details and subscription management.</p> | - | - | :heavy_check_mark: |

For mode details check the documentation under [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md).

##### Events feed

The plan you have subscribed to will determine the amount of historical data you will be able to query:

| **Type of events** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Auditing events** - COMING SOON<p>Events related to actions done on your Space, e.g. invite user, change user role or create room.</p>| 4 hours | 7 days | 90 days |
| **Topology events**<p>Node state transition events, e.g. live or offline.</p>| 4 hours | 7 days | 14 days |
| **Alert events**<p>Alert state transition events, can be seen as an alert history log.</p>| 4 hours | 7 days | 90 days |

For mode details check the documentation under [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/events-feed.md).

##### Notification integrations

Your plan will determine what type of notifications methods will be available for you to configure on your space:
* **Community** - Email and discord
* **Pro** - Email, discord and webhook
* **Business** - Unlimited, this includes slack, pagerduty, etc.

For mode details check the documentation under [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx).

### Related Topics

#### **Related Concepts**
- [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/spaces.md)
- [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx)
- [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/events-feed.md)
- [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md)


#### Related Tasks
- [Inspect alerts](https://github.com/netdata/netdata/blob/rework-learn/docs/tasks/operations/inspect-alerts.md)
- [Manage notification methods](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/manage-notification-methods.md)
- [Add webhook notification configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/add-webhook-notification-configuration.md)
- [Add discord notification configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/add-discord-notification-configuration.md)
- [Add slack notification configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/add-slack-notification-configuration.md)
- [Add pagerduty notification configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/operations/add-pagerduty-notification-configuration.md)
