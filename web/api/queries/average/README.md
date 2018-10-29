# Average or Mean

> This query is available as `average` and `mean`.

An average is a single number taken as representative of a list of numbers.

It is calculated as:

```
average = sum(numbers) / count(numbers)
```

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: average -1m unaligned of my_dimension
  warn: $this > 1000
```

`average` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=average` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average&value_color=orange)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

- [https://en.wikipedia.org/wiki/Average](https://en.wikipedia.org/wiki/Average).
