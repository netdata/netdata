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
an integer. Keep in mind the query engine may shift `after` if required. See also the [example](#example).

#### Time-frame Alignment

Alignment is a very important aspect of netdata queries. Without it, the animated
charts on the dashboards would constantly [change shape](#example) during incremental updates.

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

- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=min&after=-60&label=min&value_color=blue) finds the minimum value
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=max&after=-60&label=max&value_color=lightblue) finds the maximum value
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=average&after=-60&label=average&value_color=yellow) finds the average value
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=sum&after=-60&label=sum&units=requests&value_color=orange) adds all the values and returns the sum
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=median&after=-60&label=median&value_color=red) sorts the values and returns the value in the middle of the list
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=stddev&after=-60&label=stddev&value_color=green) finds the standard deviation of the values
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=cv&after=-60&label=cv&units=pcent&value_color=yellow) finds the relative standard deviation (coefficient of variation) of the values
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=ses&after=-60&label=ses&value_color=brown) finds the exponential weighted moving average of the values
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=des&after=-60&label=des&value_color=blue) applies Holt-Winters double exponential smoothing
- ![](https://registry.my-netdata.io/api/v1/badge.svg?chart=web_log_nginx.response_statuses&options=unaligned&dimensions=success&group=incremental_sum&after=-60&label=incremental_sum&value_color=red) finds the difference of the last vs the first value

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

## Example

When netdata is reducing metrics, it tries to return always the same boundaries. So, if we want 10s averages, it will always return points starting at a `unix timestamp % 10 = 0`.

Let's see why this is needed, by looking at the error case.

Assume we have 5 points:

| time | value |
| :-: | :-: |
| 00:01 | 1 |
| 00:02 | 2 |
| 00:03 | 3 |
| 00:04 | 4 |
| 00:05 | 5 |

At 00:04 you ask for 2 points for 4 seconds in the past. So `group = 2`. netdata would return:

| point | time | value |
| :-: | :-: | :-: |
| 1 | 00:01 - 00:02 | 1.5 |
| 2 | 00:03 - 00:04 | 3.5 |

A second later the chart is to be refreshed, and makes again the same request at 00:05. These are the points that would have been returned:

| point | time | value |
| :-: | :-: | :-: |
| 1 | 00:02 - 00:03 | 2.5 |
| 2 | 00:04 - 00:05 | 4.5 |

**Wait a moment!** The chart was shifted just one point and it changed value! Point 2 was 3.5 and when shifted to point 1 is 2.5! If you see this in a chart, it's a mess. The charts change shape constantly.

For this reason, netdata always aligns the data it returns to the `group`.

When you request `points=1`, netdata understands that you need 1 point for the whole database, so `group = 3600`. Then it tries to find the starting point which would be `timestamp % 3600 = 0` Within a database of 3600 seconds, there is one such point for sure. Then it tries to find the average of 3600 points. But, most probably it will not find 3600 of them (for just 1 out of 3600 seconds this query will return something).

So, the proper way to query the database is to also set at least `after`. The following call will returns 1 point for the last complete 10-second duration (it starts at `timestamp % 10 = 0`):

http://netdata.firehol.org/api/v1/data?chart=system.cpu&points=1&after=-10&options=seconds

When you keep calling this URL, you will see that it returns one new value every 10 seconds, and the timestamp always ends with zero. Similarly, if you say `points=1&after=-5` it will always return timestamps ending with 0 or 5.
