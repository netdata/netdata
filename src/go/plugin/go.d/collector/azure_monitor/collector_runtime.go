// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type collectorRuntime struct {
	Profiles               []*profileRuntime
	ChartTemplateYAML      string
	WorkloadResourceTagKey string
}

type profileRuntime struct {
	Name            string
	DisplayName     string
	ResourceType    string
	MetricNamespace string
	Filters         *ResourceFiltersConfig
	Metrics         []*metricRuntime
	ResourceLabels  []azureprofiles.ResourceLabel
	ResourceStates  []*resourceStateRuntime
	Template        charttpl.Group
}

type resourceStateRuntime struct {
	Name     string
	Property string
	States   []string
	Values   map[string]string
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

type discoveryState struct {
	Resources              []resourceInfo
	ByType                 map[string][]resourceInfo
	ByProfile              map[string][]resourceInfo
	ExpiresAt              time.Time
	FetchedAt              time.Time
	FetchCounter           uint64
	QueryTagsColumnMissing bool
	QueryTagsWrongShape    bool
	UnsafeWorkloadValues   map[string]int
}

type resourceTag struct {
	Key   string
	Value string
}

type resourceInfo struct {
	SubscriptionID string
	ID             string
	UID            string
	Name           string
	Type           string
	ResourceGroup  string
	Region         string
	Tags           []resourceTag
	Properties     map[string]string
	HostScope      metrix.HostScope
}

func (r resourceInfo) String() string {
	return r.Name + " (" + r.Type + ")"
}

type queryBatch struct {
	SubscriptionID string
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
	Resource   resourceIdentity
	Value      float64
}

type queryBatchResult struct {
	Samples []metricSample
	Err     error
}
