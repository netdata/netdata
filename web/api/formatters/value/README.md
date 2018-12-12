# Value formatter

The Value formatter presents [results of database queries](../../queries) as a single value.

To calculate the single value to be returned, it sums the values of all dimensions.

The Value formatter respects the following API `&options=`:

option|supported|description
:---:|:---:|:---
`percent`|yes|to replace all values with their percentage over the row total
`abs`|yes|to turn all values positive, before using them
`min2max`|yes|to return the delta from the minimum value to the maximum value (across dimensions)

The Value formatter is not exposed by the API by itself.
Instead it is used by the [`ssv`](../ssv) formatter
and [health monitoring queries](../../../../health).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fformatters%2Fvalue%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
