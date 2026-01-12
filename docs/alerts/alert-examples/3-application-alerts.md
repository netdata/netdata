# 6.3 Application-Level Alerts

## 6.3.1 MySQL Alerts

```conf
template: mysql_slow_queries
    on: mysql.queries
lookup: average -5m of slow_queries
    every: 1m
     warn: $this > 10
     crit: $this > 100
       to: dba-team
```

## 6.3.2 PostgreSQL Alerts

```conf
template: pg_deadlocks_high
    on: postgres.db_deadlocks_rate
lookup: average -5m of deadlocks
    every: 1m
     warn: $this > 0
     crit: $this > 5
       to: dba-team
```

## 6.3.3 Redis Alerts

```conf
template: redis_rejected_connections
    on: redis.connections
lookup: average -5m of rejected
    every: 1m
     warn: $this > 0
       to: cache-team
```

## 6.3.4 Related Sections

- **6.4 Anomaly-Based Alerts** - ML-based detection
- **6.5 Trend Alerts** - Capacity planning examples