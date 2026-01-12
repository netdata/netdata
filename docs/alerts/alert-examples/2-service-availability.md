# 6.2 Service and Availability Alerts

## 6.2.1 TCP Port Unreachable

```conf
template: tcp_port_closed
    on: portcheck.status
lookup: average -1m of failed
    every: 1m
     crit: $this > 0
       to: ops-team
```

## 6.2.2 HTTP Endpoint Health

```conf
template: http_endpoint_failing
    on: httpcheck.status
lookup: average -1m of bad_status
    every: 1m
     warn: $this > 0
     crit: $this > 10
       to: ops-team
```

## 6.2.3 Stale Collector Alert

```conf
template: collector_stale
    on: health.collector
lookup: average -5m of age
    units: seconds
    every: 1m
     warn: $this > 300
     crit: $this > 600
       to: ops-team
```

## 6.2.4 Related Sections

- **6.3 Application Alerts** - Database and web server examples
- **6.4 Anomaly-Based Alerts** - ML-driven detection