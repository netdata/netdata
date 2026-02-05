# Badges

Netdata can generate SVG badges that display real-time metric values or alert statuses. These badges can be embedded in external dashboards, documentation, GitHub READMEs, or any web page that supports images.

## Overview

Badges provide a simple way to share Netdata metrics and alert states externally. Each badge is an SVG image that updates dynamically and can display:

- Current metric values from any chart
- Alert states (warning, critical, clear)
- Custom labels and units
- Color-coded states based on thresholds

## Accessing Badges

Badges are available through the Netdata Agent API at:

```
http://YOUR_NETDATA_HOST:19999/api/v1/badge.svg?chart=CHART_ID&options=OPTIONS
```

### Authentication and Access Control

By default, badges can be accessed from any source. To restrict access:

1. Edit `netdata.conf`:

```ini
[web]
    allow badges from = 10.* 192.168.* YOUR_IP
```

2. Restart Netdata:

```bash
sudo systemctl restart netdata
```

## Query Parameters

### Required Parameters

| Parameter | Description             | Example                            |
| --------- | ----------------------- | ---------------------------------- |
| `chart`   | The chart ID to display | `system.cpu`, `netdata.server_cpu` |

### Optional Parameters

| Parameter            | Description                                  | Default                           | Example                        |
| -------------------- | -------------------------------------------- | --------------------------------- | ------------------------------ |
| `alarm`              | Display alert status instead of metric value | -                                 | `system.cpu.10min_cpu_usage`   |
| `dimension` or `dim` | Specific dimension(s) to display             | All dimensions                    | `user`, `system`               |
| `after`              | Time range start (negative seconds)          | `-UPDATE_EVERY` (chart-dependent) | `-600` (10 min ago)            |
| `before`             | Time range end (negative seconds)            | `0` (now)                         | `0`                            |
| `points`             | Number of data points to aggregate           | `1`                               | `60`                           |
| `group`              | Aggregation method                           | `average`                         | `average`, `sum`, `max`, `min` |
| `group_options`      | Additional grouping options                  | -                                 | `percentage`                   |
| `options`            | Query options (percentage, abs, etc.)        | -                                 | `percentage`                   |
| `label`              | Left-side label text                         | Chart name                        | `CPU Usage`                    |
| `units`              | Unit suffix to display                       | Auto-detected                     | `%`, `MB`, `requests/s`        |
| `multiply`           | Multiply value by this factor                | `1`                               | `100` (for percentages)        |
| `divide`             | Divide value by this factor                  | `1`                               | `1024` (bytes to KB)           |
| `precision`          | Decimal places (-1 for auto)                 | `-1` (auto)                       | `2`                            |
| `scale`              | Badge scale percentage                       | `100`                             | `150`                          |
| `refresh`            | Auto-refresh interval in seconds             | `0` (no refresh)                  | `auto`, `5`                    |
| `label_color`        | Left side background color                   | `grey`                            | `blue`, `red`, `#007ec6`       |
| `value_color`        | Right side background color                  | Based on value                    | `green`, `yellow`, `#4c1`      |
| `text_color_lbl`     | Left text color                              | `grey` (fallback)                 | `black`, `#fff`                |
| `text_color_val`     | Right text color                             | `grey` (fallback)                 | `black`, `#fff`                |
| `fixed_width_lbl`    | Fixed width for label (pixels)               | Auto                              | `100`                          |
| `fixed_width_val`    | Fixed width for value (pixels)               | Auto                              | `80`                           |

## Usage Examples

### Basic Metric Badge

Display current CPU usage:

```markdown
![CPU Usage](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&dimension=user)
```

### Alert Status Badge

Display alert state:

```markdown
![CPU Alert](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=system.cpu.10min_cpu_usage)
```

### Custom Label and Units

```markdown
![Memory](http://localhost:19999/api/v1/badge.svg?chart=mem.available&label=RAM&units=GB&divide=1073741824&precision=2)
```

### Aggregated Values

Show average network traffic over 5 minutes:

```markdown
![Network](http://localhost:19999/api/v1/badge.svg?chart=system.net&dimension=received&after=-300&group=average&label=Net+In)
```

### Color-Coded Badges

Static colors:

```markdown
![Status](http://localhost:19999/api/v1/badge.svg?chart=system.load&label=Load&value_color=blue)
```

Conditional colors based on value:

```markdown
![Disk](http://localhost:19999/api/v1/badge.svg?chart=disk_space._&label=Root&units=%&value_color=green<80:yellow<90:red)
```

## Color Options

### Predefined Colors

| Color                                                                                                                             | Hex Code  |
| --------------------------------------------------------------------------------------------------------------------------------- | --------- |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#4c1;"></span> `brightgreen`                | `#4c1`    |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#97CA00;"></span> `green`                   | `#97CA00` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#a4a61d;"></span> `yellowgreen`             | `#a4a61d` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#dfb317;"></span> `yellow`                  | `#dfb317` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#fe7d37;"></span> `orange`                  | `#fe7d37` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#e05d44;"></span> `red`                     | `#e05d44` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#007ec6;"></span> `blue`                    | `#007ec6` |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#555;"></span> `grey` / `gray`              | `#555`    |
| <span style="display:inline-block;width:16px;height:16px;border-radius:50%;background:#9f9f9f;"></span> `lightgrey` / `lightgray` | `#9f9f9f` |

### Special Units Formats

When using the `units` parameter, these special formats are recognized:

| Units Value                            | Display Format            |
| -------------------------------------- | ------------------------- |
| `seconds` / `seconds ago`              | Formatted time (HH:MM:SS) |
| `minutes` / `minutes ago`              | Formatted time (Xd HH:MM) |
| `hours` / `hours ago`                  | Formatted time (Xd XXh)   |
| `on/off` / `on-off` / `onoff`          | "on" or "off"             |
| `up/down` / `up-down` / `updown`       | "up" or "down"            |
| `ok/error` / `ok-error` / `okerror`    | "ok" or "error"           |
| `ok/failed` / `ok-failed` / `okfailed` | "ok" or "failed"          |
| `percentage` / `percent` / `pcent`     | Adds `%` suffix           |
| `empty` / `null`                       | Hides units entirely      |

### Custom Colors

Use any hex color code (without the `#`):

```
value_color=FF5733
```

### Conditional Colors

Set colors based on value thresholds:

```
value_color=green<80:yellow<90:red
```

This means:

- Green if value < 80
- Yellow if value ≥ 80 and < 90
- Red if value ≥ 90

Supported operators:

- `:` or `=` equality (first match wins)
- `!` inequality
- `<` less than
- `>` greater than
- `<=` less than or equal
- `>=` greater than or equal

## Grouping Methods

When aggregating multiple data points, the `group` parameter determines how values are combined:

| Method            | Description                               |
| ----------------- | ----------------------------------------- |
| `average`         | Arithmetic mean of values                 |
| `sum`             | Sum of all values                         |
| `max`             | Maximum value                             |
| `min`             | Minimum value                             |
| `median`          | Median value                              |
| `stddev`          | Standard deviation                        |
| `incremental-sum` | Incremental sum (for counters)            |
| `trimmed-mean`    | Trimmed mean (excludes outliers)          |
| `percentile`      | Percentile (specify with `group_options`) |
| `ses`             | Single exponential smoothing              |
| `des`             | Double exponential smoothing              |
| `cv`              | Coefficient of variation                  |
| `countif`         | Count values matching condition           |
| `extremes`        | Min/max for mixed sign values             |

Common methods: `average`, `sum`, `min`, `max`, `median`. See [Query Reference](/docs/developer-and-contributor-corner/rest-api/Queries/README.md) for all options.

## Alert Badges

When using the `alarm` parameter, badges display alert states:

- **CLEAR** - Green badge, alert is not triggered
- **WARNING** - Yellow badge, warning threshold exceeded
- **CRITICAL** - Red badge, critical threshold exceeded
- **UNDEFINED** - Grey badge, alert cannot be evaluated
- **UNINITIALIZED** - Black badge, alert has not been initialized

Example:

```markdown
![CPU Alert](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=system.cpu.10min_cpu_usage&label=CPU+Alert)
```

## Refresh Behavior

Badges can auto-refresh to show real-time data:

- `refresh=auto` - Automatically calculates refresh based on chart update frequency
- `refresh=5` - Refresh every 5 seconds
- `refresh=0` or omitted - No auto-refresh (static badge)

**Note:** Auto-refresh works through HTTP cache headers. Some platforms (like GitHub) cache images aggressively and may not show real-time updates.

## Common Use Cases

### GitHub README

Add to your repository README:

```markdown
## Server Status

![CPU](https://your-netdata.com/api/v1/badge.svg?chart=system.cpu&dimension=user&label=CPU)
![RAM](https://your-netdata.com/api/v1/badge.svg?chart=mem.available&units=GB&divide=1073741824&precision=1&label=RAM)
![Disk](https://your-netdata.com/api/v1/badge.svg?chart=disk_space._&units=%&label=Disk)
```

### External Dashboard

Create a simple status dashboard:

```html
<div class="status-badges">
  <img
    src="http://netdata.local:19999/api/v1/badge.svg?chart=system.cpu&label=CPU"
    alt="CPU"
  />
  <img
    src="http://netdata.local:19999/api/v1/badge.svg?chart=system.ram&label=RAM"
    alt="RAM"
  />
  <img
    src="http://netdata.local:19999/api/v1/badge.svg?chart=system.load&label=Load"
    alt="Load"
  />
</div>
```

### Slack/Discord Integration

Share metric badges in chat:

```
Current server status:
http://netdata.local:19999/api/v1/badge.svg?chart=system.cpu&dimension=user&value_color=green<50:yellow<80:red&label=CPU
```

## Troubleshooting

### Badge shows "chart not found"

- Verify the chart ID exists: Check the Netdata dashboard URL for the correct chart ID
- Chart IDs are case-sensitive
- Use `system.cpu` format, not `system.cpu.user`

### Badge shows empty value

- Check that the `dimension` parameter matches an actual dimension name
- Verify the time range (`after`/`before`) contains recent data
- Badge shows "-" when data is too old (staleness check) or unavailable

### Access denied / Cannot view badge

- Check `allow badges from` in `netdata.conf`
- Ensure the requesting IP is allowed
- Check firewall rules on the Netdata host

### Colors not working

- Use predefined color names or 6-character hex codes (without `#`)
- For conditional colors, ensure the format is correct: `color<threshold`

### Badge not updating

- Add `refresh=auto` or a specific interval
- Some platforms cache images; try adding a cache-busting parameter:
  ```
  http://netdata.local:19999/api/v1/badge.svg?chart=system.cpu&refresh=5&t=123456
  ```

## Advanced Configuration

### Multiple Dimensions

Aggregate multiple dimensions:

```
chart=system.cpu&dimension=user&dimension=system&group=average
```

### Calculated Values

Show bandwidth in Mbps:

```
chart=system.net&multiply=8&divide=1000000&units=Mbps&precision=2
```

### Complex Conditional Colors

Multiple thresholds:

```
value_color=brightgreen<50:green<70:yellowgreen<80:yellow<90:orange<95:red
```

## Security Considerations

1. **Access Control**: Always configure `allow badges from` to restrict access
2. **Sensitive Data**: Avoid exposing sensitive metrics through badges
3. **Public Exposure**: If exposing badges publicly, consider:
   - Using a reverse proxy with authentication
   - Limiting available charts via ACLs
   - Monitoring badge access logs

## Related Documentation

- [Web Dashboard](/docs/dashboards-and-charts/README.md)
- [Alert Configuration](/docs/alerts-and-notifications/README.md)
- [API Reference](/docs/developer-and-contributor-corner/rest-api/README.md)
- [Access Control](/docs/netdata-agent/securing-netdata-agents.md)
