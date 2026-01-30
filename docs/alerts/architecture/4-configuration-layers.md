# 12.4 Configuration Layers

Netdata supports multiple configuration layers for health alerts. Understanding precedence rules helps in making modifications that take effect as intended.

:::important

Stock configuration files in `/usr/lib/netdata/conf.d/health.d/` are overwritten during upgrades. Always place custom alerts in `/etc/netdata/health.d/`.

:::

## 12.4.1 Stock Configuration Layer

Stock alerts are distributed with Netdata and reside in `/usr/lib/netdata/conf.d/health.d/`. These files are installed by the Netdata package and updated with each release. Modifying stock files is not recommended because changes are overwritten during upgrades.

Stock configurations define the default alert set. They are evaluated last in precedence, meaning custom configurations override stock configurations for the same alert name.

## 12.4.2 Custom Configuration Layer

Custom alerts reside in `/etc/netdata/health.d/`. Files in this directory take precedence over stock configurations for the same alert name.

An alert defined in `/etc/netdata/health.d/` with the same name as a stock alert replaces the stock definition entirely.

## 12.4.3 Cloud Configuration Layer

Netdata Cloud can define alerts through the Alerts Configuration Manager. These Cloud-defined alerts take precedence over both stock and custom layers.

Cloud-defined alerts are stored remotely and synchronized to Agents on demand. When you edit an alert through the Cloud UI, it completely replaces any file-based definition with the same name.

## 12.4.4 Complete Precedence Order

| Priority | Type | Source | Description |
|----------|------|--------|-------------|
| 1 (highest) | Alarm | User config | Applies to specific instance, processed first |
| 2 | Alarm | Stock config | Falls through if user alarm doesn't match |
| 3 | Template | User config | Applies to all instances if no alarm matched |
| 4 (lowest) | Template | Stock config | Final fallback for unmatched instances |

This table represents the complete precedence hierarchy when multiple alerts could apply to the same (instance, name) pair.

### First-Match-Wins

Within each precedence level, Netdata uses **first-match-wins** semantics for the same alert name:

1. The first matching definition creates the alert
2. Later definitions with the same name are skipped for that instance

This is why user alerts override stock alerts—user config is processed first.

### File Shadowing

There's one more precedence rule: **file shadowing**. If a file with the same filename exists in both directories, only the user file is loaded:

| User File | Stock File | Result |
|-----------|------------|--------|
| `/etc/netdata/health.d/cpu.conf` | `/usr/lib/netdata/conf.d/health.d/cpu.conf` | Only user file loads |

Unlike name-based overriding, file shadowing skips the entire stock file. You must include ALL alerts you want from that file.

## 12.4.5 Configuration File Merging

At startup and after configuration changes, Netdata merges all configuration layers into a single effective configuration according to the precedence rules above.

Use the Alarms API to verify which definition is active for any given alert:

```bash
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.alarms | to_entries[] | select(.value.name == "alert_name") | .value'
```

Key fields to check:
- `source`: Confirms which config file is active
- `type`: Indicates `alarm` or `template`

## Related Sections

- [12.2 Alert Lifecycle](/docs/alerts/architecture/2-alert-lifecycle.md) - How alerts transition states
- [12.1 Evaluation Architecture](/docs/alerts/architecture/1-evaluation-architecture.md) - Where alerts are evaluated
- [12.5 Scaling Topologies](/docs/alerts/architecture/5-scaling-topologies.md) - Behavior in distributed setups