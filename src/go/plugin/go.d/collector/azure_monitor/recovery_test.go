// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollectRetainsDefinitionsDuringSlowFleetRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const namespace = "Microsoft.Cdn/profiles"
		var rows []map[string]any
		for i := range 8 {
			rows = append(rows, testResource(namespace, fmt.Sprintf("edge-%02d", i), nil))
		}
		rg := &mockResourceGraph{resources: rows}
		c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
		c.Config = testConfig()
		c.Discovery.RefreshEvery = 60
		c.Limits.MaxConcurrency = 1
		c.Timeout = confopt.Duration(100 * time.Millisecond)
		c.Profiles.ModeExact = testProfilesModeEntries("front_door")
		now := time.Now().Truncate(time.Minute)
		c.now = func() time.Time { return now }
		c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
			return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
				var result azmetrics.QueryResourcesResponse
				switch *opts.Aggregation {
				case "total":
					if len(names) != 1 || names[0] != "RequestCount" {
						return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "BadRequest"}
					}
					for _, id := range ids {
						result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: []azmetrics.Metric{metricWithTotal("RequestCount", now, 5)}})
					}
				case "average":
					if slices.Contains(names, "OriginLatency") {
						for _, id := range ids {
							result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: []azmetrics.Metric{metricWithAvg("OriginLatency", now, 3)}})
						}
					}
				}
				return result, nil
			}), nil
		}
		require.NoError(t, c.Init(t.Context()))
		successes := map[string]bool{}
		c.queryExecutor.lookupDefinitions = func(ctx context.Context, k definitionKey) (metricDefinitions, error) {
			timer := time.NewTimer(40 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				successes[k.resourceID] = true
				return metricDefinitions{"requestcount": {}, "originlatency": {}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		for i := range 10 {
			series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			var missing []string
			for _, row := range rows {
				name := row["name"].(string)
				_, healthy := seriesValue(series, "origin_latency_average", name)
				require.True(t, healthy, "healthy aggregation disappeared for %s", name)
				if _, ok := seriesValue(series, ".request_count_total", name); !ok {
					missing = append(missing, name)
				}
			}

			if i >= 8 {
				require.Len(t, successes, len(rows), "ARM has successfully verified every resource")
				assert.Empty(t, missing, "completed capability lookups must accumulate into a working fleet")
			}
			now = now.Add(time.Minute)
		}
	})
}

func TestCollectBoundsRecoveryAcrossRegions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const namespace = "Microsoft.Cdn/profiles"
		healthy := testResource(namespace, "east-edge", nil)
		affected := testResource(namespace, "west-a", map[string]any{"location": "westus"})
		slow := testResource(namespace, "west-b", map[string]any{"location": "westus"})
		rg := &mockResourceGraph{resources: []map[string]any{healthy, affected, slow}}
		c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
		c.Config = testConfig()
		c.Profiles.ModeExact = testProfilesModeEntries("front_door")
		phase := defaultTimeout.Duration()
		c.Timeout = confopt.Duration(phase)
		now := time.Now().Truncate(time.Minute)
		c.now = func() time.Time { return now }
		c.newMetricsClient = func(endpoint string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
			return contextMetricsFunc(func(ctx context.Context, names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
				var result azmetrics.QueryResourcesResponse
				if strings.HasPrefix(endpoint, "https://eastus.") {
					values := []azmetrics.Metric{}
					if slices.Contains(names, "OriginLatency") {
						values = append(values, metricWithAvg("OriginLatency", now, 3))
					}
					if slices.Contains(names, "RequestCount") {
						values = append(values, metricWithTotal("RequestCount", now, 5))
					}
					result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: values}}
					return result, nil
				}
				if *opts.Aggregation != "total" {
					return result, nil
				}
				if len(names) != 1 || names[0] != "RequestCount" {
					time.Sleep(15 * time.Millisecond)
					return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "BadRequest"}
				}
				result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: []azmetrics.Metric{metricWithTotal("RequestCount", now, 5)}}}
				return result, nil
			}), nil
		}
		require.NoError(t, c.Init(t.Context()))
		require.NoError(t, c.Check(t.Context()))
		c.queryExecutor.lookupDefinitions = func(ctx context.Context, k definitionKey) (metricDefinitions, error) {
			if k.resourceID == affected["id"] {
				return metricDefinitions{"requestcount": {}}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		start := time.Now()
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		elapsed := time.Since(start)
		require.NoError(t, err)
		_, ok := seriesValue(series, "origin_latency_average", "east-edge")
		require.True(t, ok)
		v, found := seriesValue(series, "request_count_total", "west-a")
		require.True(t, found, "a ready resource must use its worker before a peer exhausts the deadline")
		assert.EqualValues(t, 5, v)
		require.Less(t, elapsed, time.Minute)
	})
}

type contextMetricsFunc func(context.Context, []string, []string, *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error)

func (f contextMetricsFunc) QueryResources(ctx context.Context, _, _ string, names []string, ids azmetrics.ResourceIDList, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
	return f(ctx, names, ids.ResourceIDs, opts)
}

func TestCollectKeepsWorkingMetricsWhileARMThrottles(t *testing.T) {
	const namespace = "Microsoft.Cdn/profiles"
	row := testResource(namespace, "edge", nil)
	id := row["id"].(string)
	rg := &mockResourceGraph{resources: []map[string]any{row}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	c.Discovery.RefreshEvery = 60
	c.Limits.MaxConcurrency = 1
	start := time.Now().Truncate(time.Minute)
	now := start
	c.now = func() time.Time { return now }
	downgraded := false
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			var result azmetrics.QueryResourcesResponse
			var metrics []azmetrics.Metric
			for _, name := range names {
				switch {
				case name == "OriginLatency" && *opts.Aggregation == "average":
					metrics = append(metrics, metricWithAvg(name, now, 3))
				case *opts.Aggregation == "total" && (name == "RequestCount" || name == "OriginRequestCount" && !downgraded):
					metrics = append(metrics, metricWithTotal(name, now, 5))
				default:
					return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
				}
			}
			result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: metrics}}
			return result, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	reads := 0
	c.queryExecutor.lookupDefinitions = func(_ context.Context, k definitionKey) (metricDefinitions, error) {
		reads++
		if reads == 1 {
			now = start.Add(40 * time.Second)
			return metricDefinitions{"originlatency": {}, "requestcount": {}, "originrequestcount": {}}, nil
		}
		assert.Equal(t, strings.Replace(id, "/rg-a/", "/RG-A/", 1), k.resourceID)
		if reads == 2 {
			return nil, &azcore.ResponseError{StatusCode: 429, RawResponse: &http.Response{Header: http.Header{"Retry-After": {"180"}}}}
		}
		return metricDefinitions{"originlatency": {}, "requestcount": {}}, nil
	}
	collect := func(offset time.Duration, wantReads int, wantTotal bool) {
		t.Helper()
		now = start.Add(offset)
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		_, ok := seriesValue(series, "origin_latency_average", "edge")
		assert.True(t, ok, "working Average must survive a rejected Total or unavailable ARM")
		_, ok = seriesValue(series, "request_count_total", "edge")
		assert.Equal(t, wantTotal, ok)
		assert.Equal(t, wantReads, reads)
	}
	collect(0, 1, true)
	row = testResource(namespace, "edge", nil)
	row["id"] = strings.Replace(id, "/rg-a/", "/RG-A/", 1)
	rg.setResources([]map[string]any{row})
	collect(time.Minute, 1, true) // Definitions received 20 seconds ago, same resource.
	downgraded = true
	collect(2*time.Minute, 2, true)
	collect(3*time.Minute, 2, true)
	collect(4*time.Minute, 2, true)
	collect(5*time.Minute, 3, true)
}

func TestCollectHonorsARMSubscriptionReadHold(t *testing.T) {
	known := testResource(recoveryTestNamespace, "sub-one", nil)
	peer := testResource(recoveryTestNamespace, "peer-one", nil)
	afterHold := testResource(recoveryTestNamespace, "sub-one-after-hold", nil)
	other := testResource(recoveryTestNamespace, "sub-two", nil)
	other["id"] = strings.Replace(other["id"].(string), "/sub-1/", "/sub-2/", 1)
	rg := &mockResourceGraph{resources: []map[string]any{known}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.SubscriptionIDs = []string{"sub-1", "sub-2"}
	c.Discovery.RefreshEvery = 60
	c.Limits.MaxConcurrency = 1
	useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := start
	c.now = func() time.Time { return now }

	var reads []string
	initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
		id, _ := strings.CutSuffix(req.URL.Path, "/providers/Microsoft.Insights/metricDefinitions")
		reads = append(reads, id)
		if id == known["id"] && now.Equal(start) {
			resp, err := recoveryWireError(req, http.StatusTooManyRequests, "TooManyRequests")
			if err == nil {
				resp.Header.Set("Retry-After", start.Add(3*time.Minute).Format(http.TimeFormat))
				resp.Header.Set("x-ms-ratelimit-remaining-subscription-reads", "0")
			}
			return resp, err
		}
		return recoveryWireDefinitions(req, "A")
	}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
		if slices.Contains(query.Names, "B") {
			return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
		}
		return recoveryWireTotals(req, query, 5)
	})
	series := collectRecoveryCounters(t, c)
	requireRecoveryCounter(t, series, "A", "sub-one", 5)
	assert.Equal(t, []string{known["id"].(string)}, reads)

	reads = nil
	now = start.Add(time.Minute)
	rg.setResources([]map[string]any{known, peer, other})
	series = collectRecoveryCounters(t, c)
	requireRecoveryCounter(t, series, "A", "sub-one", 10)
	requireRecoveryCounter(t, series, "A", "peer-one", 5)
	requireRecoveryCounter(t, series, "A", "sub-two", 5)
	assert.Equal(t, []string{other["id"].(string)}, reads, "only the exhausted ARM subscription stays held")

	reads = nil
	now = start.Add(3 * time.Minute)
	rg.setResources([]map[string]any{known, peer, other, afterHold})
	series = collectRecoveryCounters(t, c)
	requireRecoveryCounter(t, series, "A", "sub-one-after-hold", 5)
	assert.Contains(t, reads, afterHold["id"].(string), "absolute Retry-After expired")
}

func TestCollectRetriesAllowHealthyPeerToProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const namespace = "Microsoft.Cdn/profiles"
		a, b := testResource(namespace, "edge-a", nil), testResource(namespace, "edge-b", nil)
		c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{a, b}}, &mockMetricsClient{})
		c.Config = testConfig()
		c.Discovery.RefreshEvery = 60
		c.Limits.MaxConcurrency = 1
		c.Limits.MaxBatchResources = 1
		c.Profiles.ModeExact = testProfilesModeEntries("front_door")
		now := time.Now()
		c.now = func() time.Time { return now }
		c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
			return contextMetricsFunc(func(ctx context.Context, names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
				var result azmetrics.QueryResourcesResponse
				if *opts.Aggregation != "total" {
					return result, nil
				}
				if !slices.Equal(names, []string{"RequestCount"}) {
					return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
				}
				if ids[0] == a["id"] {
					<-ctx.Done()
					return result, ctx.Err()
				}
				result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: []azmetrics.Metric{metricWithTotal("RequestCount", now, 5)}}}
				return result, nil
			}), nil
		}
		require.NoError(t, c.Init(t.Context()))
		c.queryExecutor.lookupDefinitions = func(context.Context, definitionKey) (metricDefinitions, error) {
			return metricDefinitions{"requestcount": {}}, nil
		}
		published := 0
		for range 3 {
			series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			if value, ok := seriesValue(series, "request_count_total", "edge-b"); ok {
				published++
				assert.EqualValues(t, 5*published, value)
			}
			now = now.Add(time.Minute)
		}
		assert.Positive(t, published)
	})
}

func TestCollectCapsProbesAndRecoversWorkingNamesAcrossAFleet(t *testing.T) {
	t.Run("one resource changes", func(t *testing.T) { testFleetProbeBudget(t, false) })
	t.Run("all resources change", func(t *testing.T) { testFleetProbeBudget(t, true) })
}

func testFleetProbeBudget(t *testing.T, allChanged bool) {
	const namespace = "Microsoft.Cdn/profiles"
	var rows []map[string]any
	for i := range 12 {
		rows = append(rows, testResource(namespace, fmt.Sprintf("edge-%02d", i), nil))
	}
	c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
	c.Config = testConfig()
	c.Limits.MaxConcurrency = 1
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	now := time.Now().Truncate(time.Minute)
	c.now = func() time.Time { return now }
	changed := false
	calls := 0
	successes := 0
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			calls++
			var result azmetrics.QueryResourcesResponse
			if *opts.Aggregation != "total" {
				successes++
				return result, nil
			}
			losesName := changed && (allChanged || slices.Contains(ids, rows[0]["id"].(string)))
			for _, name := range names {
				if name != "RequestCount" && (name != "OriginRequestCount" || losesName) {
					return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
				}
			}
			successes++
			for _, id := range ids {
				var values []azmetrics.Metric
				for _, name := range names {
					values = append(values, metricWithTotal(name, now, 5))
				}
				result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: values})
			}
			return result, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	c.queryExecutor.lookupDefinitions = func(context.Context, definitionKey) (metricDefinitions, error) {
		if changed {
			return nil, &azcore.ResponseError{StatusCode: 429, RawResponse: &http.Response{Header: http.Header{"Retry-After": {"900"}}}}
		}
		return metricDefinitions{"requestcount": {}, "originrequestcount": {}}, nil
	}
	totals := make(map[string]int)
	for cycle := range 15 {
		calls = 0
		successes = 0
		changed = cycle >= 2
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		if cycle == 0 || cycle == 2 {
			assert.LessOrEqual(t, calls, 2+maxRecoveryMetricsCalls, "one original request per aggregation and one shared probe budget")
		}
		if cycle == 1 {
			assert.Equal(t, 2, successes, "compatible healthy resources share a single successful Total request")
			assert.LessOrEqual(t, calls-successes, maxRecoveryMetricsCalls, "unconfirmed denials remain capped")
		}
		for _, row := range rows {
			name := row["name"].(string)
			value, ok := seriesValue(series, "request_count_total", name)
			if cycle >= 10 {
				require.True(t, ok, "resource %s must progress while ARM is still throttled", name)
			}
			if ok {
				totals[name] += 5
				assert.EqualValues(t, totals[name], value, "one counter delta per resource and window")
			}
		}
		now = now.Add(time.Minute)
	}
}

func TestCollectPreservesServerHoldsAcrossDiscovery(t *testing.T) {
	for _, plane := range []string{"ARM", "Metrics"} {
		t.Run(plane, func(t *testing.T) {
			const namespace = "Microsoft.Cdn/profiles"
			row := testResource(namespace, "edge", nil)
			rg := &mockResourceGraph{resources: []map[string]any{row}}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			c.Config = testConfig()
			c.Discovery.RefreshEvery = 60
			c.Limits.MaxConcurrency = 1
			c.Profiles.ModeExact = testProfilesModeEntries("front_door")
			start := time.Now().Truncate(time.Minute)
			now := start
			c.now = func() time.Time { return now }
			var attempts []time.Duration
			throttled := func() error {
				attempts = append(attempts, now.Sub(start))
				return &azcore.ResponseError{StatusCode: http.StatusTooManyRequests, RawResponse: &http.Response{Header: http.Header{"Retry-After": {"180"}}}}
			}
			c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
				return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
					var result azmetrics.QueryResourcesResponse
					if *opts.Aggregation == "total" {
						if !slices.Equal(names, []string{"RequestCount"}) {
							return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
						}
						if plane == "Metrics" {
							return result, throttled()
						}
					} else {
						result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: []azmetrics.Metric{metricWithAvg("OriginLatency", now, 3)}}}
					}
					return result, nil
				}), nil
			}
			require.NoError(t, c.Init(t.Context()))
			c.queryExecutor.lookupDefinitions = func(context.Context, definitionKey) (metricDefinitions, error) {
				if plane == "ARM" {
					return nil, throttled()
				}
				return metricDefinitions{"requestcount": {}}, nil
			}
			for minute := range 4 {
				now = start.Add(time.Duration(minute) * time.Minute)
				if minute == 1 {
					rg.setResources(nil)
				} else {
					rg.setResources([]map[string]any{row})
				}
				series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				if minute != 1 {
					_, ok := seriesValue(series, "origin_latency_average", "edge")
					assert.True(t, ok)
				}
			}
			assert.Equal(t, []time.Duration{0, 3 * time.Minute}, attempts)
		})
	}
}

func TestCollectEarlyRefreshRecoversNewNamesInTheSameWindow(t *testing.T) {
	const namespace = "Microsoft.Cdn/profiles"
	row := testResource(namespace, "edge", nil)
	c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
	c.Config = testConfig()
	c.Profiles.ModeExact = testProfilesModeEntries("front_door")
	c.Limits.MaxConcurrency = 1
	now := time.Now().Truncate(time.Minute)
	c.now = func() time.Time { return now }
	changed := false
	activeNames := func() string {
		if changed {
			return "WebSocketConnections"
		}
		return "OriginShieldRequestCount"
	}
	var totalQueries [][]string
	c.newMetricsClient = func(_ string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return metricsTestFunc(func(names, ids []string, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
			var result azmetrics.QueryResourcesResponse
			var values []azmetrics.Metric
			if *opts.Aggregation == "total" {
				totalQueries = append(totalQueries, slices.Clone(names))
				for _, name := range names {
					if name != "RequestCount" && name != activeNames() {
						return result, &azcore.ResponseError{StatusCode: http.StatusBadRequest}
					}
					values = append(values, metricWithTotal(name, now, 5))
				}
			} else {
				values = append(values, metricWithAvg("OriginLatency", now, 3))
			}
			result.Values = []azmetrics.MetricData{{ResourceID: &ids[0], Values: values}}
			return result, nil
		}), nil
	}
	require.NoError(t, c.Init(t.Context()))
	definitionReads := 0
	c.queryExecutor.lookupDefinitions = func(context.Context, definitionKey) (metricDefinitions, error) {
		definitionReads++
		return metricDefinitions{"requestcount": {}, strings.ToLower(activeNames()): {}}, nil
	}
	at := func(expectRequestCount int, expectIntroduced bool) {
		t.Helper()
		series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		require.NoError(t, err)
		value, ok := seriesValue(series, ".request_count_total", "edge")
		require.True(t, ok)
		require.EqualValues(t, expectRequestCount, value)
		newValue, ok := seriesValue(series, ".web_socket_connections_total", "edge")
		assert.Equal(t, expectIntroduced, ok, "total requests %v", totalQueries)
		if ok {
			assert.EqualValues(t, 5, newValue)
		}
	}
	at(5, false)
	require.Equal(t, 1, definitionReads)
	changed = true
	totalQueries = nil
	now = now.Add(time.Minute) // Earlier than the 300-second definition TTL.
	at(10, true)
	assert.Equal(t, 2, definitionReads)
}
