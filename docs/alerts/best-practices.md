# 12. Best Practices for Alerting

This chapter provides guidance for maintainable alerting.

## What You'll Find in This Chapter

| Section | Topic |
|---------|-------|
| **12.1 Designing Useful Alerts** | Principles for effective alerts |
| **12.2 Notification Strategy** | On-call, severity levels |
| **12.3 Maintaining Configurations** | Version control, reviews |
| **12.4 Large Environment Patterns** | Parent-based, deduplication |
| **12.5 SLIs and SLOs** | Connecting to business objectives |

## 12.1 Designing Useful Alerts

Before creating an alert, answer:
- Will someone act on this?
- How urgent is it?
- What's the false positive rate?

## 12.2 Notification Strategy

| Severity | When Used | Example |
|----------|-----------|---------|
| CRITICAL | Immediate action | Site down |
| WARNING | Action within hours | High latency |
| CLEAR | Information | Resolution recorded |

## 12.3 Maintaining Configurations

Keep alerts in Git for:
- Audit trail
- Easy rollback
- Code review
- CI/CD deployment

## 12.4 Large Environment Patterns

Use Parent-based alerting for hierarchical setups to centralize control.

## 12.5 SLIs and SLOs

Connect alert thresholds to business objectives:

```conf
template: http_error_rate
    on: nginx.requests
lookup: average -5m of 5xx
     warn: $this > 0.3  # 0.3% error rate
     crit: $this > 0.5   # 0.5% error rate
     info: ${value}% impacts SLO of 99.9% availability
```

## What's Next

- **13. Alerts Architecture** for deep-dive internals
- **6. Alert Examples** for practical templates