# 13. Alerts and Notifications Architecture

This chapter provides a deep-dive into internal behavior.

## What You'll Find in This Chapter

| Section | Topic |
|---------|-------|
| **13.1 Evaluation Architecture** | Where alerts run |
| **13.2 State Machine** | Alert lifecycle |
| **13.3 Notification Dispatch** | Alert to notification path |
| **13.4 Configuration Layers** | Precedence rules |
| **13.5 Scaling Considerations** | Complex topologies |

## 13.1 Evaluation Architecture

Alerts are **evaluated locally** on each Agent or Parent:

- **Agent** evaluates alerts for local metrics
- **Parent** evaluates alerts for streamed metrics
- **Cloud** receives events, no evaluation

## 13.2 The State Machine

```
UNINITIALIZED → CLEAR → WARNING → CRITICAL
                            ↓         ↓
                       ← CLEAR ← WARNING ← CRITICAL
```

## 13.3 Notification Dispatch

1. Alert fires
2. Check repeat interval
3. Queue notification
4. Dispatch to configured methods (Slack, PagerDuty, etc.)

## 13.4 Configuration Layers

| Layer | Location | Priority |
|-------|----------|-----------|
| Stock | `/usr/lib/netdata/conf.d/health.d/` | Lowest |
| Custom | `/etc/netdata/health.d/` | Middle |
| Cloud | Cloud-defined alerts | Highest |

## 13.5 Scaling Considerations

Multi-Parent setups:
- Children evaluate local alerts
- Parents aggregate cluster-wide alerts
- Cloud deduplicates across Parents

## What's Next

- **9. APIs for Alerts and Events** for programmatic control
- **12. Best Practices** for operational guidance