<!--
title: "Incremental Sum (`incremental_sum`)"
sidebar_label: "Incremental Sum (`incremental_sum`)"
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/api/queries/incremental_sum/README.md
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Developers/Web/Api/Queries"
-->

# Incremental Sum (`incremental_sum`)

This modules finds the incremental sum of a period, which `last value - first value`.

The result may be positive (rising) or negative (falling) depending on the first and last values.

## how to use

Use it in alarms like this:

```
 alarm: my_alarm
    on: my_chart
lookup: incremental_sum -1m unaligned of my_dimension
  warn: $this > 1000
```

`incremental_sum` does not change the units. For example, if the chart units is `requests/sec`, the result
will be again expressed in the same units. 

It can also be used in APIs and badges as `&group=incremental_sum` in the URL.

## Examples

Examining last 1 minute `successful` web server responses:

-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)
-   ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=incremental_sum&after=-60&label=incremental+sum&value_color=orange)

## References

-   none


