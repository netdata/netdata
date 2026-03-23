# Time Periods

Time periods control **when** a Nagios check job is allowed to run. Each time period is a named schedule with one or more allow rules.

- Outside the active time period, the check does not execute and its state becomes `paused`.
- The built-in `24x7` period (always allowed) is the default `check_period`.
- Set the `check_period` option in a job to reference a named time period.
- Define custom time periods using the `time_periods` option within the same job.

## Rule Types

| Type          | Description                            | Key Fields                 |
|:--------------|:---------------------------------------|:---------------------------|
| `weekly`      | Repeats on specific weekdays           | `days`, `ranges`           |
| `nth_weekday` | Nth occurrence of a weekday in a month | `weekday`, `nth`, `ranges` |
| `date`        | Specific calendar dates                | `dates`, `ranges`          |

### Fields

- **`days`** — List of weekday names: `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`, `sunday`
- **`ranges`** — List of time ranges in `HH:MM-HH:MM` format (e.g., `"09:00-18:00"`). Use `"00:00-24:00"` for all day.
- **`weekday`** — Single weekday name (for `nth_weekday` rules)
- **`nth`** — Which occurrence of the weekday in the month (1 = first, 2 = second, etc.)
- **`dates`** — List of calendar dates in `YYYY-MM-DD` format
- **`exclude`** — List of other time period names to subtract from this period

## Examples

### Business hours (Mon–Fri, 09:00–18:00)

```yaml
time_periods:
  - name: business_hours
    alias: Business Hours
    rules:
      - type: weekly
        days: [monday, tuesday, wednesday, thursday, friday]
        ranges: ["09:00-18:00"]
```

### First Monday maintenance window each month

```yaml
time_periods:
  - name: first_monday_maintenance
    alias: First Monday Maint
    rules:
      - type: nth_weekday
        weekday: monday
        nth: 1
        ranges: ["02:00-04:00"]
```

### Holiday blackout by specific dates

```yaml
time_periods:
  - name: holidays
    alias: Holiday Blackout
    rules:
      - type: date
        dates: ["2026-12-25", "2026-12-31"]
        ranges: ["00:00-24:00"]
```

### Always allowed, except during maintenance and holidays

```yaml
time_periods:
  - name: run_checks
    alias: Run Checks
    rules:
      - type: weekly
        days: [sunday, monday, tuesday, wednesday, thursday, friday, saturday]
        ranges: ["00:00-24:00"]
    exclude: [first_monday_maintenance, holidays]
```

## Notes

- Date format is strict `YYYY-MM-DD`.
- Time range format is strict `HH:MM-HH:MM`. `24:00` is valid only as an end boundary.
- The built-in `24x7` period is always available and is the default `check_period`.
