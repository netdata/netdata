# PowerStore metric fixtures

These are synthetic regression fixtures, not captures from a physical array. Resource IDs and values are examples.

The metric field names follow Dell's API models in
[dell/gopowerstore at 7a1d011963a0d42ba424de2992ae9ccca37091c6](https://github.com/dell/gopowerstore/blob/7a1d011963a0d42ba424de2992ae9ccca37091c6/metrics_types.go).
The interval mismatch and fractional login response were reported in
[netdata/netdata#24447](https://github.com/netdata/netdata/issues/24447).

- `metrics_perf_*.json` models `Five_Mins` rollups: `avg_*` IOPS, bandwidth, latency, Ethernet rates, and logins.
- Appliance CPU includes different average and maximum ratios to verify selection of the maximum. Dell's
  [exporter Zabbix template](https://github.com/dell/powerstore-metrics-exporter/blob/564df4d66274f318092529986288baeca12ec798/templates/zabbix/zbx_export_templates.yaml#L1156-L1170)
  multiplies CPU utilization by 100 to display a percentage.
- Cluster and appliance space model `One_Day` rollups; volume space models `Five_Mins`. All use `last_*` values.
- Drive wear and copy/replication keep their unprefixed fields, as defined by Dell's `WearMetricsByDriveResponse`
  and `CopyMetricsCommonFields`.

The mock server validates the requested interval. Collection tests check decoded values rather than merely checking
that a metric exists. Live PowerStoreOS and PowerStore Manager validation still requires access to an array.
