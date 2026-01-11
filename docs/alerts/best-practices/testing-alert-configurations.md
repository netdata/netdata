# 12.6 Testing Alert Configurations

Alert configurations should be tested before deployment. Untested alerts frequently have configuration errors preventing them from firing when needed.

## Syntax Validation

Netdata configurations have specific syntax requirements. Use `netdatacli reload-health` to validate configurations before deployment. This catches syntax errors that would prevent alerts from loading.

Review configuration files with linters or validators specific to your alert patterns.

## Threshold Testing

Thresholds should be tested against representative data. Before deploying a CPU alert at 90%, verify that it fires for actual CPU patterns and does not fire for normal variation.

Simulate conditions that trigger alerts. If an alert should fire when memory drops below 10%, create the condition temporarily to verify that the alert fires correctly.

## Notification Testing

Configure test notification destinations for alert testing. Send test notifications to channels that are not production-critical first to verify formatting and routing. Then verify end-to-end delivery to the intended production destination.