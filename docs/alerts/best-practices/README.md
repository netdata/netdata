# 11. Best Practices for Alerting

Effective alerting balances detection speed against noise. This chapter provides guidance derived from operational experience across thousands of Netdata deployments, helping you create alerts that drive meaningful action without alert fatigue.

:::tip

Before creating any alert, complete this sentence: "When this alert fires, **[specific person]** will **[specific action]** within **[specific timeframe]**." If you cannot complete the sentence, reconsider whether the alert serves a purpose.

:::

## What's Included

This chapter covers five practical areas:

- **[11.1 Designing Useful Alerts](/docs/alerts/best-practices/1-designing-useful-alerts.md)** — Principles for creating alerts that drive action
- **[11.2 Notification Strategy](/docs/alerts/best-practices/2-notification-strategy.md)** — Routing alerts and on-call hygiene
- **[11.3 Maintaining Configurations](/docs/alerts/best-practices/3-maintaining-configurations.md)** — Version control and ongoing upkeep
- **[11.4 Large Environment Patterns](/docs/alerts/best-practices/4-scaling-large-environments.md)** — Templates, hierarchies, and distributed setups
- **[11.5 SLIs, SLOs, and Alerts](/docs/alerts/best-practices/5-sli-slo-alerts.md)** — Connecting alerts to business objectives

## Related Sections

- **[4. Controlling Alerts and Noise](/docs/alerts/controlling-alerts-noise/README.md)** - Methods to reduce alert fatigue
- **[5. Receiving Notifications](/docs/alerts/receiving-notifications/README.md)** - Configure destinations and routing
- **[7. Troubleshooting Alert Behaviour](/docs/alerts/troubleshooting-alerts/README.md)** - Diagnose and resolve alert issues