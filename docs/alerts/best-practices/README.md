# Chapter 12: Alert Best Practices

Effective alerting balances detection speed against noise. This chapter provides guidance derived from operational experience across thousands of Netdata deployments, helping you create alerts that drive meaningful action without alert fatigue.

:::tip

Before creating any alert, complete this sentence: "When this alert fires, **[specific person]** will **[specific action]** within **[specific timeframe]**." If you cannot complete the sentence, reconsider whether the alert serves a purpose.

:::

## 12.0.1 Principles of Alert Design

Effective alerts share characteristics that distinguish them from alerts generating noise without value.

### Alerts Should Drive Action

Every alert should represent a situation requiring someone to take action. Alerts that fire automatically resolve, fire on expected conditions, or fire on conditions no one plans to address create noise without purpose.

| Characteristic | Description |
|----------------|-------------|
| **Actionable** | Someone must do something when the alert fires |
| **Owned** | Specific person or team is responsible |
| **Urgent** | Requires response within defined timeframe |
| **Specific** | Clearly states what is wrong and where |

### Alerting on Symptoms Rather Than Causes

Alert on symptoms rather than causes when possible. A symptom alert like "response time degraded" points directly to user impact. A cause alert like "CPU at 90%" requires investigation to determine if it relates to the response time degradation.

| Alert Type | Example | Value |
|------------|---------|-------|
| **Symptom** | Response time degraded | User-facing impact |
| **Cause** | CPU at 90% | May or may not affect users |

### Minimize Alert Volume

Every additional alert creates maintenance burden and potential noise. Before adding an alert, consider whether existing alerts cover the same concern.

Use templates over duplication. Rather than creating identical alerts for multiple instances of the same service, create one template that applies to all instances. This reduces configuration complexity and ensures consistent behavior.

## 12.0.2 Reducing Noise and Flapping

Alert noise erodes trust. When responders learn to assume alerts are noise, they begin ignoring them—including the alerts representing genuine problems.

Techniques for reducing noise include setting appropriate hysteresis so alerts require sustained conditions before changing status, using delays to require conditions to hold for a defined period, and configuring repeat intervals to prevent notification spam for sustained conditions.

## 12.0.3 Scaling Alert Management

As infrastructure grows, alert management becomes increasingly complex. Strategies for scaling include centralizing alert definitions in version control, using consistent patterns across environments, and implementing tiered escalation paths.

## 12.0.4 What's Included

| Section | Description |
|---------|-------------|
| **12.1 Designing Useful Alerts** | Principles for creating actionable alerts |
| **12.2 Maintaining Configurations** | Version control, testing, and deployment strategies |
| **12.3 Notification Strategy** | Routing alerts to the right people at the right time |
| **12.4 Scaling Large Environments** | Managing alerts across hundreds of nodes |
| **12.5 SLI and SLO Alerts** | Connecting alerts to service level objectives |

## 12.0.5 Related Sections

- **Chapter 11**: Built-in alerts as reference implementations
- **Chapter 4**: Controlling alert noise and reducing flapping
- **Chapter 5**: Notification dispatch and routing
- **Chapter 7**: Troubleshooting alert behavior