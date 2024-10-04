# Percentile

The percentile is the average value of a series using only the smaller N percentile of the values.
(a population or a probability distribution).

Netdata applies linear interpolation on the last point, if the percentile requested does not give a round number of
points.

The following percentile aliases are defined:

- `percentile25`
- `percentile50`
- `percentile75`
- `percentile80`
- `percentile90`
- `percentile95`
- `percentile97`
- `percentile98`
- `percentile99`

The default `percentile` is an alias for `percentile95`.
Any percentile may be requested using the `group_options` query parameter.

## how to use

Use it in alerts like this:

```
 alarm: my_alert
    on: my_chart
lookup: percentile95 -1m unaligned of my_dimension
  warn: $this > 1000
```

`percentile` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=percentile` in the URL and the additional parameter `group_options`
may be used to request any percentile (e.g. `&group=percentile&group_options=96`).

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=percentile95&after=-60&label=percentile95&value_color=orange)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

-   <https://en.wikipedia.org/wiki/Percentile>.
