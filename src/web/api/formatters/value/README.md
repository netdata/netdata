# Value formatter

The Value formatter reduces one row of a [database query result](/src/web/api/queries/README.md) to a single numeric value.
It is the common dimension-reduction step used when a caller needs one number instead of one value per selected dimension.

By default, it sums all non-empty, exposed dimensions in the row. Dimensions filtered out by the query or hidden by output
options do not contribute. If every selected dimension is empty, the result is empty unless `null2zero` is requested.

Time grouping and dimension reduction are separate operations. The query engine first reads the requested time range and
applies the selected grouping method, such as `average`, `min`, or `max`, to produce result rows. The Value formatter then
combines the dimensions in one of those rows. Consequently, `group=max` finds the maximum over time for each dimension,
while the formatter's `max` option selects the largest value across dimensions in the resulting row.

## Dimension reduction options

The Value formatter respects the following API `&options=`:

| option      | behavior |
|:------------|:---------|
| `percent`   | Convert dimension values to their percentage of the row total before reducing them. |
| `abs`       | Make values positive before reducing them. This is useful for charts that display one direction as negative. |
| `average`   | Return the arithmetic mean across the non-empty dimensions instead of their sum. |
| `min`       | Return the smallest non-empty dimension value. |
| `max`       | Return the largest non-empty dimension value. |
| `min2max`   | Return the difference between the largest and smallest dimension values. This legacy option is end-of-life; new API v2 consumers should control dimension aggregation explicitly. |
| `null2zero` | Return zero instead of an empty value when the reduced result is not a finite number. |

Only one of `average`, `min`, `max`, or `min2max` should be used for dimension reduction. Without one of them, summation is
the default.

## Example

Assume one query row contains three exposed dimension values: `10`, `20`, and `-5`.

| options | result | reason |
|:--------|-------:|:-------|
| none | `25` | Sum all three values. |
| `abs` | `35` | Convert `-5` to `5`, then sum. |
| `average` | approximately `8.33` | Divide the sum by three dimensions. |
| `min` | `-5` | Select the smallest dimension. |
| `max` | `20` | Select the largest dimension. |
| `min2max` | `25` | Subtract `-5` from `20`. |

The units remain the units of the queried chart unless an earlier query option, such as percentage conversion, changes the
meaning of the values.

## Where it is used

The Value formatter is not exposed by the API by itself.
Instead, its reduction logic is used by the [`ssv`](/src/web/api/formatters/ssv/README.md) formatter, health and alert
lookups, and internal query paths that request one value. SSV applies the same reduction to every result row and emits the
resulting sequence as space-separated values, comma-separated values, or a JSON array. Health lookups normally ask the query
engine for one grouped point over the alert's lookup window and then use this formatter to collapse its selected dimensions.

Choose dimensions deliberately. Summing dimensions is meaningful for additive values such as traffic directions after
`abs`, but it may be misleading for unrelated dimensions or percentages. When the dimensions are not additive, select one
dimension in the query or use an explicit reduction option whose meaning matches the alert or integration.

