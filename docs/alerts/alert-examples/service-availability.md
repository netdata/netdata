# 6.2 Service and Availability Alerts

## 6.2.1 Service Not Running

```conf
template: service_not_running
    on: health.service
lookup: average -1m of status
    every: 1m
     crit: $this == 0
       to: ops-team
```

## 6.2.2 Stale Collector Alert

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

## 6.2.3 HTTP Endpoint Health

```conf
template: http_endpoint_failing
    on: health.service
lookup: average -1m of status
    every: 1m
     warn: $this == 0
     crit: $this == 0
       to: ops-team
```

## 6.2.4 TCP Port Unreachable

```conf
template: tcp_port_closed
    on: health.service
lookup: average -1m of failed
    every: 1m
     crit: $this > 0
       to: ops-team
```

## 6.2.5 Related Sections

- **6.3 Application Alerts** - Database and web server examples
- **6.4 Anomaly-Based Alerts** - ML-driven detection