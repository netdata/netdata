# Alerts Automation

Alerts Automation uses AI to suggest, generate, and test alert configurations for you — no need to learn Netdata's alert syntax or manually tune thresholds. Describe what you want to monitor in plain English, review the generated configuration against historical data, and deploy it to your nodes.

## When to use it

- You want to set up alerts without learning Netdata's alert configuration syntax
- You need suggestions for what to alert on for a given service or use case
- You want to validate an alert configuration against historical data before deploying it
- You want to tune alert thresholds and reduce alert fatigue without trial and error

## How it works

Alerts Automation provides three AI-powered capabilities that work together as a single workflow:

### Suggest

Ask Netdata AI what alerts would be appropriate for your infrastructure, and it responds with suggestions described in plain English. Because suggestions are in natural language, anyone on the team can understand, discuss, and refine them before they become configurations.

### Generate

Once you decide what to alert on, Netdata AI generates the complete alert configuration automatically. It translates your intent into Netdata's alert syntax — handling lookups, thresholds, hysteresis, dimensions, and all other parameters — so you don't have to write or understand the configuration format.

### Test on historical data

Before deploying, Netdata AI tests the generated alert configuration against the historical metric data of any node you select. The test shows:

- How many times the alert **would have triggered**
- How long each alert **would have lasted**
- What **alert states** (warning, critical) would have been reached

This lets you evaluate whether the alert is too sensitive, too quiet, or just right — before it reaches production. You can then adjust the configuration and re-test until you are satisfied with the results.

### Edit and deploy

After testing, you can edit the alert definition through the UI and submit it to a single node or multiple nodes, depending on your needs.

## How alert configuration works in Netdata

Netdata provides multiple ways to configure alerts. All methods produce the same underlying alert definitions — they differ in how you author them.

| Method | How it works | Best for |
|--------|-------------|----------|
| **AI Alerts Automation** (this page) | Describe what you want in plain English; AI suggests, generates, and tests the configuration | Getting started quickly, avoiding syntax errors, validating before deployment |
| **[Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md)** | Visual UI wizard for creating and editing alerts | Manual control over every parameter via a guided interface |
| **[Manual config files](/src/health/REFERENCE.md)** | Edit `health.d/*.conf` files directly | Advanced users, automation pipelines, version-controlled configurations |

## How to access

Alerts Automation is available directly within the alert configuration workflow in Netdata Cloud.

### From any chart

1. Click the **alert bell icon** on any chart
2. Click **+ Add alert**
3. Describe the alert you want to create, or ask for suggestions

### From the Alerts tab

Click **"New Alert"** from the Alerts tab to access the AI-powered alert creation workflow.

## AI credits consumption

Usage is based on AI credits. You get 10 monthly complementary credits with a Business subscription, and you can top up credits based on your needs. You also get 10 free credits on the free trial.

- **Alert creation**: 0.2 credits per run (create up to 5 alerts per AI credit)
- **Alert suggestion**: 0.2 credits per run (up to 5 suggestions per AI credit)
- **Alert explanation**: Free to use

:::info
Track your AI credit usage from `Settings → Usage & Billing → AI Credits`.
:::

## Availability

Available for all users on a Business plan or free trial.

- **Subscription:** Business plan or Free Trial
- **Access:** Netdata Cloud account with appropriate permissions

## FAQ

### Is alert configuration in Netdata manual or automated?

Both. Alerts Automation provides a fully automated, AI-powered path where you describe what you want in plain English and the AI generates, tests, and helps you deploy the alert configuration. You can also configure alerts manually using the [Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md) UI or by [editing config files directly](/src/health/REFERENCE.md).

### Can I automatically tune alert thresholds?

Yes. Describe your goal (e.g., "warn me when CPU is consistently high but ignore short spikes") and the AI generates appropriate thresholds. You can then test the configuration against historical data to see how it would have behaved, adjust, and re-test until the thresholds are right.

### Does Netdata use machine learning for alerts?

Yes, in two ways:

1. **ML-based anomaly detection** — Every Netdata Agent runs unsupervised machine learning models that learn normal metric behavior and score anomalies in real time. You can create alerts based on anomaly rates. See [ML Anomaly Detection](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md).
2. **AI-powered alert configuration** (this page) — Use natural language to suggest, generate, test, and deploy alert definitions.

### How is this different from the Alerts Configuration Manager?

The [Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md) is a visual UI wizard where you manually select metrics, set thresholds, and configure parameters field by field. Alerts Automation uses AI to generate the entire configuration from a natural language description and lets you validate it against historical data before deployment. Both produce the same alert definitions.

## See also

- [Alert Configuration Reference](/src/health/REFERENCE.md) — full manual configuration syntax
- [Alerts Configuration Manager](/docs/alerts-and-notifications/creating-alerts-with-netdata-alerts-configuration-manager.md) — visual UI wizard for alert configuration
- [ML Anomaly Detection](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md) — machine learning based anomaly scoring
- [Alert Troubleshooting](/docs/troubleshooting/troubleshoot.md) — AI-powered alert investigation
- [Netdata AI Overview](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md) — complete AI capabilities guide

:::note
Despite our best efforts to reduce inaccuracies, AI responses may sometimes be incorrect. Please review generated configurations before deploying to production systems.
:::
