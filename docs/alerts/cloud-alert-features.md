# 10. Netdata Cloud Alert and Events Features

Netdata Cloud provides additional capabilities for **visualizing**, **filtering**, and **managing** alerts across your entire infrastructure. This chapter covers Cloud-specific features beyond basic notification dispatch.

## What This Chapter Covers

| Section | Feature | Purpose |
|---------|---------|---------|
| **10.1 Events Feed** | Unified event stream | See all alerts across your infrastructure |
| **10.2 Silencing Rules Manager** | Cloud-based silencing | Space-wide notification suppression |
| **10.3 Alert Deduplication** | Cloud aggregation | Avoid duplicate alerts from multiple nodes |
| **10.4 Room-Based Alerting** | Room scoping | Group nodes and scope alerts |

## 10.1 Events Feed

The Events feed shows a unified stream of all alert transitions across your infrastructure.

### 10.1.1 Accessing the Events Feed

1. Log in to Netdata Cloud
2. Navigate to **Events** in the left sidebar
3. Events appear in chronological order, newest first

### 10.1.2 Event Types

| Type | Description |
|------|-------------|
| **Alert Created** | New alert defined in Cloud |
| **Alert Triggered** | Alert changed to WARNING or CRITICAL |
| **Alert Cleared** | Alert returned to CLEAR |
| **Alert Updated** | Alert configuration changed |
| **Alert Deleted** | Alert removed |

### 10.1.3 Filtering Events

Use the filter bar to narrow the view:

| Filter | Example | Description |
|--------|---------|-------------|
| Status | `status:CRITICAL` | Only critical alerts |
| Node | `host:prod-db-*` | Database servers |
| Alert | `alert:*cpu*` | CPU-related alerts |
| Room | `room:Production` | Production room only |

### 10.1.4 Event Retention

| Plan | Retention |
|------|-----------|
| Free | 24 hours |
| Starter | 7 days |
| Pro | 30 days |
| Business | 1 year |

## 10.2 Silencing Rules Manager

Cloud silencing rules let you suppress notifications space-wide without touching configuration files.

### 10.2.1 Creating Silencing Rules

1. Navigate to **Settings** → **Silencing Rules**
2. Click **+ Create Rule**
3. Configure:

```yaml
name: Weekend Maintenance
description: Silence alerts during Saturday maintenance window
scope:
  nodes: env:production
  alerts: *
schedule:
  - every: Saturday 1:00 AM
    to: Monday 6:00 AM
```

### 10.2.2 Rule Scope Options

| Option | Description | Example |
|--------|-------------|---------|
| `nodes` | Target specific nodes by name or label | `env:production` |
| `alerts` | Target specific alerts | `*cpu*`, `disk_*` |
| `contexts` | Target by metric context | `system.cpu`, `disk.space` |

### 10.2.3 Scheduling Rules

| Schedule Type | Description | Example |
|---------------|-------------|---------|
| `every` | Recurring schedule | `every: Saturday 1:00 AM` |
| `at` | One-time event | `at: 2024-01-15 14:00` |
| `duration` | Fixed duration | `every: Saturday 1:00 AM, duration: 4h` |

### 10.2.4 Verifying Silencing

**Check rule status:**

1. Go to **Settings** → **Silencing Rules**
2. Active rules show a green indicator
3. Click a rule to see next activation time

**Check silenced alerts:**

```bash
# Via Cloud API
curl -s "https://app.netdata.cloud/api/v1/silenced" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq '.'
```

## 10.3 Alert Deduplication and Aggregation

Netdata Cloud automatically deduplicates alerts that would appear multiple times across nodes.

### 10.3.1 How Deduplication Works

| Scenario | Before Cloud | After Cloud |
|----------|--------------|-------------|
| Same alert fires on 5 nodes | 5 separate notifications | 1 aggregated notification |
| Multiple instances of template | One per instance | Summary showing count |

### 10.3.2 Deduplication Keys

Cloud groups alerts by:

| Key | Groups alerts with |
|-----|---------------------|
| Alert name | Same alert name across nodes |
| Context | Same metric context |
| Severity | Same severity level |

### 10.3.3 Aggregated Alert View

In the Cloud UI, aggregated alerts show:

```text
⚠️ 3 nodes with high CPU
├─ prod-db-01: 95%
├─ prod-db-02: 92%
└─ prod-db-03: 98%
```

## 10.4 Room-Based Alerting

Rooms help organize nodes and scope alerts to specific groups.

### 10.4.1 Creating Rooms

1. Navigate to **Settings** → **Rooms**
2. Click **+ Create Room**
3. Add nodes by name or label:

```yaml
name: Production Databases
criteria:
  - label: env == production
  - label: role == database
```

### 10.4.2 Room-Specific Alerts

Create alerts scoped to specific rooms:

1. In Cloud UI, go to **Alerts** → **Alert Configuration**
2. Select the target room
3. Define alert as usual
4. Alert only applies to nodes in that room

### 10.4.3 Alert Inheritance

Nodes can belong to multiple rooms, inheriting alerts from each:

```text
Node: prod-db-01
├─ Room: Production → inherits Production alerts
├─ Room: Databases → inherits Database alerts
└─ Room: US-East → inherits US-East alerts
```

## 10.5 Cloud Alert Views

### 10.5.1 Alerts Tab

**Global view** of all alerts across all rooms:

- Filter by status, room, severity
- Bulk actions (silence, delete)
- Export to CSV

### 10.5.2 Node-Specific Alerts

**Per-node view** showing alerts for one node:

- Detailed alert status
- Configuration access
- Node-specific silencing

### 10.5.3 Context-Based Filtering

Filter by metric context to find all alerts monitoring the same metric:

```bash
# In Cloud UI
context:system.cpu  # All CPU alerts
context:disk.space  # All disk space alerts
```

## Key Takeaway

Cloud features add centralized visibility and control. Use the Events feed for monitoring, silencing rules for temporary quiet periods, and rooms for organized infrastructure management.

## What's Next

- **Chapter 11: Built-In Alerts Reference** Catalog of Netdata's stock alerts
- **Chapter 12: Best Practices for Alerting** Guidance for maintainable alerting
- **Chapter 13: Alerts and Notifications Architecture** Deep-dive into internal behavior