# 12.5 SLIs, SLOs, and How They Relate to Alerts

Service level indicators and objectives provide a framework for connecting alerts to business outcomes. This connection ensures that alerting investment focuses on what matters to the business.

:::tip
When designing any alert, ask: "What SLI does this alert protect?" If no SLI connects to the alert, reconsider whether it serves a purpose.
:::

## 12.5.1 Defining SLIs for Your Services

An SLI measures some aspect of service behavior. Common SLIs include latency, availability, throughput, and error rate. Define SLIs reflecting your users' experience.

| SLI Category | Example Metrics |
|--------------|----------------|
| **Latency** | Response time, 95th percentile |
| **Availability** | Uptime percentage, error rate |
| **Throughput** | Requests per second, transactions/min |
| **Freshness** | Data staleness, update delay |

Align alerts with SLIs. A CPU alert at 90% may not map directly to any SLI, but a latency alert at 500ms does. The latency alert is more directly tied to user experience.

## 12.5.2 Setting SLO-Based Thresholds

SLOs define acceptable service behavior. An SLO might state that 99.9% of requests complete within 500ms. Alerts should fire before SLO violations occur.

| SLO Threshold | Alert Threshold | Buffer |
|---------------|-----------------|---------|
| 99.9% availability | 99.95% availability | 0.05% warning |
| 500ms latency | 450ms | 10% headroom |
| 99.95% uptime | 99.97% | 0.02% reaction time |

For an SLO with 99.9% availability, alerts should fire when availability drops below 99.9%. This allows investigation before the SLO is breached.

The gap between alert threshold and SLO breach provides reaction time. This buffer prevents SLO breaches from becoming routine.

Use historical data to calibrate SLO-based thresholds. Analyze SLI behavior over time to identify the relationship between warning signs and actual breaches.

## 12.5.3 Connecting Alerts to Business Impact

Alerts should eventually connect to business impact. An alert that fires with no business impact wastes responder time; an alert connecting to revenue impact or user experience gets appropriate attention.

| Impact Level | Examples | Response |
|--------------|----------|----------|
| **Revenue** | Payment failures, checkout errors | Immediate (< 5 min) |
| **User Experience** | Slow page loads, timeouts | Rapid (< 15 min) |
| **Operational** | CPU spikes, disk usage | Business hours |
| **Informational** | Minor errors, warnings | Review next day |

Define business impact categories for your organization. Map alerts to these categories during alert design. This mapping helps prioritization during incident response.

Connect alert thresholds to business objectives. For example, an HTTP error rate alert might trigger at 0.3% because 0.3% error rate impacts an SLO of 99.9% availability.

## What's Next

- [13. Alerts and Notifications Architecture](../architecture/index.md) - Deep-dive internals
- [1. Understanding Alerts in Netdata](../../understanding-alerts/index.md) - Start building alerts
- [2. Creating and Managing Alerts](../../creating-alerts-pages/creating-alerts.md) - Practical alert creation

## See Also

- [Designing Useful Alerts](designing-useful-alerts.md) - Alert design principles
- [Notification Strategy](notification-strategy.md) - Routing based on severity
- [Alert Examples and Common Patterns](../../alert-examples/index.md) - SLI-based alert templates