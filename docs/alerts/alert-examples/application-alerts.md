# 6.3 Application-Level Alerts

## 6.3.1 MySQL Alerts

```conf
template: mysql_slow_queries
    on: mysql.global_status
lookup: average -5m of slow_queries
    every: 1m
     warn: $this > 10
     crit: $this > 100
       to: dba-team
```

## 6.3.2 PostgreSQL Alerts

```conf
template: pg_deadlocks_high
    on: pg.stat_database
lookup: average -5m of deadlocks
    every: 1m
     warn: $this > 0
     crit: $this > 5
       to: dba-team
```

## 6.3.3 Redis Alerts

```conf
template: redis_clients_high
    on: redis.clients
lookup: average -5m of connected
    every: 1m
     warn: $this > 10000
     crit: $this > 50000
       to: cache-team
```

## 6.3.4 Nginx Alerts

```conf
template: nginx_errors_high
    on: nginx.requests
lookup: average -5m of 5xx
    every: 1m
     warn: $this > 10
     crit: $this > 100
       to: web-team
```

## 6.3.5 Related Sections

- **6.4 Anomaly-Based Alerts** - ML-based detection
- **6.5 Trend Alerts** - Capacity planning examples