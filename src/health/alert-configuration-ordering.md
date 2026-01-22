# Alert Configuration Ordering

This document explains how Netdata loads and processes alert configurations, including the precedence rules that determine which alert definition "wins" when multiple definitions could apply to the same chart.

## Configuration Loading Order

Netdata loads alert configuration files from two directories (name + default path shown; these can differ by package/prefix):

1. **User health config directory** (`[directories]` → `health config`, loaded first): default `/etc/netdata/health.d/`
2. **Stock health config directory** (`[directories]` → `stock health config`, loaded second): default `/usr/lib/netdata/conf.d/health.d/`

If your install uses a different prefix (e.g., `/opt`), these paths change. Use your `netdata.conf` `[directories]` section or `edit-config` output to confirm the exact paths on your system.

### File and Directory Shadowing

If a file or subdirectory with the **same name** exists in both directories, only the user configuration is loaded. The stock counterpart is completely ignored.

**Example (defaults):**
- Stock file: `/usr/lib/netdata/conf.d/health.d/cpu.conf`
- User file: `/etc/netdata/health.d/cpu.conf`
- Result: Only `/etc/netdata/health.d/cpu.conf` is loaded

This also applies to subdirectories - a user subdirectory shadows the entire stock subdirectory tree.

This means if you copy a stock file to the user directory, you must include **all** alert definitions you want from that file, not just the ones you want to modify.

### File Order Within a Directory

Files within each directory are loaded in the order returned by `readdir()`, which varies by filesystem and operating system. This order is **non-deterministic**.

**Implication:** If you have multiple user configuration files that define alerts with the same name for the same chart, which one "wins" depends on filesystem-specific ordering. To avoid surprises:
- Keep all overrides for the same alert in a single file
- Or use different alert names

## Alert Processing Order

When Netdata applies alert definitions to charts, it processes them in this order:

1. **Alarms** (non-templates) - processed first
2. **Templates** - processed second

Within each type, definitions from user configuration files are processed before those from stock configuration files.

### Complete Precedence Table

| Priority | Type | Source | Example Location |
|----------|------|--------|------------------|
| 1 (highest) | Alarm | User config | `/etc/netdata/health.d/*.conf` (default) |
| 2 | Alarm | Stock config | `/usr/lib/netdata/conf.d/health.d/*.conf` (default) |
| 3 | Template | User config | `/etc/netdata/health.d/*.conf` (default) |
| 4 (lowest) | Template | Stock config | `/usr/lib/netdata/conf.d/health.d/*.conf` (default) |

## First-Match-Wins Behavior

Only **one** alert can exist per (chart, alert_name) pair at runtime. When multiple alert definitions with the same name could apply to a chart:

1. The **first** matching definition creates the active alert (RRDCALC)
2. All subsequent same-named definitions that match the same chart are **silently skipped**

### Example

Consider these definitions:

```
# User config file (user health config directory, default /etc/netdata/health.d/my-alerts.conf)
alarm: disk_space_usage
   on: disk_space._mnt_data   # example: mount point "/mnt/data" becomes a sanitized chart id
 warn: $this > 95

# Stock config file (stock health config directory, default /usr/lib/netdata/conf.d/health.d/disks.conf)
template: disk_space_usage
      on: disk.space
    warn: $this > 80
```

For the chart `disk_space._mnt_data`:
1. The user `alarm` is processed first (higher priority)
2. It matches the chart and creates an alert with `warn: $this > 95`
3. When the stock `template` is processed, it also matches this chart
4. However, an alert named `disk_space_usage` already exists for this chart
5. The template is silently skipped for this chart

For all other `disk.space.*` charts:
- No user alarm matches
- The stock template creates alerts with `warn: $this > 80`

## Same-Name Alert Storage

When multiple alert definitions share the same name from configuration files, they are stored in a linked list rather than replaced. This means:

- All definitions with the same name coexist in memory
- The processing order determines which one creates alerts for each chart
- Definitions with non-overlapping matching criteria can all create alerts

### Example with Non-Overlapping Criteria

```
# Definition 1: matches Linux hosts only
template: memory_usage
      on: system.ram
      os: linux
    warn: $this > 80

# Definition 2: matches FreeBSD hosts only
template: memory_usage
      on: system.ram
      os: freebsd
    warn: $this > 75
```

Both definitions exist and both can create alerts, because they target different operating systems and will never match the same chart.

## Dynamic Configuration (DYNCFG) Exception

Alerts created or modified through the Netdata UI or API (dynamic configuration) behave differently:

- **DYNCFG alerts REPLACE** existing definitions with the same name, rather than appending to the linked list
- This is the only case where a same-named alert definition overwrites another

When you click "Edit" on an alert in the dashboard, the resulting configuration completely replaces any file-based definition with that name.

## Practical Implications

### To Override a Stock Alert

Create an alert with the **same name** in a user configuration file. Since user configs are processed first, your definition will create the alert before the stock definition is processed.

See [Overriding Stock Alerts](overriding-stock-alerts.md) for detailed examples.

### To Disable a Stock Alert

You have several options:

1. **Global disable** in `netdata.conf`:
   ```
   [health]
       enabled alarms = !alert_name *
   ```

2. **Per-alert disable** using pattern matching (trick used in stock configs):
   ```
   alarm: stock_alert_name
      on: the.chart
   host labels: _hostname=!*
   ```
   This uses a **special disable shortcut** handled by the health config parser: `!*` (or `!* *`) marks the alert as disabled.

3. **Silence notifications only** (alert still monitors):
   ```
   alarm: stock_alert_name
      on: the.chart
      to: silent
   ```

## Related Documentation

- [Health Configuration Reference](REFERENCE.md)
- [Overriding Stock Alerts](overriding-stock-alerts.md)
