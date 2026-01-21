# 9. APIs for Alerts and Events

Netdata provides APIs for querying and managing alerts programmatically.

## 9.1 Query Current Alerts

The `/api/v1/alarms` endpoint returns all currently active alerts on the agent. Response includes each alert's name, status, current value, and configured threshold.

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

### Filter Alerts

```bash
# Get all alerts including cleared
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.'

# Get only active alerts (default)
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

## 9.2 Alert History

The `/api/v1/alarm_log` endpoint returns all alert state transitions within a time window, showing when each alert transitioned between states.

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?after=-3600" | jq '.'
```

Returns all transitions in the last hour.

### Filter by Chart

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?chart=system.cpu" | jq '.'
```

## 9.3 Inspect Alert Variables

The `/api/v1/alarm_variables` endpoint returns all variables available to an alert expression for a given chart, useful for debugging why an alert triggered or didn't trigger.

```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu" | jq '.'
```

### Response Format

```json
{
  "alarm_variables": {
    "this": 45.2,
    "status": "CLEAR",
    "user": 35.5,
    "system": 9.7
  }
}
```

## 9.4 Health Management API

The `/api/v1/manage/health` endpoint provides programmatic control over the health monitoring subsystem, allowing you to enable, disable, or silence alerts individually or globally.

:::warning

The health management API requires authentication. Use a bearer token with the `Authorization` header.

:::

### Enable/Disable Alerts

Valid commands are:

| Command | Description |
|---------|-------------|
| `DISABLE ALL` | Disable all health checks |
| `SILENCE ALL` | Silence all notifications |
| `DISABLE` | Disable health checks for specific alarms (requires `alarm=` parameter) |
| `SILENCE` | Silence notifications for specific alarms (requires `alarm=` parameter) |
| `RESET` | Reset all silencers and re-enable checks |
| `LIST` | Return current silencer configuration as JSON |

```bash
# Disable all health checks (requires auth token)
curl "http://localhost:19999/api/v1/manage/health?cmd=DISABLE%20ALL" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Silence all notifications (requires auth token)
curl "http://localhost:19999/api/v1/manage/health?cmd=SILENCE%20ALL" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Disable a specific alert
curl "http://localhost:19999/api/v1/manage/health?cmd=DISABLE&alarm=10min_cpu_usage" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Silence notifications for a specific alert
curl "http://localhost:19999/api/v1/manage/health?cmd=SILENCE&alarm=10min_cpu_usage" \
  -H "Authorization: Bearer YOUR_TOKEN"

# Reset to default state (enable all checks and notifications)
curl "http://localhost:19999/api/v1/manage/health?cmd=RESET" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Reload Configuration

To reload health configuration without restarting Netdata, use the Netdata CLI:

```bash
sudo netdatacli reload-health
```

This triggers the health subsystem to reload configuration files.

## 9.5 Cloud Events API

The Cloud Events API (`/api/v2/events`) queries alert and event history stored in Netdata Cloud, providing aggregated, multi-node event data that requires authentication.

```bash
curl -s "https://app.netdata.cloud/api/v2/events?space_id=YOUR_SPACE" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq '.'
```

### Parameters

| Parameter | Description |
|-----------|-------------|
| `space_id` | Filter by Space |
| `alert_name` | Filter by alert pattern |
| `status` | Filter by severity |

## Related Sections

- **[Variables Not Found](../../troubleshooting-alerts/README.md)** for troubleshooting
- **[Variables and Special Symbols](../../alert-configuration-syntax/5-variables-and-special-symbols.md)** for reference
- **[Disabling Alerts](../../controlling-alerts-noise/1-disabling-alerts.md)** for configuration-based disabling
- **[Events Feed](../cloud-alert-features/README.md)** for Cloud UI features
- **[Cloud Notifications](../../receiving-notifications/3-cloud-notifications.md)** for Cloud routing

## What's Next

- **[Chapter 10: Cloud Alert Features](../cloud-alert-features/index.md)** for more Cloud capabilities