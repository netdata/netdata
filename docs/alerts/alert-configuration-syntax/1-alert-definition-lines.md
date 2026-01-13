# 3.1 Alert Definition Lines (Minimal and Full Forms)

This section explains the structure of an alert block in `health.d` files and what each primary line does.

:::tip

Refer to this section when you're looking at a stock alert and want to understand each line, writing or editing custom alerts in `/etc/netdata/health.d/`, or deciding which lines are required versus optional for a given rule.

:::

## 3.1.1 Basic Alert Block Structure

An alert is defined as a block of lines in a `*.conf` file under:

- Stock alerts: `/usr/lib/netdata/conf.d/health.d/`
- Custom alerts: `/etc/netdata/health.d/`

There are two types of blocks:
- `alarm` (chart-specific rule)
- `template` (context-based rule that can apply to multiple charts)

For the conceptual difference, see 1.2 Alert Types: alarm versus template.

A very simple `alarm` might look like this:

```conf
alarm: 10min_cpu_usage
    on: system.cpu
lookup: average -5m of user,system
 every: 1m
  warn: $this > 80
  crit: $this > 95
   to: sysadmin
```

Even in this minimal example you can see the main pieces:
- **Identity**: `alarm` name
- **Target**: `on` chart (or context for templates)
- **Data selection**: `lookup` and `every`
- **Conditions**: `warn` and `crit`
- **Routing/metadata**: `to` (and optionally `info`, `class`, `type`, etc.)

The next subsections break down which lines are essential and which are optional but common.

## 3.1.2 Minimal Definition: Essential Lines

Most practical alerts are built from the same core lines. A minimal, functional definition usually includes:

| Line | Required? | Purpose |
|------|-----------|---------|
| `alarm` / `template` | Yes | Names the rule so it can be referenced and managed |
| `on` | Yes | Chooses what to monitor: a chart (for `alarm`) or context (for `template`) |
| `lookup` or `calc` | Yes (at least one) | `lookup` reads and aggregates metrics; `calc` transforms values or references variables |
| `every` | Yes | Sets how often the health engine evaluates the rule |
| `warn` and/or `crit` | Yes (at least one) | Boolean expressions that decide when the alert changes status |
| `to` | Strongly recommended | Tells Netdata who should receive notifications (or `silent`) |

### Example: Minimal template

```conf
template: 10min_cpu_usage
      on: system.cpu
  lookup: average -10m of user,system,softirq,irq
   every: 1m
    warn: $this > 80
    crit: $this > 95
      to: sysadmin
```

Key ideas:
- `template:` suggests this rule can apply to multiple charts with the `system.cpu` context
- `lookup` + `every` define what data is evaluated and how frequently
- `warn` and `crit` are simple comparisons on `$this` (the value from `lookup`/`calc`)

Details about each of these core lines are expanded in 3.2 Lookup and Time Windows (for `lookup` and how `$this` is computed), 3.4 Expressions, Operators, and Functions (for `warn` / `crit` expressions), and 3.5 Variables and Special Symbols (for `$this`, `$status`, etc.).

## 3.1.3 Common Optional Lines in a Full Definition

Real-world alerts often use additional lines for clarity, routing, flapping control, or integration with automation.

The table below summarizes the most common optional lines you'll see in stock and custom alerts.

| Line | Category | Purpose / Typical Use |
|------|----------|------------------------|
| `info` | Description | Human-readable description explaining what the alert monitors and why it matters. Shown in UIs and notifications. |
| `summary` | Description | Short summary/title for the alert, often used in condensed views. |
| `calc` | Data transformation | Expression to transform `lookup` results or reference chart variables directly (see **3.3**). |
| `green` / `red` | Visualization | Static threshold lines shown on charts (green = healthy range, red = critical threshold). |
| `delay` | Flapping control | Defines how long a condition must hold before changing status (for example, delay entering WARNING/CRITICAL or CLEAR). See **4.4**. |
| `repeat` | Notifications | Controls how often notifications are sent while a condition remains active, to avoid spam. |
| `options` | Behavior flags | Modifies alert behavior (for example, `no-clear-notification`). |
| `exec` | Automation | Runs a script or command when the alert changes status (for integrations or custom actions). See **8.4**. |
| `class` | Metadata | High-level category (for example, `system`, `application`, `network`) used for grouping and filtering. |
| `type` | Metadata | Type of issue (for example, `utilization`, `availability`, `latency`). |
| `component` | Metadata | Component or subsystem name (for example, `cpu`, `disk`, `mysql`). |

A more complete alert might look like this:

```conf
alarm: disk_space_usage
    on: disk_space._
lookup: average -10m of used
 every: 1m
  warn: $this > 80
  crit: $this > 90
  info: Disk space usage over the last 10 minutes
summary: Disk space critically low
 delay: up 5m down 0
repeat: 30m
    to: sysadmin
 class: system
  type: capacity
component: disk
```

Here you can see:
- **Core logic**: `alarm`, `on`, `lookup`, `every`, `warn`, `crit`, `to`
- **Presentation**: `info`, `summary`
- **Behavior control**: `delay`, `repeat`
- **Metadata**: `class`, `type`, `component`

The syntax details of these supporting lines are covered in **4.4 Reducing Flapping and Noise** for practical use of `delay`/`repeat`, **3.6 Optional Metadata: class, type, component** for how to choose and standardize metadata, and **8.4 Custom Actions with exec and Automation** for how `exec` works and what environment variables are available.

## 3.1.4 Where to Find the Full Line List

Netdata's built-in `REFERENCE.md` includes an extensive list of all supported lines and options for alert definitions.

:::note

If you encounter a line not covered here, check `REFERENCE.md` in your Netdata installation for the definitive description, or search stock alert files in `/usr/lib/netdata/conf.d/health.d/` to see how Netdata itself uses it.

:::

**Key takeaway:** 

Most alerts follow the same pattern: name it, point it at a chart or context, define how to read the data, set thresholds, and route notifications. Everything else is refinement.

## What's Next

- **3.2 Lookup and Time Windows** explains how `lookup` chooses functions, time windows, and dimensions, and how that affects the values your alerts see
- **3.5 Variables and Special Symbols** covers `$this`, `$status`, and other variables used in `warn`/`crit` (jump ahead there if that's your main question and come back as needed)