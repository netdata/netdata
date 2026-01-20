# 10. Netdata Cloud Alert and Events Features

Netdata Cloud provides centralized features for event monitoring, alert management, and team coordination.

## What You'll Find in This Chapter

| Section | Feature |
|---------|---------|
| **[10.1 Events Feed](#101-events-feed)** | Unified event stream |
| **[10.2 Silencing Rules Manager](#102-silencing-rules-manager)** | Cloud-based silencing |
| **[10.3 Alert Deduplication](#103-alert-deduplication-and-aggregation)** | Cloud aggregation |
| **[10.4 Room-Based Alerting](#104-room-based-alerting)** | Room scoping |

---

## 10.1 Events Feed

**A unified stream of all alerts and events across your infrastructure.**

Access the Events feed to monitor alert activity in real-time. Filter by status, host, or alert name to focus on what's relevant.

### 10.1.1 Event Types

| Type | Description |
|------|-------------|
| Alert Created | New alert defined |
| Alert Triggered | Status changed to WARNING/CRITICAL |
| Alert Cleared | Returned to CLEAR |

### 10.1.2 Filtering

```text
status:CRITICAL  # Only critical alerts
host:prod-db-*   # Database servers
alert:*cpu*      # CPU-related alerts
```

### 10.1.3 Related Sections

- **9.5 Cloud Events API** for programmatic access
- **5.3 Cloud Notifications** for routing

## 10.2 Silencing Rules Manager

**Temporarily suppress alerts during maintenance windows or known issues.**

Create rules to silence specific alerts on specific nodes without disabling them globally.

### 10.2.1 Rule Scope

```yaml
name: Weekend Maintenance
scope:
  nodes: env:production
  alerts: "*"
schedule:
  - every: Saturday 1:00 AM
    to: Monday 6:00 AM
```

### 10.2.2 Related Sections

- **4.3 Silencing in Netdata Cloud** for Cloud-level silencing workflows
- **4.2 Silencing vs Disabling** for conceptual difference

## 10.3 Alert Deduplication and Aggregation

**Cloud automatically consolidates duplicate alerts from multiple nodes into single notifications.**

When the same alert fires across multiple nodes, Cloud groups them into one actionable notification with details for all affected nodes.

### 10.3.1 How It Works

| Scenario | Without Cloud | With Cloud |
|----------|--------------|-------------|
| Same alert on 5 nodes | 5 notifications | 1 aggregated |

### 10.3.2 Aggregated View

```text
⚠️ 3 nodes with high CPU
├─ prod-db-01: 95%
├─ prod-db-02: 92%
└─ prod-db-03: 98%
```

### 10.3.3 Related Sections

- **[12.4 Large Environment Patterns](../best-practices/4-scaling-large-environments.md)** for multi-node setups
- **[13.5 Scaling Topologies](../architecture/5-scaling-topologies.md)** for complex topologies

## 10.4 Room-Based Alerting

**Organize nodes into rooms to scope alerts and notifications to specific teams or environments.**

Rooms help you group infrastructure by environment (production, staging), service type (databases, web servers), or any custom criterion.

### 10.4.1 Creating Rooms

1. Navigate to **Settings** → **Rooms**
2. Click **+ Create Room**
3. Add nodes by label:

```yaml
name: Production Databases
criteria:
  - label: env == production
  - label: role == database
```

### 10.4.2 Room-Scoped Alerts

When creating an alert in Cloud UI, scope it to specific rooms using the **Scope** setting:

1. Go to **Nodes** → Select a node → **Configuration** → **Health**
2. Click **+** to create a new alert
3. In the **Scope** field, select specific rooms (e.g., "Production Databases")
4. The alert will only fire for nodes within those rooms

By default, alerts apply only to the selected node. Use room filters to apply the same alert to all nodes in designated rooms.

### 10.4.3 Related Sections

- **[8.3 Host, Chart, and Label-Based Targeting](../essential-patterns/3-label-targeting.md)** for label usage
- **[12.4 Large Environment Patterns](../best-practices/4-scaling-large-environments.md)** for multi-room strategies