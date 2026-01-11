# 12.5 SLIs, SLOs, and How They Relate to Alerts

Service level indicators and objectives provide a framework for connecting alerts to business outcomes. This connection ensures that alerting investment focuses on what matters to the business.

## Defining SLIs for Your Services

An SLI measures some aspect of service behavior. Common SLIs include latency, availability, throughput, and error rate. Define SLIs reflecting your users' experience.

Align alerts with SLIs. A CPU alert at 90% may not map directly to any SLI, but a latency alert at 500ms does. The latency alert is more directly tied to user experience.

When designing alerts, ask: "What SLI does this alert protect?" If no SLI connects to the alert, reconsider whether the alert serves a purpose.

## Setting SLO-Based Thresholds

SLOs define acceptable service behavior. An SLO might state that 99.9% of requests complete within 500ms. Alerts should fire before SLO violations occur.

For an SLO with 99.9% availability, alerts should fire when availability drops below 99.9%. This allows investigation before the SLO is breached.

The gap between alert threshold and SLO breach provides reaction time. This buffer prevents SLO breaches from becoming routine.

Use historical data to calibrate SLO-based thresholds. Analyze SLI behavior over time to identify the relationship between warning signs and actual breaches.

## Connecting Alerts to Business Impact

Alerts should eventually connect to business impact. An alert that fires with no business impact wastes responder time; an alert connecting to revenue impact or user experience gets appropriate attention.

Define business impact categories for your organization. Map alerts to these categories during alert design. This mapping helps prioritization during incident response.

Connect alert thresholds to business objectives. For example, an HTTP error rate alert might trigger at 0.3% because 0.3% error rate impacts an SLO of 99.9% availability.

## What's Next

- **13. Alerts and Notifications Architecture** for deep-dive internals
- **1. Understanding Alerts in Netdata** to start building alerts
- **2. Creating and Managing Alerts** for practical alert creation