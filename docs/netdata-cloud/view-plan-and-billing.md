# Netdata Plans & Billing

Netdata offers a **Community plan**, a free SaaS and Open Source Agent, and paid subscriptions — **Homelab**, **Business**, and **Enterprise On-Premise** — providing key business features and unlimited access to your dashboards.

For more info visit the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page.

## Plans

Plans define the features and customization options available within a Space. Different Spaces can have different plans, giving you flexibility based on your needs.

Netdata Cloud plans (excluding Community) involve:

- A yearly flat fee for [committed nodes](#committed-nodes)
- An on-demand metered component based on the [number of running nodes](#running-nodes-and-billing)

Billing options include monthly (pay-as-you-go) and yearly (annual prepayment).

### Technical Details

#### Running Nodes and Billing

Billing is based solely on active nodes, excluding offline or stale instances. Daily and P90 metrics ensure fair pricing by mitigating transient increases in node activity.

#### Committed Nodes

Yearly plans offer a discounted rate for a pre-defined number of committed nodes. Any usage exceeding this commitment will be billed at the standard rate.

#### Plan Changes and Credit Balance

You can change your plan, billing frequency, or committed nodes at any time. For guidance, see [updating your plan](#update-a-subscription-plan).

> **Note**
>
> - Changes like downgrades or cancellations keep notification configurations active for 24 hours. After that, any methods not supported by the new plan are disabled.
> - Changes may restrict user access in your Space. Review role availability under [each plan](https://netdata.cloud/pricing).
> - Any credits are valid until the end of the following year.

#### Areas That Change Upon Subscription

Please refer to the [Netdata Cloud Pricing](https://netdata.cloud/pricing) page for more information on what each plan provides.

## View Plan and Billing Information

### Prerequisites

- A Netdata Cloud account
- Admin or Billing user access to the Space

### Steps

#### View Current Plan, Billing Options, and Invoices

1. Navigate to **Space settings** (the cog above your profile icon).
2. Select the **Plan & Billing** tab.
3. You'll see:
   - **Credit** amount, if applicable, usable for future invoices or subscription changes. More on this at [Plan changes and credit balance](/docs/netdata-cloud/netdata-subscription-plans.md#plan-changes-and-credit-balance).
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

> **Note**
>
> - Checkout is performed directly if there's an active plan.
> - Plan changes, including downgrades or cancellations, may impact notification settings or user access. More details at [Plan changes and credit balance](/docs/netdata-cloud/netdata-subscription-plans.md#plan-changes-and-credit-balance).

## FAQ

### What Payment Methods are Accepted?

Netdata accepts most major Credit/Debit Cards and Bank payments through Stripe and AWS, with more options coming soon.

### What Happens if a Renewal Payment Fails?

If payment fails, attempts will be made weekly for 15 days. After three unsuccessful attempts, your Space will switch to the **Community** plan. Notification methods not supported by the Community plan will be disabled after 24 hours.

### Which Currencies Do You Support?

Currently, we accept US Dollars (USD). Plans to accept Euros (EUR) are in the works but without a set timeline.

### Can I Get a Refund?

Refunds are available if you cancel your subscription within 14 days of purchase. Request a refund via [billing@netdata.cloud](mailto:billing@netdata.cloud).

### How Do I Cancel My Paid Plan?

Cancel your plan anytime from the **Plan & Billing** section by selecting 'Cancel Plan' or switching to the **Community** plan.

### How Can I Access My Invoices/Receipts?

Find all your invoicing history under _Billing Options & Invoices_ in the **Plan & Billing** section.

### Why Do I See Two Separate Invoices?

Two invoices are generated per plan purchase or renewal:

- One for recurring fees of the chosen plan.
- Another for monthly "On-Demand - Usage" based on actual usage.

### How is the **Total Before Tax** Value Calculated on Plan Changes?

The total before tax is calculated by:

1. Calculating the residual value from unused time on your current plan.
2. Deducting any applicable discounts.
3. Subtracting credit from your balance, if necessary.
4. Applying tax to the final amount, if positive. Negative results adjust your customer credit balance.

> **Note**
>
> A move to single-invoice billing is expected in the future, although a specific timeline is not set.
