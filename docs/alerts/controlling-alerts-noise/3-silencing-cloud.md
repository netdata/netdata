# 4.3 Silencing in Netdata Cloud

Netdata Cloud provides **silencing rules** that let you suppress notifications space-wide without modifying configuration files on individual nodes.

## 4.3.1 Creating Silencing Rules

**Via Cloud UI:**

1. Log in to Netdata Cloud
2. Navigate to **Settings** → **Silencing Rules** (or use the global search)
3. Click **+ Create Silencing Rule**
4. Configure the rule:

| Field | Description | Example |
|-------|-------------|---------|
| **Rule Name** | Descriptive identifier | `Weekend Maintenance` |
| **Match Scope** | What to silence | Alert name, context, node, labels |
| **Schedule** | When rule is active | `Every Saturday 1:00 AM to Monday 6:00 AM` |
| **Duration** | Optional fixed duration | `4 hours` |

**Example Rule:**

```yaml
# Silence all disk alerts on production nodes during maintenance
name: Production Maintenance
scope:
  nodes: env:production
  alerts: *disk*
schedule:
  - every: Saturday 1:00 AM
    to: Monday 6:00 AM
```

## 4.3.2 Silencing Patterns

Silencing rules support pattern matching:

| Pattern | Matches | Example |
|---------|---------|---------|
| `*` | Any characters | `*cpu*` matches `10min_cpu_usage` and `10min_cpu_iowait` |
| `?` | Single character | `disk_?` matches `disk_space_usage`, `disk_inode_usage` |
| `|` | OR logic | `mysql|postgres|redis` matches any of these |

## 4.3.3 Personal Silencing Rules

Each user can create **personal silencing rules** that only affect notifications sent to them:

1. Click your **profile icon** → **Notification Settings**
2. Create a personal silencing rule
3. Scope it to alerts you want to quiet

This is useful when:
- You're on vacation or on-call rotation
- You want to reduce noise from certain alert types
- You're debugging and don't want alerts firing to your device

## 4.3.4 Verifying Silencing Is Active

**Check in Cloud UI:**

1. Navigate to **Settings** → **Silencing Rules**
2. Look for active rules (shown with a green indicator)
3. Check the **Next Activation** time for scheduled rules

**Check via API:**

```bash
# List active silencing rules
curl -s "http://localhost:19999/api/v1/alarms?silenced=1" | jq '.'
```

## 4.3.5 Related Sections

- **[4.1 Disabling Alerts](1-disabling-alerts.md)** - Permanent alert removal
- **[4.2 Silencing vs Disabling](2-silencing-vs-disabling.md)** - Conceptual differences
- **[4.4 Reducing Flapping and Noise](4-reducing-flapping.md)** - Using delays for stability

## What's Next

- **[4.4 Reducing Flapping](4-reducing-flapping.md)** - Delay and repeat techniques
- **[8.1 Hysteresis](../advanced-techniques/1-hysteresis.md)** - Status-based conditions
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Notification configuration