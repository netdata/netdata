# Netdata Alert Configuration

## Purpose

Configure Netdata health alert definitions. Output syntactically correct, production-ready `.conf` files.

## Alignment Rules

Alert keys MUST be right-aligned so colons appear in the same column.

**Calculate alignment:**
1. Find longest key in your alert (e.g., `chart labels` = 12 chars)
2. For each key: `leading_spaces = longest_key_length - current_key_length`
3. First line (`template:` or `alarm:`) also needs leading spaces

**Key lengths:**
| Key | Length |
|-----|--------|
| `on` | 2 |
| `to` | 2 |
| `os` | 2 |
| `calc` | 4 |
| `warn` | 4 |
| `crit` | 4 |
| `info` | 4 |
| `type` | 4 |
| `units` | 5 |
| `class` | 5 |
| `every` | 5 |
| `delay` | 5 |
| `exec` | 4 |
| `alarm` | 5 |
| `green` | 5 |
| `red` | 3 |
| `lookup` | 6 |
| `repeat` | 6 |
| `summary` | 7 |
| `options` | 7 |
| `hosts` | 6 |
| `charts` | 6 |
| `families` | 8 |
| `plugin` | 6 |
| `module` | 6 |
| `template` | 8 |
| `component` | 9 |
| `host labels` | 11 |
| `chart labels` | 12 |

**Example with `chart labels` (12 chars):**
```
    template: disk_space_usage
          on: disk.space
       class: Utilization
        type: System
   component: Disk
chart labels: mount_point=!/dev !/dev/* *
        calc: $used * 100 / ($avail + $used)
       units: %
       every: 1m
        warn: $this > 80
        crit: $this > 90
       delay: down 15m multiplier 1.5 max 1h
     summary: Disk ${label:mount_point} space usage
        info: Total space utilization of disk ${label:mount_point}
          to: sysadmin
```

**Example with `component` as longest (9 chars):**
```
 template: cpu_usage
       on: system.cpu
    class: Utilization
     type: System
component: CPU
   lookup: average -10m unaligned of user,system
    units: %
    every: 1m
     warn: $this > 80
     crit: $this > 90
    delay: down 15m multiplier 1.5 max 1h
  summary: System CPU utilization
     info: Average CPU utilization over the last 10 minutes
       to: sysadmin
```

**Line continuation (backslash):**
- A line ending with `\` continues on the next line.
- The backslash is replaced by a single space.
- Useful for long `info:` or `summary:` lines.

## Entity Types

| Type | Syntax | Use Case |
|------|--------|----------|
| `alarm:` | Specific chart ID | Single instance (e.g., `disk_space._mnt_data`) |
| `template:` | Chart context | All instances (e.g., all disks via `disk.space`) |

**Key difference:**
- `template on: disk.space` → matches ALL disks (context)
- `alarm on: disk_space._mnt_data` → matches ONE specific disk (chart ID)

## Naming Rules (code behavior)

- Names are sanitized; spaces and many symbols become `_`.
- If sanitization changes the name, Netdata logs a rename and uses the sanitized name.
- Avoid names that collide with chart/dimension/custom variable names (can shadow variables in expressions).

## Configuration Ordering & Precedence

### Precedence Rules (Same Alert Name)

When multiple alerts have the **same name**, only one can exist per instance:

| Priority | Type | Source |
|----------|------|--------|
| 1 (highest) | Alarm | User config |
| 2 | Alarm | Stock config |
| 3 | Template | User config |
| 4 (lowest) | Template | Stock config |

**First-match-wins:** The first matching definition creates the alert; later definitions with the same name are skipped.

### File Locations

| Directory | Purpose | Survives Upgrade |
|-----------|---------|------------------|
| `/etc/netdata/health.d/` | User config (loaded first) | Yes |
| `/usr/lib/netdata/conf.d/health.d/` | Stock config (loaded second) | No |

**File shadowing:** If a file with the same filename exists in both directories, only the user file is loaded.

### Overriding Stock Alerts

**Override ALL instances (template):**
```
# User file: /etc/netdata/health.d/my-overrides.conf
# Same name as stock alert
 template: 10min_cpu_usage
       on: system.cpu
    class: Utilization
     type: System
component: CPU
   lookup: average -10m unaligned of user,system,softirq,irq,guest
    units: %
    every: 1m
     warn: $this > (($status >= $WARNING) ? (60) : (75))
     crit: $this > (($status == $CRITICAL) ? (75) : (90))
    delay: down 15m multiplier 1.5 max 1h
  summary: System CPU utilization
     info: Average CPU utilization over the last 10 minutes
       to: sysadmin
```

**Override ONE instance (alarm):**
```
# Override disk_space_usage only for /mnt/data
    alarm: disk_space_usage
       on: disk_space._mnt_data
    class: Utilization
     type: System
component: Disk
   lookup: max -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 5
     crit: $this < 2
    delay: down 15m multiplier 1.5 max 1h
  summary: Disk /mnt/data space usage
     info: Space utilization of /mnt/data (custom thresholds)
       to: sysadmin
```

**Using chart labels for targeted override:**
```
    template: disk_space_usage
          on: disk.space
       class: Utilization
        type: System
   component: Disk
chart labels: mount_point=/mnt/data
        calc: $used * 100 / ($avail + $used)
       units: %
       every: 1m
        warn: $this > 95
        crit: $this > 99
       delay: down 15m multiplier 1.5 max 1h
     summary: Disk /mnt/data space usage
        info: Space utilization (custom thresholds)
          to: sysadmin
```

### Disabling Alerts

**Method 1: Global disable in netdata.conf**
```ini
[health]
    enabled alarms = !20min_steal_cpu !disk_space_usage *
```

**Method 2: Per-alert disable (match nothing)**
```
 template: 20min_steal_cpu
       on: system.cpu
host labels: _hostname=!*
```

The pattern `_hostname=!*` matches no hosts, effectively disabling the alert.

**Method 3: Silence notifications only**
```
 template: 20min_steal_cpu
       on: system.cpu
   lookup: average -20m unaligned of steal
    units: %
    every: 5m
     warn: $this > (($status >= $WARNING) ? (5) : (10))
       to: silent
```

Alert still appears in dashboard but sends no notifications.

### Applying Changes

```bash
sudo netdatacli reload-health
```

Or: `sudo killall -USR2 netdata`

## Required Lines

| Line | Required | Notes |
|------|----------|-------|
| `alarm:` or `template:` | Yes | First line; name is sanitized (only alphanumerics, `.` and `_` survive) |
| `on:` | Yes | Chart ID (alarm) or context (template) |
| `every:` | If no `lookup` | Evaluation frequency |

**Entity rule:** Each alert must include **at least one** of `lookup`, `calc`, `warn`, or `crit`.  
`warn`/`crit` are **optional** (helper alerts may omit them).

## Configuration Lines

**Quotes:** `class`, `type`, `component`, `units`, `to`, `exec`, `summary`, `info` accept quotes, but Netdata strips them.

### `on:`
```
on: system.cpu        # For alarms: chart ID
on: disk.space        # For templates: context
```

### `class:`
Free-form string. Defaults to `Unknown` if missing.

| Value | Use Case |
|-------|----------|
| `Errors` | Error counts, failures, drops |
| `Latency` | Response times, delays |
| `Utilization` | Resource usage percentages |
| `Workload` | Rates, throughput, requests |

### `type:`
Free-form string. Defaults to `Unknown` if missing.

| Type | Description |
|------|-------------|
| `Ad Filtering` | Ad filtering services (e.g., Pi-hole) |
| `Certificates` | Certificate monitoring |
| `Cgroups` | cgroups CPU/memory |
| `Computing` | Shared compute apps (e.g., BOINC) |
| `Containers` | Docker/Kubernetes/container services |
| `Database` | Databases (MySQL, PostgreSQL, etc.) |
| `Data Sharing` | Data sharing services |
| `DHCP` | DHCP services |
| `DNS` | DNS services |
| `Kubernetes` | Kubernetes-related |
| `KV Storage` | Key-value stores (Redis, Memcached) |
| `Linux` | Linux-specific services (e.g., systemd) |
| `Messaging` | Messaging systems |
| `Netdata` | Netdata internal |
| `Other` | Catch-all |
| `Power Supply` | Power systems (e.g., apcupsd) |
| `Search engine` | Search services (e.g., Elasticsearch) |
| `Storage` | Storage services (not general system disks) |
| `System` | General system alerts |
| `Virtual Machine` | VM software |
| `Web Proxy` | Web proxies (e.g., Squid) |
| `Web Server` | Web servers (Apache, Nginx) |
| `Windows` | Windows services |

### `component:`
Free-form string. Defaults to `Unknown` if missing.  
Examples: `CPU`, `Memory`, `Disk`, `Network`, `MySQL`, `Redis`, `Docker`, `PostgreSQL`.

### `host labels:`
```
host labels: _os=linux                    # Linux only
host labels: _os=linux freebsd            # Linux OR FreeBSD
host labels: environment=production       # Custom label
host labels: _hostname=!*                 # Disable (match nothing)
```

Special labels: `_os` (linux/windows/freebsd), `_hostname`

### `os:` (legacy)

Legacy syntax for OS-based filtering. Prefer `host labels: _os=linux` instead.

```
os: linux
os: linux freebsd
```

### `hosts:` (legacy)

Legacy syntax for hostname filtering. Prefer `host labels: _hostname=...` instead.

```
hosts: server1 server2
```

### `charts:` and `families:` (legacy)

Accepted but **ignored** by the parser (backward compatibility).  
Use `on:` and `chart labels:` instead.

### `plugin:` and `module:` (chart labels)

These filter charts by their internal collector plugin and module labels.

```
plugin: python         # Charts from python collector
module: mysql         # Charts from mysql module
```

Used with chart labels for combined filtering:
```
chart labels: _collect_plugin=python *
chart labels: _collect_module=mysql *
```

### `chart labels:` and `host labels:`

These use **Netdata Simple Patterns** - a space-separated list of match expressions.

**Syntax rules:**
- You must start with `label_key=...` to set the label key.
- Values without `=` belong to the **current key**.
- A value without `=` and **no current key** is ignored.
- Spaces around `=` are trimmed; commas are **not** supported.
- Multiple values for the same key are evaluated left-to-right; the first match (including `!`) wins.

#### Simple Patterns Syntax

| Pattern | Meaning | Example |
|---------|---------|---------|
| `*` | Match everything | `*` matches any value |
| `pattern` | Exact match | `linux` matches only "linux" |
| `pattern*` | Starts with | `disk*` matches "disk_space", "disk1" |
| `*pattern` | Ends with | `*_error` matches "read_error", "write_error" |
| `*pattern*` | Contains | `*sql*` matches "mysql", "postgresql" |
| `!pattern` | Exclude exact | `!linux` excludes only "linux" |
| `!pattern*` | Exclude starts with | `!/dev*` excludes "/dev", "/dev/sda" |
| `!*pattern` | Exclude ends with | `!*tmp` excludes "/tmp", "/var/tmp" |
| `!*pattern*` | Exclude contains | `!*test*` excludes "test", "testing", "mytest" |

#### Pattern Matching Rules

1. **Left to right evaluation** - patterns are checked in order
2. **First match wins** - the first pattern that matches (including `!` negation) decides the result; remaining patterns are ignored
3. **Implicit deny** - if no pattern matches, the item is excluded
4. **Trailing `*` catches all** - typically placed last as a default match

#### Examples

**Important:** All patterns after `label_name=` apply to that label **until another `key=` appears**.  
You can include multiple label keys in the same line; **all keys must match** (AND).

```
chart labels: mount_point=!/dev !/dev/* !/run !/run/* !HarddiskVolume* *
```
This is ONE label filter (`mount_point`) with multiple patterns:
- `!/dev` - exclude exact "/dev"
- `!/dev/*` - exclude anything starting with "/dev/"
- `!/run` - exclude exact "/run"
- `!/run/*` - exclude anything starting with "/run/"
- `!HarddiskVolume*` - exclude Windows volumes starting with "HarddiskVolume"
- `*` - match everything else (default accept)

**Wrong interpretation:** Do NOT split this into `mount_point=...` and `volume=...` - all patterns belong to `mount_point`.

```
chart labels: device=!wl* !docker* *
```
ONE label filter (`device`) with patterns:
- `!wl*` - exclude wireless interfaces (wlan0, wlp2s0, etc.)
- `!docker*` - exclude docker interfaces
- `*` - match everything else

```
host labels: _os=linux freebsd
```
ONE label filter (`_os`) with patterns:
- `linux` - match "linux"
- `freebsd` - match "freebsd"
- (no trailing `*`, so only these two match)

```
host labels: _hostname=!*
```
Special case: `!*` excludes everything - effectively disables the alert

#### Multiple Labels

You can filter multiple label keys in the **same line** or in **separate lines**.  
Commas are **not** supported.
Spaces around `=` are trimmed.

```
chart labels: mount_point=!/dev* * filesystem=ext4 xfs
```

### Data Flow: `lookup:` → `calc:`

**Important:** The `$this` variable is the central data carrier:

1. If `lookup:` is present, it executes first and stores the result in `$this`
2. If `calc:` is present, it evaluates using `$this` (from lookup or previous value) and overwrites `$this` with its result
3. The final `$this` value is used for `warn:` and `crit:` evaluation

This means:
- `lookup:` alone: `$this` = lookup result
- `calc:` alone: `$this` = calc result (uses dimension variables)
- `lookup:` + `calc:`: lookup runs first, calc can transform `$this`

**Evaluation order:**
- Netdata computes `$this` for all alerts first (lookup/calc pass).
- Then it evaluates `warn`/`crit` for status changes.

### `lookup:`
```
lookup: METHOD[(GROUP-OPTIONS)] AFTER [at BEFORE] [every DURATION] [OPTIONS] [of DIMENSIONS]
```

**Group options (inside parentheses):**
- `countif(CONDITION VALUE)` where CONDITION is one of `=`, `==`, `:`, `!=`, `<>`, `>`, `>=`, `<`, `<=`, `!`  
  - Examples: `countif(>5)`, `countif(<=0)`, `countif(!=0)`, `countif(:0)`  
  - If no value is provided (e.g., `countif(>)`), it defaults to `0`.
- `percentile(N)` where N is 0–100 (default `95` if omitted or empty).
- `trimmed-mean(N)` / `trimmed-median(N)` where N is trim % (default `5` if omitted or empty).

**Methods (time aggregation) + aliases:**

| Method | Aliases / Notes |
|--------|----------------|
| `average` | `avg`, `mean` |
| `min` |  |
| `max` |  |
| `sum` |  |
| `median` |  |
| `stddev` |  |
| `cv` | `rsd`, `coefficient-of-variation` |
| `ses` | `ema`, `ewma` |
| `des` |  |
| `incremental_sum` | `incremental-sum` |
| `trimmed-mean` | `trimmed-mean1/2/3/5/10/15/20/25` |
| `trimmed-median` | `trimmed-median1/2/3/5/10/15/20/25` |
| `percentile` | `percentile25/50/75/80/90/95/97/98/99` |
| `countif` |  |
| `extremes` |  |

**Time parameters:**
- `AFTER` - Start of time window (e.g., `-10m` means 10 minutes ago)
- `at BEFORE` - End of time window for historical queries (e.g., `at -50m` means ending 50 minutes ago)

Example: `lookup: min -10m at -50m` means "get minimum value from the 10-minute window that ended 50 minutes ago" (i.e., from 60 to 50 minutes ago).

**Time window behavior:**

| Parameter | Effect |
|-----------|----------|
| `-10m` | Looks at data from 10 minutes ago to now |
| `-10m at -50m` | Looks at historical data (60-50 minutes ago) |
| Window slides | As time progresses, the window moves forward |

This creates a **rolling/sliding window** - the query always examines a fixed-duration window relative to the current time.

**Evaluation frequency defaults:**
- If `every` is not provided, Netdata uses `abs(AFTER)` as the default evaluation interval.
- You can override it with `every` inside the `lookup:` line, or with a separate `every:` line (last one wins).

**Dimensions:**
- `of DIMENSIONS` uses simple patterns (space-separated).
- `of all` (or omitting `of`) includes all dimensions.

**Options:**
| Option | Effect |
|--------|--------|
| `absolute` / `abs` / `absolute_sum` | Use absolute values before aggregating |
| `percentage` | Percentage of dimensions over total |
| `unaligned` | Sliding window (don't align to boundaries) |
| `min` | Aggregate dimensions by minimum |
| `max` | Aggregate dimensions by maximum |
| `average` | Aggregate dimensions by average |
| `min2max` | Aggregate dimensions by (max - min) |
| `sum` | Aggregate dimensions by sum (default/no-op) |
| `null2zero` | Convert null values to zero |
| `match-ids` / `match_ids` | Match dimensions by IDs (default) |
| `match-names` / `match_names` | Match dimensions by names |
| `anomaly-bit` | Use anomaly rate as data source |

**Examples:**
```
lookup: average -10m unaligned of user,system,softirq,irq,guest
lookup: max -1h of used
lookup: sum -5m absolute of errors
lookup: average -1m percentage of used
lookup: min -10m at -50m unaligned of avail   # Historical baseline
```

Result stored in `$this`. Timestamps available as `$after`, `$before`.

### `calc:`

Evaluates an expression and stores result in `$this`.

```
calc: $used * 100 / ($avail + $used)              # Percentage from dimensions
calc: ($rate > 0) ? ($remaining / $rate) : (inf)  # Predictive
calc: ($condition) ? (value_if_true) : (false)    # Conditional
calc: abs($value)                                  # Absolute value
calc: 100 - $this                                  # Transform lookup result
calc: $this / 1024                                 # Unit conversion
```

When both `lookup:` and `calc:` are present, `calc:` can reference `$this` to transform the lookup result.

### `units:`
Examples: `%`, `seconds`, `ms`, `MB`, `GB`, `GB/hour`, `requests/s`, `packets`, `errors`, `hours`

### `every:`
```
every: 10s    # Critical metrics
every: 1m     # Standard
every: 5m     # Trend analysis
```

### `green:` and `red:`

Set chart visualization thresholds (not alert thresholds). Values available as `$green` and `$red` variables.

```
green: 50
red: 90
```

**Important:**
- These are for chart display, not alert triggers
- Values can be referenced in expressions as `$green` and `$red`
- Only first alert's thresholds per chart are used for visualization

### `warn:` and `crit:`

**Simple:**
```
warn: $this > 80
crit: $this > 90
```

**With hysteresis (anti-flapping):**

The ternary operator `(condition) ? (value_if_true) : (value_if_false)` enables hysteresis:

```
warn: $this > (($status >= $WARNING) ? (75) : (85))
```

Breaking this down:
- `$status >= $WARNING` - Is the alert ALREADY in warning (or critical) state?
- If YES (already alarmed): use threshold `75`
- If NO (currently clear): use threshold `85`

**Result:**
- To ENTER warning: value must exceed 85%
- To STAY in warning: value only needs to exceed 75%
- To CLEAR: value must drop below 75%

This creates a 10% "dead zone" that prevents flapping when values hover near a threshold.

```
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

Same pattern for critical:
- To ENTER critical: value must exceed 95%
- To STAY in critical: value only needs to exceed 85%

**Memory aid:** The FIRST value (after `?`) is the LOWER threshold used when ALREADY alarmed. The SECOND value (after `:`) is the HIGHER threshold used to initially TRIGGER the alarm.

**Low-value alerts (inverted):**
```
warn: $this < (($status >= $WARNING) ? (15) : (10))
```

**With conditions:**
```
warn: ($requests > 120) ? ($this > 1) : (0)       # Data availability guard
crit: ($this > 90) && $avail < 5                  # Compound condition
warn: $this != nan AND $this > 0                   # NaN safety
```

**Evaluation notes:**
- Expressions return a number: `0` = false, non-zero = true.
- In logical checks, `nan` is false and `inf` is true.
- If the **final** expression result is `nan` or `inf`, evaluation fails and that check becomes **UNDEFINED**.
- `nan`/`inf` can exist inside comparisons or short-circuited branches without failing the whole expression.

### `delay:`
```
delay: [up U] [down D] [multiplier M] [max X]
```

| Param | Purpose |
|-------|---------|
| `up U` | Delay when worsening (CLEAR→WARNING) |
| `down D` | Delay when improving (WARNING→CLEAR) |
| `multiplier M` | Multiply delay on repeated changes |
| `max X` | Maximum delay |

**Common patterns:**
```
delay: down 15m multiplier 1.5 max 1h            # Standard
delay: up 1m down 15m multiplier 1.5 max 1h      # Quick clear, slow trigger
delay: up 30s down 15m multiplier 1.5 max 1h     # Quick trigger, slow clear
```

### `repeat:`
```
repeat: warning 1h critical 30m
repeat: off
```

### `options:`
```
options: no-clear-notification
options: no-clear
```

Use when clearing conditions are unreliable (baseline comparisons, volatile metrics).
Aliases: `no-clear-notification` and `no-clear` do the same thing (disable clear notifications).

### `to:`
```
to: sysadmin      # System issues
to: dba           # Database issues
to: webmaster     # Web server issues
to: silent        # Monitor only, no notifications
```

First parameter passed to `exec` script when alert changes status.
If omitted, the default recipient is `root` (from `[health]` in `netdata.conf`).

### `exec:`

Custom script to execute when alert status changes.

```
exec: /path/to/script.sh
```

**Default:** `alarm-notify.sh` (from `[health]` -> `script to execute on alarm` in `netdata.conf`)

**Important:**
- Script receives alert details as parameters
- The `to:` value is the first parameter
- Status changes trigger execution

### `summary:` and `info:`

**Important:** Values are plain strings. Quotes are allowed but stripped, so avoid them.

```
summary: Disk ${label:mount_point} space usage
info: Total space utilization of disk ${label:mount_point}
```

**Variables available:**
- `${family}` - chart family
- `${label:LABEL_NAME}` - value of chart label

These variables are expanded at runtime to show context-specific information.

## Variables

### Special
| Variable | Contains |
|----------|----------|
| `$this` | Current calculated value |
| `$status` | Current alert status (numeric: -2,-1,0,1,3,4) |
| `$now` | Current Unix timestamp |
| `$after` | Query start timestamp |
| `$before` | Query end timestamp |
| `$update_every` | Chart update frequency |
| `$green`, `$red` | Threshold values |

### Status Constants

The `$status` variable holds a numeric value that can be compared with these constants:

| Constant | Value | Meaning |
|----------|-------|---------|
| `$REMOVED` | -2 | Alert was deleted |
| `$UNDEFINED` | -1 | Expression evaluation failed |
| `$UNINITIALIZED` | 0 | Not yet calculated |
| `$CLEAR` | 1 | Normal state, no issue |
| `$WARNING` | 3 | Warning threshold exceeded |
| `$CRITICAL` | 4 | Critical threshold exceeded |

**Understanding status comparisons:**
- `$status >= $WARNING` is true when `$status >= 3`, i.e., when alert is in WARNING (3) or CRITICAL (4) state
- `$status == $CRITICAL` is true when `$status == 4`, i.e., only when alert is in CRITICAL state

**Note:** An internal `RAISED = 2` exists, but it is **not** exposed as `$RAISED` in expressions.

This enables hysteresis: check if the alert is ALREADY triggered before deciding which threshold to use.

### Dimension Variables
- `$dimension_name` - Last calculated value
- `$dimension_name_raw` - Last raw value
- `$dimension_name_last_collected_t` - Collection timestamp

### Chart Variables
- `$last_collected_t` - Unix timestamp of last data collection
- Custom chart variables may exist (collector-defined). Use the `alarm_variables` API to list them.

**Common dimensions:**
- CPU: `$user`, `$system`, `$softirq`, `$irq`, `$guest`, `$iowait`, `$nice`, `$steal`
- Memory: `$used`, `$available`, `$free`, `$cached`, `$buffers`, `$avail`
- Disk: `$used`, `$avail`, `$free`
- Network: `$received`, `$sent`, `$inbound`, `$outbound`

### Cross-Chart References
```
calc: $this * 100 / ${system.ram.used}
calc: ($system.ram.used + $system.ram.cached + $system.ram.free)
```

Format: `${chart.dimension}` or `$chart.dimension`

**Label-aware resolution:**
- If multiple chart instances match, Netdata picks the one with the **highest label overlap** with the current chart.
- Ties depend on internal iteration order (not guaranteed stable).

### Other Alert References

Alerts can reference values from other alerts by using the alert name as a variable:

```
calc: $this * 100 / $baseline_alert
warn: $this > $other_alert_threshold
```

**Important:** Variable names must match the exact alert name. If an alert is named `disk_fill_rate`, reference it as `$disk_fill_rate` - not `$fill_rate` or `$diskFillRate`.

**Dependency Resolution:**

Alerts reference the **most recent stored value** of other alerts (`rc->value`).

**Evaluation order:**
- Netdata does **not** build a dependency graph.
- Calculations run sequentially; order is not guaranteed.
- If Alert B references Alert A and A runs **later**, B uses A’s **previous** value.
- Circular references are **not resolved** and may yield `nan`/`UNDEFINED`.

## Operators

### Operator Precedence (highest to lowest)

| Priority | Operators | Description |
|----------|-----------|-------------|
| 1 (highest) | `()` | Parentheses (grouping) |
| 2 | `!`, `NOT`, unary `+`/`-`, `abs()` | Unary operators |
| 3 | `*`, `/`, `%` | Multiplication, division, modulo |
| 4 | `+`, `-` | Addition, subtraction |
| 5 | `<`, `<=`, `>`, `>=` | Relational comparisons (left-to-right) |
| 6 | `==`, `!=`, `<>` | Equality comparisons (left-to-right) |
| 7 | `&&`, `AND`, `\|\|`, `OR` | Logical operators (same precedence, left-to-right) |
| 8 (lowest) | `? :` | Ternary conditional |

### Operators Reference

| Type | Operators |
|------|-----------|
| Arithmetic | `+`, `-`, `*`, `/`, `%` |
| Comparison | `<`, `>`, `<=`, `>=`, `==`, `!=`, `<>` |
| Logical | `&&`, `||`, `!`, `AND`, `OR`, `NOT` |

**Important distinctions:**
- `&&` and `AND` are logical AND (both conditions must be true)
- `||` and `OR` are logical OR (either condition must be true)
- **AND and OR have the same precedence** - evaluated left-to-right
- Equality (`==`, `!=`, `<>`) has lower precedence than relational (`<`, `<=`, `>`, `>=`)
- **Short-circuiting applies to logical operators** (`&&`, `||`, `AND`, `OR`)
- Arithmetic operators always evaluate both sides
- Use parentheses to ensure intended evaluation order when mixing AND/OR

**Functions:** `abs()`

**Conditional (ternary):** `(condition) ? (true_value) : (false_value)`

**Special values:**
- `nan` - not a number (use `$this != nan` to check)
- `inf` - infinite (useful inside comparisons or guards)

**Unary signs:** `+value` and `-value` are supported.

## Common Patterns

### Simple Threshold with Hysteresis
```
    template: disk_space_usage
          on: disk.space
       class: Utilization
        type: System
   component: Disk
chart labels: mount_point=!/dev !/dev/* !/run !/run/* *
        calc: $used * 100 / ($avail + $used)
       units: %
       every: 1m
        warn: $this > (($status >= $WARNING) ? (80) : (90))
        crit: ($this > (($status == $CRITICAL) ? (90) : (98))) && $avail < 5
       delay: up 1m down 15m multiplier 1.5 max 1h
     summary: Disk ${label:mount_point} space usage
        info: Total space utilization of disk ${label:mount_point}
          to: sysadmin
```

### Rate-Based Alert
```
 template: mysql_slow_queries
       on: mysql.queries
    class: Latency
     type: Database
component: MySQL
   lookup: sum -10s of slow_queries
    units: slow queries
    every: 10s
     warn: $this > (($status >= $WARNING) ? (5) : (10))
     crit: $this > (($status == $CRITICAL) ? (10) : (20))
    delay: down 5m multiplier 1.5 max 1h
  summary: MySQL slow queries
     info: Number of slow queries in the last 10 seconds
       to: dba
```

### Predictive (Time to Full)
```
   template: disk_fill_rate
         on: disk.space
     lookup: min -10m at -50m unaligned of avail
       calc: ($this - $avail) / (($now - $after) / 3600)
      every: 1m
      units: GB/hour
       info: Average rate the disk fills up over the last hour

   template: out_of_disk_space_time
         on: disk.space
       calc: ($disk_fill_rate > 0) ? ($avail / $disk_fill_rate) : (inf)
      units: hours
      every: 10s
       warn: $this > 0 and $this < (($status >= $WARNING) ? (48) : (8))
       crit: $this > 0 and $this < (($status == $CRITICAL) ? (24) : (2))
      delay: down 15m multiplier 1.2 max 1h
    summary: Disk ${label:mount_point} will run out of space
       info: Estimated time the disk will run out of space
         to: sysadmin
```

### Ratio with Data Availability Guard
```
 template: web_log_1m_requests
       on: web_log.type_requests
    class: Workload
     type: Web Server
component: Web log
   lookup: sum -1m unaligned
     calc: ($this == 0) ? (1) : ($this)
    units: requests
    every: 10s
     info: Number of HTTP requests in the last minute

 template: web_log_successful_requests
       on: web_log.type_requests
    class: Workload
     type: Web Server
component: Web log
   lookup: sum -1m unaligned of success
     calc: $this * 100 / $web_log_1m_requests
    units: %
    every: 10s
     warn: ($web_log_1m_requests > 120) ? ($this < 95) : (0)
     crit: ($web_log_1m_requests > 120) ? ($this < 85) : (0)
    delay: up 2m down 15m multiplier 1.5 max 1h
  summary: Web log successful requests
     info: Ratio of successful HTTP requests over the last minute
       to: webmaster
```

### Status/Boolean Check
```
 template: redis_master_link_down
       on: redis.master_link_down_since_time
    class: Errors
     type: KV Storage
component: Redis
    every: 10s
     calc: $time
    units: seconds
     crit: $this != nan && $this > 0
  summary: Redis master link down
     info: Time elapsed since the link between master and slave is down
       to: dba
```

### Hit Ratio (Inverted)
```
 template: postgres_db_cache_io_ratio
       on: postgres.db_cache_io_ratio
    class: Workload
     type: Database
component: PostgreSQL
   lookup: average -1m unaligned of miss
     calc: 100 - $this
    units: %
    every: 1m
     warn: $this < (($status >= $WARNING) ? (70) : (60))
     crit: $this < (($status == $CRITICAL) ? (60) : (50))
    delay: down 15m multiplier 1.5 max 1h
  summary: PostgreSQL DB ${label:database} cache hit ratio
     info: Average cache hit ratio in db ${label:database} over the last minute
       to: dba
```

### Zero-Tolerance Error
```
 template: btrfs_device_write_errors
       on: btrfs.device_errors
    class: Errors
     type: System
component: File system
    units: errors
   lookup: max -10m every 1m of write_errs
     crit: $this > 0
    delay: up 1m down 15m multiplier 1.5 max 1h
  summary: BTRFS device write errors
     info: Number of encountered BTRFS write errors
       to: sysadmin
```

### OS-Specific
```
   template: 10min_cpu_usage
         on: system.cpu
      class: Utilization
       type: System
  component: CPU
host labels: _os=linux
     lookup: average -10m unaligned of user,system,softirq,irq,guest
      units: %
      every: 1m
       warn: $this > (($status >= $WARNING) ? (75) : (85))
       crit: $this > (($status == $CRITICAL) ? (85) : (95))
      delay: down 15m multiplier 1.5 max 1h
    summary: System CPU utilization
       info: Average CPU utilization over the last 10 minutes
         to: sysadmin
```

### Spike Detection
```
   template: 10s_received_packets_storm
         on: net.packets
      class: Workload
       type: System
  component: Network
     lookup: average -10s unaligned of received
       calc: $this * 100 / (($1m_received_packets_rate < 1000) ? (1000) : ($1m_received_packets_rate))
      every: 10s
      units: %
       warn: $this > (($status >= $WARNING) ? (200) : (5000))
       crit: $this > (($status == $CRITICAL) ? (5000) : (6000))
    options: no-clear-notification
    summary: Network interface ${label:device} inbound packet storm
       info: Ratio of average received packets over last 10 seconds vs last minute
         to: silent
```

## Best Practices

### Thresholds
- Warning: 70-85% capacity
- Critical: 85-95% capacity
- Hysteresis gap: 10-20%

### Time Windows
| Purpose | Window |
|---------|--------|
| Real-time | `-10s` to `-1m` |
| Short-term | `-5m` to `-10m` |
| Medium-term | `-1h` to `-4h` |
| Baseline | `-1d`, `-1w` |

### Evaluation Frequency
| Type | Frequency |
|------|-----------|
| Critical metrics | `10s` to `30s` |
| Resource usage | `1m` to `5m` |
| Trend analysis | `5m` to `1h` |

### Required Fields
Strongly recommended:
- `class`, `type`, `component` - Classification
- `summary`, `info` - Documentation
- `units` - Display units
- `to` - Notification recipient
- `delay` - Hysteresis control

### Safety Patterns
```
calc: ($divisor > 0) ? ($result) : (fallback)   # Division safety
calc: ($this == 0) ? (1) : ($this)              # Zero protection
warn: $this != nan AND $this > threshold        # NaN safety
```

**Expression Error Handling:**

- If the final result is `nan` or `inf`, evaluation fails and that check becomes **UNDEFINED**.
- Division by zero yields `inf` (so it fails).
- `1 * nan` yields `nan` (so it fails).
- Unknown variables yield `nan` **unless** they are short-circuited away by `&&`, `||`, or `?:`.

**Important:**
- `nan` and `inf` can appear inside expressions (comparisons handle them)
- `nan == nan` is **true** and `nan != nan` is **false** (Netdata special handling)
- `inf == inf` is **true** and `inf != inf` is **false**
- Use `$this != nan` to guard against NaN
- Division by zero does NOT crash but yields `inf`

## Validation Checklist

1. First line is `alarm:` or `template:`
2. `on:` line present
3. At least one of: `lookup`, `calc`, `warn`, `crit`
4. `every:` present if no `lookup`
5. All colons aligned in same column
6. Units match calculated value semantics
7. Variables exist on target chart
