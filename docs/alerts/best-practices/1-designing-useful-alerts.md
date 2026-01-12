# 12.1 Designing Useful Alerts

Effective alerting requires balancing multiple concerns: detecting problems quickly, avoiding noise that causes fatigue, and ensuring that alerts drive meaningful action. This chapter provides guidance derived from operational experience across thousands of Netdata deployments.

:::tip
Before creating any alert, complete this sentence: "When this alert fires, **[specific person]** will **[specific action]** within **[specific timeframe]**." If you cannot complete the sentence, reconsider whether the alert serves a purpose.
:::

## 12.1.1 Principles of Effective Alert Design

Effective alerts share characteristics that distinguish them from alerts generating noise without value.

### 12.1.1.1 Alerts Should Drive Action

Every alert should represent a situation requiring someone to take action. Alerts that fire automatically resolve, fire on expected conditions, or fire on conditions no one plans to address create noise without purpose.

| Characteristic | Description |
|----------------|-------------|
| **Actionable** | Someone must do something when the alert fires |
| **Owned** | Specific person or team is responsible |
| **Urgent** | Requires response within defined timeframe |
| **Specific** | Clearly states what is wrong and where |

Actionable alerts have clear ownership. Someone is responsible for investigating each alert that fires. When alerts fire in off-hours, the on-call rotation includes escalation paths. When alerts fire during business hours, the responsible team has bandwidth to respond.

### 12.1.1.2 Alerting on Symptoms Rather Than Causes

Alert on symptoms rather than causes when possible. A symptom alert like "response time degraded" points directly to user impact. A cause alert like "CPU at 90%" requires investigation to determine if it relates to the response time degradation.

| Alert Type | Example | Value |
|-----------|----------|-------|
| **Symptom** | Response time degraded | User-facing impact |
| **Cause** | CPU at 90% | May or may not affect users |

Symptoms are conditions that users experience. Causes are technical conditions that may or may not affect users. Prioritizing symptoms ensures alerts represent genuine problems.

### 12.1.1.3 Keep Alert Rules Minimal

Every additional alert creates maintenance burden and potential noise. Before adding an alert, consider whether existing alerts cover the same concern.

Use templates over duplication. Rather than creating identical alerts for multiple instances of the same service, create one template that applies to all instances. This reduces configuration complexity and ensures consistent behavior.

### 12.1.1.4 Minimize False Positives

False positives train responders to ignore alerts. The first few times an alert fires for a non-problem, responders investigate. After dozens of false activations, responders learn to assume alerts are noise and begin ignoring them.

Minimizing false positives does not mean making alerts less sensitive. It means tuning thresholds to match your environment's normal behavior and adding context that distinguishes genuine problems from expected variation.

Use multiple thresholds for the same metric. A warning threshold triggers investigation; a critical threshold triggers immediate response. This graduated response helps operators prioritize.

### 12.1.1.5 Alerts Should Be Specific and Diagnosable

Vague alerts waste time. An alert stating "system problem" requires investigation to identify the issue. An alert stating "disk space on /var below 5%" enables immediate action.

Diagnosable alerts include context. When firing, an alert should state what is wrong, which resource is affected, and how severe the situation is.

Include relevant values in alert messages. When a threshold is exceeded, include the actual value that triggered the alert. This helps responders prioritize and provides starting points for investigation.

## 12.1.2 Naming Conventions

Consistent naming conventions make alerts searchable and filterable.

| Good Naming Pattern | Example | Why It Works |
|--------------------|---------|--------------|
| `{service}_{metric}_{threshold}` | `mysql_connections_warning` | Immediately understandable |
| `{component}_{condition}` | `disk_space_critical` | Clear severity indication |
| `{location}_{issue}` | `var_disk_full` | Pinpoints location |

Avoid generic names providing no context. Names like `warning1` or `alert_cpu` tell nothing about when the alert fires or what to do.

Group related alerts by service or component. This helps responders find relevant alerts during incidents and helps with long-term alert management.

## What's Next

- [12.2 Notification Strategy](2-notification-strategy.md) - Routing alerts to the right people
- [12.3 Maintaining Configurations](3-maintaining-configurations.md) - Ongoing upkeep
- [12.4 Large Environment Patterns](4-scaling-large-environments.md) - Scaling best practices
- [12.5 SLI and SLO Alerts](5-sli-slo-alerts.md) - Connecting alerts to business objectives

## See Also

- [Creating and Managing Alerts](../../creating-alerts-pages/creating-alerts.md) - Practical alert creation
- [Controlling Alerts and Noise](../controlling-alerts-noise/index.md) - Reducing alert noise
- [Built-In Alerts Reference](../built-in-alerts/index.md) - Stock alerts to build upon