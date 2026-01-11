# 11.8 Managing Alert Configurations Over Time

Alert configurations require ongoing maintenance to remain aligned with the environment they monitor.

## Regular Review Cadence

Review alert configurations quarterly. This review should verify that thresholds still match current workloads, that notification routing remains accurate, and that all active alerts remain necessary.

During quarterly reviews, check for several indicators. Alerts that have never fired may indicate thresholds set too high or conditions that no longer occur in your environment. Alerts that fire daily without action indicate noise that should be silenced or thresholds that should be adjusted.

Look for drift in alert behavior. An alert that used to fire rarely but now fires frequently may indicate changing workload patterns that require threshold adjustments.

## Decommissioning Unused Alerts

Alerts that have not fired in months may no longer be relevant. Before removing an alert, investigate whether it fires rarely because the condition is rare or because no one is looking.

If investigation reveals the alert serves no purpose, remove it from configuration. Unused alerts create maintenance overhead without providing value.

Document the removal decision. A log of removed alerts and the reasoning helps future maintainers understand the configuration history.

## Updating Thresholds As Workloads Change

Workloads evolve over time. A CPU threshold calibrated for a ten-node cluster may need adjustment as the cluster grows to twenty nodes. Memory patterns change as applications add features or optimize their footprint.

Monitor the relationship between alert behavior and workload changes. When significant workload changes occur, review affected alerts proactively rather than waiting for problems to reveal misconfigurations.

## Coordination with Upgrade Cycles

Netdata upgrades may update stock alerts. After any upgrade, review stock alert changes and their impact on custom configurations.

The upgrade may add new stock alerts that provide valuable monitoring. The upgrade may modify thresholds in stock alerts that your customizations should reflect.

After upgrades, run `netdatacli health configuration` to compare your effective configuration against the updated stock configuration.