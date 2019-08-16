# double exponential smoothing

Exponential smoothing is one of many window functions commonly applied to smooth data in signal
processing, acting as low-pass filters to remove high frequency noise.

Simple exponential smoothing does not do well when there is a trend in the data.
In such situations, several methods were devised under the name "double exponential smoothing"
or "second-order exponential smoothing.", which is the recursive application of an exponential
filter twice, thus being termed "double exponential smoothing".

In simple terms, this is like an average value, but more recent values are given more weight
and the trend of the values influences significantly the result.

> **IMPORTANT**
>
> It is common for `des` to provide "average" values that far beyond the minimum or the maximum
> values found in the time-series.
> `des` estimates these values because of it takes into account the trend.

This module implements the "Holt-Winters double exponential smoothing".

Netdata automatically adjusts the weight (`alpha`) and the trend (`beta`) based on the number
of values processed, using the formula:

```
window = max(number of values, 15)
alpha  = 2 / (window + 1)
beta   = 2 / (window + 1)
```

You can change the fixed value `15` by setting in `netdata.conf`:

```
[web]
   des max window = 15
```

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: des -1m unaligned of my_dimension
  warn: $this > 1000
```

`des` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=des` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average&value_color=yellow)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=ses&after=-60&label=single+exponential+smoothing&value_color=yellow)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=des&after=-60&label=double+exponential+smoothing&value_color=orange)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

-   <https://en.wikipedia.org/wiki/Exponential_smoothing>.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fqueries%2Fdes%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
