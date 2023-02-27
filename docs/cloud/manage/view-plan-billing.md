<!--
title: "View Plan & Billing"
sidebar_label: "View Plan & Billing"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/manage/view-plan-billing.md"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
learn_docs_purpose: "How to check details on your space plan and billing"
-->

From the Cloud interface, you can view and manage your space's plan and billing settings, and see the space's usage in terms of running nodes.
To view and manage some specific settings, related to billing options and invoices, you'll be redirected to our billing provider Customer Portal.

#### Prerequisites

To see your plan and billing setting you need:

- A Cloud account
- Access to the space as an Administrator or Billing user

#### Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Plan & Billing** tab
1. On this page you will be presented with information on your current plan, billing settings, and usage information:
   1. At the top of the page you will see:
      * **Credit** amount which refers to any amount you have available to use on future invoices or subscription changes (https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plan-changes-and-credit-balance) - this is displayed once you have had an active paid subscription with us
      * **Billing email** the email that was specified to be linked to tha plan subscription. This is where invoices, payment, and subscription-related notifications will be sent.
      * **Billing options and Invoices** is the link to our billing provider Customer Portal where you will be able to:
         * See the current subscription. There will always be 2 subscriptions active for the two pricing components mentioned on [Netdata Plans documentation page](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plans)
         * Change directly the payment method associated to current subscriptions
         * View, add, delete or change your default payment methods
         * View or change or Billing information:
            * Billing email
            * Address
            * Phone number
            * Tax ID
         * View your invoice history
   1. At the middle, you'll see details on your current plan as well as means to:
      * Upgrade or cancel your plan
      * View full plan details page
   1. At the bottom, you will find your Usage chart that displays:
      * Daily count - The weighted 90th percentile of the live node count during the day, taking time as the weight. If you have 30 live nodes throughout the day
      except for a two hour peak of 44 live nodes, the daily value is 31.
      * Period count: The 90th percentile of the daily counts for this period up to the date. The last value for the period is used as the number of nodes for the bill for that period. See more details in [running nodes and billing](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#running-nodes-and-billing) (only applicable if you are on a paid plan subscription)
      * Committed nodes: The number of nodes committed to in the yearly plan. In case the period count is higher than the number of committed nodes, the difference is billed as overage.

> ⚠️ At the moment, any changes to an active paid plan, upgrades, change billing frequency or committed nodes, will be a manual two-setup flow:
> 1. cancel your current subscription - move you to the Community plan
> 2. chose the plan with the intended changes
>
> This is a temporary process that we aim to sort out soon so that it will effortless for you to do any of these actions.

#### Related topics

- [Netdata Plans](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)
