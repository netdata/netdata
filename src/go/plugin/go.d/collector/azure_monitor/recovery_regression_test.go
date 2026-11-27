// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollectKeepsCorrectiveCohortsSeparateFromDenialConfirmations(t *testing.T) {
	for name, test := range map[string]struct {
		names         []string
		skus          []string
		warm          bool
		unreadableSKU string
		wantCalls     int
	}{
		"cold two SKUs":               {names: []string{"A", "B"}, skus: []string{"A", "B"}, wantCalls: 5},
		"cold six SKUs":               {names: []string{"A", "B", "C"}, skus: []string{"A", "BC", "B", "AC", "C", "AB"}},
		"warm two SKUs":               {names: []string{"A", "B"}, skus: []string{"A", "B"}, warm: true, wantCalls: 5},
		"warm six SKUs":               {names: []string{"A", "B", "C"}, skus: []string{"A", "BC", "B", "AC", "C", "AB"}, warm: true},
		"warm with unreadable advice": {names: []string{"A", "B"}, skus: []string{"A", "B"}, warm: true, unreadableSKU: "B"},
	} {
		t.Run(name, func(t *testing.T) {
			var rows []map[string]any
			support := make(map[string][]string)
			var unreadableID string
			for _, sku := range test.skus {
				row := testResource(recoveryTestNamespace, strings.ToLower(sku), nil)
				rows = append(rows, row)
				id := row["id"].(string)
				support[id] = strings.Split(sku, "")
				if sku == test.unreadableSKU {
					unreadableID = id
				}
			}
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
			c.Config = testConfig()
			c.Limits.MaxConcurrency = 1
			useRecoveryTestProfile(t, c, test.names, nil)
			now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			c.now = func() time.Time { return now }
			healthy := test.warm
			supports := func(id string, names []string) bool {
				for _, name := range names {
					if !slices.Contains(support[id], name) {
						return false
					}
				}
				return true
			}

			var requests []recoveryWireQuery
			var reads []string
			initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
				id, _ := strings.CutSuffix(req.URL.Path, "/providers/Microsoft.Insights/metricDefinitions")
				assert.Contains(t, support, id)
				reads = append(reads, id)
				if id == unreadableID {
					return recoveryWireError(req, http.StatusForbidden, "AuthorizationFailed")
				}
				return recoveryWireDefinitions(req, support[id]...)
			}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
				requests = append(requests, query)
				if healthy && unreadableID != "" {
					// This resource previously returned A but no B. After the change
					// it supports B and its ARM read cannot disprove the old A sample.
					var result azmetrics.MetricResults
					for _, id := range query.ResourceIDs {
						row := azmetrics.MetricData{ResourceID: new(id)}
						for _, metric := range query.Names {
							if id != unreadableID || metric != test.unreadableSKU {
								row.Values = append(row.Values, metricWithTotal(metric, now.Add(-3*time.Minute-time.Second), 5))
							}
						}
						result.Values = append(result.Values, row)
					}
					return azureTestResponse(req, http.StatusOK, result)
				}
				for _, id := range query.ResourceIDs {
					if !healthy && !supports(id, query.Names) {
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
				}
				return recoveryWireTotals(req, query, 5)
			})

			if test.warm {
				initial := collectRecoveryCounters(t, c)
				require.Len(t, requests, 1, "an established healthy fleet starts with one ordinary POST")
				require.Empty(t, reads)
				for _, sku := range test.skus {
					for _, metric := range test.names {
						if sku != test.unreadableSKU || metric != test.unreadableSKU {
							requireRecoveryCounter(t, initial, metric, strings.ToLower(sku), 5)
						}
					}
				}
				healthy = false
				now = now.Add(time.Minute)
				requests = nil
			}
			series := collectRecoveryCounters(t, c)
			require.NotEmpty(t, requests)
			assert.ElementsMatch(t, test.names, requests[0].Names)
			assert.Len(t, requests[0].ResourceIDs, len(rows))
			assert.ElementsMatch(t, requests[0].ResourceIDs, reads, "each affected resource contributes one definition lookup")
			assert.LessOrEqual(t, len(requests), 9, "the original POST plus eight supplemental POSTs")
			if test.wantCalls != 0 {
				assert.Len(t, requests, test.wantCalls, "one rejection, two corrections, and two denial confirmations")
			}
			for _, query := range requests[1:] {
				require.NotEmpty(t, query.ResourceIDs)
				available := supports(query.ResourceIDs[0], query.Names)
				for _, id := range query.ResourceIDs[1:] {
					assert.Equal(t, available, supports(id, query.Names), "corrective batches must not include contradicted or unverifiable members: %v", query)
				}
			}
			for _, sku := range test.skus {
				for _, metric := range test.names {
					label := fmt.Sprintf("resource_name=%q,", strings.ToLower(sku))
					value, found := seriesValue(series, "."+strings.ToLower(metric)+"_total", label)
					if !strings.Contains(sku, metric) {
						assert.False(t, found, "%s for SKU %s must be a gap", metric, sku)
						continue
					}
					if assert.True(t, found, "%s for SKU %s missing after %d POSTs", metric, sku, len(requests)) {
						wantTotal := 5
						if test.warm && (sku != test.unreadableSKU || metric != test.unreadableSKU) {
							wantTotal += 5
						}
						assert.EqualValues(t, wantTotal, value)
					}
				}
			}
		})
	}
}

func TestCollectEarlyAdviceProbePreservesDenialSchedule(t *testing.T) {
	for name, test := range map[string]struct {
		earlyStatus int
		wantMinutes []int
	}{
		"definitive rejection": {earlyStatus: http.StatusBadRequest, wantMinutes: []int{0, 5, 10, 30}},
		"transient failure":    {earlyStatus: http.StatusServiceUnavailable, wantMinutes: []int{0, 5, 10, 15, 35}},
	} {
		t.Run(name, func(t *testing.T) {
			row := testResource(recoveryTestNamespace, "edge", nil)
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
			c.Config = testConfig()
			c.Limits.MaxConcurrency = 1
			useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
			start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			now := start
			c.now = func() time.Time { return now }

			var bAttempts []int
			initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
				if now.Before(start.Add(10 * time.Minute)) {
					return recoveryWireDefinitions(req, "A")
				}
				return recoveryWireDefinitions(req, "A", "B")
			}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
				if !slices.Contains(query.Names, "B") {
					return recoveryWireTotals(req, query, 5)
				}
				minute := int(now.Sub(start) / time.Minute)
				if len(query.Names) == 1 {
					bAttempts = append(bAttempts, minute)
				}
				if minute == 10 && test.earlyStatus == http.StatusServiceUnavailable {
					return recoveryWireError(req, http.StatusServiceUnavailable, "ServiceUnavailable")
				}
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			})

			for i, minute := range []int{0, 1, 5, 6, 10, 11, 14, 15, 16, 25, 29, 30, 34, 35} {
				now = start.Add(time.Duration(minute) * time.Minute)
				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "A", "edge", 5*(i+1))
				_, hasB := seriesValue(series, ".b_total", "edge")
				assert.False(t, hasB)
			}
			assert.Equal(t, test.wantMinutes, bAttempts)
		})
	}
}

func TestCollectCarriesSlowCounterWhenOnlySupplementalWorkFails(t *testing.T) {
	for name, status := range map[string]int{"definitive rejection": http.StatusBadRequest, "transient failure": http.StatusServiceUnavailable} {
		t.Run(name, func(t *testing.T) {
			row := testResource(recoveryTestNamespace, "edge", nil)
			c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
			c.Config = testConfig()
			c.Limits.MaxConcurrency = 1
			useRecoveryTestProfile(t, c, []string{"B"}, []string{"A"})
			start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			now := start
			c.now = func() time.Time { return now }

			var requests []string
			initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
				return recoveryWireDefinitions(req, "A")
			}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
				requests = append(requests, strings.Join(query.Names, ","))
				if slices.Contains(query.Names, "B") {
					if now.Equal(start) || status == http.StatusBadRequest {
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
					return recoveryWireError(req, http.StatusServiceUnavailable, "ServiceUnavailable")
				}
				if now.Equal(start.Add(30 * time.Minute)) {
					return recoveryWireError(req, http.StatusForbidden, "AuthorizationFailed")
				}
				return recoveryWireTotals(req, query, 5)
			})

			for _, minute := range []int{0, 1, 5, 6} {
				now = start.Add(time.Duration(minute) * time.Minute)
				requests = nil
				series, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				if assert.NoError(t, err, "minute %d", minute) {
					requireRecoveryCounter(t, series, "A", "edge", 5)
					_, hasB := seriesValue(series, ".b_total", "edge")
					assert.False(t, hasB)
				}
				switch minute {
				case 0:
					assert.ElementsMatch(t, []string{"A", "B"}, requests)
				case 1:
					assert.Empty(t, requests, "a carry-only poll needs no Metrics POST")
				case 5:
					assert.Equal(t, []string{"B"}, requests, "only the supplemental failure reaches the wire")
				}
			}
			now = start.Add(30 * time.Minute)
			_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.ErrorContains(t, err, "all Azure Monitor batch queries failed", "the ordinary query failed and its old sample expired")
		})
	}
}

func TestCollectLateRejectionUsesRemainingPollBudget(t *testing.T) {
	for name, test := range map[string]struct {
		earlyRejection bool
		repairTime     time.Duration
		wantFinish     time.Duration
		wantWest       bool
	}{
		"early east success":   {repairTime: 2 * time.Second, wantFinish: 31 * time.Second, wantWest: true},
		"early east rejection": {earlyRejection: true, repairTime: 2 * time.Second, wantFinish: 31 * time.Second, wantWest: true},
		"poll recovery cutoff": {earlyRejection: true, repairTime: 26 * time.Second, wantFinish: 54 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				east := testResource(recoveryTestNamespace, "east", nil)
				west := testResource(recoveryTestNamespace, "west", map[string]any{"location": "westus"})
				westID := west["id"].(string)
				c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{east, west}}, &mockMetricsClient{})
				c.Config = testConfig()
				c.Limits.MaxConcurrency = 2
				useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
				start := time.Now()
				c.now = time.Now

				type repair struct{ began, ended time.Duration }
				var mu sync.Mutex
				var eastRepairs []time.Duration
				var westReads []time.Duration
				var westRepairs []repair
				initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
					if strings.HasPrefix(req.URL.Path, westID+"/") {
						mu.Lock()
						westReads = append(westReads, time.Since(start))
						mu.Unlock()
					}
					return recoveryWireDefinitions(req, "A")
				}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
					if len(query.ResourceIDs) != 1 {
						return nil, fmt.Errorf("expected a single regional resource, got %v", query.ResourceIDs)
					}
					assert.Equal(t, start.Add(-4*time.Minute).UTC().Format(time.RFC3339), req.URL.Query().Get("starttime"))
					assert.Equal(t, start.Add(-3*time.Minute).UTC().Format(time.RFC3339), req.URL.Query().Get("endtime"))
					isWest := query.ResourceIDs[0] == westID
					if slices.Contains(query.Names, "B") && (isWest || test.earlyRejection) {
						if isWest && len(query.Names) == 2 {
							time.Sleep(29 * time.Second)
						}
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
					if isWest {
						began := time.Since(start)
						defer func() {
							mu.Lock()
							westRepairs = append(westRepairs, repair{began, time.Since(start)})
							mu.Unlock()
						}()
						select {
						case <-time.After(test.repairTime):
						case <-req.Context().Done():
							return nil, req.Context().Err()
						}
					} else if test.earlyRejection {
						mu.Lock()
						eastRepairs = append(eastRepairs, time.Since(start))
						mu.Unlock()
					}
					return recoveryWireTotals(req, query, 5)
				})

				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "A", "east", 5)
				if test.earlyRejection {
					assert.Equal(t, []time.Duration{0}, eastRepairs, "east recovery proceeds while the west ordinary request is still active")
				}
				assert.Equal(t, []time.Duration{29 * time.Second}, westReads, "the west rejection gets its own ARM preparation opportunity")
				assert.Equal(t, []repair{{29 * time.Second, test.wantFinish}}, westRepairs)
				assert.Equal(t, test.wantFinish, time.Since(start))
				if test.wantWest {
					requireRecoveryCounter(t, series, "A", "west", 5)
				} else {
					_, hasWest := seriesValue(series, ".a_total", `resource_name="west",`)
					assert.False(t, hasWest, "the late request cannot publish a sample after the poll cutoff")
				}
			})
		})
	}
}
