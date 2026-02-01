# Overriding Stock Alerts

This guide explains how to customize Netdata's stock alerts. User configurations survive upgrades, making this the recommended approach.

## Quick Reference

| Goal | Method |
|------|--------|
| Change thresholds for ALL instances | Create a template with the same name |
| Change thresholds for ONE instance | Create an alarm with the same name |
| Disable an alert completely | Use `enabled alarms` in netdata.conf |
| Silence notifications only | Set `to: silent` |

## Understanding Overrides

Netdata's alerting uses **templates** (match all instances of a context) and **alarms** (match one specific instance). Stock alerts are mostly templates—they apply to all disks, all CPUs, etc.

To override, create an alert with the **same name**. User definitions are processed before stock definitions, so yours wins.

See [Alert Configuration Ordering](/docs/alerts-and-notifications/alert-configuration-ordering) for the full conceptual explanation.

## Where to Put Your Overrides

**User config directory** (default): `/etc/netdata/health.d/`

Files here survive upgrades. Stock files in `/usr/lib/netdata/conf.d/health.d/` are replaced during updates.

Check your `netdata.conf` `[directories]` section for exact paths on your system.

## Method 1: Override All Instances (Template)

Create a template with the same name to change thresholds for ALL instances.

**Example: Raise CPU steal thresholds globally**

Stock alert in `/usr/lib/netdata/conf.d/health.d/cpu.conf`:
```yaml
template: 20min_steal_cpu
      on: system.cpu
  lookup: average -20m unaligned of steal
   units: %
   every: 5m
    warn: $this > (($status >= $WARNING) ? (5) : (10))
```

Your override in `/etc/netdata/health.d/my-overrides.conf`:
```yaml
template: 20min_steal_cpu
      on: system.cpu
  lookup: average -20m unaligned of steal
   units: %
   every: 5m
    warn: $this > (($status >= $WARNING) ? (10) : (20))
```

**Why it works:** Same name + same context. Your template is processed first, creating the alert. The stock template is then skipped.

> **Note:** Most stock alerts are templates. If a stock alert is an alarm (rare), you must override it with an alarm, not a template—alarms are always processed before templates.

## Method 2: Override One Instance (Alarm)

Create an alarm to override thresholds for ONE specific instance while keeping stock thresholds for others.

**Example: Different disk space threshold for `/mnt/data`**

Stock template (applies to all disks):
```yaml
template: disk_space_usage
      on: disk.space
   lookup: max -1m percentage of avail
     warn: $this < 20
     crit: $this < 10
```

Your override in `/etc/netdata/health.d/my-overrides.conf`:
```yaml
alarm: disk_space_usage
   on: disk_space._mnt_data
   lookup: max -1m percentage of avail
     warn: $this < 5
     crit: $this < 2
```

**Why it works:**
- Both have the **same name** (`disk_space_usage`)
- Your **alarm** targets the specific chart ID `disk_space._mnt_data`
- Alarms are processed before templates (when names match)
- For `/mnt/data`: your alarm creates the alert, stock template is skipped
- For all other disks: stock template creates alerts normally

**Key difference:** Templates use `on:` with a **context** (`disk.space`). Alarms use `on:` with a **chart ID** (`disk_space._mnt_data`).

### Finding Chart IDs

To find the exact chart ID for an instance:

```bash
curl -s "http://localhost:19999/api/v1/charts" | grep -o '"id":"disk_space[^"]*"'
```

Or check the chart title in the Netdata dashboard—the chart ID is shown in the URL when you click on a chart.

### Alternative: Using Chart Labels

Instead of chart IDs, you can match by labels:

```yaml
template: disk_space_usage
      on: disk.space
chart labels: mount_point=/mnt/data
   lookup: max -1m percentage of avail
     warn: $this < 5
     crit: $this < 2
```

Use labels when:
- You want to target multiple instances sharing a label
- Chart IDs are dynamic or unpredictable

## Method 3: Copy Entire Stock File

If you want to modify many alerts in one stock file, copy it entirely:

```bash
cd /etc/netdata
sudo ./edit-config health.d/cpu.conf
```

**Important:** When a file with the same name exists in both directories, Netdata loads **only** the user file. The stock file is completely ignored.

This means your copy must include ALL alerts you want—not just the ones you're changing.

## Disabling Alerts

### Option A: Global Disable

In `/etc/netdata/netdata.conf`:

```ini
[health]
    enabled alarms = !20min_steal_cpu !disk_space_usage *
```

This disables `20min_steal_cpu` and `disk_space_usage` while keeping all other alerts (`*`).

### Option B: Per-Alert Disable

Create an override that never matches:

```yaml
template: 20min_steal_cpu
      on: system.cpu
host labels: _hostname=!*
```

The pattern `!*` is a special disable shortcut—the health config parser recognizes it and disables the alert.

### Option C: Silence Notifications Only

Keep the alert monitoring but stop notifications:

```yaml
template: 20min_steal_cpu
      on: system.cpu
  lookup: average -20m unaligned of steal
   units: %
   every: 5m
    warn: $this > (($status >= $WARNING) ? (5) : (10))
      to: silent
```

The alert still appears in the dashboard but sends no notifications.

## Applying Changes

Reload the health configuration:

```bash
sudo netdatacli reload-health
```

If `netdatacli` isn't available, send `SIGUSR2` to the Netdata process.

### Verify Your Override

Check via API:
```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms | to_entries[] | select(.value.name == "20min_steal_cpu") | .value'
```

Key fields to check:
- `source`: confirms which config file is active (user override vs stock)
- `lookup_*`: data query parameters
- `warn`, `crit`: threshold expressions

Or navigate to the Alerts tab in the dashboard and verify thresholds match your override.

### Check for Errors

If your override isn't working, check the logs:

```bash
# systemd journal (most Linux distributions):
journalctl --namespace netdata -g health --no-pager | tail -20

# Log files (if journal not available):
grep -i health /var/log/netdata/error.log | tail -20
```

Common issues:
- Syntax errors in configuration
- Alert name doesn't match exactly (case-sensitive)
- File permissions prevent Netdata from reading your config

## Troubleshooting

### Override Not Taking Effect

1. **Reload configuration**: `sudo netdatacli reload-health`
2. **Check file permissions**: Netdata must be able to read your file
3. **Verify exact name match**: Alert names are case-sensitive
4. **Check for syntax errors**: Look in error.log

### Both Stock and Override Alerts Appear

This happens when matching criteria don't overlap. For example:
- Your override has `host labels: production`
- Stock alert has no host labels restriction

Both can create alerts on different hosts. Ensure your override matches at least the same scope as the stock alert.

### UI Edit Replaced My Config

Editing an alert through the dashboard UI creates a dynamic configuration that **replaces** any file-based definition. To restore file-based behavior, remove the dynamic config through the UI or API.

## FAQ

### Do I need to copy all fields when overriding an alert?

Yes. Your override is a complete alert definition, not a "patch" on the stock alert. Include all fields: `lookup`, `calc`, `warn`, `crit`, `units`, etc.

If you omit a field, the alert uses its default value—not the stock alert's value.

### How do I override the same alert differently on different hosts?

Use `host labels` to create host-specific overrides:

```yaml
# Production servers: stricter thresholds
template: cpu_usage
      on: system.cpu
host labels: environment=production
     warn: $this > 70

# Development servers: relaxed thresholds
template: cpu_usage
      on: system.cpu
host labels: environment=development
     warn: $this > 90
```

Both can coexist because they match different hosts.

### How do I find what stock alerts exist?

List all stock alert files:
```bash
ls /usr/lib/netdata/conf.d/health.d/
```

View a specific stock alert:
```bash
cat /usr/lib/netdata/conf.d/health.d/cpu.conf
```

Or use the API to list all alert names:
```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms | to_entries[].value.name' | sort -u
```

### Can I add new alerts without affecting stock alerts?

Yes. Create alerts with **different names** than stock alerts. They'll coexist independently.

```yaml
# This is a NEW alert, not an override
template: my_custom_disk_alert
      on: disk.space
   lookup: max -5m percentage of avail
     warn: $this < 15
```

### What happens to my overrides after a Netdata upgrade?

User config files in `/etc/netdata/health.d/` are preserved. Stock files in `/usr/lib/netdata/conf.d/health.d/` are replaced.

Your overrides continue working. However, if a stock alert is renamed or removed in a new version, your override may become orphaned (still works, but no longer overriding anything).

### How do I override for multiple specific instances?

Option 1: Create multiple alarms (one per instance):
```yaml
alarm: disk_space_usage
   on: disk_space._mnt_data
     warn: $this < 5

alarm: disk_space_usage
   on: disk_space._mnt_backup
     warn: $this < 5
```

Option 2: Use chart labels if instances share a label:
```yaml
template: disk_space_usage
      on: disk.space
chart labels: storage_tier=bulk
     warn: $this < 5
```

### Can I see what overrides are currently active?

Check which config files Netdata loaded:
```bash
# systemd journal:
journalctl --namespace netdata -g "health.*load\|health.*read" --no-pager

# Log files:
grep -iE "health.*(load|read)" /var/log/netdata/error.log
```

Compare your active alert config vs stock:
```bash
# Your override
cat /etc/netdata/health.d/my-overrides.conf

# Stock definition
cat /usr/lib/netdata/conf.d/health.d/disks.conf
```

### Why does editing an alert in the UI override my file-based config?

UI edits create dynamic configurations that take precedence over all file-based configs. This is by design—it allows quick adjustments without SSH access.

To restore file-based control, remove the dynamic config through the UI (reset to default) or via the API.

## Related Documentation

- [Health Configuration Reference](/docs/alerts-and-notifications/alert-configuration-reference)
- [Alert Configuration Ordering](/docs/alerts-and-notifications/alert-configuration-ordering)
