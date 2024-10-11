# Median

The median is the value separating the higher half from the lower half of a data sample
(a population or a probability distribution). For a data set, it may be thought of as the
"middle" value.

`median` is not an accurate average. However, it eliminates all spikes, by sorting
all the values in a period, and selecting the value in the middle of the sorted array.

Netdata also supports `trimmed-median`, which trims a percentage of the smaller and bigger values prior to finding the
median. The following `trimmed-median` functions are defined:

- `trimmed-median1`
- `trimmed-median2`
- `trimmed-median3`
- `trimmed-median5`
- `trimmed-median10`
- `trimmed-median15`
- `trimmed-median20`
- `trimmed-median25`

The function `trimmed-median` is an alias for `trimmed-median5`.

## how to use

Use it in alerts like this:

```
 alarm: my_alert
    on: my_chart
lookup: median -1m unaligned of my_dimension
  warn: $this > 1000
```

`median` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=median` in the URL. Additionally, a percentage may be given with
`&group_options=` to trim all small and big values before finding the median.

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=median&after=-60&label=median&value_color=orange)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

-   <https://en.wikipedia.org/wiki/Median>.


