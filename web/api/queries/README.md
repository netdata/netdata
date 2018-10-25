# Database Queries

Netdata database can be queried with `/api/v1/data` and `/api/v1/badge.svg` REST API methods.

Every data query accepts the following parameters:

name|required|description
:----:|:----:|:---
`chart`|yes|The chart to be queried.
`points`|no|The number of points to be returned. Netdata can reduce number of points by applying query grouping methods. If not given, the result will have the same granularity as the database (although this relates to `gtime`).
`before`|no|The absolute timestamp or the relative (to now) time the query should finish evaluating data. If not given, it defaults to the timestamp of the latest point in the database.
`after`|no|The absolute timestamp or the relative (to `before`) time the query should start evaluating data. if not given, it defaults to the timestamp of the oldest point in the database.
`group`|no|The grouping method to use when reducing the points the database has. If not given, it defaults to `average`.
`gtime`|no|A resampling period to change the units of the metrics (i.e. setting this to `60` will convert `per second` metrics to `per minute`. If not given it defaults to granularity of the database.
`options`|no|A bitmap of options that can affect the operation of the query. Only 2 options are used by the query engine: `unaligned` and `percentage`. All the other options are used by the output formatters. The default is to return aligned data.
`dimensions`|no|A simple pattern to filter the dimensions to be queried. The default is to return all the dimensions of the chart.

## Operation

The query engine works as follows (in this order):

#### Time-frame

`after` and `before` define a time-frame, accepting:

- **absolute timestamps** (unix timestamps, i.e. seconds since epoch).

- **relative timestamps**:

  `before` is relative to now and `after` is relative to `before`.
  
  Example: `before=-60&after=-60` evaluates to the time-frame from -120 up to -60 seconds in
  the past, relative to the latest entry of the database of the chart. 

The engine verifies that the time-frame requested is available at the database:

- If the requested time-frame overlaps with the database, the excess requested
   will be truncated.
   
- If the requested time-frame does not overlap with the database, the engine will
   return an empty data set.

At the end of this operation, `after` and `before` are absolute timestamps.

#### Data grouping

Database points grouping is applied when the caller requests a time-frame to be
expressed with fewer points, compared to what is available at the database.

There are 2 uses that enable this feature:

- The caller requests a specific number of `points` to be returned.
  
  For example, for a time-frame of 10 minutes, the database has 600 points (1/sec),
  while the caller requested these 10 minutes to be expressed in 200 points.
  
  This feature is used by netdata dashboards when you zoom-out the charts.
  The dashboard is requesting the number of points the user's screen has.
  This saves bandwidth and speeds up the browser (fewer points to evaluate for drawing the charts).
  
- The caller requests a **re-sampling** of the database, by setting `gtime` to any value
  above the granularity of the chart.
  
  For example, the chart's units is `requests/sec` and caller wants `requests/min`.
  
Using `points` and `gtime` the query engine tries to find a best fit for **database-points**
vs **result-points** (we call this ratio `group points`). It always tries to keep `group points`
an integer. Keep in mind the query engine may shift `after` if required.

#### Time-frame Alignment

Alignment is a very important aspect of netdata queries. Without it, the animated
charts on the dashboards would constantly change shape during incremental updates.

To provide consistent grouping through time, the query engine (by default) aligns
`after` and `before` to be a multiple of `group points`.

For example, if `group points` is 60 and alignment is enabled, the engine will return
each point with durations XX:XX:00 - XX:XX:59, matching whole minutes.

To disable alignment, pass `&options=unaligned` to the query.
   
#### Query Execution

To execute the query, the engine evaluates all dimensions of the chart, one after another.

The engine does not evaluate dimensions that do not match the [simple pattern](../../../libnetdata/simple_pattern)
given at the `dimensions` parameter, except when `options=percentage` is given (this option
requires all the dimensions to be evaluated to find the percentage of each dimension vs to chart
total).

For each dimension, it starts evaluating values starting at `after` (not inclusive) towards
`before` (inclusive).

For each value it calls the **grouping method** given with the `&group=` query parameter
(the default is `average`).

## Grouping methods

The following grouping methods are supported. These are given all the values in the time-frame
and they group the values every `group points`.

Identifier(s)|Description|Live 1-minute Example
:---:|:---:|:--------
`min`|finds the minimum value|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min)
`max`|finds the maximum value|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max)
`average`, `mean`|finds the average value|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average)
`sum`|adds all the values and returns the sum|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=sum&after=-60&label=1m+sum&units=requests)
`median`|sorts the values and returns the value in the middle of the list|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=median&after=-60&label=median)
`stddev`|finds the standard deviation of the values|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=stddev&after=-60&label=stddev)
`cv`, `rds`|finds the relative standard deviation of the values|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=cv&after=-60&label=cv&units=pcent)
`ses`, `ema`, `ewma`|finds the exponential weighted moving average of the values|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=ses&after=-60&label=ses)
`des`|applies Holt-Winters double exponential smoothing|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=des&after=-60&label=des)
`incremental_sum`,<br/>`incremental-sum`|find the difference of the last vs the first values|![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=incremental_sum&after=-60&label=inc.+sum)

The examples shown above, are live information from the `successful` web requests of the global netdata registry.

## Further processing

The result of the query engine is always a structure that has dimensions and values
for each dimension.

Formatting modules are then used to convert this result in many different formats and return it
to the caller.

## Performance

The query engine is highly optimized for speed. Most of its modules implement "online"
versions of the algorithms, requiring just one pass on the database values to produce
the result.
