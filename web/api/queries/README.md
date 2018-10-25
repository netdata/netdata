# Database Queries

Netdata database can be queried with `/api/v1/data` and `/api/v1/badge.svg` API methods.

Every data query accepts the following parameters:

name|description
:----:|:----:
`chart`|The chart to be queried.
`points`|The number of points to be returned. Netdata can reduce number of points by applying query grouping methods.
`before`|The absolute timestamp or the relative (to now) time the query should finish evaluating data.
`after`|The absolute timestamp or the relative (to `before`) time the query should start evaluating data.
`group`|The grouping method to use when reducing the points the database has.
`gtime`|A resampling period to change the units of the metrics (i.e. setting this to `60` will convert `per second` metrics to `per minute`.
`options`|A bitmap of options that can affect the operation of the query. Only 2 options are used by the query engine: `unaligned` and `percentage`. All the other options are used by the output formatters.
`dimensions`|A simple pattern to filter the dimensions to be queried.

## Operation

The query engine works as follows (in this order):

1. **Identify the exact time-frame required, in absolute timestamps.**

   `after` and `before` define a time-frame:
   
   - in **absolute timestamps** (unix timestamps, i.e. seconds since epoch).
   
   - in **relative timestamps**:
   
      `before` is relative to now and `after` is relative to `before`.
      
      So, `before=-60&after=-60` evaluates to the time-frame from -120 up to -60 seconds in
      the past, relative to now. 

   At the end of this operation, `after` and `before` are absolute timestamps.
   The engine verifies that the time-frame is available at the database. If it is not,
   it will adjust `after` and `before` accordingly so that usable data can be returned,
   or no data at all if the time-frame is entirely outside the current range of the
   database.

2. **Identify the grouping of database points required.**

   Grouping database points is used when the caller requests a longer time-frame to be
   expressed with fewer points, compared to what is available at the database.
   
   There are 2 uses of this (that can be combined):
   
   - The caller requests a specific number of `points` to be returned.
      
      For example, for a time-frame of 10 minutes, the database has 600 points (1/sec),
      while the caller requested these 10 minutes to be expressed in 200 points.
      
      This feature is used by netdata dashboards when you zoom-out the charts.
      The dashboard is requesting the number of points the user's screen has, and netdata
      returns that many points to perfectly match the screen. This saves bandwidth
      and makes drawing the charts a lot faster.
      
   - The caller requests a **re-sampling** of the database, by setting `gtime` to any value
      above `1`. For example, the database maintains the metrics in the form of `X/sec`
      but the caller set `gtime=60` to get `X/min`.
      
   Using the above information the query engine tries to find a best fit for database-points
   to result-points ratio (we call this `group points`). It always tries to keep `group points`
   an integer. Keep in mind the query engine may alter a bit `after` if required. So, the engine
   may decide to shift the starting point of the time-frame to keep the query optimal.
   
3. **Align the time-frame.**

   Alignment is a very important aspect of netdata queries. Without it, the animated
   charts on the dashboards would constantly change shape during incremental updates.
   To provide consistent grouping of all points, the query engine (by default) aligns
   `after` and `before` to be a multiple of `group points`.
   
   For example, if `group points` is 60 and alignment is enabled, the engine will return
   each point with durations XX:XX:00 - XX:XX:59 matching minutes. Of course, depending
   on the database granularity for the specific chart and the requested points to be
   returned, the engine may use any integer number for `group points`.
   
   To disable alignment, pass `&options=unaligned` to the query.
   
4. **Execute the query**

   To execute the query, the engine evaluates all dimensions of the chart, one after another.
   The engine will not evaluate dimensions that do not match the simple pattern given at
   the `dimensions` parameter, except when `options=percentage` is given (this option requires
   all the dimensions to be evaluated to find the percentage of each dimension vs to chart
   total).
   
   For each dimension, it starts evaluating values from `after` towards `before`.
   For each value it calls the **grouping method** specified (the default is `average`).

## Grouping methods

The following grouping methods are supported. These are given all the values in the time-frame
and they group the values every `group points`.

name|identifier(s)|description
:---:|:---:|:---:
Min|`min`|finds the minimum value
Max|`max`|finds the maximum value
Average|`average` `mean`|finds the average value
Sum|`sum`|adds all the values and returns the sum
Median|`median`|sorts the values and returns the value in the middle of the list
Standard Deviation|`stddev`|finds the standard deviation of the values
Coefficient of Variation|`cv` `rds`|finds the relative standard deviation of the values
Single Exponential Smoothing|`ses` `ema` `ewma`|finds the exponential weighted moving average of the values
Double Exponential Smoothing|`des`|applies Holt-Winters double exponential smoothing
Incremental Sum|`incremental_sum` `incremental-sum`|find the difference of the last vs the first values

## Further processing

The result of the query engine is always a structure that has dimensions and values
for each dimension.

Formatting modules are then used to convert this result in many different formats and return it
to the caller.

## Performance

The query engine is highly optimized for speed. Most of its modules implement "online"
versions of the algorithms, requiring just one pass on the database values to produce
the result.


