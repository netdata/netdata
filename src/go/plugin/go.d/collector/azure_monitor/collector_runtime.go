// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
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
	namespaceKey    string
	Filters         *ResourceFiltersConfig
	chunks          []*metricChunk
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

type metricQueryGroup struct {
	TimeGrain   string
	Aggregation string
}

// Compiled once per profile. Masks are local to this immutable metric ordering.
type metricChunk struct {
	profile *profileRuntime
	metricQueryGroup
	metrics []*metricRuntime
	names   []string
}

// A query series requests exactly one aggregation of one Azure metric.
type metricRuntime struct {
	ID          string
	AzureName   string
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
	idKey          string
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

// Limit examples in each diagnostic category independently of inventory size.
const diagnosticExampleLimit = 3

func (r resourceInfo) diagnosticName() string {
	return fmt.Sprintf("%.80q[%s]", r.Name, r.UID)
}

type queryBatch struct {
	chunk          *metricChunk
	mask           uint32 // Selected names in the original metric chunk.
	SubscriptionID string
	Profile        *profileRuntime
	Metrics        []*metricRuntime
	MetricNames    []string
	Aggregation    string
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
	incompatible map[string]uint32
	metricErrors map[string]uint32
	Diagnostics  *batchDiagnostics
	Samples      []metricSample
	Err          error
}

func (r resourceInfo) definitionKey(profile *profileRuntime) definitionKey {
	return definitionKey{r.idKey, profile.namespaceKey}
}
