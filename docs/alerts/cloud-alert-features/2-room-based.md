# 10.4 Room-Based Alerting

Rooms organize nodes and scope alerts to specific groups.

## 10.4.1 Creating Rooms

1. Navigate to **Settings** → **Rooms**
2. Click **+ Create Room**
3. Add nodes by label:

```yaml
name: Production Databases
criteria:
  - label: env == production
  - label: role == database
```

## 10.4.2 Room-Specific Alerts

Create alerts scoped to specific rooms in the Cloud UI.

## 10.4.3 Related Sections

- **8.3 Label-Based Targeting** for label usage
- **12.4 Large Environment Patterns** for multi-room strategies