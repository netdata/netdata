// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type collectorRuntime struct {
	Profiles          []*profileRuntime
	ChartTemplateYAML string
	Instruments       map[string]*instrumentRuntime
}

type profileRuntime struct {
	ID              string
	Name            string
	ResourceType    string
	MetricNamespace string
	Metrics         []*metricRuntime
	Template        charttpl.Group
}

type metricRuntime struct {
	ID             string
	AzureName      string
	TimeGrain      string
	TimeGrainEvery time.Duration
	Series         []*seriesRuntime
}

type seriesRuntime struct {
	Aggregation string
	Kind        string
	Instrument  string
}

type instrumentRuntime struct {
	Kind    string
	Gauge   metrix.SnapshotGaugeVec
	Counter metrix.SnapshotCounterVec
}

func (i *instrumentRuntime) observe(labelValues []string, value float64) {
	if i == nil {
		return
	}
	switch i.Kind {
	case azureprofiles.SeriesKindCounter:
		i.Counter.WithLabelValues(labelValues...).ObserveTotal(value)
	default:
		i.Gauge.WithLabelValues(labelValues...).Observe(value)
	}
}

type discoveryState struct {
	Resources    []resourceInfo
	ByType       map[string][]resourceInfo
	ExpiresAt    time.Time
	FetchedAt    time.Time
	FetchCounter uint64
}

type resourceInfo struct {
	ID            string
	UID           string
	Name          string
	Type          string
	ResourceGroup string
	Region        string
}

func (r resourceInfo) String() string {
	return r.Name + " (" + r.Type + ")"
}

type queryBatch struct {
	Profile        *profileRuntime
	Metrics        []*metricRuntime
	MetricNames    []string
	Aggregations   []string
	TimeGrain      string
	TimeGrainEvery time.Duration
	Region         string
	Resources      []resourceInfo
}

type metricSample struct {
	Instrument string
	Kind       string
	Labels     metrix.Labels
	Value      float64
}

type queryBatchResult struct {
	Samples []metricSample
	Err     error
}

type lastObservation struct {
	instrument  string
	labelValues []string
	value       float64
}
