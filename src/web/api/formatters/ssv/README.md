# SSV formatter

The SSV formatter sums all dimensions in [results of database queries](/src/web/api/queries/README.md)
to a single value and returns a list of such values showing how it changes through time.

It supports the following formats:

| format     | content type     | description                      |
|:----:|:----------:|:----------|
| `ssv`      | text/plain       | a space separated list of values |
| `ssvcomma` | text/plain       | a comma separated list of values |
| `array`    | application/json | a JSON array                     |

The SSV formatter respects the following API `&options=`:

| option    | supported | description                                                                         |
| :----:|:-------:|:----------|
| `nonzero` | yes       | to return only the dimensions that have at least a non-zero value                   |
| `flip`    | yes       | to return the numbers older to newer (the default is newer to older)                |
| `percent` | yes       | to replace all values with their percentage over the row total                      |
| `abs`     | yes       | to turn all values positive, before using them                                      |
| `min2max` | yes       | to return the delta from the minimum value to the maximum value (across dimensions) |

## Examples

Get the average system CPU utilization of the last hour, in 6 values (one every 10 minutes):

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=system.cpu&format=ssv&after=-3600&points=6&group=average'
1.741352 1.6800467 1.769411 1.6761112 1.629862 1.6807968
```

---

Get the total mysql bandwidth (in + out) for the last hour, in 6 values (one every 10 minutes):

Netdata returns bandwidth in `kilobits`.

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=mysql_local.net&format=ssvcomma&after=-3600&points=6&group=sum&options=abs'
72618.7936215,72618.778889,72618.788084,72618.9195918,72618.7760612,72618.6712421
```

---

Get the web server max connections for the last hour, in 12 values (one every 5 minutes)
in a JSON array:

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&format=array&after=-3600&points=12&group=max'
[278,258,268,239,259,260,243,266,278,318,264,258]
```


