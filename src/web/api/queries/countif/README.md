# Percentage of samples

> This query is available as `percentage-of-samples`, and as `countif`, its
> historical name.

It returns the percentage (0 to 100) of the samples in the time-frame that
satisfied a condition.

The condition is an operator followed by a value, given in the
`group_options` query parameter:

| operator | meaning |
|---|---|
| `!` or `!=` or `<>` | different than |
| `=` or `==` or `:` | equal to (also what a bare value means) |
| `>` | greater than |
| `>=` or `>:` | greater than or equal to |
| `<` | less than |
| `<=` or `<:` | less than or equal to |

The value is one of:

- a **number**, e.g. `!0` matches anything except zero, `>=-3` matches
  anything from -3 upwards.
- a **gap token** - `gap`, `nan`, `null` or `empty` are synonyms for "no data
  was collected here". `==gap` matches the uncollected slots and `!=gap` the
  collected ones. Naming a gap token is what makes gaps count at all;
  without one they are invisible, as they are to every other aggregation.
- the **previous collected sample** - `previous` or `last`, e.g. `<previous`
  matches every sample lower than the one before it, which is what counts
  counter resets. Gaps are skipped, so a drop across a gap still counts, and
  the first sample of a query never matches.

There are no `and`/`or` compounds. An unreadable condition compares equal to
zero rather than failing the query, so check the condition when a result
looks wrong. The same grammar is used by `percentage-of-time`,
`number-of-flaps` and `number-of-times`, and by the `lookup` line of an
[alert](/src/health/REFERENCE.md), which rejects an unreadable condition
instead of accepting it.

Over a window long enough to be served from lower-resolution data this
grouping evaluates each STORED point as one sample, rather than reasoning
about the samples behind it. `percentage-of-time` is the one that estimates
across a stored window; see
[Accuracy over long windows](/src/web/api/queries/README.md#accuracy-over-long-windows).

## how to use

This query is available in alerts, e.g. `lookup: percentage-of-samples(>10) -5m`.

`countif` is an alias of `percentage-of-samples`, which is the canonical name; both behave identically. It changes the units of charts. The result of the calculation is always from 0 to 100, expressing the percentage of database points that matched the condition.

In APIs and badges can be used like this: `&group=countif&group_options=>10` in the URL.


