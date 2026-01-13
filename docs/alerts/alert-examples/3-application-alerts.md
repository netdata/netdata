# 6.3 Application-Level Alerts

These templates demonstrate application-specific monitoring using contexts provided by database and web server collectors.

:::tip

The examples below show simplified alert configurations. Stock alerts include additional metadata fields and conditional thresholds. These examples highlight the essential parameters for quick implementation.

:::

## 6.3.1 MySQL Alerts

```conf
template: mysql_10s_slow_queries
    on: mysql.queries
lookup: sum -10s of slow_queries
     units: slow queries
     every: 10s
       warn: $this > 5
       crit: $this > 10
      delay: down 5m multiplier 1.5 max 1h
         to: dba-team
```

## 6.3.2 PostgreSQL Alerts

```conf
template: postgres_db_deadlocks_rate
    on: postgres.db_deadlocks_rate
lookup: average -5m of deadlocks
     every: 1m
      warn: $this > 0
      crit: $this > 5
        to: dba-team
```

## 6.3.3 Redis Alerts

```conf
template: redis_connections_rejected
    on: redis.connections
lookup: average -5m of rejected
     every: 1m
      warn: $this > 0
        to: cache-team
```

## 6.3.4 Related Sections

- **[6.4 Anomaly-Based Alerts](4-anomaly-alerts.md)** - ML-based detection
- **[6.5 Trend and Capacity Alerts](5-trend-capacity.md)** - Capacity planning examples

## What's Next

- **[Chapter 7: Troubleshooting Alerts](../troubleshooting-alerts/index.md)** - Debugging alert issues
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Configure alert delivery