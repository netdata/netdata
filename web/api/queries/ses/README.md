<!--
---
title: "Single (or Simple) Exponential Smoothing (`ses`)"
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/api/queries/ses/README.md
---
-->

# Single (or Simple) Exponential Smoothing (`ses`)

> This query is also available as `ema` and `ewma`.

An exponential moving average (`ema`), also known as an exponentially weighted moving average (`ewma`)
is a first-order infinite impulse response filter that applies weighting factors which decrease
exponentially. The weighting for each older datum decreases exponentially, never reaching zero.

In simple terms, this is like an average value, but more recent values are given more weight.

Netdata automatically adjusts the weight (`alpha`) based on the number of values processed,
using the formula:

```
window = max(number of values, 15)
alpha  = 2 / (window + 1)
```

You can change the fixed value `15` by setting in `netdata.conf`:

```
[web]
   ses max window = 15
```

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: ses -1m unaligned of my_dimension
  warn: $this > 1000
```

`ses` does not change the units. For example, if the chart units is `requests/sec`, the exponential
moving average will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=ses` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average&value_color=yellow)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=ses&after=-60&label=single+exponential+smoothing&value_color=orange)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)

## References

-   <https://en.wikipedia.org/wiki/Moving_average#exponential-moving-average>
-   <https://en.wikipedia.org/wiki/Exponential_smoothing>.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fqueries%2Fses%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
