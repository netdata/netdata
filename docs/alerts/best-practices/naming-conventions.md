# 12.4 Naming Conventions

Consistent naming conventions make alerts searchable and filterable. Establish naming patterns and follow them across all alerts.

## Naming Patterns

Effective alert names indicate the target, the condition, and optionally the severity. A name like `cpu_usage_critical_90` immediately communicates the target (cpu_usage), the condition (above 90%), and the severity (critical). This convention scales across all alert types.

Avoid generic names providing no context. Names like `warning1` or `alert_cpu` tell nothing about when the alert fires or what to do when it fires.

Include context in alert descriptions but not in names. Names should be concise; descriptions can be detailed.

## Organizing by Service and Component

Group alerts by the service or component they monitor. This grouping helps responders find alerts relevant to their area of ownership and helps capacity planning for alert volume.

Consider creating alert packages for each service. A database package includes all database-related alerts with consistent severity assignments and notification routing.