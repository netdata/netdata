# Overriding Stock Alerts

This guide explains how to customize Netdata's stock alerts without modifying the original files. User configurations survive Netdata upgrades, making this the recommended approach for customization.

## Why Override Instead of Edit

Stock alert files live in the **stock health config directory** (`[directories]` → `stock health config`, default `/usr/lib/netdata/conf.d/health.d/`). These files are replaced during upgrades, so direct edits will be lost.

User overrides belong in the **user health config directory** (`[directories]` → `health config`, default `/etc/netdata/health.d/`), which is preserved across upgrades.

If your install uses a different prefix (e.g., `/opt`), these paths will differ. Use your `netdata.conf` `[directories]` section or `edit-config` output to confirm the exact paths on your system.

## Method 1: File-Level Override

Copy the entire stock file to your user configuration directory (from your Netdata config dir, default `/etc/netdata`):

```bash
sudo ./edit-config health.d/cpu.conf
```

Or manually (using default paths):

```bash
sudo cp /usr/lib/netdata/conf.d/health.d/cpu.conf /etc/netdata/health.d/cpu.conf
sudo nano /etc/netdata/health.d/cpu.conf
```

**Important:** When a file with the same name exists in both directories, Netdata only loads the user configuration file. The stock file is completely ignored.

This means you must include **all** alert definitions you want from that file. If the stock file has 10 alerts and you only want to modify 1, your user file must still contain all 10 (with your modification to the one).

**When to use this method:**
- You want to modify most or all alerts in a stock file
- You want complete control over the file's contents
- You want to remove alerts that exist in the stock file

## Method 2: Alert-Name Override (Recommended)

Create a new configuration file with just the alerts you want to override:

```bash
sudo nano /etc/netdata/health.d/my-overrides.conf
```

Define an alert with the **same name** as the stock alert you want to override:

**Example: Override the `cpu_steal` threshold**

Stock alert (in the stock health config directory, default `/usr/lib/netdata/conf.d/health.d/cpu.conf`):
```
alarm: cpu_steal
   on: system.cpu
class: Utilization
 type: System
component: CPU
   lookup: average -10m percentage of steal
    units: %
    every: 5m
     warn: $this > 30
     crit: $this > 50
```

Your override (in `/etc/netdata/health.d/my-overrides.conf`):
```
alarm: cpu_steal
   on: system.cpu
class: Utilization
 type: System
component: CPU
   lookup: average -10m percentage of steal
    units: %
    every: 5m
     warn: $this > 50
     crit: $this > 80
```

**How it works:** User configuration files are loaded before stock files. Your `cpu_steal` definition creates the alert first. When the stock definition is processed, an alert named `cpu_steal` already exists for `system.cpu`, so the stock definition is skipped.

**When to use this method:**
- You want to modify specific alerts while keeping other stock alerts unchanged
- You want to maintain a single file with all your customizations
- You want minimal configuration maintenance

## Override a Template for a Specific Instance

Templates apply to all charts matching a context (e.g., all disks). To override a template for just **one specific instance**, create an `alarm` (not a template) with the same name:

**Example: Different disk space threshold for `/mnt/data`**

Stock template (applies to all disks):
```
template: disk_space_usage
      on: disk.space
   lookup: max -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 20
     crit: $this < 10
```

Your override (in `/etc/netdata/health.d/my-overrides.conf`):
```
alarm: disk_space_usage
   on: disk_space._mnt_data   # example: mount point "/mnt/data" becomes a sanitized chart id
   lookup: max -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 5
     crit: $this < 2
```

**How it works:**
1. Alarms are processed before templates
2. Your alarm matches `disk_space.<mount_id>` and creates an alert
3. The stock template matches all `disk.space.*` charts, including `/mnt/data`
4. For `/mnt/data`: An alert named `disk_space_usage` already exists, so the template is skipped
5. For all other disks: The template creates alerts normally

**Key difference:** The `on` line in your alarm uses the **specific chart ID** (type `disk_space` + sanitized mount ID), not the context (`disk.space`).

To find the exact chart ID, check the Netdata dashboard or use:
```bash
curl -s "http://localhost:19999/api/v1/charts" | grep -o '"id":"disk_space[^"]*"'
```

### Alternative: Using Chart Labels

If charts have labels attached, you can use `chart labels:` to filter which instances a template applies to:

```
# Override for disks with mount_point label = /mnt/data
template: disk_space_usage
      on: disk.space
chart labels: mount_point=/mnt/data
   lookup: max -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 5
     crit: $this < 2
```

**When to use chart labels:**
- When multiple charts share a common label value you want to target
- When you want to apply the same override to a group of instances
- When chart IDs are dynamic or unpredictable

**When to use specific chart ID (alarm method):**
- When you need to override exactly one specific instance
- When the chart ID is stable and predictable
- When the chart doesn't have useful labels

## Disabling Stock Alerts

### Option A: Global Disable in netdata.conf

Disable specific alerts for all charts:

```bash
sudo nano /etc/netdata/netdata.conf
```

```ini
[health]
    enabled alarms = !cpu_steal !disk_space_usage *
```

This disables `cpu_steal` and `disk_space_usage` while keeping all other alerts (`*`).

### Option B: Per-Alert Disable Using Pattern Matching (trick used in stock configs)

Create an override that can never match any host:

```
alarm: cpu_steal
   on: system.cpu
host labels: _hostname=!*
```

This uses a **special disable shortcut** handled by the health config parser: `!*` (or `!* *`) marks the alert as disabled. This pattern is used in stock configs (e.g., for selectively disabled alerts).

### Option C: Silence Notifications Only

Keep the alert monitoring but stop notifications:

```
alarm: cpu_steal
   on: system.cpu
class: Utilization
 type: System
component: CPU
   lookup: average -10m percentage of steal
    units: %
    every: 5m
     warn: $this > 30
     crit: $this > 50
      to: silent
```

The alert still appears in the dashboard and API, but no notifications are sent.

## Verifying Your Override

After reloading health configuration, verify your override is active:

```bash
sudo netdatacli reload-health
```

If `netdatacli` is not available on your system, you can send `SIGUSR2` to the Netdata daemon instead.

### Check via API

```bash
# List all active alerts
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms | keys'

# Check specific alert details
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms["cpu_steal"]'
```

### Check via Dashboard

Navigate to the Alerts tab in the Netdata dashboard. Find your alert and verify the thresholds match your override.

### Check Configuration Loading

Review the Netdata error log for configuration parsing messages:

```bash
grep -i "health" /var/log/netdata/error.log | tail -50
```

## Troubleshooting

### Override Not Taking Effect

1. **Verify file permissions:** The netdata user must be able to read your configuration file
   ```bash
   ls -la /etc/netdata/health.d/my-overrides.conf
   ```

2. **Check for syntax errors:** Look for parsing errors in the log
   ```bash
   grep -i "health.*error\|health.*warning" /var/log/netdata/error.log
   ```

3. **Verify alert name matches exactly:** The name is case-sensitive

4. **Reload health configuration:**
   ```bash
   sudo netdatacli reload-health
   ```

### Both Stock and Override Alerts Appear

This happens when the matching criteria don't overlap. For example:
- Your override has `host labels: production`
- Stock alert has no host labels restriction
- Result: Both can create alerts on different hosts

Ensure your override matches the same charts as the stock alert, or uses broader matching criteria.

### UI Edit Replaced My File-Based Config

When you edit an alert through the Netdata dashboard UI, it creates a dynamic configuration (DYNCFG) that **completely replaces** any file-based definition with the same name. This is different from file-based configs, which coexist in a linked list.

To restore file-based behavior, remove the dynamic configuration through the UI or API.

## Related Documentation

- [Health Configuration Reference](REFERENCE.md)
- [Alert Configuration Ordering](alert-configuration-ordering.md)
