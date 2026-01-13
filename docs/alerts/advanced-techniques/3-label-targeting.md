# 8.3 Host, Chart, and Label-Based Targeting

Labels enable fine-grained alert scoping to specific hosts, services, or chart instances using metadata attached to nodes and charts.

## 8.3.1 Understanding Labels

Netdata supports:
- **Host labels**: `env:production`, `role:database`
- **Chart labels**: `mount_point:/data`

## 8.3.2 Label-Based Alert Scope

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -5m of user,system
     every: 1m
      warn: $this > 80
    calc: if($host_labels.role == "database" && $host_labels.env == "production", $this, 0)
```

This targets only production database servers.

## 8.3.3 Related Sections

- **[3.6 Optional Metadata](../alert-configuration-syntax/6-optional-metadata.md)** for labels documentation
- **[4.4 Room-Based Alerting](../cloud-alert-features/2-room-based.md)** for Cloud label usage

## What's Next

- **[8.4 Custom Actions](4-custom-actions.md)** for exec-based automation
- **[8.5 Performance Considerations](5-performance.md)** for optimizing alert evaluation