# Sum

This module sums all the values in the time-frame requested.

You can use `sum` to find the volume of something over a period.

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: sum -1m unaligned of my_dimension
  warn: $this > 1000
```

`sum` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=sum` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=sum&after=-60&label=1m+sum&value_color=orange&units=requests)

## References

- [https://en.wikipedia.org/wiki/Summation](https://en.wikipedia.org/wiki/Summation).
