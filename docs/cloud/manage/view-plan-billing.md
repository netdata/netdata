# View Plan & Billing

From the Cloud interface, you can view and manage your space's plan and billing settings, and see the space's usage in terms of running nodes.

To view and manage some specific settings, related to billing options and invoices, you'll be redirected to our billing provider Customer Portal.

## Prerequisites

To see your plan and billing setting you need:

- A Cloud account
- Access to the space as an Administrator or Billing user

## Steps

### View current plan and Billing options and Invoices

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Plan & Billing** tab
1. On this page you will be presented with information on your current plan, billing settings, and usage information:
   1. At the top of the page you will see:
      - **Credit** amount which refers to any amount you have available to use on future invoices or subscription changes ([Plan changes and credit balance](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plan-changes-and-credit-balance)) - this is displayed once you have had an active paid subscription with us
      - **Billing email** the email that was specified to be linked to tha plan subscription. This is where invoices, payment, and subscription-related notifications will be sent.
      - **Billing options and Invoices** is the link to our billing provider Customer Portal where you will be able to:
         - See the current subscription. There will always be 2 subscriptions active for the two pricing components mentioned on [Netdata Plans documentation page](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plans)
         - Change directly the payment method associated to current subscriptions
         - View, add, delete or change your default payment methods
         - View or change or Billing information:
            - Billing email
            - Address
            - Phone number
            - Tax ID
         - View your invoice history
   1. At the middle, you'll see details on your current plan as well as means to:
      - Upgrade or cancel your plan
      - View **All Plans** details page
   1. At the bottom, you will find your Usage chart that displays:
      - Daily count - The weighted 90th percentile of the live node count during the day, taking time as the weight. If you have 30 live nodes throughout the day
      except for a two hour peak of 44 live nodes, the daily value is 31.
      - Period count: The 90th percentile of the daily counts for this period up to the date. The last value for the period is used as the number of nodes for the bill for that period. See more details in [running nodes and billing](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#running-nodes-and-billing) (only applicable if you are on a paid plan subscription)
      - Committed nodes: The number of nodes committed to in the yearly plan. In case the period count is higher than the number of committed nodes, the difference is billed as overage.


### Update plan

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Plan & Billing** tab
1. On this page you will be presented with information on your current plan, billing settings, and usage information
   1. Depending on your plan there could be shortcuts to immediately take you to change, for example, the billing frequency to **Yearly**
   1. Most actions will be available under the **Change plan** link that take you to the **All plans** details page where you can
      1. Downgrade or upgrade your plan
      1. Change the billing frequency
      1. Change committed nodes, in case you are on a Yearly plan
   1. Once you chose an action to update your plan a modal will pop-up on the right with
      1. Billing frequency displayed on the top right-corner
      1. Committed Nodes, when applicable
      1. Current billing information:
         - Billing email
         - Default payment method
         - Business name and VAT number, when these are applicable
         - Billing Address
         Note: Any changes to these need to done through our billing provider Customer Portal prior to confirm the checkout. You can click on the link **Change billing info and payment method** to access it.
      1. Promotion code, so you can review any applied promotion or enter one you may have
      1. Detailed view on Node and Space charges 
      1. Breakdown of: 
         - Subscription Total
         - Discount from promotion codes, if applicable
         - credit value for Unused time from current plan, if applicable
         - Credit amount used from balance, if applicable
         - Total Before Tax
         - VAT rate and amount, if applicable
      1. Summary of:
         - Total payable amount
         - credit adjustment value for any Remaining Unused time from current plan, if applicable
         - Final credit balance
   
Notes:
* Since there is an active plan you won't be redirected to our billing provider, the checkout if performed as soon as you click on **Checkout**
* The change to your plan will be applied as soon as the checkout process is completed successfully
* Downgrade or cancellations may have impacts on some of notification method settings or user accesses to your space, for more details please check [Plan changes and credit balance](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plan-changes-and-credit-balance)

## FAQ

### 1. What Payment Methods are accepted?

You can easily pay online via most major Credit/Debit Cards. More payment options are expected to become available in the near future.

### 2. What happens if a renewal payment fails?

After an initial failed payment, we will attempt to process your payment every week for the next 15 days. After three failed attempts your Space will be moved to the **Community** plan (free forever).

For the next 24 hours, you will be able to use all your current notification method configurations. After 24 hours, any of the notification method configurations that aren't available on your space's plan will be automatically disabled.

Cancellation might affect users in your Space. Please check what roles are available on the [Community plan](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#areas-impacted-by-plans). Users with unavailable roles on the Community plan will immediately have restricted access to the Space.

### 3. Which currencies do you support?

We currently accept payments only in US Dollars (USD). We currently have plans to also accept payments in Euros (EUR), but do not currently have an estimate for when such support will be available.

### 4. Can I get a refund? How?

Payments for Netdata subscriptions are refundable **only** if you cancel your subscription within 14 days of purchase. The refund will be credited to the Credit/Debit Card used for making the purchase. To request a refund, please email us at [billing@netdata.cloud](mailto:billing@netdata.cloud).

### 5. How do I cancel my paid Plan?

Your annual or monthly Netdata Subscription plan will automatically renew until you cancel it. You can cancel your paid plan at any time by clicking ‘Cancel Plan’ from the **Plan & Billing** section under settings. You can also cancel your paid Plan by clicking the _Select_ button under **Community** plan in the **Plan & Billing** Section under Settings.

### 6. How can I access my Invoices/Receipts after I paid for a Plan?

You can visit the _Billing Options & Invoices_ in the **Plan & Billing** section under settings in your Netdata Space where you can find all your Invoicing history.

### 7. Why do I see two separate Invoices?

Every time you purchase or renew a Plan, two separate Invoices are generated:

- One Invoice includes the recurring fees of the Plan you have chosen

  We have waived off the space subscription free ($0.00), so the only recurring fee will be on annual plans for the committed nodes.

- The other Invoice includes your monthly “On Demand - Usage”.

  Right after the activation of your subscription, you will receive a zero value Invoice since you had no usage when you subscribed.
  
  On the following month you will receive an Invoice based on your monthly usage.

You can find some further details on the [Netdata Plans page](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md#plans).

> ⚠️ We expect this to change to a single invoice in the future, but currently do not have a concrete timeline for when this change will happen.

### 8. How is the **Total Before Tax** value calculated on plan changes?

When you change your plan we will be calculating the residual before tax value you have from the _Unused time on your current plan_ in order to credit you with this value.  

After that, we will be performing the following calculations:

1. Get the **Subscription total** (total amount to be paid for Nodes and Space)
2. Deduct any Discount applicable from promotion codes
3. If an amount remains, then we deduct the sum of the _Unused time on current plan_ then and the Credit amount from any existing credit balance. 
4. The result, if positive, is the Total Before Tax, if applicable, any sales tax (VAT or other) will apply. 

If  the calculation of step 3 returns a negative amount then this amount will be your new customer credit balance.
