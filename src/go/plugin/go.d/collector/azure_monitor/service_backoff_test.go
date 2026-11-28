// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func TestCollectMetricsBackoffIgnoresRequestsStartedBeforeHold(t *testing.T) {
	for _, test := range []struct {
		name       string
		peerStatus int
		retryAfter string
	}{
		{name: "late throttle", peerStatus: metricsBackendThrottled},
		{name: "late success", peerStatus: http.StatusOK},
		{name: "late explicit throttle", peerStatus: metricsBackendThrottled, retryAfter: "90"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				row := testResource(recoveryTestNamespace, "edge", nil)
				graph := &mockResourceGraph{resources: []map[string]any{row}}
				c := newTestCollectorWithMocks(graph, &mockMetricsClient{})
				c.Config = testConfig()
				c.Timeout = confopt.Duration(90 * time.Second)
				c.Limits.MaxConcurrency = 2
				// The two namespace spellings overlap on the same canonical Azure cell.
				useHoldAdmissionProfiles(t, c, [][]string{{"A"}, {"A"}, {"B"}})

				start := time.Now()
				peerSent := make(chan struct{})
				var mu sync.Mutex
				var sends []time.Duration
				initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("unexpected definitions read: %s", req.URL.Path)
				}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
					if slices.Equal(query.Names, []string{"B"}) {
						return recoveryWireTotals(req, query, 5)
					}
					if !slices.Equal(query.Names, []string{"A"}) {
						return nil, fmt.Errorf("unexpected Metrics names: %v", query.Names)
					}
					at := time.Since(start)
					mu.Lock()
					sends = append(sends, at)
					attempt := len(sends)
					mu.Unlock()
					switch attempt {
					case 1:
						if err := waitForAdmissionSignal(req.Context(), peerSent); err != nil {
							return nil, err
						}
						return recoveryWireError(req, metricsBackendThrottled, "Overloaded")
					case 2:
						close(peerSent)
						select {
						case <-time.After(70 * time.Second):
						case <-req.Context().Done():
							return nil, req.Context().Err()
						}
						if test.peerStatus != http.StatusOK {
							resp, err := recoveryWireError(req, test.peerStatus, "Overloaded")
							if test.retryAfter != "" {
								resp.Header.Set("Retry-After", test.retryAfter)
							}
							return resp, err
						}
					default:
						if test.peerStatus == http.StatusOK && (at == 2*time.Minute || at == 5*time.Minute) {
							return recoveryWireError(req, metricsBackendThrottled, "Overloaded")
						}
					}
					return recoveryWireTotals(req, query, 7)
				})

				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "B", "edge", 5)
				require.Equal(t, 70*time.Second, time.Since(start), "both admitted responses completed")
				require.Equal(t, []time.Duration{0, 0}, sends)
				collectAt := func(at time.Duration) []time.Duration {
					before := len(sends)
					time.Sleep(at - time.Since(start))
					collectRecoveryCounters(t, c)
					return sends[before:]
				}
				switch {
				case test.retryAfter != "":
					assert.Empty(t, collectAt(2*time.Minute), "the late explicit server deadline is still honored")
					assert.Equal(t, []time.Duration{3 * time.Minute, 3 * time.Minute}, collectAt(3*time.Minute))
				case test.peerStatus == http.StatusOK:
					assert.NotEmpty(t, collectAt(2*time.Minute), "a fresh attempt follows the first cooldown")
					assert.Empty(t, collectAt(3*time.Minute), "the late success did not erase the first throttle")
					assert.Equal(t, []time.Duration{4 * time.Minute, 4 * time.Minute}, collectAt(4*time.Minute))
					assert.NotEmpty(t, collectAt(5*time.Minute), "the fresh throttle reached Azure")
					assert.Equal(t, []time.Duration{6 * time.Minute, 6 * time.Minute}, collectAt(6*time.Minute),
						"a genuine post-hold success resets the next headerless delay to one minute")
				default:
					assert.Equal(t, []time.Duration{2 * time.Minute, 2 * time.Minute}, collectAt(2*time.Minute),
						"overlapping throttles must not impose a second fallback cooldown")
				}
			})
		})
	}
}

func TestCollectARMBackoffIgnoresReadsStartedBeforeHold(t *testing.T) {
	for _, test := range []struct {
		name         string
		peerSucceeds bool
	}{
		{name: "late throttle"},
		{name: "late success", peerSucceeds: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				first := testResource(recoveryTestNamespace, "first", nil)
				peer := testResource(recoveryTestNamespace, "peer", nil)
				graph := &mockResourceGraph{resources: []map[string]any{first, peer}}
				c := newTestCollectorWithMocks(graph, &mockMetricsClient{})
				c.Config = testConfig()
				c.Timeout = confopt.Duration(3 * time.Minute)
				c.UpdateEvery = 180
				c.Discovery.RefreshEvery = 120 // The peer's t=70 success is still fresh at t=180, and stale at t=360.
				c.Limits.MaxConcurrency = 2
				c.Limits.MaxBatchResources = 1
				useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)
				start := time.Now()
				peerSent := make(chan struct{})
				var mu sync.Mutex
				reads := make(map[string][]time.Duration)
				initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
					id, _ := strings.CutSuffix(req.URL.Path, "/providers/Microsoft.Insights/metricDefinitions")
					at := time.Since(start)
					mu.Lock()
					reads[id] = append(reads[id], at)
					mu.Unlock()
					throttle := func() (*http.Response, error) {
						resp, err := recoveryWireError(req, http.StatusTooManyRequests, "TooManyRequests")
						resp.Header.Set("x-ms-ratelimit-remaining-subscription-reads", "0")
						return resp, err
					}
					wait := func() error {
						select {
						case <-time.After(70 * time.Second):
							return nil
						case <-req.Context().Done():
							return req.Context().Err()
						}
					}
					switch {
					case id == first["id"] && at == 0:
						if err := waitForAdmissionSignal(req.Context(), peerSent); err != nil {
							return nil, err
						}
						return throttle()
					case id == peer["id"] && at == 0:
						close(peerSent)
						if err := wait(); err != nil {
							return nil, err
						}
						if !test.peerSucceeds {
							return throttle()
						}
					case id == first["id"] && at == 3*time.Minute && test.peerSucceeds:
						if err := wait(); err != nil {
							return nil, err
						}
						return throttle()
					}
					return recoveryWireDefinitions(req, "A")
				}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
					if slices.Contains(query.Names, "B") {
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					}
					return recoveryWireTotals(req, query, 5)
				})

				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "A", "first", 5)
				requireRecoveryCounter(t, series, "A", "peer", 5)
				require.Equal(t, 70*time.Second, time.Since(start), "both admitted responses completed")
				firstID, peerID := first["id"].(string), peer["id"].(string)
				require.Equal(t, []time.Duration{0}, reads[firstID])
				require.Equal(t, []time.Duration{0}, reads[peerID])
				time.Sleep(3*time.Minute - time.Since(start))
				collectRecoveryCounters(t, c)
				assert.Equal(t, []time.Duration{0, 3 * time.Minute}, reads[firstID],
					"a late peer response must not extend the original subscription cooldown")
				if test.peerSucceeds {
					assert.Equal(t, []time.Duration{0}, reads[peerID], "the peer's advice is still fresh")
					time.Sleep(6*time.Minute - time.Since(start))
					collectRecoveryCounters(t, c)
					assert.Equal(t, []time.Duration{0}, reads[peerID],
						"the late success must leave subscription history for the fresh throttle to double")
					time.Sleep(9*time.Minute - time.Since(start))
					collectRecoveryCounters(t, c)
					assert.Equal(t, []time.Duration{0, 3 * time.Minute, 9 * time.Minute}, reads[firstID])
					assert.Equal(t, []time.Duration{0, 9 * time.Minute}, reads[peerID])
				}
			})
		})
	}
}

func waitForAdmissionSignal(ctx context.Context, signal <-chan struct{}) error {
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func useHoldAdmissionProfiles(t *testing.T, c *Collector, selections [][]string) {
	t.Helper()
	var profiles []string
	sources := make(map[string]string, len(selections))
	for i, selection := range selections {
		profile := fmt.Sprintf("hold_%d", i)
		namespace := recoveryTestNamespace
		if i%2 != 0 {
			namespace = strings.ToUpper(namespace)
		}
		profiles = append(profiles, profile)
		var metrics, dimensions strings.Builder
		for _, name := range selection {
			metric := strings.ToLower(name)
			fmt.Fprintf(&metrics, `
  - id: %s
    azure_name: %s
    series:
      - aggregation: total
        kind: counter`, metric, name)
			fmt.Fprintf(&dimensions, `
        - selector: %s
          name: %s`, azureprofiles.ExportedSeriesName(profile, metric, "total"), metric)
		}
		sources[profile+".yaml"] = fmt.Sprintf(`
display_name: %s
resource_type: %s
metric_namespace: %s
metrics:%s
template:
  family: Recovery
  context_namespace: %s
  charts:
    - id: am_recovery_counts_%s
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
`, profile, recoveryTestNamespace, namespace, metrics.String(), profile, profile, dimensions.String())
	}
	c.Profiles.ModeExact = testProfilesModeEntries(profiles...)
	catalog := mustLoadStockCatalog(t, sources)
	c.loadProfileCatalog = func() (azureprofiles.Catalog, error) { return catalog, nil }
}
