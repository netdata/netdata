
# standard deviation (`stddev`)

The standard deviation is a measure that is used to quantify the amount of variation or dispersion
of a set of data values.

A low standard deviation indicates that the data points tend to be close to the mean (also called the
expected value) of the set, while a high standard deviation indicates that the data points are spread
out over a wider range of values.

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: stddev -1m unaligned of my_dimension
  warn: $this > 1000
```

`stdev` does not change the units. For example, if the chart units is `requests/sec`, the standard
deviation will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=stddev` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=min&after=-60&label=min)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=average&after=-60&label=average&value_color=yellow)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=stddev&after=-60&label=standard+deviation&value_color=orange)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=max&after=-60&label=max)

## References

Check [https://en.wikipedia.org/wiki/Standard_deviation](https://en.wikipedia.org/wiki/Standard_deviation).

---

# Coefficient of variation (`cv`)

> This query is also available as `rsd`.

The coefficient of variation (`cv`), also known as relative standard deviation (`rsd`),
is a standardized measure of dispersion of a probability distribution or frequency distribution.

It is defined as the ratio of the **standard deviation** to the **mean**.  

In simple terms, it gives the percentage of change. So, if the average value of a metric is 1000
and its standard deviation is 100 (meaning that it variates from 900 to 1100), then `cv` is 10%.

This is an easy way to check the % variation, without using absolute values.

For example, you may trigger an alarm if your web server requests/sec `cv` is above 20 (`%`)
over the last minute. So if your web server was serving 1000 reqs/sec over the last minute,
it will trigger the alarm if had spikes below 800/sec or above 1200/sec.

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: cv -1m unaligned of my_dimension
 units: %
  warn: $this > 20
```

The units reported by `cv` is always `%`.

It can also be used in APIs and badges as `&group=cv` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=min&after=-60&label=min)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=average&after=-60&label=average&value_color=yellow)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=cv&after=-60&label=coefficient+of+variation&value_color=orange&units=pcent)
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&dimensions=success&group=max&after=-60&label=max)

## References

Check [https://en.wikipedia.org/wiki/Coefficient_of_variation](https://en.wikipedia.org/wiki/Coefficient_of_variation).
