# 12.4 Patterns for Large and Distributed Environments

Large environments face different challenges than small ones. Alert volume scales with environment size, and manual management becomes impossible.

## Template-Based Alert Configuration

Define alert templates parameterized per service. A template for database alerts can accept service name, instance count, and critical thresholds as parameters. This single template generates consistent alerts across all instances.

Store templates in version control alongside configurations. When templates change, all instances benefit from the improvement without manual per-service updates.

## Parent-Based Alerting

Use Parent-based alerting for hierarchical setups to centralize control. Child nodes send metrics to parents; parents evaluate alerts for aggregate views.

Design hierarchical alerting to respect ownership boundaries. A team responsible for a service should receive alerts for that service regardless of where in the hierarchy they originate.

Avoid duplication across hierarchical levels. A condition should alert at one level, not at multiple levels producing redundant notifications.

## Streaming and Distributed Topologies

For multi-region or multi-cloud deployments, consider how alert evaluation handles distributed data. Some alerts require local evaluation; others aggregate across regions.

Design alert scope to match data locality. Alerts on local metrics evaluate locally. Alerts on aggregate metrics need centralized evaluation.

## Avoiding Duplication

In large environments, the same alert may fire across many nodes. Alert deduplication and aggregation prevent notification storms from distributed problems.

Configure Cloud deduplication to combine similar alerts from multiple sources. This reduces noise while preserving visibility into problem scope.

Use labels and tags to scope alerts appropriately. An alert should fire for the appropriate scope: per-instance, per-service, or per-cluster.

## Automated Alert Management

Automate routine alert maintenance. Scripts can audit alert configurations for common problems, generate reports on alert volume, and identify alerts that have not fired recently.

Automate alert deployment and testing. When alert configurations change, automated tests should validate correctness before deployment.

## What's Next

- **12.5 SLIs and SLOs** for connecting alerts to business objectives
- **13. Alerts and Notifications Architecture** for internal behavior details
- **1. Understanding Alerts in Netdata** to revisit fundamentals