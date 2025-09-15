# Helper Packages

- if you need to run an external command, please use [`ndexec`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/ndexec).
- if you need IP ranges consider to
  use [`iprange`](/src/go/plugin/go.d/pkg/iprange).
- if you parse an application log files, then [`log`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/logs) is
  handy.
- if you need filtering
  check [`matcher`](/src/go/pkg/matcher).
- if you collect metrics from an HTTP endpoint use [`web`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/web).
- if you collect metrics from a prometheus endpoint,
  then [`prometheus`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/prometheus)
  and [`web`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/web) is what you need.
- [`tlscfg`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/tlscfg) provides TLS support.
- [`stm`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg/stm) helps you to convert any struct to a `map[string]int64`.
