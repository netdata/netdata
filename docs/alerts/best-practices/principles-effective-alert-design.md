# 12.1 Principles of Effective Alert Design

Effective alerts share characteristics that distinguish them from alerts generating noise without value.

## Alerts Should Drive Action

Every alert should represent a situation requiring someone to take action. Alerts that fire automatically resolve, fire on expected conditions, or fire on conditions no one plans to address create noise without purpose.

Before creating an alert, complete this sentence: "When this alert fires, [specific person] will [specific action] within [specific timeframe]." If you cannot complete the sentence, reconsider whether the alert serves a purpose.

Actionable alerts have clear ownership. Someone is responsible for investigating each alert that fires. When alerts fire in off-hours, the on-call rotation includes escalation paths. When alerts fire during business hours, the responsible team has bandwidth to respond.

## Alerts Should Minimize False Positives

False positives train responders to ignore alerts. The first few times an alert fires for a non-problem, responders investigate. After dozens of false activations, responders learn to assume alerts are noise and begin ignoring them. This conditioning is nearly impossible to reverse.

Minimizing false positives does not mean making alerts less sensitive. It means tuning thresholds to match your environment's normal behavior and adding context that distinguishes genuine problems from expected variation.

Consider using multiple thresholds for the same metric. A warning threshold triggers investigation; a critical threshold triggers immediate response. This graduated response helps operators prioritize.

## Alerts Should Be Specific and Diagnosable

Vague alerts waste time. An alert stating "system problem" requires investigation to identify the issue. An alert stating "disk space on /var below 5%" enables immediate action.

Diagnosable alerts include context. When firing, an alert should state what is wrong, which resource is affected, and how severe the situation is.

Include relevant values in alert messages. When a threshold is exceeded, include the actual value that triggered the alert. This helps responders prioritize and provides starting points for investigation.