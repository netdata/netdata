# 12.3 Maintaining Alert Configurations Over Time

Alert configurations require ongoing maintenance to remain aligned with the environment they monitor. Without regular attention, they drift out of alignment and may miss genuine problems or generate excessive noise.

:::note
Schedule quarterly reviews to verify thresholds match current workloads and notification routing remains accurate.
:::

## 12.3.1 Version Control for Alert Configurations

Alert configurations are infrastructure code. They should live in version control, be reviewed via pull requests, and follow deployment practices consistent with other infrastructure.

| Benefit | Description |
|---------|-------------|
| **Audit trail** | Who modified what and when |
| **Rollback capability** | Revert problematic changes |
| **Testing** | Validate before deployment |
| **Collaboration** | PR review for all changes |

Version control provides audit trails for alert changes. When an alert was modified, who modified it, and why? This history matters for debugging and understanding the evolution of monitoring coverage.

Version control enables rollback. If a new alert introduces unexpected noise, reverting to the previous configuration should restore the previous state.

Version control enables testing. Alert configurations can be validated before deployment using static analysis tools, catching syntax errors before they affect production monitoring.

## 12.3.2 Regular Review Cadence

Review alert configurations quarterly. During quarterly reviews, check for several indicators:

| Indicator | Action Required |
|-----------|-----------------|
| Never fired | Review threshold or relevance |
| Fires daily without action | Silence or adjust threshold |
| Suddenly more frequent | Review workload patterns |
| Notification routing changed | Update routing rules |

Alerts that have never fired may indicate thresholds set too high or conditions that no longer occur in your environment. Alerts that fire daily without action indicate noise that should be silenced or thresholds that should be adjusted.

Look for drift in alert behavior. An alert that used to fire rarely but now fires frequently may indicate changing workload patterns that require threshold adjustments.

## 12.3.3 Cleaning Up Obsolete Alerts

Decommission unused alerts. Alerts that have not fired in months may no longer be relevant. Before removing an alert, investigate whether it fires rarely because the condition is rare or because no one is looking.

If investigation reveals the alert serves no purpose, remove it from configuration. Unused alerts create maintenance overhead without providing value.

Document the removal decision. A log of removed alerts and the reasoning helps future maintainers understand the configuration history.

## 12.3.4 Testing Alert Configurations

Alert configurations should be tested before deployment. Untested alerts frequently have configuration errors preventing them from firing when needed.

Use `netdatacli reload-health` to validate configurations before deployment. This catches syntax errors that would prevent alerts from loading.

Test thresholds against representative data. Before deploying a CPU alert at 90%, verify that it fires for actual CPU patterns and does not fire for normal variation.

Configure test notification destinations for alert testing. Verify formatting and routing before delivering to production channels.

## 12.3.5 Validating Behavior After Changes

After any configuration change, validate that alerts behave as expected. Test that alerts fire when conditions are met and remain silent when conditions are not met.

| Validation Step | Purpose |
|-----------------|---------|
| Syntax check | Catch configuration errors |
| Threshold test | Verify firing conditions |
| Notification test | Confirm routing |
| Staged rollout | Minimize impact |

Use staged rollouts for significant changes. Deploy to a subset of nodes first, observe behavior, then deploy more broadly.

Monitor alert volume after changes. An unexpected increase or decrease in firing count may indicate configuration errors.

## 12.3.6 Updating Thresholds As Workloads Change

Workloads evolve over time. A CPU threshold calibrated for a ten-node cluster may need adjustment as the cluster grows to twenty nodes. Memory patterns change as applications add features or optimize their footprint.

Monitor the relationship between alert behavior and workload changes. When significant workload changes occur, review affected alerts proactively rather than waiting for problems to reveal misconfigurations.

## What's Next

- [12.4 Large Environment Patterns](4-scaling-large-environments.md) - Parent-based and distributed setups
- [12.5 SLI and SLO Alerts](5-sli-slo-alerts.md) - Connecting to business objectives
- [13. Alerts and Notifications Architecture](../architecture/index.md) - Deep-dive internals

## See Also

- [Creating and Editing Alerts via Config Files](../../creating-alerts-pages/creating-and-editing-alerts-via-config-files.md) - File-based editing
- [Reloading and Validating Configuration](../../creating-alerts-pages/reloading-and-validating-alert-configuration.md) - Validation procedures
- [Disabling Alerts](../../controlling-alerts-noise/disabling-alerts.md) - How to temporarily disable alerts