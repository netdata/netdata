# Netdata Plans & Billing

Netdata offers a **Community plan**, a free SaaS, Open Source Agent, and paid subscriptions — **Homelab**, **Business**, and **Enterprise On-Premise** — providing key business features and unlimited access to your dashboards.

For more info visit the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page.

## Plans

Plans define the features and customization options available within a Space. Different Spaces can have different plans, giving you flexibility based on your needs.

Netdata Cloud plans (excluding Community) involve:

- A yearly flat fee for [committed nodes](#committed-nodes)
- An on-demand metered component based on the [number of running nodes](#active-nodes-and-billing)

Billing options include monthly (pay-as-you-go) and yearly (annual prepayment).

### Technical Details

#### Active Nodes and Billing

Billing is based solely on active nodes, excluding offline or stale instances. Daily and P90 metrics ensure fair pricing by mitigating transient increases in node activity.

#### Committed Nodes

Yearly plans offer a discounted rate for a pre-defined number of committed nodes. Any usage exceeding this commitment will be billed at the standard rate.

#### Plan Changes and Credit Balance

You can change your plan, billing frequency, or committed nodes at any time. For guidance, see [updating your plan](#update-a-subscription-plan).

:::note

 - Changes like downgrades or cancellations keep notification configurations active for 24 hours. After that, any methods not supported by the new plan are disabled.
 - Changes may restrict user access in your Space. Review role availability under [each plan](https://netdata.cloud/pricing).
 - Any credits are valid until the end of the following year.

:::

#### Areas That Change Upon Subscription

Please refer to the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page for more information on what each plan provides.

## View Plan and Billing Information

### Prerequisites

- A Netdata Cloud account
- Admin or Billing user access to Space

### Steps

#### View Current Plan, Billing Options, and Invoices

1. Navigate to **Space settings** (the cog above your profile icon).
2. Select the **Plan & Billing** tab.
3. You'll see:
    - **Credit** amount, if applicable, usable for future invoices or subscription changes. More on this at [Plan changes and credit balance](/docs/netdata-cloud/view-plan-and-billing.md#plan-changes-and-credit-balance).
    - **Billing email** linked to your subscription, where all related notifications are sent.
    - A link to the **Billing options and Invoices** in our billing provider's Customer Portal, where you can:
        - Manage subscriptions and payment methods.
        - Update billing information such as email, address, phone number, and Tax ID.
        - View invoice history.
    - The **Change plan** button, showing details of your current plan with options to upgrade or cancel.
    - Your **Usage chart**, displaying daily and period counts of live nodes and how they relate to your billing.

#### Update a Subscription Plan

1. In the **Plan & Billing** tab, click **Change plan** to see:
    - Billing frequency and committed nodes (if applicable).
    - Current billing information, which must be updated through our billing provider's Customer Portal via **Change billing info and payment method** link.
    - Options to enter a promotion code and a breakdown of charges, including subscription total, applicable discounts, credit usage, tax details, and total payable amount.

:::note

 Checkout is performed directly if there's an active plan.

:::

## FAQ

<details><summary><strong>What Payment Methods are Accepted?</strong></summary>
Netdata accepts most major Credit/Debit Cards and Bank payments through Stripe and AWS, with more options coming soon.
</details>

<details><summary><strong>What Happens if a Renewal Payment Fails?</strong></summary>
If payment fails, attempts will be made weekly for 15 days. After three unsuccessful attempts, your Space will switch to the Community plan. Notification methods not supported by the Community plan will be disabled after 24 hours.
When a plan is downgraded to Community due to payment failures, we do not automatically revert to the previous plan when payment succeeds. The amount you paid is available as credit. To restore your paid plan, manually change your plan back to your previous subscription (Homelab, Business, etc.), and this credit balance will be applied.
</details>

<details><summary><strong>Which Currencies Do You Support?</strong></summary>
Currently, we accept US Dollars (USD). Plans to accept Euros (EUR) are in the works but without a set timeline.
</details>

<details><summary><strong>Can I Get a Refund?</strong></summary>
Refunds are available if you cancel your subscription within 14 days of purchase. Request a refund via billing@netdata.cloud.
</details>

<details><summary><strong>How Do I Cancel My Paid Plan?</strong></summary>
Cancel your plan anytime from the Plan & Billing section by selecting 'Cancel Plan' or switching to the Community plan.
</details>

<details><summary><strong>How Can I Access My Invoices/Receipts?</strong></summary>
Find all your invoicing history under Billing Options & Invoices in the Plan & Billing section.
</details>

<details><summary><strong>Why Do I See Two Separate Invoices?</strong></summary>
Two invoices are generated per plan purchase or renewal:

One for recurring fees of the chosen plan.
Another for monthly "On-Demand - Usage" based on actual usage.

</details>
<details><summary><strong>How is the Total Before Tax Value Calculated on Plan Changes?</strong></summary>
The total before tax is calculated by:

Calculating the residual value from unused time on your current plan.
Deducting any applicable discounts.
Subtracting credit from your balance, if necessary.
Applying tax to the final amount, if positive. Negative results adjust your customer credit balance.

</details>
