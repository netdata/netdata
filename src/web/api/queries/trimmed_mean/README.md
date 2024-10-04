# Trimmed Mean

The trimmed mean is the average value of a series excluding the smallest and biggest points.

Netdata applies linear interpolation on the last point, if the percentage requested to be excluded does not give a
round number of points.

The following percentile aliases are defined:

- `trimmed-mean1`
- `trimmed-mean2`
- `trimmed-mean3`
- `trimmed-mean5`
- `trimmed-mean10`
- `trimmed-mean15`
- `trimmed-mean20`
- `trimmed-mean25`

The default `trimmed-mean` is an alias for `trimmed-mean5`.
Any percentage may be requested using the `group_options` query parameter.

## how to use

Use it in alerts like this:

```
 alarm: my_alert
    on: my_chart
lookup: trimmed-mean5 -1m unaligned of my_dimension
  warn: $this > 1000
```

`trimmed-mean` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=trimmed-mean` in the URL and the additional parameter `group_options`
may be used to request any percentage (e.g. `&group=trimmed-mean&group_options=29`).

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=trimmed-mean5&after=-60&label=trimmed-mean5&value_color=orange)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

-   <https://en.wikipedia.org/wiki/Truncated_mean>.
