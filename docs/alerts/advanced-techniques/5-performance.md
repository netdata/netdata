# 8.5 Performance Considerations

## 8.5.1 What Affects Performance

| Factor | Impact | Recommendation |
|--------|--------|----------------|
| `every` frequency | Higher = more CPU | Use 1m for most alerts |
| `lookup` window | Longer windows need more processing | Match to needs |
| Number of alerts | More alerts = more evaluation | Disable unused alerts |

## 8.5.2 Efficient Configuration

```conf
# Efficient: 1-minute evaluation
template: 10min_cpu_usage
    on: system.cpu
lookup: average -5m of user,system
     every: 1m     # Good: 60 evaluations/hour
      warn: $this > 80

# Inefficient: 10-second evaluation
template: 10min_cpu_usage
    on: system.cpu
lookup: average -1m of user,system
     every: 10s    # Bad: 360 evaluations/hour
```

## 8.5.3 Related Sections

- **13.1 Evaluation Architecture** for internal behavior