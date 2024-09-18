<!--
title: "Helper Packages"
custom_edit_url: "/src/go/plugin/go.d/pkg/README.md"
sidebar_label: "Helper Packages"
learn_status: "Published"
learn_rel_path: "Developers/External plugins/go.d.plugin/Helper Packages"
-->

# Helper Packages

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
