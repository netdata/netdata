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

Netdata Cloud plans, with the exception of Community, work as subscriptions and overall consist of two pricing components:
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
   * Issue a credit to you for the unused period, in case you are on a **yearly plan**
   * Charge you only for the current used period and issue a credit for the unused period related to the flat fee, in case you are on a **monthly plan**
* Select the new subscription with the change that you want

> ⚠️ On a move to Community (cancellation of an active subscription), please note that you will have all your notification methods configurations active **for a period of 24 hours**.
>   After that, any notification methods unavailable in your new plan at that time will be automatically disabled. You can always re-enable them once you move to a paid plan that includes them.

> ⚠️ Any credit given to you will be available to use on future paid subscriptions with us. It will be available until the the **end of the following year**.

### Areas impacted by plans

##### Role-Based Access model

Depending on the plan associated to your space you will have different roles available:

| **Role** | **Community** | **Pro** | **Business** |
| :-- | :--: | :--: | :--: |
| **Administrators**<p>This role allows users to manage Spaces, War Rooms, Nodes, and Users, this includes the Plan & Billing settings.</p><p>Provides access to all War Rooms in the space</p> | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| **Managers**<p>This role allows users to manage War Rooms and Users.</p><p>Provides access to all War Rooms and Nodes in the space.</p> | - | - | :heavy_check_mark: |
| **Troubleshooters**<p>This role is for users that will be just focused on using Netdata to troubleshoot, not manage entities.</p><p>Provides access to all War Rooms and Nodes in the space.</p> | - | :heavy_check_mark: | :heavy_check_mark: |
| **Observers**<p>This role is for read-only access with restricted access to explicit War Rooms and only the Nodes that appear in those War Rooms.</p>💡 Ideal for restricting your customer's access to their own dedicated rooms.<p></p> | - | - | :heavy_check_mark: |
| **Billing**<p>This role is for users that need to manage billing options and see invoices, with no further access to the system.</p> | - | - | :heavy_check_mark: |

For more details check the documentation under [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md).

##### Events feed

The plan you have subscribed on your space will determine the amount of historical data you will be able to query:

| **Type of events** | **Community** | **Pro** | **Business** |
| :-- | :-- | :-- | :-- |
| **Auditing events** - COMING SOON<p>Events related to actions done on your Space, e.g. invite user, change user role or create room.</p>| 4 hours | 7 days | 90 days |
| **Topology events**<p>Node state transition events, e.g. live or offline.</p>| 4 hours | 7 days | 14 days |
| **Alert events**<p>Alert state transition events, can be seen as an alert history log.</p>| 4 hours | 7 days | 90 days |

For more details check the documentation under [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/events-feed.md).

##### Notification integrations

The plan on your space will determine what type of notifications methods will be available to you:
* **Community** - Email and Discord
* **Pro** - Email, Discord and webhook
* **Business** - Unlimited, this includes Slack, PagerDuty, etc.

For mode details check the documentation under [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx).

### Related Topics

#### **Related Concepts**
- [Spaces](https://github.com/netdata/netdata/blob/master/docs/cloud/spaces.md)
- [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.mdx)
- [Events feed](https://github.com/netdata/netdata/blob/master/docs/cloud/events-feed.md)
- [Role-Based Access model](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/role-based-access-model.md)


#### Related Tasks
- [View Plan & Billing](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/view-plan-billing.md)
