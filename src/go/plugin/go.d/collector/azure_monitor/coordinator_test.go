// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

const recoveryTestNamespace = "Microsoft.Cdn/profiles"

type recoveryWireQuery struct {
	Names []string
	azmetrics.ResourceIDList
}

func initRecoveryWire(
	t *testing.T, c *Collector,
	get func(*http.Request) (*http.Response, error),
	post func(*http.Request, recoveryWireQuery) (*http.Response, error),
) {
	t.Helper()
	cred := new(fake.TokenCredential)
	opts := azcore.ClientOptions{Transport: azureTestTransport(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodGet:
			return get(req)
		case http.MethodPost:
			query := recoveryWireQuery{Names: strings.Split(req.URL.Query().Get("metricnames"), ",")}
			if err := json.NewDecoder(req.Body).Decode(&query.ResourceIDList); err != nil {
				return nil, err
			}
			return post(req, query)
		default:
			return nil, fmt.Errorf("unexpected request method %s", req.Method)
		}
	})}
	c.newMetricsClient = func(endpoint string, _ azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
		return azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: withAzureRequestTransport(opts)})
	}
	require.NoError(t, c.Init(t.Context()))
	definitions, err := newMetricDefinitionsClient(cred, opts)
	require.NoError(t, err)
	c.queryExecutor.lookupDefinitions = definitions.list
	require.NoError(t, c.Check(t.Context()))
}

// Use only the names under test so unknown columns from a stock profile do
// not consume the probe budget of otherwise independent scenarios.
func useRecoveryTestProfile(t *testing.T, c *Collector, fast, slow []string) {
	t.Helper()
	var metrics, dimensions strings.Builder
	for _, group := range []struct {
		grain string
		names []string
	}{{"PT1M", fast}, {"PT30M", slow}} {
		for _, name := range group.names {
			id := strings.ToLower(name)
			fmt.Fprintf(&metrics, `
  - id: %s
    azure_name: %s
    time_grain: %s
    series:
      - aggregation: total
        kind: counter`, id, name, group.grain)
			fmt.Fprintf(&dimensions, `
        - selector: %s
          name: %s`, azureprofiles.ExportedSeriesName("recovery", id, "total"), id)
		}
	}
	c.Profiles.ModeExact = testProfilesModeEntries("recovery")
	catalog := mustLoadStockCatalog(t, map[string]string{
		"recovery.yaml": fmt.Sprintf(`
display_name: Recovery
resource_type: %s
metrics:%s
template:
  family: Recovery
  context_namespace: recovery
  charts:
    - id: am_recovery_counts
      title: Recovery counts
      context: counts
      family: Recovery
      type: line
      units: counts/s
      algorithm: incremental
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:%s
`, recoveryTestNamespace, metrics.String(), dimensions.String()),
	})
	c.loadProfileCatalog = func() (azureprofiles.Catalog, error) { return catalog, nil }
}

func recoveryWireError(req *http.Request, status int, code string) (*http.Response, error) {
	return azureTestResponse(req, status, map[string]any{"error": map[string]string{"code": code}})
}

func recoveryWireDefinitions(req *http.Request, names ...string) (*http.Response, error) {
	definitions := make([]metricDefinition, 0, len(names))
	for _, name := range names {
		var definition metricDefinition
		definition.Name.Value = name
		definition.SupportedAggregationTypes = []string{"Total"}
		definitions = append(definitions, definition)
	}
	return azureTestResponse(req, http.StatusOK, map[string]any{"value": definitions})
}

func recoveryWireTotals(req *http.Request, query recoveryWireQuery, value float64) (*http.Response, error) {
	// Return the bucket the collector requested so these tests also exercise
	// the data-window contract when ARM work advances wall time.
	end, err := time.Parse(time.RFC3339, req.URL.Query().Get("endtime"))
	if err != nil {
		return nil, err
	}
	var result azmetrics.MetricResults
	for _, id := range query.ResourceIDs {
		var metrics []azmetrics.Metric
		for _, name := range query.Names {
			metrics = append(metrics, metricWithTotal(name, end.Add(-time.Second), value))
		}
		result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(id), Values: metrics})
	}
	return azureTestResponse(req, http.StatusOK, result)
}

func collectRecoveryCounters(t *testing.T, c *Collector) map[string]metrix.SampleValue {
	t.Helper()
	series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	return series
}

func requireRecoveryCounter(t *testing.T, series map[string]metrix.SampleValue, name, resource string, expected int) {
	t.Helper()
	value, ok := seriesValue(series, "."+strings.ToLower(name)+"_total", fmt.Sprintf("resource_name=%q,", resource))
	require.True(t, ok, "counter %s for %s missing from %v", name, resource, keysFromSeries(series))
	assert.EqualValues(t, expected, value)
}

func TestCollectKeepsDisjointAcceptedShapesAfterUnionRejection(t *testing.T) {
	row := testResource(recoveryTestNamespace, "edge", nil)
	c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
	c.Config = testConfig()
	c.Limits.MaxConcurrency = 1
	useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	var requests []string
	var reads int
	initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
		reads++
		return recoveryWireDefinitions(req, "A", "B")
	}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
		requests = append(requests, strings.Join(query.Names, ","))
		if len(query.Names) > 1 {
			return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
		}
		return recoveryWireTotals(req, query, 5)
	})

	for index, minute := range []int{0, 1, 2, 5, 6, 15} {
		now = time.Date(2026, 1, 1, 12, minute, 0, 0, time.UTC)
		requests = nil
		series := collectRecoveryCounters(t, c)
		requireRecoveryCounter(t, series, "A", "edge", 5*(index+1))
		requireRecoveryCounter(t, series, "B", "edge", 5*(index+1))
		if index == 0 {
			assert.ElementsMatch(t, []string{"A,B", "A", "B"}, requests)
		} else {
			assert.ElementsMatch(t, []string{"A", "B"}, requests, "minute %d", minute)
		}
	}
	assert.Equal(t, 1, reads, "a stable working split has no unresolved cells to refresh")
}

func TestCollectContinuesDataPlaneRecoveryAfterDefinitionExpiry(t *testing.T) {
	for label, status := range map[string]int{"false denial": http.StatusOK, "unreadable": http.StatusForbidden} {
		t.Run(label, func(t *testing.T) {
			row := testResource(recoveryTestNamespace, "edge", nil)
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
			c.Config = testConfig()
			c.Limits.MaxConcurrency = 1
			useRecoveryTestProfile(t, c, []string{"A", "B", "C"}, nil)
			start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			now := start
			c.now = func() time.Time { return now }
			var reads []time.Duration
			initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
				reads = append(reads, now.Sub(start))
				if now.Equal(start) {
					return recoveryWireDefinitions(req, "A", "B")
				}
				if status != http.StatusOK {
					return recoveryWireError(req, status, "AuthorizationFailed")
				}
				// At expiry the advertised sibling is denied by Metrics, and
				// the working name is missing from ARM's list.
				return recoveryWireDefinitions(req, "B")
			}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
				if slices.Contains(query.Names, "C") || (!now.Equal(start) && slices.Contains(query.Names, "B")) {
					return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
				}
				return recoveryWireTotals(req, query, 5)
			})

			for index, minute := range []int{0, 5, 6} {
				now = start.Add(time.Duration(minute) * time.Minute)
				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "A", "edge", 5*(index+1))
				_, hasB := seriesValue(series, ".b_total", "edge")
				assert.Equal(t, index == 0, hasB, "a rejected counter is a gap")
			}
			require.GreaterOrEqual(t, len(reads), 2)
			assert.Equal(t, []time.Duration{0, 5 * time.Minute}, reads[:2])
		})
	}
}

func TestCollectDecodesMetricErrorsAndDuplicateRows(t *testing.T) {
	const namespace = recoveryTestNamespace
	var rows []map[string]any
	for _, name := range []string{"valid", "empty", "missing"} {
		rows = append(rows, testResource(namespace, name, nil))
	}
	c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
	c.Config = testConfig()
	c.Limits.MaxConcurrency = 1
	useRecoveryTestProfile(t, c, []string{"A", "B", "C"}, nil)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := start
	c.now = func() time.Time { return now }
	valid, empty := rows[0]["id"].(string), rows[1]["id"].(string)
	var reads int
	var requests []recoveryWireQuery
	initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
		reads++
		assert.Equal(t, valid+"/providers/Microsoft.Insights/metricDefinitions", req.URL.Path, "only the rejected resource needs recovery")
		return recoveryWireDefinitions(req, "A", "B", "C")
	}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
		requests = append(requests, query)
		if !slices.Contains(query.ResourceIDs, valid) {
			return azureTestResponse(req, http.StatusOK, azmetrics.MetricResults{})
		}
		a := metricWithTotal("A", now, 5)
		a.ErrorCode = new("Success")
		b := metricWithTotal("B", now, 900)
		b.ErrorCode = new("InvalidSamplingType")
		unrelated := metricWithTotal("C", now, 800)
		unrelated.ErrorCode = new("InternalServerError")
		var values []azmetrics.Metric
		for _, metric := range []azmetrics.Metric{a, b, unrelated} {
			if slices.Contains(query.Names, *metric.Name.Value) {
				values = append(values, metric)
			}
		}
		result := azmetrics.MetricResults{Values: []azmetrics.MetricData{{ResourceID: new(valid), Values: values}}}
		if slices.Contains(query.Names, "A") {
			duplicate := metricWithTotal("A", now, 100)
			result.Values[0].Values = append(result.Values[0].Values, duplicate)
			result.Values = append(result.Values, azmetrics.MetricData{
				ResourceID: new(strings.ToUpper(valid)),
				Values:     []azmetrics.Metric{duplicate},
			})
		}
		if slices.Contains(query.ResourceIDs, empty) {
			result.Values = append(result.Values, azmetrics.MetricData{ResourceID: new(empty), Values: []azmetrics.Metric{
				{Name: &azmetrics.LocalizableString{Value: new("A")}},
			}})
		}
		return azureTestResponse(req, http.StatusOK, result)
	})

	for poll := range 2 {
		now = start.Add(time.Duration(poll) * time.Minute)
		requests = nil
		series := collectRecoveryCounters(t, c)
		requireRecoveryCounter(t, series, "A", "valid", 5*(poll+1))
		for _, key := range []string{".b_total", ".c_total", `resource_name="empty",`, `resource_name="missing",`} {
			_, ok := seriesValue(series, key)
			assert.False(t, ok, "unexpected sample for %s", key)
		}
		if poll == 0 {
			require.Greater(t, len(requests), 1, "the embedded sampling error must trigger a corrective request")
			for _, query := range requests[1:] {
				// Empty, missing and unrelated errors should never trigger isolation.
				assert.Equal(t, recoveryWireQuery{
					Names:          []string{"B"},
					ResourceIDList: azmetrics.ResourceIDList{ResourceIDs: []string{valid}},
				}, query)
			}
		}
	}
	assert.LessOrEqual(t, reads, 1)
}

func TestCollectRecoversWhenHoldExpiresDuringOrdinaryQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		row := testResource(recoveryTestNamespace, "edge", nil)
		c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
		c.Config = testConfig()
		c.Limits.MaxConcurrency = 2
		useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
		start := time.Now()
		c.now = time.Now
		var attemptsB []time.Duration
		initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
			return recoveryWireDefinitions(req, "A")
		}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
			if len(query.Names) > 1 {
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			}
			switch query.Names[0] {
			case "B":
				elapsed := time.Since(start)
				attemptsB = append(attemptsB, elapsed)
				if elapsed < 80*time.Second {
					resp, err := recoveryWireError(req, http.StatusTooManyRequests, "TooManyRequests")
					resp.Header.Set("Retry-After", "80")
					return resp, err
				}
			case "A":
				if time.Since(start) >= time.Minute {
					time.Sleep(25 * time.Second)
				}
			}
			return recoveryWireTotals(req, query, 5)
		})
		requireRecoveryCounter(t, collectRecoveryCounters(t, c), "A", "edge", 5)
		time.Sleep(time.Minute)
		series := collectRecoveryCounters(t, c)
		requireRecoveryCounter(t, series, "A", "edge", 10)
		requireRecoveryCounter(t, series, "B", "edge", 5)
		assert.Equal(t, []time.Duration{0, 80 * time.Second}, attemptsB, "an open slot can recover B while A is still running")
	})
}

func TestCollectScopedMetricsHoldPreservesDisjointCounter(t *testing.T) {
	row := testResource(recoveryTestNamespace, "edge", nil)
	rg := &mockResourceGraph{resources: []map[string]any{row}}
	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Discovery.RefreshEvery = 60
	c.Limits.MaxConcurrency = 1
	useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := start
	c.now = func() time.Time { return now }
	var bRequests []time.Duration
	initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
		if now.Equal(start) {
			return recoveryWireDefinitions(req, "A")
		}
		return recoveryWireDefinitions(req, "A", "B")
	}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
		if slices.Contains(query.Names, "B") {
			elapsed := now.Sub(start)
			bRequests = append(bRequests, elapsed)
			switch {
			case elapsed == 0:
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			case elapsed < 9*time.Minute:
				assert.Equal(t, []string{"B"}, query.Names, "the hold is for the optional name alone")
				resp, err := recoveryWireError(req, http.StatusTooManyRequests, "TooManyRequests")
				if err == nil {
					resp.Header.Set("Retry-After", "240")
				}
				return resp, err
			}
		}
		return recoveryWireTotals(req, query, 5)
	})

	for _, poll := range []struct{ minute, wantA, wantB int }{
		{0, 5, 0},
		{5, 10, 0},
		{6, 15, 0},
		{7, 0, 0}, // Temporarily absent from discovery.
		{8, 5, 0},
		{9, 10, 5},
	} {
		now = start.Add(time.Duration(poll.minute) * time.Minute)
		if poll.wantA == 0 {
			rg.setResources(nil)
		} else {
			rg.setResources([]map[string]any{row})
		}
		series := collectRecoveryCounters(t, c)
		if poll.wantA > 0 {
			requireRecoveryCounter(t, series, "A", "edge", poll.wantA)
		}
		if poll.wantB > 0 {
			requireRecoveryCounter(t, series, "B", "edge", poll.wantB)
		} else {
			_, ok := seriesValue(series, ".b_total", "edge")
			assert.False(t, ok)
		}
	}
	assert.Equal(t, []time.Duration{0, 0, 5 * time.Minute, 9 * time.Minute}, bRequests)
}

func TestCollectRechecksDeniedMetricWithExponentialBackoff(t *testing.T) {
	defaultRecheckMinutes := []int{0, 5, 15, 35, 75, 135, 195}
	for label, test := range map[string]struct {
		unavailable    bool
		recheckMinutes []int
	}{
		"unchanged ARM":                  {recheckMinutes: defaultRecheckMinutes},
		"unreadable ARM":                 {unavailable: true, recheckMinutes: defaultRecheckMinutes},
		"configured TTL beyond one hour": {recheckMinutes: []int{0, 90, 180, 270}},
	} {
		t.Run(label, func(t *testing.T) {
			row := testResource(recoveryTestNamespace, "edge", nil)
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
			c.Config = testConfig()
			c.Limits.MaxConcurrency = 1
			c.Discovery.RefreshEvery = test.recheckMinutes[1] * 60
			useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
			start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			now := start
			recoveryMinute := test.recheckMinutes[len(test.recheckMinutes)-1]
			c.now = func() time.Time { return now }
			var rechecks []time.Duration
			initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
				if test.unavailable {
					return recoveryWireError(req, http.StatusForbidden, "AuthorizationFailed")
				}
				return recoveryWireDefinitions(req, "A")
			}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
				elapsed := now.Sub(start)
				if slices.Contains(query.Names, "B") {
					if len(query.Names) == 1 {
						rechecks = append(rechecks, elapsed)
					}
					if elapsed < time.Duration(recoveryMinute)*time.Minute {
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
				}
				return recoveryWireTotals(req, query, 5)
			})

			pollMinutes := []int{0, 1}
			wantRechecks := []time.Duration{0}
			for _, minute := range test.recheckMinutes[1:] {
				pollMinutes = append(pollMinutes, minute-1, minute)
				wantRechecks = append(wantRechecks, time.Duration(minute)*time.Minute)
			}
			pollMinutes = append(pollMinutes, recoveryMinute+1)
			wantRechecks = append(wantRechecks, time.Duration(recoveryMinute+1)*time.Minute)
			for index, minute := range pollMinutes {
				now = start.Add(time.Duration(minute) * time.Minute)
				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "A", "edge", 5*(index+1))
				if minute >= recoveryMinute {
					requireRecoveryCounter(t, series, "B", "edge", 5*(minute-recoveryMinute+1))
				} else {
					_, ok := seriesValue(series, ".b_total", "edge")
					assert.False(t, ok, "B is a gap at minute %d", minute)
				}
			}
			assert.Equal(t, wantRechecks, rechecks)
		})
	}
}

func TestCollectPacksCompatibleCorrectionsAcrossTwelveResources(t *testing.T) {
	for _, concurrency := range []int{1, 4} {
		t.Run(fmt.Sprintf("concurrency-%d", concurrency), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var rows []map[string]any
				var ids []string
				for i := range 12 {
					row := testResource(recoveryTestNamespace, fmt.Sprintf("edge-%02d", i), nil)
					rows = append(rows, row)
					ids = append(ids, row["id"].(string))
				}
				c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
				c.Config = testConfig()
				c.Limits.MaxConcurrency = concurrency
				useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
				var mu sync.Mutex
				var reads, active, maxActive int
				var corrections [][]string
				enter := func() {
					mu.Lock()
					active++
					maxActive = max(maxActive, active)
					mu.Unlock()
				}
				leave := func() {
					mu.Lock()
					active--
					mu.Unlock()
				}
				initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
					enter()
					defer leave()
					mu.Lock()
					reads++
					mu.Unlock()
					time.Sleep(10 * time.Millisecond)
					return recoveryWireDefinitions(req, "A")
				}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
					enter()
					defer leave()
					time.Sleep(10 * time.Millisecond)
					if !slices.Equal(query.Names, []string{"A"}) {
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
					mu.Lock()
					corrections = append(corrections, query.ResourceIDs)
					mu.Unlock()
					return recoveryWireTotals(req, query, 5)
				})
				for poll := range 2 {
					corrections = nil
					series := collectRecoveryCounters(t, c)
					for _, row := range rows {
						requireRecoveryCounter(t, series, "A", row["name"].(string), 5*(poll+1))
					}
					require.Len(t, corrections, 1, "exactly one compatible A cohort on poll %d", poll)
					assert.ElementsMatch(t, ids, corrections[0])
					assert.Equal(t, len(ids), reads, "every affected resource contributes its own ARM evidence")
					assert.LessOrEqual(t, maxActive, concurrency)
					time.Sleep(time.Minute)
				}
			})
		})
	}
}

func TestCollectSlowGrainLeavesMetricsTimeAfterStalledARM(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		row := testResource(recoveryTestNamespace, "edge", nil)
		c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
		c.Config = testConfig()
		c.Limits.MaxConcurrency = 1
		useRecoveryTestProfile(t, c, []string{"A"}, []string{"B", "C"})
		start := time.Now()
		var pollAt time.Time
		var armDurations []time.Duration
		var bRequests []time.Duration
		initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
			began := time.Now()
			<-req.Context().Done()
			armDurations = append(armDurations, time.Since(began))
			return nil, req.Context().Err()
		}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
			if slices.Contains(query.Names, "C") {
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			}
			if slices.Contains(query.Names, "B") {
				bRequests = append(bRequests, time.Since(start))
				values := req.URL.Query()
				assert.Equal(t, "PT30M", values.Get("interval"))
				assert.Equal(t, pollAt.Add(-time.Hour).UTC().Format(time.RFC3339), values.Get("starttime"))
				assert.Equal(t, pollAt.Add(-30*time.Minute).UTC().Format(time.RFC3339), values.Get("endtime"))
			}
			return recoveryWireTotals(req, query, 5)
		})

		for i, minute := range []int{0, 30, 60} {
			time.Sleep(time.Duration(minute)*time.Minute - time.Since(start))
			pollAt = time.Now()
			series := collectRecoveryCounters(t, c)
			requireRecoveryCounter(t, series, "A", "edge", 5*(i+1))
			requireRecoveryCounter(t, series, "B", "edge", 5*(i+1))
			require.Len(t, bRequests, i+1)
			assert.GreaterOrEqual(t, bRequests[i], time.Duration(minute)*time.Minute)
			assert.Less(t, bRequests[i], time.Duration(minute)*time.Minute+c.Timeout.Duration())
		}
		require.NotEmpty(t, armDurations, "the test must actually stall ARM before recovering")
		assert.Greater(t, bRequests[0], time.Duration(0))
		for _, duration := range armDurations {
			assert.Positive(t, duration)
			assert.Less(t, duration, c.Timeout.Duration(), "ARM must stop before the Metrics recovery deadline")
		}
	})
}

func TestCollectCapsAndRotatesSupplementalWireRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var rows []map[string]any
		accepted := make(map[string]bool)
		for i := range 9 {
			row := testResource(recoveryTestNamespace, fmt.Sprintf("edge-%02d", i), nil)
			rows = append(rows, row)
			accepted[row["id"].(string)+" A,B"] = true // Configured ordinary shapes in the first poll.
			accepted[row["id"].(string)+" C"] = true
		}
		healthy := rows[8]
		c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
		c.Config = testConfig()
		c.Limits.MaxConcurrency = 1
		c.Limits.MaxBatchResources = 1
		c.Limits.MaxMetricsPerQuery = 2
		useRecoveryTestProfile(t, c, []string{"A", "B", "C"}, nil)
		start := time.Now()
		var requests []string
		var successes map[string]bool
		visitedA := make(map[string]bool)
		var attemptsA map[string]int
		var changed bool
		var gotHealthyA bool
		initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
			time.Sleep(10 * time.Millisecond)
			return recoveryWireDefinitions(req, "A")
		}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
			assert.Len(t, query.ResourceIDs, 1)
			id := query.ResourceIDs[0]
			shape := id + " " + strings.Join(query.Names, ",")
			requests = append(requests, shape)
			if changed && len(query.Names) > 1 {
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			}
			if changed && query.Names[0] == "A" {
				attemptsA[id]++
				visitedA[id] = true
				if id != healthy["id"] {
					return recoveryWireError(req, http.StatusServiceUnavailable, "ServiceUnavailable")
				}
			}
			successes[shape] = true
			gotHealthyA = gotHealthyA || id == healthy["id"] && slices.Contains(query.Names, "A")
			return recoveryWireTotals(req, query, 5)
		})

		published := 0
		for poll := range 6 {
			requests = nil
			successes = make(map[string]bool)
			attemptsA = make(map[string]int)
			gotHealthyA = false
			changed = poll > 0
			series := collectRecoveryCounters(t, c)
			count := 0
			for _, shape := range requests {
				// Freeze exact name sets and resource IDs; A and B learned separately
				// on one resource can count as two different ordinary requests.
				if !accepted[shape] {
					count++
				}
			}
			switch poll {
			case 0:
				assert.Zero(t, count)
				assert.Len(t, requests, len(rows)*2)
			case 1:
				assert.Equal(t, 8, count, "all supplemental wire credits are used by the rejection")
			default:
				assert.LessOrEqual(t, count, 8)
				for _, shape := range requests {
					assert.NotContains(t, shape, " A,B", "the known-rejected union cannot be sent as ordinary work")
				}
			}
			for id, attempts := range attemptsA {
				assert.Equal(t, 1, attempts, "an optional 503 must not retry inside the SDK for %s", id)
			}
			name := healthy["name"].(string)
			requireRecoveryCounter(t, series, "C", name, 5*(poll+1))
			if gotHealthyA {
				published++
				requireRecoveryCounter(t, series, "A", name, 5*published)
			} else {
				_, ok := seriesValue(series, ".a_total", fmt.Sprintf("resource_name=%q,", name))
				assert.False(t, ok, "a missed window must stay a gap")
			}
			accepted = successes
			time.Sleep(time.Duration(poll+1)*time.Minute - time.Since(start))
		}
		assert.Len(t, visitedA, len(rows), "every continuously eligible resource gets a turn")
		assert.Greater(t, published, 1, "a resource continuously eligible for A gets a turn despite transient peers")
	})
}

func TestCollectDistinguishesOmittedHistoryFromUnavailableAdvice(t *testing.T) {
	live := testResource(recoveryTestNamespace, "live", nil)
	removed := testResource(recoveryTestNamespace, "removed", nil)
	c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{live, removed}}, &mockMetricsClient{})
	c.Config = testConfig()
	c.Limits.MaxConcurrency = 1
	useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	warm := true
	var probes []recoveryWireQuery
	initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.Path, live["id"].(string)+"/") {
			// ARM omits A, which continues to work at the regional endpoint.
			return recoveryWireDefinitions(req)
		}
		return recoveryWireError(req, http.StatusForbidden, "Forbidden")
	}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
		if warm {
			query.Names = []string{"A"}
			return recoveryWireTotals(req, query, 5)
		}
		probes = append(probes, query)
		if slices.Equal(query.Names, []string{"A"}) && slices.Equal(query.ResourceIDs, []string{live["id"].(string)}) {
			return recoveryWireTotals(req, query, 5)
		}
		return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
	})

	first := collectRecoveryCounters(t, c)
	requireRecoveryCounter(t, first, "A", "live", 5)
	requireRecoveryCounter(t, first, "A", "removed", 5)
	warm = false
	now = now.Add(time.Minute)
	next := collectRecoveryCounters(t, c)
	requireRecoveryCounter(t, next, "A", "live", 10)
	require.NotEmpty(t, probes)
	assert.LessOrEqual(t, len(probes), 9)
	for _, query := range probes[1:] {
		if slices.Equal(query.Names, []string{"A"}) {
			assert.Len(t, query.ResourceIDs, 1, "unknown advice and an explicit ARM omission do not support coalescing two historical samples")
		}
	}
}
