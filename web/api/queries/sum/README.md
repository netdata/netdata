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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fqueries%2Fsum%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
