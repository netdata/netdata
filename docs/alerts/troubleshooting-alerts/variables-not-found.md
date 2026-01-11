# 7.4 Variables or Metrics Not Found

The alert fails to load or shows errors about missing variables.

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

- **3.5 Variables and Special Symbols** for variable reference
- **9.3 Inspect Alert Variables** for API usage