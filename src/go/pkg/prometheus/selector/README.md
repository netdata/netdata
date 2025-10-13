# Time series selector

Selectors allow selecting and filtering of a set of time series.

## Simple Selector

In the simplest form you need to specify only a metric name.

### Syntax

```cmd
 <line>                 ::= <metric_name_pattern>
 <metric_name_pattern>  ::= simple pattern
```

The metric name pattern syntax is [simple pattern](/src/libnetdata/simple_pattern/README.md).

### Examples

This example selects all time series that have the `go_memstats_alloc_bytes` metric name:

```cmd
go_memstats_alloc_bytes
```

This example selects all time series with metric names starts with `go_memstats_`:

```cmd
go_memstats_*
```

This example selects all time series with metric names starts with `go_` except `go_memstats_`:

```cmd
!go_memstats_* go_*
``` 

## Advanced Selector

It is possible to filter these time series further by appending a comma separated list of label matchers in curly braces (`{}`).

### Syntax

```cmd
 <line>                 ::= [ <metric_name_pattern> ]{ <list_of_selectors> }
 <metric_name_pattern>  ::= simple pattern
 <list_of_selectors>    ::= a comma separated list <label_name><op><label_value_pattern>
 <label_name>           ::= an exact label name
 <op>                   ::= [ '=', '!=', '=~', '!~', '=*', '!*' ]
 <label_value_pattern>  ::= a label value pattern, depends on <op>
```

The metric name pattern syntax is [simple pattern](/src/libnetdata/simple_pattern/README.md).

Label matching operators:

-   `=`: Match labels that are exactly equal to the provided string.
-   `!=`: Match labels that are not equal to the provided string.
-   `=~`: Match labels that [regex-match](https://golang.org/pkg/regexp/syntax/) the provided string.
-   `!~`: Match labels that do not [regex-match](https://golang.org/pkg/regexp/syntax/) the provided string.
-   `=*`: Match labels that [simple-pattern-match](/src/libnetdata/simple_pattern/README.md) the provided string.
-   `!*`: Match labels that do not [simple-pattern-match](/src/libnetdata/simple_pattern/README.md) the provided string.

### Examples

This example selects all time series that:

-   have the `node_cooling_device_cur_state` metric name and
-   label `type` value not equal to `Fan`:

```cmd
node_cooling_device_cur_state{type!="Fan"}
```

This example selects all time series that:

-   have the `node_filesystem_size_bytes` metric name and
-   label `device` value is either `/dev/nvme0n1p1` or `/dev/nvme0n1p2` and
-   label `fstype` is equal to `ext4`

```cmd
node_filesystem_size_bytes{device=~"/dev/nvme0n1p1$|/dev/nvme0n1p2$",fstype="ext4"}
```

Label matchers can also be applied to metric names by matching against the internal `__name__` label.

For example, the expression `node_filesystem_size_bytes` is equivalent to `{__name__="node_filesystem_size_bytes"}`.
This allows using all operators (other than `=*`) for metric names matching.

The following expression selects all metrics that have a name starting with `node_`:

```cmd
{__name__=*"node_*"}
```
