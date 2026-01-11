# 11.7 Adjusting Stock Alerts

Stock alerts provide conservative defaults, but your environment may require different thresholds or additional alerts entirely. The recommended approach is to copy stock alerts into `/etc/netdata/health.d/` and modify them there.

## When to Adjust Stock Alerts

Consider adjusting stock alerts when thresholds do not match your workload characteristics. A CPU alert at 80% may be appropriate for some workloads but trigger daily for CPU-intensive batch jobs.

Consider adjusting when notification destinations do not match your team structure. Stock alerts may route to generic addresses that no one monitors.

Consider adjusting when alert timing does not match your operational model. Some organizations need faster detection; others prioritize noise reduction over detection speed.

## How to Adjust Stock Alerts Safely

The recommended method is to copy stock alerts to the custom layer before modifying. Create a file in `/etc/netdata/health.d/` with a descriptive name like `custom-system-alerts.conf`.

Copy only the alerts you intend to modify. This preserves the stock configuration for alerts that work correctly and isolates your changes to only what requires adjustment.

When adjusting thresholds, document the reasoning. A threshold adjustment without documented rationale is difficult to review and impossible to audit later.

## Threshold Adjustment Guidelines

When adjusting thresholds, start from observed behavior. Run `netdatacli health alarm values` to see current metric values. Identify where normal operation falls relative to stock thresholds.

Adjust in small increments. A threshold moved from 80% to 85% provides different behavior than a jump to 95%. Small adjustments let you find the right balance for your environment.

Test changes in a non-production environment when possible. This lets you verify behavior without affecting production monitoring.

## Disabling Stock Alerts

Avoid disabling alerts entirely unless you have replaced them with equivalent monitoring. A disabled stock alert without replacement creates a monitoring gap.

If a stock alert does not apply to your environment, disable it explicitly rather than ignoring it. Create a file in `/etc/netdata/health.d/` that contains only the alerts being disabled:

```conf
# Disable alerts that do not apply to our environment
alarm: mysql_gtid_binlog_gtid_0
```

This explicit disabling makes the intent clear and prevents accidental re-enablement during upgrades.