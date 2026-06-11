// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	Name               string         `yaml:"name,omitempty" json:"name"`
	Application        string         `yaml:"app,omitempty" json:"app"`
	Selector           selector.Expr  `yaml:"selector,omitempty" json:"selector"`
	Relabeling         []RelabelBlock `yaml:"relabeling,omitempty" json:"relabeling,omitempty"`
	ExpectedPrefix     string         `yaml:"expected_prefix,omitempty" json:"expected_prefix"`
	MaxTS              int            `yaml:"max_time_series" json:"max_time_series"`
	MaxTSPerMetric     int            `yaml:"max_time_series_per_metric" json:"max_time_series_per_metric"`
	FallbackType       struct {
		Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
		Counter []string `yaml:"counter,omitempty" json:"counter"`
	} `yaml:"fallback_type,omitempty" json:"fallback_type"`
}

// RelabelBlock is one job-level metric-relabeling block: a required metric-name
// match that scopes a list of Prometheus relabel rules to a metric-name subset.
// Use match "*" to target every metric. Rules run pre-assembly on the sample
// stream, in block order.
type RelabelBlock struct {
	Match                string           `yaml:"match" json:"match"`
	MetricRelabelConfigs []relabel.Config `yaml:"metric_relabel_configs" json:"metric_relabel_configs"`
}
