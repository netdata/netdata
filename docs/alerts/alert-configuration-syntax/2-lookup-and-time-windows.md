# 3.2 Lookup and Time Windows

The `lookup` line defines what data an alert evaluates: which chart dimensions to read, over what time window, and with which aggregation function.

:::tip

Refer to this section when you're:
- Reading an alert and want to know what value `$this` actually represents
- Designing a new alert and need to choose the right window and function
- Debugging an alert that feels "too noisy" or "too slow to react" (often a lookup/window issue)

:::

## 3.2.1 What the `lookup` Line Does

In most alerts, `lookup` is where you specify:

| Component | Purpose | Examples |
|-----------|---------|----------|
| **Function** | How to aggregate values | `average`, `min`, `max`, `sum`, etc. |
| **Time window** | How far back to look | `-1m`, `-5m`, `-1h`, etc. |
| **Options** | Modifiers for processing | `unaligned`, `percentage`, `absolute` |
| **Dimensions** | Which chart dimensions to include | `used`, `user,system`, `*`, etc. |

Typical form:

```conf
lookup: <function> <time_window> [at <before>] [every <resampling>] [options] of <dimensions>
```

:::note 

The `every` parameter can appear within lookups for data resampling but is **extremely rare** in practice. Most alerts control evaluation frequency using the alert-level `every` line instead.

:::

**Example:**

```conf
lookup: average -5m unaligned of user,system
```

This means:
- Take the `user` and `system` CPU dimensions
- Over the last 5 minutes
- Using a precise sliding window (`unaligned`)
- Compute their average value
- Expose the result as `$this` for `warn` / `crit` expressions

:::tip

**Key idea:** 

`lookup` turns a time series (many raw samples) into a single number (`$this`) that your alert condition can evaluate.

:::

## 3.2.2 Lookup Function Basics

The first token after `lookup:` is the aggregation function. Common functions include:

| Function | What It Does | Example Use Case |
|----------|--------------|------------------|
| `average` | Mean value over the window | CPU utilization, average latency |
| `min` | Minimum value | Track dips (for example, minimum free space) |
| `max` | Maximum value | Peak usage (for example, highest response time) |
| `sum` | Sum of values | Total operations, total bytes, total errors |
| `median` | Middle value (robust against outliers) | Latency monitoring when spikes occur |
| `percentile95` | 95th percentile value | High-percentile latency, P95/P99 metrics |

**Examples:**

```conf
# Average CPU usage over 10 minutes
lookup: average -10m unaligned of user,system

# Maximum latency over 5 minutes
lookup: max -5m unaligned of latency

# Total HTTP requests in the last minute
lookup: sum -1m unaligned of requests

# 95th percentile latency
lookup: percentile95 -5m unaligned of response_time
```

:::tip

Pick a function that matches the question you're asking:
- "Is this too high most of the time?" → use `average`
- "Did it spike at any point?" → use `max`
- "Are we doing too many operations?" → use `sum`
- "What's the high percentile experience?" → use `percentile95` or `percentile99`

:::

:::tip Method Aliases

Many functions have aliases: `avg`/`mean` for `average`, `ema`/`ewma` for `ses`, `abs` for `absolute`. You may see these in older alert configurations—they work identically to their primary names.

:::

:::note Additional Functions

Netdata supports many additional aggregation methods:

| Category | Functions | Description |
|----------|-----------|-------------|
| **Statistical** | `stddev`, `cv` | Standard deviation, coefficient of variation |
| **Percentiles** | `percentile25`, `percentile50`, `percentile75`, `percentile80`, `percentile90`, `percentile95`, `percentile97`, `percentile98`, `percentile99` | Various percentile calculations |
| **Trimmed aggregations** | `trimmed-mean`, `trimmed-median` | Remove a configurable percentage of outliers (default 5%, adjustable via `group_options`) before aggregating |
| **Smoothing** | `ses`, `des` | Single/double exponential smoothing |
| **Special** | `incremental_sum`, `countif`, `extremes` | Incremental sums, conditional counting, extreme values |

For the complete list with syntax details, refer to Netdata's `REFERENCE.md`.

:::

## 3.2.3 Time Window Syntax and Trade-Offs

The time window is specified as a negative duration:

```conf
-10s   # last 10 seconds
-1m    # last 1 minute
-5m    # last 5 minutes
-30m   # last 30 minutes
-1h    # last 1 hour
```

**How to choose a window:**

<details>
<summary><strong>Short windows (for example, `-10s`, `-1m`)</strong></summary>

- React quickly to changes
- More sensitive to noise or brief spikes
- Good for incident detection where you care about fast response

**Example:**

```conf
# Fast-reacting CPU alert
lookup: average -1m unaligned of user,system
```

</details>

<details>
<summary><strong>Longer windows (for example, `-10m`, `-30m`, `-1h`)</strong></summary>

- Smooth out short-lived spikes
- React more slowly
- Good for trend/capacity style alerts

**Example:**

```conf
# Smoother capacity-style disk alert
lookup: average -30m unaligned of used
```

See **6.5 Trend and Capacity Alerts** for concrete patterns that combine longer windows with appropriate thresholds.

</details>

### Historical Comparisons with `at`

You can specify a time offset for the lookup window using `at`:

```conf
lookup: <function> <time_window> at <before> [options] of <dimensions>
```

This is useful for comparing current values to historical baselines.

**Examples:**

```conf
# Disk fill rate - compare current space to 50 minutes ago
lookup: min -10m at -50m unaligned of avail
# Combined with calc: ($this - $avail) / (($now - $after) / 3600)
# Calculates space decrease rate per hour

# Traffic baseline - 5 minutes starting 5 minutes ago
lookup: average -5m at -5m unaligned of requests
# Compare to current 5-minute window for traffic changes

# Cluster size change detection
lookup: max -2m at -1m unaligned of cluster_size
# Baseline from 1-3 minutes ago
```

The `at` syntax enables rate-of-change calculations and historical comparisons critical for capacity planning alerts.

## 3.2.4 Critical Lookup Options

Options modify how the lookup processes data. **Always include `unaligned`** and add others as needed.

### The `unaligned` Option (Always Use This)

**The `unaligned` keyword appears in 95% of Netdata's stock alerts** and should be your default choice.

**Without `unaligned`:**
- The engine may align windows to internal boundaries (for example, whole minutes)
- This can cause sampling artifacts and inconsistent behavior

**With `unaligned`:**
- The window slides exactly relative to `now`
- Each evaluation uses "the last 5 minutes from right now," not rounded to a boundary

```conf
# ✅ Recommended (standard pattern)
lookup: average -10m unaligned of user,system

# ❌ Avoid (can cause time-boundary artifacts)
lookup: average -10m of user,system
```

**Recommendation: Always include `unaligned` unless you explicitly need aligned windows.**

### The `percentage` Option

Calculates selected dimensions as a percentage of the total across **all dimensions on the chart** (not just selected ones).

**How it works:**
- Selected dimensions are summed
- All chart dimensions are summed (total)
- Result = (selected_sum / total_sum) × 100

**Example:**

```conf
# Calculate success rate automatically
lookup: average -1m unaligned percentage of success
# If chart has dimensions: success=950, failures=50
# Calculation: (950 / 1000) × 100 = 95%
# Result: $this = 95
```

**Common use cases:**
- Success/failure rates (HTTP checks, database transactions)
- Resource usage percentages
- Error rates (errors / total_operations × 100)

### The `absolute` Option

Converts all values to positive (absolute value). Essential for metrics that can be negative.

**Common use cases:**
- Network traffic (sent/received can have negative deltas in some collectors)
- Ensuring error counts are always positive
- Rate calculations that might produce negative values

**Example:**

```conf
# Network bandwidth monitoring
lookup: average -1m unaligned absolute of received
# Ensures positive values even if collector reports negative deltas

# Error counting
lookup: sum -10m unaligned absolute of errors
# Guarantees positive error counts
```

### Combining Multiple Options

Options can be combined in a single lookup:

```conf
# Network monitoring: sliding window + absolute values
lookup: average -1m unaligned absolute of received

# Service health: sliding window + percentage calculation
lookup: average -1m unaligned percentage of success

# QoS monitoring: sum + sliding window + absolute
lookup: sum -5m unaligned absolute of dropped_packets
```

:::note Additional Options

Other options include `null2zero` (convert missing values to zero), `anomaly-bit` (for ML-based detection), `match-names` (dimension matching by name), and more. See Netdata's `REFERENCE.md` for documented options, or consult stock alert examples in `/usr/lib/netdata/conf.d/health.d/` for advanced patterns.

:::

## 3.2.5 Selecting Dimensions

The `of` clause at the end of the lookup controls which dimensions from the chart are included in the computation.

Common patterns:

```conf
# All dimensions on the chart
lookup: average -5m unaligned of *

# A single named dimension
lookup: max -1m unaligned of used

# Multiple specific dimensions
lookup: average -10m unaligned of user,system,softirq,irq
```

How Netdata handles the dimensions depends on the chart and function. For most stock alerts:
- Multiple dimensions are combined in a way that makes sense for that chart (for example, summing CPU states)
- The combined result is then fed into the aggregation function and exposed as `$this`

If you're not sure which dimensions a chart has:
- Open the chart in the Netdata dashboard and inspect its legend, or
- Use the chart metadata APIs (see **9.1** and **9.3** for details)

:::tip

Start by copying dimension selections from stock alerts that monitor the same chart. They usually already reflect the "right" combination of dimensions for that metric.

:::

## 3.2.6 Common Lookup Patterns

Real-world examples from Netdata's stock alerts:

<details>
<summary><strong>Pattern 1: Standard infrastructure monitoring</strong></summary>

```conf
lookup: average -10m unaligned of user,system
# Used for: CPU, disk I/O, containers, cgroups
```

</details>

<details>
<summary><strong>Pattern 2: Network bandwidth</strong></summary>

```conf
lookup: average -1m unaligned absolute of received
# Ensures positive values for rate calculations
```

</details>

<details>
<summary><strong>Pattern 3: Error accumulation</strong></summary>

```conf
lookup: sum -10m unaligned absolute of errors
# Counts total errors over window
```

</details>

<details>
<summary><strong>Pattern 4: Service health percentage</strong></summary>

```conf
lookup: average -1m unaligned percentage of success
# Calculates success rate automatically
```

</details>

<details>
<summary><strong>Pattern 5: Capacity planning (rate of change)</strong></summary>

```conf
lookup: min -10m at -50m unaligned of avail
# Historical baseline for fill-rate calculation
```

</details>

## 3.2.7 Quick Reference Guide

**Choosing the Right Lookup Pattern:**

| Use Case | Function | Window | Options | Example |
|----------|----------|--------|---------|---------|
| CPU/Memory utilization | `average` | `-10m` | `unaligned` | `average -10m unaligned of user,system` |
| Network bandwidth | `average` | `-1m` | `unaligned absolute` | `average -1m unaligned absolute of received` |
| Error accumulation | `sum` | `-10m` | `unaligned absolute` | `sum -10m unaligned absolute of errors` |
| Service success rate | `average` | `-1m` | `unaligned percentage` | `average -1m unaligned percentage of success` |
| Disk fill rate | `min` | `-10m at -50m` | `unaligned` | `min -10m at -50m unaligned of avail` |
| Peak latency (P95) | `percentile95` | `-5m` | `unaligned` | `percentile95 -5m unaligned of latency` |
| Smoothed trends | `ses` | `-30m` | `unaligned` | `ses -30m unaligned of requests` |

**Default recommendation:** Start with `average -10m unaligned` and adjust based on metric behavior.

## 3.2.8 How Lookup Interacts with Storage (dbengine)

Under the hood, Netdata's dbengine stores high-resolution metrics and serves them to the health engine when it evaluates lookups.

Practical implications:

<details>
<summary><strong>Short windows (seconds to minutes)</strong></summary>

- Data is often still in memory
- Evaluations are very fast, even with many alerts

</details>

<details>
<summary><strong>Long windows (tens of minutes to hours)</strong></summary>

- dbengine may need to read more data from disk
- Evaluations can be more expensive, especially on high-cardinality charts

</details>

**Guidelines:**

- Don't be afraid of moderate windows (for example, `-5m`, `-10m`, `-30m`). Netdata is designed to handle them.
- For very large fleets or extremely long windows, review **8.5 Performance Considerations for Large Alert Sets** and consider:
  - Using slightly shorter windows where acceptable
  - Reducing evaluation frequency (`every`) for heavy alerts

You normally don't need to "tune for dbengine" explicitly. Just be aware that an alert with:

```conf
lookup: average -1h unaligned of *
```

on thousands of charts will cost more than one with:

```conf
lookup: average -5m unaligned of *
```

on a handful.

## Key takeaway

`lookup` turns raw time-series data into the single value `$this` that your alert expressions evaluate. Choosing the right function, window, and options is often the difference between a noisy alert and a useful one.

## What's Next

- **[3.3 Calculations and Transformations (`calc`, `absolute`, `percentage`, `anomaly-bit`)](3-calculations-and-transformations.md)** explains how to transform lookup results (or build alerts without explicit lookups) using `calc` and related flags
- **[6.5 Trend and Capacity Alerts](../alert-examples/5-trend-capacity.md)** gives concrete examples of how different time windows change alert behavior in practice
- **[9.3 Querying Alert Variables and Metadata](../apis-alerts-events/3-inspect-variables.md)** for using the `alarm_variables` API to inspect available dimensions when building lookups