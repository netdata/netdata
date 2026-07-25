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

. When an invalid condition is given, the web server can deliver a not accurate response.

## how to use

This query is available in alerts, e.g. `lookup: percentage-of-samples(>10) -5m`.

`countif` is an alias of `percentage-of-samples`, which is the canonical name; both behave identically. It changes the units of charts. The result of the calculation is always from 0 to 100, expressing the percentage of database points that matched the condition. 

In APIs and badges can be used like this: `&group=countif&group_options=>10` in the URL.


