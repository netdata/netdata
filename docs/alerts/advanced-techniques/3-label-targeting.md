# 8.3 Host, Chart, and Label-Based Targeting

Labels enable fine-grained alert scoping to specific hosts, services, or chart instances using metadata attached to nodes and charts.

## 8.3.1 Understanding Labels

Netdata supports:
- **Host labels**: `env:production`, `role:database`
- **Chart labels**: `mount_point:/data`

## 8.3.2 Label-Based Alert Scope

Labels enable scoping alerts to specific hosts or chart instances. Use `host labels:` to target hosts with specific labels:

```conf
template: 10min_cpu_usage
      on: system.cpu
   class: Utilization
    type: System
host labels: role=database
      lookup: average -10m unaligned of user,system
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 95
       to: dba-team
```

This creates alerts only for hosts with `role=database` label. The alert still fires for all matching database hosts.

## 8.3.3 Related Sections

- **[3.6 Optional Metadata](../alert-configuration-syntax/6-optional-metadata.md)** for labels documentation
- **[4.4 Room-Based Alerting](../cloud-alert-features/2-room-based.md)** for Cloud label usage

## What's Next

- **[8.4 Custom Actions](4-custom-actions.md)** for exec-based automation
- **[8.5 Performance Considerations](5-performance.md)** for optimizing alert evaluation