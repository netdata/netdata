# CountIf

> This query is available as `countif`.

CountIf returns the percentage of points in the database that satisfy the condition supplied.

The following conditions are available:

- `!` or `!=` or `<>`, different than
- `=` or `==` or `:`, equal to
- `>`, greater than
- `<`, less than
- `>=`, greater or equal to
- `<=`, less or equal to

The target number and the desired condition can be set using the `group_options` query parameter, as a string, like in these examples:

- `!0`, to match any number except zero.
- `>=-3` to match any number bigger or equal to -3.

An invalid condition is rejected with an API error rather than returning an inaccurate response.

## How to use

CountIf can be used in APIs and badges, and in health lookup expressions.

For a non-empty group, `countif` returns the percentage of database points that matched the condition, from zero to 100.
For example, with one or more observed values, the health lookup `countif(!=1) -15m unaligned of value` returns zero only
when every observed value in the 15-minute lookup is exactly `1`.

In APIs and badges, use `&group=countif&group_options=>10` in the URL.
