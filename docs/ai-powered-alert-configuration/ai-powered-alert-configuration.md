# AI-Powered Alert Configuration

## Overview

Netdata has an incredibly powerful alerting engine that offers immense flexibility to build specific, intelligent alerts. However, mastering its syntax can feel like learning a new language.

To ensure accessibility for every member on your engineering team, we created the AI-Powered Alert Configuration feature. A bridge between your intent and Netdata's syntax, letting you describe conditions in plain English while the AI handles the rest.

## Capabilities

### Creating production-ready alerts

All you have to do to create production-ready alerts, is to simply describe your goal in plain English. For example, if you want to get ahead of a disk filling up:

**"Create an alert that warns me when a disk will be more than 85% full in the next 24 hours, and goes critical if it will be more than 95% full."**

Netdata AI translates your request into a perfectly formatted, syntactically correct alert definition and presents it for your review.

### Suggest what to alert on

You can ask Netdata AI for intelligent suggestions based on the context of your infrastructure.

Netdata AI will suggest what would be a suitable alert for a specific use-case or context.

### Explain what every alert does

Netdata ships with hundreds of pre-configured alerts. To get masterfull insights of their underlying logic:

Click on any alert definition and ask Netdata AI to **Explain** this alert. It will break down the entire definition line by line.

## AI credits consumption

Just like other Netdata AI functionality, usage is based on AI credits. You get 10 monthly complementary credits with a Business subscription, and you can top up credits yourself based on your needs. You also get 10 free credits on the free trial to experiment with.

- **Alert creation**: 0.2 credits per run (create up to 5 alerts per AI credit)
- **Alert suggestion**: 0.2 credits per run (up to 5 suggestions per AI credit)
- **Alert explanation**: Free to use

:::info

Track your AI credit usage from `Settings → Usage & Billing → AI Credits`.

:::

## How to access

You'll find these new AI capabilities directly within the alert configuration workflow in Netdata Cloud.

### From any chart

**Step 1:** Click on the **alert bell icon** (locate it by navigating your cursor on any chart)
**Step 2:** Click on **+ Add alert**
**Step 3:** Describe the alert you want to create

You can now seamlessly:
- Create alerts using natural language
- Get AI suggestions for what to monitor
- Explain existing alerts

### From the Alerts tab

Click on **"New Alert"** from the alerts tab to access the AI-powered alert creation workflow.

### Alternative: Manual configuration

You always have the option to use the manual dynamic configuration alert wizard, which gives you full control over every parameter.

## Requirements

- **Subscription:** Business plan or Free Trial
- **Access:** Netdata Cloud account with appropriate permissions

## Availability

This feature is available today for all users on a Business plan or a free trial.

## See also

- [Alert Troubleshooting](/docs/troubleshooting/troubleshoot.md) - AI-powered alert investigation
- [Netdata AI Overview](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md) - Complete AI capabilities guide


By letting Netdata AI handle the syntactic complexity, we're lowering the barrier to entry for effective alerting and freeing up your team to focus on what matters: building reliable, high-performance systems.

:::note

Despite our best efforts to eliminate inaccuracies, AI responses may sometimes be incorrect. Please review generated configurations before deploying to production systems.

:::