# 10.3 Alert Deduplication and Aggregation

Cloud automatically deduplicates alerts from multiple nodes.

## 10.3.1 How It Works

| Scenario | Without Cloud | With Cloud |
|----------|--------------|-------------|
| Same alert on 5 nodes | 5 notifications | 1 aggregated |

## 10.3.2 Aggregated View

```text
⚠️ 3 nodes with high CPU
├─ prod-db-01: 95%
├─ prod-db-02: 92%
└─ prod-db-03: 98%
```

## 10.3.3 Related Sections

- **[12.4 Large Environment Patterns](../best-practices/4-scaling-large-environments.md)** for multi-node setups
- **[13.6 Scaling Topologies](../architecture/5-scaling-topologies.md)** for complex topologies