// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

var supportedTimeGrains = map[string]time.Duration{
	"PT1M":  time.Minute,
	"PT5M":  5 * time.Minute,
	"PT15M": 15 * time.Minute,
	"PT30M": 30 * time.Minute,
	"PT1H":  time.Hour,
}

type resourceGraphClient interface {
	Resources(ctx context.Context, query armresourcegraph.QueryRequest, options *armresourcegraph.ClientResourcesOptions) (armresourcegraph.ClientResourcesResponse, error)
}

type metricsQueryClient interface {
	QueryResources(ctx context.Context, subscriptionID string, metricNamespace string, metricNames []string, resourceIDs azmetrics.ResourceIDList, options *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error)
}

type armClientOptions struct {
	Cloud azcloud.Configuration
}

func (a armClientOptions) toARM() *arm.ClientOptions {
	return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Cloud: a.Cloud}}
}

type syncMapMutex struct {
	sync.Mutex
}

type runtimePlan struct {
	Profiles          []*profileRuntime
	ChartTemplateYAML string
	Instruments       map[string]metrix.SnapshotGaugeVec
}

type profileRuntime struct {
	Key             string
	Name            string
	ResourceType    string
	MetricNamespace string
	Metrics         []*metricRuntime
	Charts          []charttpl.Chart
}

type metricRuntime struct {
	Name            string
	Units           string
	TimeGrain       string
	TimeGrainEvery  time.Duration
	Aggregations    []string
	InstrumentByAgg map[string]string
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

type queryTask struct {
	Profile        *profileRuntime
	MetricSubset   []*metricRuntime
	MetricNames    []string
	Aggregations   []string
	TimeGrain      string
	TimeGrainEvery time.Duration
	Region         string
	Resources      []resourceInfo
}

type metricSample struct {
	Instrument  string
	Labels      metrix.Labels
	Value       float64
	Aggregation string
}

type taskResult struct {
	Samples []metricSample
	Err     error
}

func stringsLowerTrim(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func stringsTrim(v string) string {
	return strings.TrimSpace(v)
}

func hashShort(v string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(v))))
	return hex.EncodeToString(sum[:6])
}

func encodeIDPart(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "na"
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			continue
		}
		b.WriteString(fmt.Sprintf("_%02x", c))
	}
	return b.String()
}

func cloudConfigFromName(name string) (azcloud.Configuration, error) {
	switch stringsLowerTrim(name) {
	case cloudPublic, "":
		return azcloud.AzurePublic, nil
	case cloudGovernment:
		return azcloud.AzureGovernment, nil
	case cloudChina:
		return azcloud.AzureChina, nil
	default:
		return azcloud.Configuration{}, fmt.Errorf("unsupported cloud %q", name)
	}
}
