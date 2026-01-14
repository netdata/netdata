# 3.3 Calculations and Transformations (`calc`)

The `lookup` line (3.2) decides which raw data an alert sees. This section explains how to transform that data before comparing it to thresholds, and how to build alerts that work directly on derived values such as percentages or ratios.

:::tip

Refer to this section when you're:
- Converting units (for example, bytes to MB, nanoseconds to ms)
- Building percentages or ratios from multiple values
- Using chart variables (like `$total`) instead of just the raw lookup result
- Integrating ML-based anomaly information into alerts

:::

## 3.3.1 How `lookup`, `calc`, and `$this` Fit Together

In most alerts, the evaluation pipeline is:

| Step | Component | What It Does |
|------|-----------|--------------|
| **1** | `lookup` | Reads and aggregates data from a chart over a time window |
| **2** | `calc` (optional) | Applies a mathematical expression on top of that value and other variables |
| **3** | `$this` | The final result exposed for use in `warn`/`crit` expressions |

<details>
<summary><strong>Example: Simple pipeline with unit conversion</strong></summary>

```conf
# Convert bytes to megabytes
lookup: average -5m unaligned of used
  calc: $this / (1024 * 1024)
 units: MB
  warn: $this > 800  # More than 800 MB
  crit: $this > 900  # More than 900 MB
```

**Pipeline:**
1. `lookup` produces bytes used (e.g., 838,860,800 bytes)
2. `calc` converts to MB (838,860,800 / 1,048,576 = 800 MB)
3. `$this` now equals 800, compared against thresholds

</details>

<details>
<summary><strong>Example: Pipeline with percentage calculation</strong></summary>

```conf
lookup: average -5m unaligned of used
  calc: $used * 100 / ($avail + $used)
 units: %
  warn: $this > 80
  crit: $this > 95
```

**Pipeline:**
1. `lookup` reads the `used` dimension
2. `calc` computes percentage using chart variables `$used` and `$avail`
3. `$this` contains the percentage value for comparison

</details>

:::tip

**Key idea:** 

Use `lookup` to describe what data you read and `calc` to describe how you want to see it (units, ratios, combinations).

:::

## 3.3.2 Using `calc` for Unit Conversions

Many stock alerts use `calc` to convert raw values into more readable units.

**Example: bytes to megabytes**

```conf
lookup: average -5m unaligned of used
  calc: $this / (1024 * 1024)
 units: MB
  warn: $this > 800
  crit: $this > 900
```

**Example: kilobytes to megabytes with percentage**

```conf
# From swap.conf
lookup: sum -30m unaligned absolute of out
  calc: $this / 1024 * 100 / ( $system.ram.used + $system.ram.cached + $system.ram.free )
 units: % of RAM
  warn: $this > 200
  crit: $this > 400
```

**Guidelines**:

- Keep unit conversions in `calc` so that thresholds in `warn`/`crit` are easy to read and reason about.
- Always add a `units:` line to document the result unit.
- If you see thresholds that look oddly large (for example, `warn: $this > 1000000000`), check if a missing `calc` is the reason.

## 3.3.3 Building Percentages and Ratios

`calc` is also where you build percentages, ratios, or normalized values from chart variables.

**Example: disk usage as a percentage of total**

```conf
# From disks.conf
lookup: average -5m unaligned of used
  calc: $used * 100 / ($avail + $used)
 units: %
  warn: $this > 80
  crit: $this > 95
```

Here:
- `$used` and `$avail` are chart variables
- `calc` computes used space as a percentage
- `warn` and `crit` compare against percent thresholds

**Example: safe percentage calculation with division-by-zero protection**

```conf
# From swap.conf
calc: (($used + $free) > 0) ? ($used * 100 / ($used + $free)) : 0
```

The conditional operator `(condition) ? (true_expr) : (false_expr)` prevents division by zero.

:::tip Two Ways to Calculate Percentages

**Method 1: calc-based (manual)** — Use when you need custom logic or work with chart variables:
```conf
calc: $used * 100 / ($used + $avail)
```

**Method 2: percentage lookup option (automatic)** — Use when you want selected dimensions as % of total (see **3.2.4**):
```conf
lookup: average -5m unaligned percentage of success
```

Choose the `percentage` option when it matches your use case—it's simpler and less error-prone.
:::

For details on which variables are available for specific charts (such as `$total`, `$avail`, `$used`), see **3.5 Variables and Special Symbols** and use the `alarm_variables` API (**9.3**) to inspect a chart's variables.

## 3.3.4 Alerts Built Primarily with `calc`

Some alerts don't need an explicit `lookup` if they work entirely on chart variables or derived expressions. In those cases, you might see:

```conf
calc: <expression using variables>
warn: <condition on $this or variables>
crit: <condition on $this or variables>
```

or a combination of `lookup` and more complex `calc`.

**Example: working directly with chart variables**

```conf
# From cockroachdb.conf
template: cockroachdb_used_storage_capacity
      on: cockroachdb.storage_used_capacity_percentage
    calc: $total
   units: %
    warn: $this > 60
    crit: $this > 80
```

**Example: combining multiple variables**

```conf
lookup: average -5m unaligned of *
  calc: ($read_latency + $write_latency) / 2
 units: ms
  warn: $this > 100
  crit: $this > 200
```

In this pattern:
- `lookup` ensures that underlying chart data is up-to-date
- `calc` combines multiple variables into one composite measure

## 3.3.5 Common `calc` Patterns from Stock Alerts

Real-world patterns you'll encounter:

<details>
<summary><strong>Rate of Change Calculation</strong></summary>

```conf
# From disks.conf - disk fill rate
template: disk_fill_rate
      on: disk.space
  lookup: min -10m at -50m unaligned of avail
    calc: ($this - $avail) / (($now - $after) / 3600)
   units: GB/hour
    warn: $this > 0
    crit: $this > 0
```

**What's happening:**
- `lookup` gets historical baseline (`$this` from 50 minutes ago)
- `calc` compares to current value (`$avail`)
- Time difference calculated from `$after` and `$now` variables
- Result: rate of change per hour

</details>

<details>
<summary><strong>Conditional with Infinity</strong></summary>

```conf
# From disks.conf - hours until disk full
calc: ($disk_fill_rate > 0) ? ($avail / $disk_fill_rate) : (inf)
```

Uses `inf` (infinity) to indicate "never fills up" when fill rate is zero or negative.

</details>

<details>
<summary><strong>Division-by-Zero Protection</strong></summary>

```conf
# From swap.conf - safe percentage calculation
calc: (($used + $free) > 0) ? ($used * 100 / ($used + $free)) : 0
```

Checks denominator before dividing to avoid undefined results.

</details>

<details>
<summary><strong>Absolute Value Using abs() Function</strong></summary>

```conf
# Treat negative and positive deviations equally
lookup: average -5m unaligned of deviation
  calc: abs($this)
  warn: $this > 10
  crit: $this > 20
```

The `abs()` function returns the absolute value. Both `+15` and `-15` deviation are treated as `15`.

**Note:** You can also use the `absolute` lookup option (see **3.2.4**):
```conf
lookup: average -5m unaligned absolute of deviation
```

</details>

## 3.3.6 Expression Syntax Quick Reference

The `calc` expression syntax supports:

| Category | Syntax | Examples |
|----------|--------|----------|
| **Arithmetic** | `+`, `-`, `*`, `/`, `%` (modulo) | `$this * 100`, `$used / $total` |
| **Comparisons** | `>`, `<`, `>=`, `<=`, `==`, `!=` | `$this > 80`, `$status == $WARNING` |
| **Logical** | `AND`, `OR`, `NOT` (also `&&`, `\|\|`, `!`) | `$this > 80 AND $status < $CRITICAL` |
| **Conditional** | `(condition) ? (true) : (false)` | `($total > 0) ? ($used / $total) : 0` |
| **Functions** | `abs()` | `abs($deviation)` |
| **Special values** | `inf`, `nan` | `inf` (infinity), `nan` (not a number) |

For complete expression syntax and operator precedence, see **3.4 Expressions, Operators, and Functions**.

## 3.3.7 Anomaly-Based Transformations and `anomaly-bit`

Netdata's machine learning (ML) engine can flag anomalous behavior in metrics. You can use this information in alerts via:

- Anomaly scores/flags exposed as metrics
- The `anomaly-bit` lookup option that tells the health engine how to interpret those signals

A typical ML-based pattern:
- Use a metric that represents anomaly information
- Optionally use the `anomaly-bit` option in the lookup line
- Evaluate simple conditions such as "if anomalous, raise WARNING/CRITICAL"

**Example (illustrative):**

```conf
# Example from ml.conf (commented in stock alerts)
lookup: average -5m anomaly-bit of *
  warn: $this > 0.5
  crit: $this > 0.8
```

In practice:
- Netdata's ML engine trains on historical data and maintains per-metric anomaly indicators
- The `anomaly-bit` option (or similar) maps those indicators into values you can compare in `warn`/`crit`

:::note

For concrete, current examples:
- See **6.4 Anomaly-Based Alerts** for practical alert patterns using anomaly metrics
- Consult the latest ML documentation on Netdata anomaly detection and your local `REFERENCE.md`

:::

:::tip

Use anomaly-based alerts when:
- Normal behavior varies by time of day/week and static thresholds are hard to tune
- You want to augment, not replace, threshold-based alerts with "this looks unusual" signals

:::

## 3.3.8 Choosing Between `lookup`, `calc`, and Plain Expressions

When designing an alert:

<details>
<summary><strong>Use `lookup` only when</strong></summary>

- Raw metric values are already in the units you want
- You just need to smooth/noise-control via a time window

</details>

<details>
<summary><strong>Use `lookup` + `calc` when</strong></summary>

- You need unit conversions (bytes to MB, KB to GB)
- You want to build percentages/ratios or combine variables
- You need rate-of-change calculations

</details>

<details>
<summary><strong>Use `calc`-heavy alerts when</strong></summary>

- The interesting signal is a derived expression of several variables
- The chart already exposes everything you need as variables
- You're working with existing calculated metrics

</details>

In all cases, your `warn`/`crit` lines should ideally be:
- Simple, readable comparisons on `$this` or a small number of variables
- Free of complex math (keep that in `calc` where possible)

## Key takeaway

Use `calc` to keep `warn`/`crit` expressions simple and human-readable, while still capturing complex relationships and derived metrics behind the scenes.

## What's Next

- **[3.4 Expressions, Operators, and Functions](4-expressions-operators-functions.md)** explains the expression language used in `warn`, `crit`, and `calc` lines: arithmetic, comparisons, logical operators, and helper functions
- **[3.5 Variables and Special Symbols](5-variables-and-special-symbols.md)** lists the variables you can use inside those expressions (`$this`, `$status`, `$now`, and chart/context variables) and how to inspect them via the `alarm_variables` API