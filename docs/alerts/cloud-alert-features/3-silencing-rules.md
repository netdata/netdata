# 10.2 Silencing Rules Manager

1. Navigate to **Settings** → **Silencing Rules**
2. Click **+ Create Rule**

## 10.2.1 Rule Scope

```yaml
name: Weekend Maintenance
scope:
  nodes: env:production
  alerts: "*"
schedule:
  - every: Saturday 1:00 AM
    to: Monday 6:00 AM
```

## 10.2.2 Related Sections

- **4.3 Silencing in Netdata Cloud** for Cloud-level silencing workflows
- **4.2 Silencing vs Disabling** for conceptual difference