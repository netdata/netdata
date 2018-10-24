# Median

The median is the value separating the higher half from the lower half of a data sample
(a population or a probability distribution). For a data set, it may be thought of as the
"middle" value.

`median` is not an accurate average. However, it eliminates all spikes, by sorting
all the values in a period, and selecting the value in the middle of the sorted array.

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: median -1m unaligned of my_dimension
  warn: $this > 1000
```

`median` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=median` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=median&after=-60&label=median&value_color=orange)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

- [https://en.wikipedia.org/wiki/Median](https://en.wikipedia.org/wiki/Median).
