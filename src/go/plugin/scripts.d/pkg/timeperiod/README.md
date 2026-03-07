# timeperiod

`timeperiod` compiles scripts.d schedule definitions into runtime period predicates used by the Nagios collector.

## What are "Nagios-style scheduling periods"?

In this plugin, a time period is a named allow-window that says **when a check may run**.

- A period has one or more allow rules (`weekly`, `nth_weekday`, `date`).
- A period can exclude other named periods (`exclude`).
- `Allows(t)` answers: "is this timestamp allowed?"
- `NextAllowed(t)` finds the next allowed timestamp.

This is "Nagios-style" because checks are gated by named time periods and exclusions, instead of only a single fixed interval.

## What this package does

- Defines schedule config types (`Config`, `RuleConfig`) for YAML/JSON.
- Compiles raw config into a resolved set of named periods (`Compile`, `Set.Resolve`).
- Evaluates whether a timestamp is allowed (`Period.Allows`).
- Finds the next allowed execution slot (`Period.NextAllowed`).
- Ensures the builtin always-on period exists (`EnsureDefault`).

## Supported rule types

- `weekly`: weekdays + time ranges (`HH:MM-HH:MM`)
- `nth_weekday`: Nth weekday in month (`weekday` + `nth`) + time ranges
- `date`: explicit calendar dates (`YYYY-MM-DD`) + time ranges

## Config examples

### 1) Business-hours checks (Mon-Fri, 09:00-18:00)

```yaml
- name: business_hours
  alias: Business Hours
  rules:
    - type: weekly
      days: [monday, tuesday, wednesday, thursday, friday]
      ranges: ["09:00-18:00"]
```

### 2) First Monday maintenance window each month

```yaml
- name: first_monday_maintenance
  alias: First Monday Maint
  rules:
    - type: nth_weekday
      weekday: monday
      nth: 1
      ranges: ["02:00-04:00"]
```

### 3) Holiday blackout by specific dates

```yaml
- name: holidays
  alias: Holiday Blackout
  rules:
    - type: date
      dates: ["2026-12-25", "2026-12-31"]
      ranges: ["00:00-24:00"]
```

### 4) Allow always, except maintenance/holidays

```yaml
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
- Range format is strict `HH:MM-HH:MM` (`24:00` is valid only as an end boundary).
- The package is scheduler-facing infrastructure and should remain generic (no parser/output domain logic).
- `DefaultPeriodName` / `DefaultPeriodConfig` define the implicit `24x7` fallback period.
