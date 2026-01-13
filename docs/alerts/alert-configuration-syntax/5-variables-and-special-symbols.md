# 3.5 Variables and Special Symbols

Alert expressions in Netdata can reference variables that represent metric values, alert state, time information, and chart metadata. Understanding which variables are available and how to discover them is essential for writing effective alerts.

:::tip

Refer to this section when you're writing `calc`, `warn`, or `crit` expressions and need to know which variables exist, debugging alerts that reference missing or incorrect variable names, building alerts that combine multiple dimensions or chart metadata, or using the `alarm_variables` API to explore what's available on a chart.

:::

## 3.5.1 Core Expression Variables

These variables are always available in alert expressions after `lookup` or `calc` has been evaluated.

### `$this` (Current Alert Value)

**Contains:** The primary value being evaluated by this alert. If `calc` is defined, this is the result of the `calc` expression. If no `calc`, this is the result of the `lookup` aggregation.

**Scope:** Available in all `calc`, `warn`, `crit`, and `ok` expressions.

**Example:**
```conf
lookup: average -5m unaligned of used
  calc: $this / (1024 * 1024)  # Convert bytes to MB
  warn: $this > 800             # $this now in MB
  crit: $this > 900
```

### `$status` (Current Alert Status)

**Contains:** The alert's current status as a numeric value.

**Scope:** Available in all expressions, commonly used for hysteresis and status-aware conditions.

**Typical Usage:**
```conf
# Only enter WARNING from CLEAR (not from CRITICAL)
warn: ($this > 80) && ($status == $CLEAR)
crit: $this > 95

# Use different thresholds based on current status
warn: $this > (($status >= $WARNING) ? (75) : (85))
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

For numeric status values, see 3.5.3 Status Constants below.

### `$now` (Current Timestamp)

**Contains:** Current Unix timestamp (seconds since epoch).

**Scope:** Available in all expressions.

**Typical Usage:** Time-based calculations, particularly for monitoring data collection delays.

**Example:**
```conf
# Alert if data collection is stale
calc: $now - $last_collected_t
warn: $this > (5 * $update_every)
crit: $this > (10 * $update_every)
```

### `$after` and `$before` (Lookup Time Window)

**Contains:**
- `$after`: Start timestamp of the database lookup timeframe (Unix timestamp)
- `$before`: End timestamp of the database lookup timeframe (Unix timestamp)

**Scope:** Available after `lookup` is executed.

**Typical Usage:** Calculate rates of change over time.

**Example:**
```conf
lookup: min -10m at -50m unaligned of avail
  calc: ($this - $avail) / (($now - $after) / 3600)  # Hourly rate
  warn: $this > 100  # Disk filling at >100 GB/hour
```

## 3.5.2 Dimension Variables

Every dimension on a chart automatically becomes a variable you can reference in alert expressions.

### How Dimensions Become Variables

When you write an alert with `on: system.cpu`, every dimension of the `system.cpu` chart becomes available:

| Chart | Dimensions | Available Variables |
|-------|------------|-------------------|
| `system.cpu` | `user`, `system`, `nice`, `iowait`, `irq`, `softirq`, `steal`, `guest`, `guest_nice` | `$user`, `$system`, `$nice`, `$iowait`, `$irq`, `$softirq`, `$steal`, `$guest`, `$guest_nice` |
| `disk.space` | `used`, `avail` | `$used`, `$avail` |
| `mem.available` | `available` | `$available` |

### Three Forms of Dimension Variables

Each dimension is exposed in three forms:

<details>
<summary><strong>1. Normal (Stored Value): `$dimension_name`</strong></summary>

Contains the last stored value (calculated, interpolated, as shown in charts).

**Example:**
```conf
on: disk.space
calc: $used * 100 / ($avail + $used)  # Percentage used
warn: $this > 80
```

</details>

<details>
<summary><strong>2. Raw (Collected Value): `$dimension_name_raw`</strong></summary>

Contains the last collected value (before any processing or interpolation).

:::tip

Use when you need the exact raw value from the collector, not the interpolated chart value.

:::

**Example:**
```conf
on: system.mem
calc: $mem_raw > 5000000  # Raw collected value in KiB
warn: $this > 4000000
```

</details>

<details>
<summary><strong>3. Timestamp: `$dimension_name_last_collected_t`</strong></summary>

Contains Unix timestamp when this dimension was last collected.

When to use: Time-based calculations, detecting stale data.

**Example:**
```conf
on: system.cpu
calc: $now - $user_last_collected_t
warn: $this > 60  # Warn if CPU data is >60 seconds old
```

</details>

<details>
<summary><strong>Real Configuration Example from Stock Alerts</strong></summary>

From Netdata's stock alerts (`disks.conf`):

```conf
template: disk_space_usage
          on: disk.space
       calc: $used * 100 / ($avail + $used)
        warn: $this > (($status >= $WARNING ) ? (80) : (90))
        crit: ($this > (($status == $CRITICAL) ? (90) : (98))) && $avail < 5
```

Variables used: `$used`, `$avail`, `$this`, `$status`, `$WARNING`, `$CRITICAL`

</details>

## 3.5.3 Status Constants

Alert status values are exposed as constants you can use in expressions for status-aware logic and hysteresis.

### Complete Status Constants Table

| Variable Name | Numeric Value | Meaning | When This Status Occurs |
|--------------|---------------|---------|------------------------|
| `$REMOVED` | -2 | Alert deleted | Alert removed during configuration reload |
| `$UNDEFINED` | -1 | Calculation failed | Expression evaluation or database lookup failed |
| `$UNINITIALIZED` | 0 | Not yet evaluated | Alert created but not yet run |
| `$CLEAR` | 1 | Alert OK | Neither `warn` nor `crit` conditions are true |
| `$WARNING` | 3 | Warning condition | `warn` expression returned true, `crit` did not |
| `$CRITICAL` | 4 | Critical condition | `crit` expression returned true |

:::tip

**Why values 1, 3, 4 instead of 1, 2, 3?**

There's an internal status value 2 (`RAISED`) used by the health engine for state transitions. This status is not exposed as a constant variable (no `$RAISED`), but you may see `$status == 2` in logs or API responses during alert lifecycle transitions.

:::note

Always use the constants (`$WARNING`, `$CRITICAL`) rather than hardcoding numeric values.

:::

### Usage Patterns

<details>
<summary><strong>Pattern 1: Status-Aware Thresholds (Hysteresis)</strong></summary>

```conf
# Enter WARNING at 85, but once in WARNING, only clear below 75
warn: $this > (($status >= $WARNING) ? (75) : (85))
crit: $this > (($status == $CRITICAL) ? (85) : (95))
```

Why this works: When `$status` is already `$WARNING` (value 3), the ternary uses the lower threshold (75), creating hysteresis.

</details>

<details>
<summary><strong>Pattern 2: Preventing Direct Transitions</strong></summary>

```conf
# Can only enter WARNING from CLEAR, not from CRITICAL
warn: ($this > 80) && ($status == $CLEAR)
crit: $this > 95
```

</details>

<details>
<summary><strong>Pattern 3: Status-Based Notifications</strong></summary>

```conf
# Different thresholds for different alert states
warn: ($status == $CLEAR) && ($this > 80)
crit: ($status != $CRITICAL) && ($this > 95)
```

For more advanced hysteresis patterns, see **8.1 Hysteresis and Status-Based Conditions**.

</details>

## 3.5.4 Chart-Level Variables

These variables provide metadata about the chart itself, not individual dimensions.

### `$last_collected_t` (Chart Last Collection Timestamp)

**Contains:** Unix timestamp when the chart last collected data.

**Example:**
```conf
# Alert if chart data is stale
calc: $now - $last_collected_t
warn: $this > (5 * $update_every)
crit: $this > (10 * $update_every)
```

### `$update_every` (Chart Update Frequency)

**Contains:** How often the chart collects data (in seconds).

**Example:**
```conf
# Alert if collection delay exceeds 5 update cycles
calc: $now - $last_collected_t
warn: $this > (5 * $update_every)
```

### `$collected_total_raw` (Sum of All Dimensions)

**Contains:** Sum of all dimensions' last collected values (raw).

When to use: Total capacity calculations.

**Example:**
```conf
# Total disk capacity = used + available
calc: $used * 100 / $collected_total_raw
warn: $this > 80
```

### `$green` and `$red` (Threshold Values)

**Contains:** The `green` and `red` threshold values **explicitly defined** in the alert configuration using the `green:` and `red:` lines.

**Important:** These are NOT automatically set from `warn`/`crit` thresholds — you must define them explicitly if you want to reference them in expressions.

**When to use:** Use `green` and `red` when you want to:
- Reference thresholds in `calc` expressions
- Compute relative positions between thresholds
- Display threshold values in `info` or `summary` text

**Example:**

```conf
green: 80
  red: 95
 calc: ($this - $green) / ($red - $green) * 100  # Percentage between thresholds
 warn: $this > $green
 crit: $this > $red
```

:::note

These variables are `NaN` (not a number) if `green:` and `red:` lines are not present in the alert configuration. Test for `NaN` in expressions if these might be undefined.

:::

## 3.5.5 Variable Naming Rules and Syntax

### Allowed Characters

Valid characters in variable names: letters (`a-z`, `A-Z`), digits (`0-9`), underscore (`_`), and period (`.` for host-level variables like `$system.cpu.user`).

Invalid characters (these terminate variable names): whitespace, operators (`&`, `|`, `!`, `>`, `<`, `=`, `+`, `-`, `*`, `/`, `?`, `%`), and parentheses (`)`, `}`).

### Maximum Variable Name Length

Limit: 300 characters

### Two Variable Syntax Forms

<details>
<summary><strong>Form 1: Simple Variable (`$variable`)</strong></summary>

**Syntax:** `$` followed by valid characters until the first invalid character.

**Examples:**
```conf
$this
$status
$used
$user
$system.cpu.user
```

Parsing stops at first operator, space, or closing parenthesis.

</details>

<details>
<summary><strong>Form 2: Braced Variable (`${variable name}`)</strong></summary>

**Syntax:** `${` followed by any characters until `}`.

**Purpose:** Allows spaces and special characters in variable names.

**When to use:** Template variables (`${family}` for chart family name) and label variables (`${label:LABEL_NAME}` for chart label value).

**Examples:**
```conf
${family}
${label:mount_point}
${label:device}
${label:interface}
```

**Real Example** (from `disks.conf`):
```conf
summary: Disk ${label:mount_point} space usage
info: Total space utilization of disk ${label:mount_point}
```

</details>

:::note

**Template substitutions versus expression variables:**

`${family}` and `${label:NAME}` are string substitutions used in `summary`, `info`, and other text fields. They are not available as numeric variables in `calc`, `warn`, or `crit` expressions. For numeric calculations, use dimension variables (`$dimension`) and chart variables (`$last_collected_t`, etc.).

:::

## 3.5.6 Discovering Available Variables

When writing alerts, you often need to know which variables exist for a specific chart. Netdata provides an API to list all available variables.

### The `alarm_variables` API

**Endpoint:** `/api/v1/alarm_variables`

**HTTP Method:** GET

**Required Parameter:** `chart` (chart ID or name)

**Example Requests:**
```bash
# List all variables for system.cpu chart
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu" | jq

# List all variables for disk.space chart
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=disk.space" | jq

# List all variables for mem.available chart
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=mem.available" | jq
```

### API Response Structure

The API returns a JSON object with five main sections providing complete variable information for the specified chart.

<details>
<summary><strong>Response Sections Overview</strong></summary>

**1. Chart Metadata**
- `chart`: Chart ID
- `chart_name`: Human-readable chart name
- `chart_context`: Chart context
- `family`: Chart family
- `host`: Hostname

**2. Current Alert Values (`current_alert_values`)**

Core expression variables and status constants with their current values.

**3. Dimensions Last Stored Values (`dimensions_last_stored_values`)**

Normal dimension variables (interpolated, as shown in charts).

**4. Dimensions Last Collected Values (`dimensions_last_collected_values`)**

Raw dimension variables with `_raw` suffix (exact collected values).

**5. Dimensions Last Collected Time (`dimensions_last_collected_time`)**

Timestamp variables with `_last_collected_t` suffix for each dimension.

**6. Chart Variables (`chart_variables`)**

Chart-level metadata: `last_collected_t`, `update_every`, `collected_total_raw`.

</details>

### Practical Workflow for Writing Alerts

<details>
<summary><strong>Step-by-Step Process</strong></summary>

**Step 1:** Identify the chart you want to alert on
```bash
# List all charts
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts | keys[]'
```

**Step 2:** Query available variables
```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu" | jq
```

**Step 3:** Review dimension names in `dimensions_last_stored_values`

**Step 4:** Write your alert using those variable names
```conf
alarm: 10min_cpu_usage
    on: system.cpu
  calc: $user + $system
   warn: $this > 80
   crit: $this > 95
```

**Step 5:** Reload health configuration
```bash
sudo netdatacli reload-health
```

</details>

### Alternative: Variable Lookup Trace API

For debugging why a specific variable resolves to a particular value:

**Endpoint:** `/api/v1/variable`

**Parameters:** `chart` (chart ID or name, required) and `variable` (variable name to lookup, required)

**Example:**
```bash
curl -s "http://localhost:19999/api/v1/variable?chart=system.cpu&variable=user" | jq
```

**Purpose:** 

Shows all possible matches for a variable name with their scores (label-based matching), helping you understand which chart/dimension was selected.

## 3.5.7 Advanced Variable Types

<details>
<summary><strong>Host-Level Variables</strong></summary>

**Format:** `$CHART.VARIABLE`

**Scope:** Reference dimensions and alerts from any chart on the host.

**Examples:**
- `$system.cpu.user` (User CPU from system.cpu chart)
- `$disk.sda.reads` (Read operations from disk.sda chart)
- `$mem.available.available` (Available memory)

**When to use:** Cross-chart calculations, correlating metrics from different sources.

**Example:**
```conf
alarm: 10min_cpu_usage
    on: system.cpu
  calc: $this + ($ram_available < 1000 ? 50 : 0)
   warn: $this > 100
```

</details>

<details>
<summary><strong>Context-Level Variables</strong></summary>

**Format:** `$context.dimension`

**Scope:** Reference dimensions from all charts sharing the same context.

**Example:**
```conf
template: disk_space_usage
          on: disk.space
       calc: $used * 100 / ($avail + $used)
        warn: $this > 80
```

This works across all disk instances (sda, sdb, nvme0n1, etc.) because they all share the `disk.space` context.

</details>

<details>
<summary><strong>Running Alert Values</strong></summary>

**Format:** `$alert_name`

**Scope:** Reference the current value of another running alert.

**When to use:** Cascading alerts, composite conditions.

**Example:**
```conf
# Alert if both CPU and memory alerts are firing
alarm: 10min_cpu_usage
    on: system.cpu
  calc: $10min_cpu_usage + $ram_available
   warn: $this > 1
   crit: $this > 2
```

</details>

## 3.5.8 Variable Lookup Order and Scoring

When Netdata resolves a variable name, it searches in this order:

1. Special variables: `$this`, `$status`, `$now`, status constants (highest priority)
2. Chart dimensions: Dimension IDs and names from current chart
3. Chart variables: Custom chart variables
4. Host variables: Custom host variables
5. Running alerts: Values from other alerts
6. Context lookups: Variables in format `context.dimension`

:::note

If multiple matches are found, Netdata uses a label-based scoring system: count common labels between the alert's chart and the variable's source chart. Higher score equals better match. The variable with the highest score is selected.

:::

## 3.5.9 Common Variable Pitfalls

<details>
<summary><strong>Case Sensitivity</strong></summary>

All variable names are case-sensitive. `$User` and `$user` are different variables.

**Solution:** Use exact dimension names from the `alarm_variables` API.

</details>

<details>
<summary><strong>Using Non-Existent Dimensions</strong></summary>

Example: `$total` when the chart only has `$used` and `$avail`.

**Solution:** Always query `alarm_variables` before writing alerts to confirm exact dimension names.

</details>

<details>
<summary><strong>Confusing Template Substitutions with Expression Variables</strong></summary>

`${family}` works in `info:` and `summary:` but not in `calc:` or `warn:`.

**Solution:** Use `${...}` syntax only for text field substitutions. Use `$dimension` for numeric calculations.

</details>

<details>
<summary><strong>Forgetting Suffixes for Raw/Timestamp Variables</strong></summary>

Example: Using `$used` when you meant `$used_raw` or `$used_last_collected_t`.

**Solution:** Remember the three forms: `$dimension`, `$dimension_raw`, `$dimension_last_collected_t`.

</details>

For systematic alert troubleshooting, see **Chapter 7: Troubleshooting Alert Behaviour**.

## Key takeaway

When writing alerts, always use the `alarm_variables` API to discover exact variable names. This eliminates guesswork and prevents common naming errors.

## What's Next

- **3.6 Optional Metadata: class, type, component, and labels** explains how to classify and organize alerts for filtering and UI display
- **Chapter 4: Controlling Alerts and Noise** covers disabling, silencing, and reducing alert noise
- **9.3 Inspect Alert Variables (alarm_variables API)** provides detailed API documentation with advanced filtering and debugging techniques
- **8.1 Hysteresis and Status-Based Conditions** shows advanced patterns using `$status` and status constants for complex alert logic