# Alert Configuration Ordering

This document explains how Netdata's alerting system is designed and how it determines which alert definition applies when multiple definitions could match the same data.

## The Problem: Alerts at Scale

Netdata monitors infrastructure that can range from a single server to thousands of nodes, each with dozens of components: disks, network interfaces, databases, containers, and more.

The challenge: How do you configure alerts that automatically apply to all Redis instances, all disks, or all network interfaces—without manually defining alerts for each one? And when one specific instance needs different thresholds, how do you override just that one predictably?

## The Solution: Contexts and Instances

Netdata's alerting system is built around two concepts:

| Concept | What it is | Example |
|---------|------------|---------|
| **Context** | Defines what the metrics ARE—their meaning and units | `disk.space` (disk space utilization in %) |
| **Instance** | An individual component being monitored | `/mnt/data`, `/home`, `/var` |

A context groups all instances that share the same metric definition. For example, the `disk.space` context includes every mounted filesystem on the system.

## Templates vs Alarms

Netdata provides two ways to define alerts:

### Templates: Match by Context

A **template** applies to ALL instances of a context automatically.

```yaml
template: disk_space_usage
      on: disk.space              # matches the CONTEXT
   lookup: max -1m percentage of avail
     warn: $this < 20
     crit: $this < 10
```

This single definition creates alerts for every disk on every node—automatically. When a new disk is mounted, it gets this alert. No manual configuration needed.

### Alarms: Match by Instance

An **alarm** applies to ONE specific instance.

```yaml
alarm: disk_space_usage
   on: disk_space._mnt_data       # matches a specific INSTANCE (chart ID)
   lookup: max -1m percentage of avail
     warn: $this < 5
     crit: $this < 2
```

Use alarms when a specific instance needs different treatment—like a data disk that's expected to run fuller than others.

**Key difference**: The `on:` line specifies a **context** for templates, but a **chart ID** (specific instance) for alarms.

## Precedence: Same Name Required

Multiple alerts with **different names** can coexist on the same instance. You can have `disk_space_usage`, `disk_io_latency`, and `disk_errors` all monitoring the same disk.

The precedence rules only apply when alerts have the **same name**. In that case, only one alert with that name can exist per instance:

| Priority | Type | What it matches |
|----------|------|-----------------|
| 1 (higher) | Alarm | Specific instance |
| 2 (lower) | Template | All instances of a context |

**Example scenario:**

1. Stock template `disk_space_usage` on context `disk.space` → warn at 20%
2. User alarm `disk_space_usage` on instance `disk_space._mnt_data` → warn at 5%

Both have the **same name** (`disk_space_usage`), so precedence applies:
- `/mnt/data` gets the alarm's thresholds (warn at 5%)
- All other disks get the template's thresholds (warn at 20%)

## Configuration Loading Order

Netdata loads alert configurations from two directories:

1. **User config** (loaded first): `/etc/netdata/health.d/` (default)
2. **Stock config** (loaded second): `/usr/lib/netdata/conf.d/health.d/` (default)

These paths can vary by installation. Check your `netdata.conf` `[directories]` section for exact paths.

### File Shadowing

If a file with the **same name** exists in both directories, only the user file is loaded. The stock file is completely ignored.

**Example:**
- Stock: `/usr/lib/netdata/conf.d/health.d/cpu.conf`
- User: `/etc/netdata/health.d/cpu.conf`
- Result: Only the user file is loaded

This means if you copy a stock file to override it, you must include **all** alerts you want from that file, not just the ones you're modifying.

### Complete Precedence

Combining type precedence with source precedence:

| Priority | Type | Source |
|----------|------|--------|
| 1 (highest) | Alarm | User config |
| 2 | Alarm | Stock config |
| 3 | Template | User config |
| 4 (lowest) | Template | Stock config |

## First-Match-Wins (Same Name Only)

Only **one** alert can exist per (instance, alert_name) pair. When multiple definitions **with the same name** could apply to an instance:

1. The first matching definition (by precedence) creates the alert
2. Later definitions with the same name are skipped for that instance

This is why overriding works: create an alert with the same name, and yours is processed first.

## Dynamic Configuration Exception

Alerts created or modified through the Netdata UI or API behave differently:

- **UI/API changes replace** any existing definition with the same name
- This is the only case where a definition overwrites another

When you edit an alert through the dashboard, it completely replaces any file-based definition with that name.

## Summary

| Goal | Use |
|------|-----|
| Alert on ALL instances of a type | Template matching a context |
| Alert on ONE specific instance | Alarm matching a chart ID |
| Override a stock alert globally | User template with same name |
| Override for just one instance | User alarm with same name |

## FAQ

### Can I have multiple alerts monitoring the same instance?

Yes. Different alert names create independent alerts. You can have `disk_space_usage`, `disk_io_latency`, and `disk_write_errors` all monitoring the same disk simultaneously.

The "only one alert per instance" rule applies only to alerts **with the same name**.

### What happens if I define the same alert name twice in my user config?

The first one processed wins. Since file loading order within a directory is non-deterministic (depends on filesystem), keep all definitions for the same alert name in a single file to avoid surprises.

### How do I know which alert definition is currently active?

Query the API:
```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms | to_entries[] | select(.value.name == "alert_name") | .value'
```

Key fields to check:
- `source`: which config file is active
- `lookup_*`: data query parameters
- `warn`, `crit`: threshold expressions

Or check the Alerts tab in the dashboard—click on an alert to see its current configuration.

### Why isn't my override working?

Common causes:
1. **Name mismatch**: Alert names are case-sensitive
2. **Not reloaded**: Run `sudo netdatacli reload-health`
3. **File permissions**: Netdata must be able to read your config file
4. **Syntax error**: Check logs with `journalctl --namespace netdata -g health` or `grep -i health /var/log/netdata/error.log`

### What's the difference between context and chart ID?

- **Context** (`disk.space`): The metric type—shared by all instances
- **Chart ID** (`disk_space._mnt_data`): A specific instance

Templates use contexts. Alarms use chart IDs.

### Do user configs completely replace stock configs?

No. User configs are processed **before** stock configs, but both are loaded (unless file shadowing applies). Your alert with the same name wins because it's processed first, but you're not deleting the stock definition—just preempting it.

### What is file shadowing?

If a file with the **same filename** exists in both user and stock directories, only the user file is loaded. The stock file is completely ignored.

This is different from alert-level overriding. With shadowing, you must include ALL alerts you want from that file.

## Related Documentation

- [Health Configuration Reference](/src/health/REFERENCE.md)
- [Overriding Stock Alerts](/src/health/overriding-stock-alerts.md)
