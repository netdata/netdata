# 9. APIs for Alerts and Events

Netdata provides comprehensive APIs for **querying alert status**, **inspecting variables**, **managing configuration**, and **accessing alert history**. These APIs enable integration with external tools, automation, and custom dashboards.

:::tip
These APIs run on **Agents and Parents** (port 19999 by default). Cloud APIs are documented separately in **Chapter 10**.
:::

## What This Chapter Covers

| Section | API Endpoint | Purpose |
|---------|--------------|---------|
| **9.1 Query Current Alerts** | `/api/v1/alarms` | List active alerts and status |
| **9.2 Alert History** | `/api/v1/alarm_log` | Get alert transitions |
| **9.3 Inspect Variables** | `/api/v1/alarm_variables` | Debug alert expressions |
| **9.4 Health Management** | `/api/v1/health` | Enable/disable alerts at runtime |
| **9.5 Cloud Events API** | `/api/v2/alerts` (Cloud) | Query alert events from Cloud |

## 9.1 Query Current Alerts

The Alarms API returns the current status of all loaded alerts.

### 9.1.1 List All Active Alerts

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

**Response format:**

```json
{
  "alarms": {
    "high_cpu": {
      "id": 123,
      "name": "high_cpu",
      "chart": "system.cpu",
      "context": "system.cpu",
      "status": "CLEAR",
      "value": 45.2,
      "warn": ">80",
      "crit": ">95",
      "source": "conf",
      "last_status_change": 1704067200,
      "last_updated": 1704067200,
      "update_every": 60,
      "delay": {
        "up": 300,
        "down": 60
      }
    }
  }
}
```

### 9.1.2 Filter by Status

```bash
# Only WARNING and CRITICAL alerts
curl -s "http://localhost:19999/api/v1/alarms?status=WARNING,CRITICAL" | jq '.'
```

### 9.1.3 Get Specific Alert Details

```bash
curl -s "http://localhost:19999/api/v1/alarms/high_cpu" | jq '.'
```

### 9.1.4 API Parameters

| Parameter | Values | Description |
|-----------|--------|-------------|
| `status` | CLEAR, WARNING, CRITICAL, UNINITIALIZED, UNDEFINED | Filter by status |
| `all` | true/false | Include disabled alerts |
| `json` | true/false | Return JSON (default) |

## 9.2 Query Alert History

The Alarm Log API returns alert transitions (status changes over time).

### 9.2.1 Get Recent Transitions

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?after=-3600" | jq '.'
```

This returns all transitions in the last hour.

### 9.2.2 Filter by Alert Name

```bash
# Get transitions only for high_cpu alert
curl -s "http://localhost:19999/api/v1/alarm_log?alarm=high_cpu" | jq '.'
```

### 9.2.3 Time-Based Queries

```bash
# Last 24 hours
curl -s "http://localhost:19999/api/v1/alarm_log?after=-86400" | jq '.'

# Specific time range
curl -s "http://localhost:19999/api/v1/alarm_log?after=1703980800&before=1704067200" | jq '.'
```

### 9.2.4 Response Format

```json
{
  "alarm_log": [
    {
      "alarm_id": 123,
      "name": "high_cpu",
      "chart": "system.cpu",
      "status": "WARNING",
      "old_status": "CLEAR",
      "value": 85.3,
      "time": 1704067200,
      "unix_time": "2024-01-01 00:00:00"
    }
  ]
}
```

## 9.3 Inspect Alert Variables

The Variables API shows what variables are available for a specific chart.

### 9.3.1 Get All Variables for a Chart

```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu" | jq '.'
```

### 9.3.2 Response Format

```json
{
  "alarm_variables": {
    "this": 45.2,
    "status": "CLEAR",
    "now": 1704067200,
    "user": 35.5,
    "system": 9.7,
    "softirq": 0.5,
    "irq": 0.2,
    "guest": 0.0,
    "guest_nice": 0.0,
    "chart": "system.cpu",
    "host": "your-hostname"
  }
}
```

### 9.3.3 Use Case: Debugging Expressions

When building complex alert expressions, use this API to verify variable names:

```bash
# Check available dimensions for disk.space
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=disk.space" | jq '.'
```

## 9.4 Health Management API

Control alert behavior at runtime without restarting Netdata.

### 9.4.1 Enable/Disable Specific Alerts

```bash
# Disable an alert
curl -s "http://localhost:19999/api/v1/health?cmd=disable&alarm=high_cpu"

# Enable an alert
curl -s "http://localhost:19999/api/v1/health?cmd=enable&alarm=high_cpu"
```

### 9.4.2 Disable All Alerts

```bash
# Disable all health checks
curl -s "http://localhost:19999/api/v1/health?cmd=disable_all"

# Re-enable all
curl -s "http://localhost:19999/api/v1/health?cmd=enable_all"
```

### 9.4.3 Reload Configuration

```bash
# Force reload of health configuration
curl -s "http://localhost:19999/api/v1/health?cmd=reload"
```

This is equivalent to running `netdatacli reload-health` on the command line.

### 9.4.4 API Summary

| Command | Purpose |
|---------|---------|
| `?cmd=disable&alarm=NAME` | Disable specific alert |
| `?cmd=enable&alarm=NAME` | Enable specific alert |
| `?cmd=disable_all` | Disable all health checks |
| `?cmd=enable_all` | Enable all health checks |
| `?cmd=reload` | Reload configuration from disk |

## 9.5 Cloud APIs for Events and Notifications

Netdata Cloud provides additional APIs for querying alert events across your infrastructure.

### 9.5.1 Cloud Events API

```bash
# Get recent alert events from Cloud
curl -s "https://app.netdata.cloud/api/v2/events?space_id=YOUR_SPACE" \
  -H "Authorization: Bearer YOUR_API_TOKEN" | jq '.'
```

### 9.5.2 Parameters

| Parameter | Description |
|-----------|-------------|
| `space_id` | The Space UID |
| `room_id` | Filter by specific room |
| `alert_name` | Filter by alert pattern |
| `status` | Filter by status (WARNING, CRITICAL, CLEAR) |
| `after` | Start time (Unix timestamp) |
| `before` | End time (Unix timestamp) |

### 9.5.3 Response Format

```json
{
  "data": [
    {
      "id": "evt_abc123",
      "alert_name": "high_cpu",
      "host": "your-hostname",
      "status": "CRITICAL",
      "value": 98.5,
      "time": 1704067200,
      "room": "Production",
      "space": "Your Space"
    }
  ]
}
```

## 9.6 Integration Examples

### 9.6.1 Prometheus Alertmanager Integration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'netdata'
    static_configs:
      - targets: ['localhost:19999']
    metrics_path: /api/v1/alarms
```

### 9.6.2 Grafana Data Source

Add Netdata as a data source using the JSON API plugin, then query:
- `/api/v1/alarms` for current status
- `/api/v1/alarm_log` for historical transitions

### 9.6.3 Custom Dashboard Script

```python
#!/usr/bin/env python3
import requests
import json

def get_alerts():
    url = "http://localhost:19999/api/v1/alarms"
    response = requests.get(url)
    return response.json()['alerts']

def get_active_alerts():
    alerts = get_alerts()
    return {
        name: data for name, data in alerts.items()
        if data['status'] in ['WARNING', 'CRITICAL']
    }

if __name__ == "__main__":
    active = get_active_alerts()
    print(f"Active alerts: {len(active)}")
    for name, alert in active.items():
        print(f"  {name}: {alert['status']} ({alert['value']})")
```

## Key Takeaway

APIs enable full programmatic control over alerts. Use the Alarms API for monitoring dashboards, Alarm Log for compliance and auditing, and Health Management API for automation.

## What's Next

- **Chapter 10: Netdata Cloud Alert and Events Features** Cloud-specific alert views and features
- **Chapter 13: Alerts and Notifications Architecture** Deep-dive into internal API behavior