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

```
[web]
    allow badges from = 10.* 192.168.* YOUR_IP
```

The `allow badges from` parameter goes under the `[web]` section in `netdata.conf`.

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

| Parameter            | Description                                 | Default                           | Example                        |
| -------------------- | ------------------------------------------- | --------------------------------- | ------------------------------ |
| `alarm`              | Display alert status (shows status + value) | -                                 | `system.cpu.10min_cpu_usage`   |
| `dimension` or `dim` | Specific dimension(s) to display            | All dimensions                    | `user`, `system`               |
| `after`              | Time range start (negative seconds)         | `-UPDATE_EVERY` (chart-dependent) | `-600` (10 min ago)            |
| `before`             | Time range end (negative seconds)           | `0` (now)                         | `0`                            |
| `points`             | Number of data points to aggregate          | `1`                               | `60`                           |
| `group`              | Aggregation method                          | `average`                         | `average`, `sum`, `max`, `min` |
| `group_options`      | Additional grouping options                 | -                                 | `percentage`                   |
| `options`            | Query options (percentage, abs, etc.)       | -                                 | `percentage%7Cabsolute`        |
| `label`              | Left-side label text                        | Chart name                        | `CPU Usage`                    |
| `units`              | Unit suffix to display                      | Auto-detected                     | `%`, `MB`, `requests/s`        |
| `multiply`           | Multiply value by this factor               | `1`                               | `100` (for percentages)        |
| `divide`             | Divide value by this factor                 | `1`                               | `1024` (MiB to MB)             |
| `precision`          | Decimal places (-1 for auto)                | `-1` (auto)                       | `2`                            |
| `scale`              | Badge scale percentage                      | `100`                             | `150`                          |
| `refresh`            | Auto-refresh interval in seconds            | `0` (no refresh)                  | `auto`, `5`                    |
| `label_color`        | Left side background color                  | `grey`                            | `blue`, `red`, `#007ec6`       |
| `value_color`        | Right side background color                 | Based on value                    | `green`, `yellow`, `#4c1`      |
| `text_color_lbl`     | Left text color                             | `fff` (white)                     | `black`, `#fff`                |
| `text_color_val`     | Right text color                            | `fff` (white)                     | `black`, `#fff`                |
| `fixed_width_lbl`    | Fixed width for label (pixels)              | Auto                              | `100`                          |
| `fixed_width_val`    | Fixed width for value (pixels)              | Auto                              | `80`                           |

## Usage Examples

### Basic Metric Badge

Display CPU usage from the `user` dimension:

```markdown
![CPU Usage](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&dimension=user)
```

To display aggregated CPU (all dimensions combined):

```markdown
![CPU Usage](http://localhost:19999/api/v1/badge.svg?chart=system.cpu)
```

### Alert Status Badge

The badge displays the alert status text (like "OK", "WARNING", "CRITICAL") with the configured color. The alarm name must match your Netdata health configuration.

Example:

```markdown
![CPU Alert](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=system.cpu_usage&label=CPU+Alert)
```

**Note:** Alarm names vary by Netdata configuration. Check your health configuration (`/etc/netdata/health.d/*.conf`) for the exact alarm name to use.

### Custom Label and Units

Display available memory in GB with one decimal place:

```markdown
![Memory](http://localhost:19999/api/v1/badge.svg?chart=mem.available&label=RAM&precision=1)
```

**Note:** The `mem.available` chart shows total available memory and does not have dimensions. To display specific memory components like `free` or `used`, use the `system.ram` chart:

### Aggregated Values

Show average network traffic over 5 minutes:

```markdown
![Network](http://localhost:19999/api/v1/badge.svg?chart=system.net&dimension=received&after=-300&group=average&label=NetTraffic)
```

### Color-Coded Badges

Static colors:

Display system load average (all 3 dimensions):

```markdown
![Status](http://localhost:19999/api/v1/badge.svg?chart=system.load&label=Load)
```

**Note:** This example shows all dimensions combined. To display specific load dimensions, use the `dimension` parameter:

```markdown
![Status](http://localhost:19999/api/v1/badge.svg?chart=system.load&dimension=load1&label=Load+1)
```

Conditional colors based on value:

```markdown
![Disk](http://localhost:19999/api/v1/badge.svg?chart=disk_space._&label=Root&units=%&value_color=green<80:yellow<90:red)
```

## Color Options

### Predefined Colors

Use these color names in `label_color`, `value_color`, `text_color_lbl`, or `text_color_val`:

![brightgreen](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=brightgreen&units=)
![green](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=green&units=)
![yellowgreen](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=yellowgreen&units=)
![yellow](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=yellow&units=)
![orange](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=orange&units=)
![red](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=red&units=)
![blue](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=blue&units=)
![grey](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=grey&units=)
![lightgrey](https://registry.my-netdata.io/api/v1/badge.svg?chart=system.cpu&label=test&value_color=lightgrey&units=)

Hex codes: `brightgreen:#4c1` · `green:#97CA00` · `yellowgreen:#a4a61d` · `yellow:#dfb317` · `orange:#fe7d37` · `red:#e05d44` · `blue:#007ec6` · `grey:#555` · `lightgrey:#9f9f9f`

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

To display a literal `/` in units, escape it with `\` (e.g., `units=requests\/s`).

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
- `!` or `<>` inequality
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

Common methods: `average`, `sum`, `min`, `max`, `median`. See [Query Reference](/src/web/api/queries/README.md) for all options.

### Query Options

The `options` parameter accepts pipe-delimited values:

| Option             | Description                                                |
| ------------------ | ---------------------------------------------------------- |
| `percentage`       | Calculate value as percentage of dimension total           |
| `absolute` / `abs` | Convert all values to positive before summing              |
| `display_absolute` | Use signed value for color, display absolute               |
| `min2max`          | For multiple dimensions, return `max - min` instead of sum |
| `unaligned`        | Skip time alignment for aggregated data                    |

## Alert Badges

When using the `alarm` parameter, badges display the alert status along with the current metric value. The badge color reflects the alert state:

- **CLEAR** - Bright green badge, alert is not triggered
- **WARNING** - Orange badge, warning threshold exceeded
- **CRITICAL** - Red badge, critical threshold exceeded
- **UNDEFINED** - Grey badge, alert cannot be evaluated
- **UNINITIALIZED** - Black badge, alert has not been initialized
- **REMOVED** - Grey badge, alert has been removed (shutdown, disconnect)

Example (replace `YOUR_ALARM_NAME` with your actual alarm name):

```markdown
![CPU Alert](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&alarm=YOUR_ALARM_NAME&label=CPU+Alert)
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
![RAM](https://your-netdata.com/api/v1/badge.svg?chart=mem.available&units=GB&divide=1024&precision=1&label=RAM)
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
    src="http://netdata.local:19999/api/v1/badge.svg?chart=mem.available&label=RAM"
    alt="RAM"
  />
  <img
    src="http://netdata.local:19999/api/v1/badge.svg?chart=system.load&dimension=load1&label=Load"
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

<details>
<summary>Badge shows "chart not found"</summary>

When requesting a non-existent chart, the API returns a placeholder badge displaying "chart not found" on the left side and "-" for the value. This is expected behavior—not an HTTP error.

To resolve:

1. Verify the chart ID exists in your Netdata dashboard
2. Chart IDs are case-sensitive—use `system.cpu`, not `system.cpu.user`
3. Check the dashboard URL for the correct chart ID format
4. List available charts: `http://YOUR_HOST:19999/api/v1/charts`

</details>

<details>
<summary>Badge shows empty value ("-")</summary>

Check that the `dimension` parameter matches an actual dimension name. Verify the time range (`after`/`before`) contains recent data. The badge shows "-" when data is too old (staleness check) or unavailable.

</details>

<details>
<summary>Access denied / Cannot view badge</summary>

Check `allow badges from` in `netdata.conf`. Ensure the requesting IP is allowed. Check firewall rules on the Netdata host.

</details>

<details>
<summary>Colors not working</summary>

Use predefined color names or 6-character hex codes (without `#`). For conditional colors, ensure the format is correct: `color<threshold`.

</details>

<details>
<summary>Badge not updating</summary>

Add `refresh=auto` or a specific interval. Some platforms cache images; try adding a cache-busting parameter:

```
http://netdata.local:19999/api/v1/badge.svg?chart=system.cpu&refresh=5&t=123456
```

</details>

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

## URL Escaping

When embedding badges in HTML, certain characters must be escaped:

| Character | Description                  | Escape Sequence |
| --------- | ---------------------------- | --------------- |
| Space     | Spaces in labels/units       | `%20`           |
| `#`       | Hash for colors              | `%23`           |
| `%`       | Percent in units             | `%25`           |
| `<`       | Less than                    | `%3C`           |
| `>`       | Greater than                 | `%3E`           |
| `\`       | Backslash (for `/` in units) | `%5C`           |
| `\|`      | Pipe (delimit parameters)    | `%7C`           |

Example with escaped spaces:

```markdown
![CPU](http://localhost:19999/api/v1/badge.svg?chart=system.cpu&label=CPU%20Usage)
```

## Performance

Netdata can generate approximately **2,000 badges per second per CPU core**, with each badge taking about 0.5ms to generate on modern hardware.

For badges calculating aggregates over long durations (days or more), response times will increase. Check `access.log` for timing information. Consider caching such badges or using a cron job to periodically save them.

## Auto-Refresh

For pages that support HTTP refresh headers, use `refresh=auto` or `refresh=SECONDS`:

```markdown
![CPU](http://YOUR_NETDATA:19999/api/v1/badge.svg?chart=system.cpu&refresh=auto)
```

For `embed` or `iframe` elements that don't support HTTP refresh headers, use JavaScript:

```html
<embed
  src="http://YOUR_NETDATA:19999/api/v1/badge.svg?chart=system.cpu&refresh=auto"
  type="image/svg+xml"
  height="20"
/>
```

## Auto-Refresh with JavaScript

For pages without HTTP refresh header support, use JavaScript to auto-refresh badges:

```html
<img
  class="netdata-badge"
  src="http://netdata.local:19999/api/v1/badge.svg?chart=system.cpu&label=CPU"
/>
<script>
  var NETDATA_BADGES_AUTOREFRESH_SECONDS = 5;
  function refreshNetdataBadges() {
    var now = new Date().getTime().toString();
    document.querySelectorAll(".netdata-badge").forEach(function (img) {
      img.src = img.src.replace(/&_=\d*/, "") + "&_=" + now;
    });
    setTimeout(refreshNetdataBadges, NETDATA_BADGES_AUTOREFRESH_SECONDS * 1000);
  }
  setTimeout(refreshNetdataBadges, NETDATA_BADGES_AUTOREFRESH_SECONDS * 1000);
</script>
```

Alternatively, include the Netdata refresh script:

```html
<script src="http://YOUR_NETDATA:19999/refresh-badges.js"></script>
```

## Character Escaping

When embedding badges in HTML, special characters must be URL-encoded:

| Character |           Name           | Escape Sequence |
| :-------: | :----------------------: | :-------------: |
|  (space)  | Space (in labels/units)  |      `%20`      |
|    `#`    |    Hash (for colors)     |      `%23`      |
|    `%`    |    Percent (in units)    |      `%25`      |
|    `<`    |        Less than         |      `%3C`      |
|    `>`    |       Greater than       |      `%3E`      |
|    `\`    |   Backslash (for `/`)    |      `%5C`      |
|   `\|`    | Pipe (delimiting params) |      `%7C`      |

## GitHub Limitations

GitHub fetches images through a proxy and rewrites URLs, preventing auto-refresh. Badges in GitHub READMEs are static. To refresh manually, run this in the browser console:

```javascript
document.querySelectorAll("img").forEach(function (img) {
  if (img.src.includes("badge.svg")) {
    img.src =
      img.src.replace(/\?cacheBuster=\d*/, "") +
      "?cacheBuster=" +
      new Date().getTime();
  }
});
```

## Notes on Chart and Alert Behavior

- **Chart availability varies**: Not all charts are available on all Netdata installations. Use the dashboard to verify chart IDs before using them in badges.
- **Missing chart parameter**: Omitting the `chart` parameter returns HTTP 400 (Bad Request)—badges require a valid chart ID.
- **Invalid chart ID**: Requesting a non-existent chart returns a placeholder badge with "chart not found" label and "-" value—this graceful degradation keeps badges visible.
- **Dimension behavior**: When no dimension is specified, charts may show aggregated values (sum, average, or total) depending on the chart type. Specify individual dimensions for more granular control.
- **Alarm names are configuration-dependent**: The exact alarm names depend on your Netdata health configuration files in `/etc/netdata/health.d/`. Check your configuration to find the correct alarm name.
- **Memory charts**: The `mem.available` chart shows total available memory without dimensions. The `system.ram` chart provides `free` and `used` dimensions for specific memory components.
- **System load**: The `system.load` chart has multiple dimensions (load1, load5, load15). Without specifying a dimension, badges show the sum of all load averages. Use specific dimensions for individual load metrics.

## Related Documentation

- [Web Dashboard](/docs/dashboards-and-charts/README.md)
- [Alert Configuration](/src/health/README.md)
- [API Reference](/src/web/api/README.md)
- [Access Control](/docs/netdata-agent/securing-netdata-agents.md)
