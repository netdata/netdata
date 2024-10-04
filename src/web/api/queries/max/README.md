# Max

This module finds the max value in the time-frame given.

## how to use

Use it in alerts like this:

```
 alarm: my_alert
    on: my_chart
lookup: max -1m unaligned of my_dimension
  warn: $this > 1000
```

`max` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=max` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max&value_color=orange)

## References

-   <https://en.wikipedia.org/wiki/Sample_maximum_and_minimum>.


