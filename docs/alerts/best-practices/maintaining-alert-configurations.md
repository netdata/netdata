# 12.7 Maintaining Alert Configurations Over Time

Alert configurations require ongoing maintenance. Without regular attention, they drift out of alignment with the environment they monitor.

## Regular Review Cadence

Review alert configurations quarterly. This review should verify thresholds still match current workloads, notification routing remains accurate, and all active alerts remain necessary.

Decommission unused alerts. Alerts that have not fired in months may no longer be relevant. Investigate whether they fire rarely because the condition is rare or because no one is looking.

Update thresholds as workloads change. A CPU threshold calibrated for a ten-node cluster may need adjustment as the cluster grows to twenty nodes.

## Managing Alert Volume

As services grow, alert volume grows. At some point, the volume becomes unmanageable. Address this by consolidating alerts, reducing noise from individual services, and establishing clear priorities.

Aggregate alerts at the service level. Instead of alerting on every instance, alert when a significant percentage are unhealthy.