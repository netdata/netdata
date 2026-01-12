# 4.1 Disabling Alerts

Disabling an alert stops it from being **evaluated entirely**. The health engine won't run the check, assign status, or generate events.

<details>
<summary><strong>When to Disable Instead of Silence</strong></summary>

Disable an alert when:

- **The alert is not relevant** to your environment (for example, you don't run MySQL)
- **You've replaced it** with a custom alert (same name)
- **The monitored service is retired** and the alert will never fire correctly
- **You want a permanent solution** rather than temporary silencing

For temporary quiet periods, see **4.3 Silencing in Netdata Cloud** instead.

</details>

## 4.1.1 Disable a Specific Alert

**Method 1: Via Configuration File**

Create or edit a file in `/etc/netdata/health.d/`:

```bash
sudo /etc/netdata/edit-config health.d/disabled.conf
```

Add the alert you want to disable:

```conf
# Disable stock alert that doesn't apply to our environment
alarm: mysql_gtid_binlog_gtid_0

# Disable by setting enabled to no
template: some_stock_alert
   enabled: no
```

Reload configuration:

```bash
sudo netdatacli reload-health
```

**Method 2: Disable All Alerts for a Host**

To disable **all** health checks on a specific host, set in `netdata.conf`:

```ini
[health]
    enabled = no
```

This is useful when:
- A node is being decommissioned but you want to keep it running for diagnostics
- You're doing maintenance and don't want alerts firing
- You want to completely stop health monitoring on a development or test node

## 4.1.2 Disable Evaluation While Keeping Notifications

If you want to stop the alert from **ever changing status** but still see it in the UI, set both conditions to false:

```conf
template: noisy_alert
   on: system.cpu
   warn: ($this) == 0
   crit: ($this) == 0
```

This keeps the alert loaded but ensures it never triggers notifications.

## 4.1.3 Disable via Health Management API

You can also disable alerts programmatically:

```bash
# Disable a specific alert
curl -s "http://localhost:19999/api/v1/health?cmd=disable&alarm=my_alert"

# Disable all alerts
curl -s "http://localhost:19999/api/v1/health?cmd=disable_all"
```

See **9.4 Health Management API** for full documentation.

## 4.1.4 Related Sections

- **4.2 Silencing vs Disabling** - Understand the critical difference
- **4.3 Silencing in Netdata Cloud** - Cloud-managed silencing rules
- **4.4 Reducing Flapping and Noise** - Techniques for preventing flapping