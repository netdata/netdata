# Statsd HTTP API

Netdata statsd accepts metrics via its HTTP REST API, at the normal web port of Netdata (usually port 19999).

The endpoint to send metrics via this endpoint is:

```
http://your.netdata.ip:19999/api/v2/statsd
```

The endpoint supports the following query parameters:

- `type`<br/>
  The type defines the type of the metric you want to push. For each type, Netdata maintains a unique index of metrics. For best results, do not use the same metric name under different types.
  <br/>**THIS FIELD IS REQUIRED**

  - `meter` or `counter`<br/>
    The final chart presents the **rate per second** of the values collected. Each sample is assumed to be an increment. Good to calculate the rate something is happening (send +1 events from everywhere, and Netdata will give you the rate this happening).
    
  - `gauge`<br/>
    The final chart presents the **last value** collected. Values can be prefixed with `+` or `-` to increment or decrement the previous value. Good to render the state of something (e.g. the amount of free/used memory).
  
  - `histogram`
    The final chart presents statistics related to the events pushed to Netdata. You push individual event values, like the bytes transferred per response, and Netdata provides the min, max, sum, average, median, stddev and percentile of all of them.
  
  - `timer`
    The final chart presents statistics related to the times pushed to Netdata. You push individual event timings, like the time it took for a response to complete, and Netdata provides the min, max, sum, average, median, stddev and percentile of all of them.
  
  - `set`
    The final chart present the total of unique values used.
    
  - `dictionary`
    The final chart presents the rate each value was used.


- `metric`<br/>
  the unique metric name, the equivalent of the `context` in Netdata lingo.
  <br/>**THIS FIELD IS REQUIRED**

- `value`<br/>
  the value you want to set this metric. Depending on the metric type, the value is interpreted differently.
  <br/>**THIS FIELD IS REQUIRED**

- `units`<br/>
  the units of the metric

- `label`, `labels`, `tag`, or `tags`<br/>
  set labels for the chart. Multiple labels can be set as `&labels=key1:value1|key2:value2`. Another way is `&labels=key1:value1&labels=key2:value2`.

  When setting labels, remember that the unique combination of labels creates an `instance` in Netdata. So, you can push the same `metric` of the same `type` multiple times, with different labels, to create multiple instances of it.

- `name`<br/>
  overwrite the default dimension name in charts.
  The `name` is used in `meters`, `counters`, `gauges`, and `sets`.

- `family`<br/>
  overwrite the default family of the chart.

- `title`<br/>
  overwrite the default title of the chart.

- `instance`<br/>
  by default the instance is determined by the metric and its labels, appended with a hash to make it unique.
  Passing `instance` overwrites the instance name, bypassing this automatic naming algorithm.

## Best practices

1. Prefix all metrics with YOUR APPLICATION NAME. So, `requests.received` should be `myapp_requests.received`. Netdata will create the section `Statsd Myapp` on the dashboard.
2. The families control the submenu under your application.

## Responses

Netdata responds with `200 OK` and something like this:

```json
{
    "metric":"a.b.c.d__aaa_bbb_ccc_ddd_0xb52380ba",
    "context":"a.b.c.d",
    "value":"+10",
    "family":"",
    "title":"hello world",
    "name":"value",
    "labels":{
        "aaa":"bbb",
        "ccc":"ddd"
    },
    "status":{
        "counter_value":10, << the current value according to 'type'
        "events":1,         << the number of events recorded ever
        "count":1           << the count of event for this flush period
    }
}
```

