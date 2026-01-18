# 7.4 Variables or Metrics Not Found

Configuration errors referencing non-existent variables or dimensions prevent alerts from loading.

## 7.4.1 Debugging Variables

```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu"
```

This shows all available variables for a chart.

## 7.4.2 Common Variable Errors

| Error | Fix |
|-------|-----|
| `unknown variable 'thiss'` | Correct to `$this` |
| `no dimension 'usr'` | Use actual dimension name from API |
| `$this is NaN` | Check data availability |

## 7.4.3 Related Sections

- **[3.5 Variables and Special Symbols](../alert-configuration-syntax/5-variables-and-special-symbols.md)** for variable reference
- **[9.3 Inspect Alert Variables](../apis-alerts-events/3-inspect-variables.md)** for API usage