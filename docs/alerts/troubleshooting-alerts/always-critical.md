# 7.2 Alert Always Critical or Warning

The alert never returns to `CLEAR`. This typically indicates threshold issues, calculation errors, or variable problems.

## 7.2.1 Common Causes

| Cause | Example |
|-------|---------|
| Threshold unit mismatch | Alert checks `> 80` but metric is in KB/s |
| Calculation error | `calc` expression always true |
| Variable typo | `$thiss` instead of `$this` |

## 7.2.2 Diagnostic Steps

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name.value'
```

Check if the value actually crosses the threshold.

## 7.2.3 Fixing Calculation Errors

```conf
# WRONG: Division by zero possible
calc: $this / ($var - $var2)

# RIGHT: Handle edge cases
calc: ($this / ($var - $var2)) * ($var > $var2 ? 1 : 0)
```

See **[3.3 Calculations and Transformations](../alert-configuration-syntax/3-calculations-and-transformations.md)** for expression details.

## 7.2.4 Related Sections

- **[7.1 Alert Never Triggers](alert-never-triggers.md)** for complementary diagnosis
- **[7.3 Alert Flapping](flapping.md)** for rapid status changes