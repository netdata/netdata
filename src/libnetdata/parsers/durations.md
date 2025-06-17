## Durations in Netdata

Netdata provides a flexible and powerful way to specify durations for various configurations and operations, such as alerts, database retention, and other configuration options. Durations can be expressed in a variety of units, ranging from nanoseconds to years, allowing users to define time intervals in a human-readable format.

### Supported Duration Units

Netdata supports a wide range of duration units. The system follows the Unified Code for Units of Measure (UCUM) standard where applicable. Below is a table of all the supported units, their corresponding representations, and their compatibility:

| Symbol | Description  |  Value   | Compatibility | Formatter | Full Names |
|:------:|:------------:|:--------:|:-------------:|:---------:|:----------:|
|  `ns`  | Nanoseconds  |  `1ns`   |     UCUM      |  **Yes**  | nanosecond, nanoseconds |
|  `us`  | Microseconds | `1000ns` |     UCUM      |  **Yes**  | microsecond, microseconds |
|  `ms`  | Milliseconds | `1000us` |     UCUM      |  **Yes**  | millisecond, milliseconds |
|  `s`   |   Seconds    | `1000ms` |     UCUM      |  **Yes**  | sec, secs, second, seconds |
|  `m`   |   Minutes    |  `60s`   |    Natural    |  **Yes**  | minute, minutes |
| `min`  |   Minutes    |  `60s`   |     UCUM      |    No     | minute, minutes |
|  `h`   |    Hours     |  `60m`   |     UCUM      |  **Yes**  | hr, hrs, hour, hours |
|  `d`   |     Days     |  `24h`   |     UCUM      |  **Yes**  | day, days |
|  `w`   |    Weeks     |   `7d`   |    Natural    |    No     | week, weeks |
|  `wk`  |    Weeks     |   `7d`   |     UCUM      |    No     | week, weeks |
|  `mo`  |    Months    |  `30d`   |     UCUM      |  **Yes**  | month, months |
|  `M`   |    Months    |  `30d`   |   Backwards   |    No     | month, months |
|  `q`   |   Quarters   |  `3mo`   |    Natural    |    No     | quarter, quarters |
|  `y`   |    Years     |  `365d`  |    Natural    |  **Yes**  | year, years |
|  `Y`   |    Years     |  `365d`  |   Backwards   |    No     | year, years |
|  `a`   |    Years     |  `365d`  |     UCUM      |    No     | year, years |

- **UCUM**: The unit is specified in the Unified Code for Units of Measure (UCUM) standard.
- **Natural**: We feel that this is more natural for expressing durations with single letter units.
- **Backwards**: This unit has been used in the past in Netdata, and we support it for backwards compatibility.
- **Full Names**: All full unit names are supported in addition to the abbreviated forms and are case-insensitive.

### Duration Expression Format

Netdata allows users to express durations in both simple and complex formats.

- **Simple Formats**: A duration can be specified using a number followed by a unit, such as `5m` (5 minutes), `2h` (2 hours), or `1d` (1 day). Fractional numbers are also supported, such as `1.5d`, `3.5mo` or `1.2y`.

- **Full Unit Names**: In addition to abbreviated units, Netdata also supports full unit names for better readability. For example:
    - `7 days` or `7days` (equivalent to `7d`)
    - `2 hours` or `2hours` (equivalent to `2h`)
    - `30 seconds` or `30seconds` (equivalent to `30s`)
    - All unit names are case-insensitive: `7 DAYS`, `7 Days`, `7 days` are all valid

- **Complex Formats**: A duration can also be composed of multiple units added together. For example:
    - `1y2mo3w4d` represents 1 year, 2 months, 3 weeks, and 4 days.
    - `15d-12h` represents 15 days minus 12 hours (which equals 14 days and 12 hours).
    - `2 hours 30 minutes` or `2hours30minutes` (equivalent to `2h30m`)

Each number given in durations can be either positive or negative. For example `1h15m` is 1 hour and 15 minutes, but `1h-15m` results to `45m`.

The same unit can be given multiple times, so that `1d0.5d` is `1d12h` and `1d-0.5d` is `12h`.

The order of units in the expressions is irrelevant, so that `1m2h3d` is the same to `3d2h1m`.

The system will parse durations with spaces in them, but we suggest to write them down in compact form, without spaces. This is required, especially in alerts configuration, since spaces in durations will affect how parent expressions are tokenized.

### Duration Rounding

Netdata provides various functions to parse and round durations according to specific needs:

- **Default Rounding to Seconds**: Most duration uses in Netdata are rounded to the nearest second. For example, a duration of `1.4s` would round to `1s`, while `1.5s` would round to `2s`.

- **Rounding to Larger Units**: In some cases, such as database retention, durations are rounded to larger units like days. Even when rounding to a larger unit, durations can still be expressed in smaller units (e.g., `24h86400s` for `2d`).

### Maximum and Minimum Duration Limits

Netdata's duration expressions can handle durations ranging from the minimum possible value of `-INT64_MAX` to the maximum of `INT64_MAX` in nanoseconds. This range translates approximately to durations between -292 years to +292 years.

### Inconsistencies in Duration Units

While Netdata provides a flexible system for specifying durations, some inconsistencies arise due to the way different units are defined:

- **1 Year (`y`) = 365 Days (`d`)**: In Netdata, a year is defined as 365 days. This is an approximation, since the average year is about 365.25 days.

- **1 Month (`mo`) = 30 Days (`d`)**: Similarly, a month in Netdata is defined as 30 days, which is also an approximation. In reality, months vary in length (28 to 31 days).

- **1 Quarter (`q`) = 3 Months (`mo`) = 90 Days (`d`)**: A quarter is defined as 3 months, or 90 days, which aligns with the approximation of each month being 30 days.

These definitions can lead to some unexpected results when performing arithmetic with durations:

**Example of Inconsistency**:

`1y-1d` in Netdata calculates to `364d` but also as `12mo4d` because `1y = 365d` and `1mo = 30d`. This is inconsistent because `1y` is defined as `12mo5d` or `4q5d` (given the approximations above).

### Negative Durations

When the first letter of a duration expression is the minus character, Netdata parses the entire expression as positive and then it negates the result. for example: `-1m15s` is `-75s`, not `-45s`. To get `-45s` the expression should be `-1m-15s`. So the initial `-` is treated like `-(expression)`.

The same rule is applied when generating duration expressions.

### "Ago" Suffix

Netdata supports the "ago" suffix to express durations in the past. When a duration expression ends with "ago" (case-insensitive), the resulting value is negated. This provides a more natural way to express past time intervals:

- **`7 days ago`** is equivalent to **`-7d`**
- **`2 hours ago`** is equivalent to **`-2h`**
- **`30 minutes ago`** is equivalent to **`-30m`**

The "ago" suffix works with all duration formats:
- Abbreviated: `7d ago`, `2h ago`, `30m ago`
- Full names: `7 days ago`, `2 hours ago`, `30 minutes ago`
- Complex expressions: `2 hours 30 minutes ago`, `1d12h ago`
- Case variations: `7 days AGO`, `7 days Ago`, `7daysago`

Note: If you use a negative duration with "ago", it results in a positive value: `-7 days ago` = 7 days (double negative).

### Example Duration Expressions

Here are some examples of valid duration expressions:

1. **`30s`** or **`30 seconds`**: 30 seconds.
2. **`5m`** or **`5 minutes`**: 5 minutes.
3. **`2h30m`** or **`2 hours 30 minutes`**: 2 hours and 30 minutes.
4. **`1.5d`** or **`1.5 days`**: 1 day and 12 hours.
5. **`1w3d4h`** or **`1 week 3 days 4 hours`**: 1 week, 3 days, and 4 hours.
6. **`1y2mo3d`** or **`1 year 2 months 3 days`**: 1 year, 2 months, and 3 days.
7. **`15d-12h`** or **`15 days -12 hours`**: 14 days and 12 hours.
8. **`7Days`** (case-insensitive): 7 days.
9. **`2HOURS30MINUTES`** (case-insensitive): 2 hours and 30 minutes.
10. **`7 days ago`** or **`7d ago`**: 7 days in the past (equivalent to -7 days).
11. **`2 hours 30 minutes ago`**: 2.5 hours in the past (equivalent to -2h30m).
