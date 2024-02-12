<!--
title: "Helper Packages"
custom_edit_url: "https://github.com/netdata/go.d.plugin/edit/master/pkg/README.md"
sidebar_label: "Helper Packages"
learn_status: "Published"
learn_rel_path: "Developers/External plugins/go.d.plugin/Helper Packages"
-->

# Helper Packages

- if you need IP ranges consider to
  use [`iprange`](https://github.com/netdata/go.d.plugin/blob/master/pkg/iprange/README.md).
- if you parse an application log files, then [`log`](https://github.com/netdata/go.d.plugin/tree/master/pkg/logs) is
  handy.
- if you need filtering
  check [`matcher`](https://github.com/netdata/go.d.plugin/blob/master/pkg/matcher/README.md).
- if you collect metrics from an HTTP endpoint use [`web`](https://github.com/netdata/go.d.plugin/tree/master/pkg/web).
- if you collect metrics from a prometheus endpoint,
  then [`prometheus`](https://github.com/netdata/go.d.plugin/tree/master/pkg/prometheus)
  and [`web`](https://github.com/netdata/go.d.plugin/blob/master/pkg/web/README.md) is what you need.
- [`tlscfg`](https://github.com/netdata/go.d.plugin/blob/master/pkg/tlscfg/README.md) provides TLS support.
- [`stm`](https://github.com/netdata/go.d.plugin/blob/master/pkg/stm/README.md) helps you to convert any struct to a `map[string]int64`.
