# Min

The `min` grouping method returns the smallest collected value in each requested time bucket. Use it when a brief drop is
operationally important, such as the lowest available capacity, request rate, replication count, voltage, or service count
seen during an alert window.

`min` operates on the values produced for each selected dimension. It does not add dimensions together and it does not change
the chart's units. If a chart uses `requests/sec`, the result is still expressed in `requests/sec`.

## Use `min` in alerts

An alert lookup can reduce its complete time window to the minimum value. This example evaluates the smallest value of
`my_dimension` during the last minute:

```yaml
 alarm: my_alert
    on: my_chart
lookup: min -1m unaligned of my_dimension
  warn: $this < 1000
```

The alert enters warning state when any grouped value in that window falls below `1000`. `unaligned` asks for the exact
rolling window relative to evaluation time. Without it, Netdata aligns time boundaries so repeated dashboard and API queries
return stable buckets.

Use `min` when a single low sample should influence the alert. Do not use it when the sustained level or typical behavior
matters more; `average` or percentile methods may represent those conditions better. A minimum is sensitive to isolated
drops, so choose a lookup duration that matches the failure you intend to detect. For metrics where zero is normal during
idle periods, a minimum-of-zero alert usually needs additional conditions or a different grouping method.

## Use `min` in API queries

Pass `group=min` to `/api/v1/data` or `/api/v1/badge.svg`. The `after` and `before` parameters select the time frame, while
`points` controls how many buckets are returned. For example:

```text
/api/v1/data?chart=system.cpu&dimensions=idle&after=-600&points=10&group=min
```

This asks for ten buckets across the last ten minutes and returns the minimum `idle` value in each bucket. Setting
`points=1` produces one minimum for the complete available window. When the requested time frame does not overlap stored
data, the query returns an empty data set rather than inventing a minimum.

Dimension filtering happens before grouping, except for options that explicitly require all dimensions. Select the relevant
dimension or dimensions so the result answers a precise question.

## Examples

These badges compare the minimum, average, and maximum successful web-server response rate over the last minute. The minimum
badge is orange:

- ![Minimum successful response rate](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min&value_color=orange)
- ![Average successful response rate](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
- ![Maximum successful response rate](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

Open a badge URL directly to inspect or change its query parameters. For custom applications that need numeric data rather
than an SVG response, use the same grouping method with the data API and choose an appropriate output formatter.

## References

- [Sample maximum and minimum](https://en.wikipedia.org/wiki/Sample_maximum_and_minimum)
- [Database queries and grouping methods](/src/web/api/queries/README.md#grouping-methods)

