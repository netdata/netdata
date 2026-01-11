# 13.7 Debugging and Troubleshooting

Alert problems often manifest as alerts not firing when expected or firing incorrectly.

## Checking Alert Configuration

Verify that alerts are loaded with `netdatacli health configuration`. Compare expected and actual configurations to identify discrepancies.

Check that alerts are enabled. Use `netdatacli health enabled` to list enabled status.

## Examining Alert Values

Query alert values with `netdatacli health alarm values`. This shows current variable values for firing alerts.

Examine lookup results separately using the query API. Compare query results against alert thresholds.

## Reviewing Logs

Alert evaluation logs contain detailed information about firing decisions. Check `/var/log/netdata/error.log` for evaluation diagnostics.

Filter logs by alert name to focus on specific problems.

## Testing Alert Firing

Force an alert to evaluate by temporarily lowering thresholds. This active testing confirms that the alert mechanism works correctly.

Use the health test API to simulate alert conditions without waiting for actual threshold crossings.