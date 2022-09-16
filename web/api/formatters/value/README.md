<!--
title: "Value formatter"
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/api/formatters/value/README.md
-->

# Value formatter

The Value formatter presents [results of database queries](/web/api/queries/README.md) as a single value.

To calculate the single value to be returned, it sums the values of all dimensions.

The Value formatter respects the following API `&options=`:

| option    | supported | description |
|:----:     |:-------:  |:----------  |
| `percent` | yes       | to replace all values with their percentage over the row total|
| `abs`     | yes       | to turn all values positive, before using them |
| `min2max` | yes       | to return the delta from the minimum value to the maximum value (across dimensions)|

The Value formatter is not exposed by the API by itself.
Instead it is used by the [`ssv`](/web/api/formatters/ssv/README.md) formatter
and [health monitoring queries](/health/README.md).


