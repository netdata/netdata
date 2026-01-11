# 3. Alert Configuration Syntax (Reference)

This chapter provides a precise reference for alert configuration syntax.

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **3.1 Alert Definition Lines** | alarm, template, on, lookup, every, warn, crit, exec, and more |
| **3.2 Lookup and Time Windows** | Functions like average/min/max/sum, time windows, dimension selection |
| **3.3 Calculations and Transformations** | calc expressions, absolute, percentage, anomaly-bit flags |
| **3.4 Expressions, Operators, and Functions** | Arithmetic, comparisons, logical operators, helper functions |
| **3.5 Variables and Special Symbols** | $this, $status, $now, chart/dimension variables, context variables |
| **3.6 Optional Metadata** | class, type, component, tags for grouping and filtering |

## How to Navigate This Chapter

- Use as a **reference** while writing alerts
- Start at **3.1** for definition structure
- Go to **3.2** for lookup options
- See **3.5** when debugging variable issues

## Key Concepts

- Each alert needs: `alarm`/`template`, `on`, `lookup`, `every`, `warn`, `crit`
- Optional lines: `delay`, `exec`, `to`, `summary`, `info`
- Use `calc` to transform lookup results
- Variables like `$this` and `$status` provide dynamic values

## What's Next

- **Chapter 4**: Controlling Alerts and Noise
- **Practical Application**: Creating your first alert

See also: [Creating Alerts](../creating-alerts-pages/creating-alerts.md)