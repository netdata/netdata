// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricDefinitionsDoesNotRegisterProviders(t *testing.T) {
	const resource = "/subscriptions/00000000-0000-4000-8000-000000000001/resourceGroups/rg/providers/Microsoft.Cdn/profiles/edge"
	var requests []string
	client, err := newMetricDefinitionsClient(new(fake.TokenCredential), azcore.ClientOptions{
		Transport: azureTestTransport(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.Method+" "+req.URL.Path)
			if req.Method == http.MethodGet {
				return recoveryWireError(req, http.StatusConflict, "MissingSubscriptionRegistration")
			}
			return recoveryWireError(req, http.StatusForbidden, "AuthorizationFailed")
		}),
	})
	require.NoError(t, err)
	_, err = client.list(t.Context(), definitionKey{resourceID: resource, namespace: recoveryTestNamespace})
	var response *azcore.ResponseError
	require.ErrorAs(t, err, &response)
	assert.Equal(t, "MissingSubscriptionRegistration", response.ErrorCode)
	assert.Equal(t, []string{"GET " + resource + "/providers/Microsoft.Insights/metricDefinitions"}, requests)
}

func TestCollectRequestsOnlyCompatibleAggregations(t *testing.T) {
	rg := &mockResourceGraph{resources: []map[string]any{testResource("Microsoft.Cdn/profiles", "edge", nil)}}
	mx := &mockMetricsClient{}
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	catalog, err := c.loadProfileCatalog()
	require.NoError(t, err)
	profiles, err := catalog.ResolveBaseNames([]string{"front_door"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	type metricAggregation struct{ name, aggregation string }
	var want []metricAggregation
	for _, metric := range profiles[0].Metrics {
		for _, series := range metric.Series {
			want = append(want, metricAggregation{metric.AzureName, series.Aggregation})
		}
	}

	require.NoError(t, c.Init(t.Context()))
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	var seen []metricAggregation
	for _, call := range mx.queryCalls() {
		require.NotEmpty(t, call.Aggregation)
		require.False(t, strings.Contains(call.Aggregation, ","), "request mixes aggregations: %v", call)
		for _, name := range call.MetricNames {
			seen = append(seen, metricAggregation{name, call.Aggregation})
		}
	}
	assert.ElementsMatch(t, want, seen)
}

type azureTestTransport func(*http.Request) (*http.Response, error)

func (f azureTestTransport) Do(req *http.Request) (*http.Response, error) { return f(req) }

func azureTestResponse(req *http.Request, status int, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: req}, nil
}

func TestCollectRecoversPerResourceAndAggregation(t *testing.T) {
	const namespace = "Microsoft.Web/serverfarms"
	a, b := testResource(namespace, "plan-basic", nil), testResource(namespace, "plan-premium", nil)
	idA, idB := a["id"].(string), b["id"].(string)
	rg := &mockResourceGraph{resources: []map[string]any{a, b}}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Both resources support CPU average. Only the second supports CPU maximum
	// until the SKU change. Both reject other names in the stock profile.
	var upgraded bool
	var mu sync.Mutex
	reads := map[string]int{}
	rejections := 0
	calls := 0
	transport := azureTestTransport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		q := req.URL.Query()
		assert.Equal(t, namespace, q.Get("metricnamespace"))
		switch req.Method {
		case http.MethodGet:
			assert.Equal(t, "management.azure.com", req.URL.Host)
			assert.Equal(t, "2023-10-01", q.Get("api-version"))
			id, ok := strings.CutSuffix(req.URL.Path, "/providers/Microsoft.Insights/metricDefinitions")
			assert.True(t, ok)
			assert.Contains(t, []string{idA, idB}, id)
			reads[id]++
			aggs := []string{"Average"}
			if id == idB || upgraded {
				aggs = append(aggs, "Maximum")
			}
			definition := map[string]any{
				"name":                      map[string]string{"value": "cpuPercentage"},
				"supportedAggregationTypes": aggs,
				"metricAvailabilities":      []any{map[string]string{"timeGrain": "PT1M"}},
			}
			return azureTestResponse(req, 200, map[string]any{"value": []any{definition}})
		case http.MethodPost:
			calls++
			assert.Equal(t, "eastus.metrics.monitor.azure.com", req.URL.Host)
			assert.Equal(t, now.Add(-4*time.Minute).Format(time.RFC3339), q.Get("starttime"))
			assert.Equal(t, now.Add(-3*time.Minute).Format(time.RFC3339), q.Get("endtime"))
			var body struct{ ResourceIDs []string }
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			names := strings.Split(q.Get("metricnames"), ",")
			agg := q.Get("aggregation")
			if len(names) != 1 || names[0] != "CpuPercentage" || (agg == "maximum" && !upgraded && slices.Contains(body.ResourceIDs, idA)) {
				rejections++
				return azureTestResponse(req, 400, map[string]any{"error": map[string]string{"code": "BadRequest", "message": "metric unavailable"}})
			}
			var result azmetrics.MetricResults
			for _, id := range body.ResourceIDs {
				metric := metricWithAvg(names[0], now, 0)
				if agg == "maximum" {
					metric.TimeSeries[0].Data[0].Average = nil
					metric.TimeSeries[0].Data[0].Maximum = new(7.0)
				}
				result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: []azmetrics.Metric{metric}})
			}
			return azureTestResponse(req, 200, result)
		default:
			return nil, fmt.Errorf("unexpected request method %s", req.Method)
		}
	})
	cred := new(fake.TokenCredential)
	opts := azcore.ClientOptions{Transport: transport}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Discovery.RefreshEvery = 0
	c.Profiles.ModeExact = testProfilesModeEntries("app_service_plan")
	c.now = func() time.Time { return now }
	c.newMetricsClient = func(endpoint string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: opts})
	}
	require.NoError(t, c.Init(t.Context()))
	armClient, err := arm.NewClient("test", "v1.0.0", cred, &arm.ClientOptions{ClientOptions: opts})
	require.NoError(t, err)
	c.queryExecutor.lookupDefinitions = (&metricDefinitionsClient{armClient}).list
	require.NoError(t, c.Check(t.Context()))
	collect := func() map[string]metrix.SampleValue {
		t.Helper()
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		return series
	}
	checkValue := func(series map[string]metrix.SampleValue, name, agg string, want int) {
		t.Helper()
		value, ok := seriesValue(series, "cpu_percentage_"+agg, name)
		require.True(t, ok, "%s %s missing from %v", name, agg, keysFromSeries(series))
		assert.EqualValues(t, want, value)
	}

	series := collect()
	checkValue(series, "plan-basic", "average", 0)
	checkValue(series, "plan-premium", "maximum", 7)
	_, ok := seriesValue(series, "cpu_percentage_maximum", "plan-basic")
	assert.False(t, ok)
	require.Equal(t, map[string]int{idA: 1, idB: 1}, reads)
	require.Positive(t, rejections)
	rejections = 0
	calls = 0
	now = now.Add(time.Minute)
	series = collect()
	checkValue(series, "plan-premium", "maximum", 7)
	assert.LessOrEqual(t, rejections, maxRecoveryMetricsCalls, "still-ambiguous rejected subsets share the supplemental budget")
	assert.Equal(t, 2, calls-rejections, "pack the common average and the premium-only maximum")
	require.Equal(t, map[string]int{idA: 1, idB: 1}, reads)

	// Recheck capabilities even when Resource Graph discovery refresh is disabled.
	upgraded = true
	now = now.Add(time.Hour)
	series = collect()
	checkValue(series, "plan-basic", "maximum", 7)
	require.Equal(t, map[string]int{idA: 2, idB: 2}, reads)
	// A previously observed series that becomes unsupported must become a gap.
	upgraded = false
	now = now.Add(time.Minute)
	series = collect()
	checkValue(series, "plan-premium", "maximum", 7)
	_, ok = seriesValue(series, "cpu_percentage_maximum", "plan-basic")
	assert.False(t, ok)
	now = now.Add(10 * time.Second)
	series = collect()
	_, ok = seriesValue(series, "cpu_percentage_maximum", "plan-basic")
	assert.False(t, ok, "an unsupported series must remain absent between scheduled polls")
	checkValue(series, "plan-premium", "maximum", 7)
}

// A failed definitions lookup is unknown. It cannot exclude metrics or keep a peer with
// complete definitions in the same rejected resource batch.
func TestCollectSeparatesUnreadableMetricDefinitions(t *testing.T) {
	const namespace = "Microsoft.Cdn/profiles"
	a, b := testResource(namespace, "edge-a", nil), testResource(namespace, "edge-b", nil)
	idA, idB := a["id"].(string), b["id"].(string)
	rg := &mockResourceGraph{resources: []map[string]any{a, b}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	now := time.Now()
	c.now = func() time.Time { return now }
	badResource := true
	var mu sync.Mutex
	lookups := map[string]int{}
	var rejectedNames []string
	var readablePeerRequests [][]string
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, options *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			mu.Lock()
			defer mu.Unlock()
			var result azmetrics.QueryResourcesResponse
			if *options.Aggregation == "total" && badResource {
				if slices.Contains(ids, idB) {
					if rejectedNames == nil {
						rejectedNames = slices.Clone(names)
						slices.Sort(rejectedNames)
					}
					return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "BadRequest"}
				}
				if slices.Equal(ids, []string{idA}) {
					requested := slices.Clone(names)
					slices.Sort(requested)
					readablePeerRequests = append(readablePeerRequests, requested)
				}
			}
			for _, id := range ids {
				if slices.Contains(names, "RequestCount") {
					metric := metricWithTotal("RequestCount", now, 5)
					result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: []azmetrics.Metric{metric}})
				}
			}
			return result, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	c.queryExecutor.lookupDefinitions = func(_ context.Context, k definitionKey) (metricDefinitions, error) {
		mu.Lock()
		defer mu.Unlock()
		lookups[k.resourceID]++
		if k.resourceID == idB {
			return nil, &azcore.ResponseError{StatusCode: http.StatusForbidden}
		}
		// All names for this resource are supported; its only difference from
		// the rejected original request must be removal of the unresolved peer.
		defs := metricDefinitions{}
		for _, name := range rejectedNames {
			defs[strings.ToLower(name)] = metricDefinition{}
		}
		return defs, nil
	}
	series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	v, ok := seriesValue(series, ".request_count_total", "edge-a")
	require.True(t, ok)
	assert.EqualValues(t, 5, v)
	_, ok = seriesValue(series, ".request_count_total", "edge-b")
	assert.False(t, ok)
	assert.Equal(t, map[string]int{idA: 1, idB: 1}, lookups)
	require.Greater(t, len(rejectedNames), 1)
	assert.Contains(t, readablePeerRequests, rejectedNames, "recover the readable peer with the original metric names")
	badResource = false
	now = now.Add(time.Minute)
	series, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	v, ok = seriesValue(series, ".request_count_total", "edge-a")
	require.True(t, ok)
	assert.EqualValues(t, 10, v)
	v, ok = seriesValue(series, ".request_count_total", "edge-b")
	require.True(t, ok)
	assert.EqualValues(t, 5, v)
	assert.Equal(t, map[string]int{idA: 1, idB: 2}, lookups, "only unreadable ARM needs its next eligible lookup")
}

func TestCollectInvalidSamplingTypeLeavesGap(t *testing.T) {
	row := testResource("Microsoft.Cdn/profiles", "edge", nil)
	rg := &mockResourceGraph{resources: []map[string]any{row}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	now := time.Now()
	c.now = func() time.Time { return now }
	rejected := false
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, _ *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			var values []azmetrics.Metric
			if slices.Contains(names, "ByteHitRatio") {
				metric := metricWithAvg("ByteHitRatio", now, 3)
				if rejected {
					metric.ErrorCode = new("InvalidSamplingType")
					metric.TimeSeries = nil
				}
				values = append(values, metric)
			}
			if slices.Contains(names, "OriginLatency") {
				values = append(values, metricWithAvg("OriginLatency", now, 1))
			}
			return azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{
				Values: []azmetrics.MetricData{{ResourceID: &ids[0], Values: values}},
			}}, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	c.queryExecutor.lookupDefinitions = func(context.Context, definitionKey) (metricDefinitions, error) {
		return metricDefinitions{"bytehitratio": {}, "originlatency": {}}, nil
	}
	collect := func(wantRatio bool) {
		t.Helper()
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		_, hasRatio := seriesValue(series, "byte_hit_ratio_average", "edge")
		assert.Equal(t, wantRatio, hasRatio)
		_, hasLatency := seriesValue(series, "origin_latency_average", "edge")
		assert.True(t, hasLatency)
	}
	collect(true)
	now = now.Add(time.Minute)
	rejected = true
	collect(false)
	now = now.Add(10 * time.Second)
	collect(false)
	now = now.Add(5*time.Minute - 10*time.Second)
	rejected = false
	collect(true)
}

func TestCollectSlowDefinitionLookupsShareDeadline(t *testing.T) {
	for label, test := range map[string]struct {
		refreshEvery int
		advance      time.Duration
	}{
		"inventory refresh":           {refreshEvery: 60, advance: time.Minute},
		"expiry with fixed inventory": {advance: 5 * time.Minute},
	} {
		t.Run(label, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) { testSlowDefinitionLookups(t, test.refreshEvery, test.advance) })
		})
	}
}

func testSlowDefinitionLookups(t *testing.T, refreshEvery int, advance time.Duration) {
	const namespace = "Microsoft.Cdn/profiles"
	var rows []map[string]any
	for _, name := range []string{"edge-a", "edge-b", "edge-c"} {
		rows = append(rows, testResource(namespace, name, nil))
	}
	rg := &mockResourceGraph{resources: rows}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Discovery.RefreshEvery = refreshEvery
	c.Limits.MaxConcurrency = 1
	c.Timeout = confopt.Duration(100 * time.Millisecond)
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	now := time.Now()
	c.now = func() time.Time { return now }
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			var result azmetrics.QueryResourcesResponse
			if *opts.Aggregation == "total" && len(names) != 1 {
				return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
			}
			for _, id := range ids {
				var metric azmetrics.Metric
				switch {
				case slices.Contains(names, "ByteHitRatio"):
					metric = metricWithAvg("ByteHitRatio", now, 3)
				case slices.Contains(names, "RequestCount"):
					metric = metricWithTotal("RequestCount", now, 5)
				default:
					continue
				}
				result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: []azmetrics.Metric{metric}})
			}
			return result, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	var attempts []string
	stalled := true
	c.queryExecutor.lookupDefinitions = func(ctx context.Context, key definitionKey) (metricDefinitions, error) {
		attempts = append(attempts, key.resourceID)
		if stalled {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return metricDefinitions{"requestcount": {}, "bytehitratio": {}}, nil
	}
	series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, attempts, 1, "queued lookups must share the first lookup's deadline")
	for _, row := range rows {
		_, ok := seriesValue(series, "byte_hit_ratio_average", row["name"].(string))
		assert.True(t, ok, "healthy batch must publish even when recovery times out")
	}
	first := attempts[0]
	previous := make(map[string]float64)
	for _, row := range rows {
		name := row["name"].(string)
		previous[name], _ = seriesValue(series, "request_count_total", name)
	}
	attempts = nil
	stalled = false
	now = now.Add(advance)
	series, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Len(t, attempts, len(rows))
	assert.Equal(t, first, attempts[len(attempts)-1], "give previously unattempted resources priority")
	for _, row := range rows {
		value, ok := seriesValue(series, "request_count_total", row["name"].(string))
		assert.True(t, ok)
		assert.EqualValues(t, previous[row["name"].(string)]+5, value)
	}
	if refreshEvery > 0 {
		rg.setResources(rows[1:])
		now = now.Add(advance)
		series, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		_, ok := seriesValue(series, "request_count_total", rows[0]["name"].(string))
		assert.False(t, ok)
		rg.setResources(rows)
		attempts = nil
		now = now.Add(advance)
		series, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		assert.Contains(t, attempts, rows[0]["id"].(string), "a returning member requires fresh capability evidence")
		value, ok := seriesValue(series, "request_count_total", rows[0]["name"].(string))
		require.True(t, ok)
		assert.EqualValues(t, 5, value, "returning members start a new counter lifetime")
	}
}

func metricWithTotal(name string, ts time.Time, value float64) azmetrics.Metric {
	return azmetrics.Metric{
		Name: &azmetrics.LocalizableString{Value: &name},
		TimeSeries: []azmetrics.TimeSeriesElement{
			{Data: []azmetrics.MetricValue{{TimeStamp: &ts, Total: &value}}},
		},
	}
}

type metricsTestFunc func([]string, []string, *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error)

func (f metricsTestFunc) QueryResources(_ context.Context, _, _ string, names []string, ids azmetrics.ResourceIDList, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
	return f(names, ids.ResourceIDs, opts)
}
