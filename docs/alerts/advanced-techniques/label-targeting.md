# 8.3 Host, Chart, and Label-Based Targeting

## 8.3.1 Understanding Labels

Netdata supports:
- **Host labels**: `env:production`, `role:database`
- **Chart labels**: `mount_point:/data`

## 8.3.2 Label-Based Alert Scope

```conf
template: db_cpu_high
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: $this > 80
   calc: if($host_labels.role == "database" && $host_labels.env == "production", $this, 0)
```

This targets only production database servers.

## 8.3.3 Related Sections

- **3.6 Optional Metadata** for labels documentation
- **10.4 Room-Based Alerting** for Cloud label usage