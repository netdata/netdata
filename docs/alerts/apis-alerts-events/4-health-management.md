# 9.4 Health Management API

:::note

The health management API requires authentication. See [9.4 Health Management API](#944-authentication) for details.

:::

## 9.4.1 Enable/Disable Alerts

The Health Management API uses `/api/v1/manage/health` endpoint.

```bash
# Disable a specific alert (requires auth token)
curl "http://localhost:19999/api/v1/manage/health?cmd=DISABLE&alarm=10min_cpu_usage" \
  -H "X-Auth-Token: YOUR_TOKEN"

# Silence notifications for a specific alert
curl "http://localhost:19999/api/v1/manage/health?cmd=SILENCE&alarm=10min_cpu_usage" \
  -H "X-Auth-Token: YOUR_TOKEN"

# Reset to default state (enable all checks and notifications)
curl "http://localhost:19999/api/v1/manage/health?cmd=RESET" \
  -H "X-Auth-Token: YOUR_TOKEN"
```

## 9.4.2 Reload Configuration

To reload health configuration, use the Netdata CLI:

```bash
sudo netdatacli reload-health
```

This is equivalent to sending SIGUSR2 to the Netdata process.

## 9.4.3 Authentication

The health management API requires an authorization token. Find the token in your `netdata.conf`:

```ini
[registry]
    # netdata management api key file = /var/lib/netdata/netdata.api.key
```

Default access is from localhost only. Configure `allow management from` in `netdata.conf` for remote access.

## 9.4.3 Related Sections

- **4.1 Disabling Alerts** for configuration-based disabling
- **8.4 Custom Actions** for exec-based automation