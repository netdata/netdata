# 9.4 Health Management API

:::warning

The health management API requires authentication. See [9.4.3 Authentication](#943-authentication) for details.

:::

## 9.4.1 Enable/Disable Alerts

The Health Management API uses `/api/v1/manage/health` endpoint. Valid commands are:

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

## 9.4.2 Reload Configuration

To reload health configuration without restarting Netdata, use the Netdata CLI:

```bash
sudo netdatacli reload-health
```

This triggers the health subsystem to reload configuration files. On success, the command returns exit code 0 with no output.

## 9.4.4 Related Sections

- **[4.1 Disabling Alerts](../../controlling-alerts-noise/1-disabling-alerts.md)** for configuration-based disabling
- **[8.4 Custom Actions with exec](../advanced-techniques/4-custom-actions.md)** for exec-based automation

## What's Next

- **[9.5 Cloud Events API](5-cloud-events.md)** for Cloud-based event querying