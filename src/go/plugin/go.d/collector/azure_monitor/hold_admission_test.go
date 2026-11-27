// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"io"
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

type admissionThrottleBody struct {
	ctx     context.Context
	started chan struct{}
	once    sync.Once
}

func (b *admissionThrottleBody) Read([]byte) (int, error) {
	// The SDK reaches the body only after the transport has returned the headers.
	b.once.Do(func() { close(b.started) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func admissionThrottleResponse(req *http.Request, status int, started chan struct{}) *http.Response {
	header := make(http.Header)
	header.Set("Retry-After", "180")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(&admissionThrottleBody{ctx: req.Context(), started: started}),
		Request:    req,
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

func TestCollectAppliesARMSubscriptionHoldBeforeBodyCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := testResource(recoveryTestNamespace, "hold-a", nil)
		inFlight := testResource(recoveryTestNamespace, "hold-b", nil)
		queued := testResource(recoveryTestNamespace, "hold-c", nil)
		rows := []map[string]any{first, inFlight, queued}
		c := newTestCollectorWithMocks(&mockResourceGraph{resources: rows}, &mockMetricsClient{})
		c.Config = testConfig()
		c.Limits.MaxConcurrency = 2
		useRecoveryTestProfile(t, c, []string{"A", "B"}, nil)

		start := time.Now()
		type lookup struct {
			id string
			at time.Duration
		}
		var mu sync.Mutex
		var reads []lookup
		var completedPeer bool
		peerSent, bodyStarted := make(chan struct{}), make(chan struct{})
		initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
			id, _ := strings.CutSuffix(req.URL.Path, "/providers/Microsoft.Insights/metricDefinitions")
			mu.Lock()
			reads = append(reads, lookup{id: id, at: time.Since(start)})
			mu.Unlock()
			switch id {
			case first["id"]:
				if err := waitForAdmissionSignal(req.Context(), peerSent); err != nil {
					return nil, err
				}
				resp := admissionThrottleResponse(req, http.StatusTooManyRequests, bodyStarted)
				resp.Header.Set("x-ms-ratelimit-remaining-subscription-reads", "0")
				return resp, nil
			case inFlight["id"]:
				close(peerSent)
				if err := waitForAdmissionSignal(req.Context(), bodyStarted); err != nil {
					return nil, err
				}
				time.Sleep(time.Second)
				completedPeer = true
			}
			return recoveryWireDefinitions(req, "A")
		}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
			if slices.Contains(query.Names, "B") {
				return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
			}
			return recoveryWireTotals(req, query, 5)
		})

		series := collectRecoveryCounters(t, c)
		requireRecoveryCounter(t, series, "A", "hold-b", 5)
		require.True(t, completedPeer, "the request already in flight may complete")
		assert.ElementsMatch(t, []lookup{{id: first["id"].(string)}, {id: inFlight["id"].(string)}}, reads,
			"the queued GET in the same subscription must respect the received hold")
	})
}

func TestCollectAppliesCanonicalMetricsHoldBeforeBodyCompletes(t *testing.T) {
	for label, test := range map[string]struct {
		queued       []string
		claimsReplay bool
		supplemental bool
	}{
		"queued ordinary POST":       {queued: []string{"A"}},
		"queued mixed POST":          {queued: []string{"A", "B"}},
		"ordinary claims replay":     {claimsReplay: true},
		"supplemental claims replay": {claimsReplay: true, supplemental: true},
	} {
		t.Run(label, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				row := testResource(recoveryTestNamespace, "edge", nil)
				c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
				c.Config = testConfig()
				c.Limits.MaxConcurrency = 2
				selections := [][]string{{"A"}, {"A"}}
				if test.supplemental {
					selections = [][]string{{"A", "X"}, {"A", "Y"}}
				}
				if len(test.queued) != 0 {
					selections = append(selections, test.queued)
				}
				if !slices.Contains(test.queued, "B") {
					selections = append(selections, []string{"B"})
				}
				useHoldAdmissionProfiles(t, c, selections)

				start := time.Now()
				var mu sync.Mutex
				var aCalls []time.Duration
				var aNamespaces []string
				var bCalls, mixedCalls, challengeResponses int
				peerSent, bodyStarted := make(chan struct{}), make(chan struct{})
				initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
					if !test.supplemental {
						t.Error("a throttle must not trigger a definitions lookup")
					}
					return recoveryWireDefinitions(req, "A")
				}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
					switch {
					case slices.Equal(query.Names, []string{"A", "B"}):
						mu.Lock()
						mixedCalls++
						mu.Unlock()
						return recoveryWireTotals(req, query, 7)
					case slices.Equal(query.Names, []string{"B"}):
						mu.Lock()
						bCalls++
						mu.Unlock()
						return recoveryWireTotals(req, query, 7)
					case slices.Equal(query.Names, []string{"A"}):
						mu.Lock()
						aCalls = append(aCalls, time.Since(start))
						aNamespaces = append(aNamespaces, req.URL.Query().Get("metricnamespace"))
						attempt := len(aCalls)
						mu.Unlock()
						switch attempt {
						case 1:
							if err := waitForAdmissionSignal(req.Context(), peerSent); err != nil {
								return nil, err
							}
							return admissionThrottleResponse(req, 529, bodyStarted), nil
						case 2:
							close(peerSent)
							if err := waitForAdmissionSignal(req.Context(), bodyStarted); err != nil {
								return nil, err
							}
							time.Sleep(time.Second)
							if test.claimsReplay {
								challengeResponses++
								resp, err := azureTestResponse(req, http.StatusUnauthorized, nil)
								if err == nil {
									resp.Header.Set("WWW-Authenticate", `Bearer error="insufficient_claims", claims="e30="`)
								}
								return resp, err
							}
						}
						return recoveryWireTotals(req, query, 5)
					case test.supplemental && (slices.Contains(query.Names, "X") || slices.Contains(query.Names, "Y")):
						return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
					default:
						return nil, fmt.Errorf("unexpected Metrics names %v", query.Names)
					}
				})

				series := collectRecoveryCounters(t, c)
				requireRecoveryCounter(t, series, "B", "edge", 7)
				assert.Zero(t, mixedCalls, "the held metric must be removed from a queued mixed request")
				assert.Equal(t, 1, bCalls, "a disjoint metric stays eligible")
				require.GreaterOrEqual(t, len(aNamespaces), 2, "both original requests reached Azure")
				assert.ElementsMatch(t, []string{recoveryTestNamespace, strings.ToUpper(recoveryTestNamespace)}, aNamespaces[:2])
				assert.Equal(t, []time.Duration{0, 0}, aCalls, "only A sends admitted before the received hold may go to Azure")
				var totals []float64
				for key, value := range series {
					if strings.Contains(key, ".a_total{") {
						totals = append(totals, value)
					}
				}
				if test.claimsReplay {
					assert.Equal(t, 1, challengeResponses, "Azure challenged the already admitted request")
					assert.Empty(t, totals, "a blocked replay cannot supply a sample")
				} else {
					assert.Equal(t, []float64{5}, totals, "the already admitted request can still publish")
				}
			})
		})
	}
}

func TestCollectHoldContinuationKeepsOriginalRequestDeadline(t *testing.T) {
	for label, test := range map[string]struct {
		supplemental bool
		timeout      time.Duration
		holdAt       time.Duration
		resumesAt    time.Duration
		fits         time.Duration
		overruns     time.Duration
	}{
		"ordinary SDK retry": {
			timeout:   45 * time.Second,
			holdAt:    time.Second,
			resumesAt: 40 * time.Second,
			fits:      2 * time.Second,
			overruns:  25 * time.Second,
		},
		"supplemental claims replay": {
			supplemental: true,
			timeout:      30 * time.Second,
			holdAt:       10 * time.Second,
			resumesAt:    29 * time.Second,
			fits:         500 * time.Millisecond,
			overruns:     4 * time.Second,
		},
	} {
		t.Run(label, func(t *testing.T) {
			for outcome, duration := range map[string]time.Duration{"fits original deadline": test.fits, "exceeds original deadline": test.overruns} {
				t.Run(outcome, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						row := testResource(recoveryTestNamespace, "edge", nil)
						c := newTestCollectorWithMocks(&mockResourceGraph{resources: []map[string]any{row}}, &mockMetricsClient{})
						c.Config = testConfig()
						c.Timeout = confopt.Duration(test.timeout)
						c.Limits.MaxConcurrency = 2
						selections := [][]string{{"A", "B"}, {"A"}, {"C"}}
						if test.supplemental {
							c.Limits.MaxConcurrency = 4
							selections = [][]string{{"A", "B", "X"}, {"A"}, {"D", "E"}, {"C"}}
						}
						useHoldAdmissionProfiles(t, c, selections)

						start := time.Now()
						mixedSent := make(chan struct{})
						var mixedOnce sync.Once
						type call struct {
							began, deadline, finished time.Duration
						}
						var mu sync.Mutex
						calls := make(map[string][]call)
						initRecoveryWire(t, c, func(req *http.Request) (*http.Response, error) {
							assert.True(t, test.supplemental, "ordinary retries and throttles do not require ARM")
							return recoveryWireDefinitions(req, "A", "B", "D")
						}, func(req *http.Request, query recoveryWireQuery) (*http.Response, error) {
							names := strings.Join(query.Names, ",")
							began := time.Since(start)
							deadline, _ := req.Context().Deadline()
							defer func() {
								trace := call{began, deadline.Sub(start), time.Since(start)}
								mu.Lock()
								calls[names] = append(calls[names], trace)
								mu.Unlock()
							}()

							switch names {
							case "A,B":
								mixedOnce.Do(func() { close(mixedSent) })
								if test.supplemental {
									time.Sleep(test.resumesAt)
									resp, err := azureTestResponse(req, http.StatusUnauthorized, nil)
									if err == nil {
										resp.Header.Set("WWW-Authenticate", `Bearer error="insufficient_claims", claims="e30="`)
									}
									return resp, err
								}
								resp, err := recoveryWireError(req, http.StatusServiceUnavailable, "ServiceUnavailable")
								if err == nil {
									resp.Header.Set("Retry-After", "40")
								}
								return resp, err
							case "A":
								if err := waitForAdmissionSignal(req.Context(), mixedSent); err != nil {
									return nil, err
								}
								time.Sleep(test.holdAt)
								resp, err := recoveryWireError(req, http.StatusTooManyRequests, "TooManyRequests")
								if err == nil {
									resp.Header.Set("Retry-After", "180")
								}
								return resp, err
							case "D,E":
								time.Sleep(28 * time.Second)
								return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
							case "A,B,X", "E", "X":
								return recoveryWireError(req, http.StatusBadRequest, "BadRequest")
							case "B":
								select {
								case <-time.After(duration):
								case <-req.Context().Done():
									return nil, req.Context().Err()
								}
							case "D":
								// This can succeed only with time granted to
								// the independent rejection at 28 seconds.
								select {
								case <-time.After(4 * time.Second):
								case <-req.Context().Done():
									return nil, req.Context().Err()
								}
							case "C":
							default:
								return nil, fmt.Errorf("unexpected Metrics names: %v", query.Names)
							}
							return recoveryWireTotals(req, query, 5)
						})

						series := collectRecoveryCounters(t, c)
						requireRecoveryCounter(t, series, "C", "edge", 5)
						require.Len(t, calls["A,B"], 1, "the held full selection cannot replay")
						assert.Equal(t, test.timeout, calls["A,B"][0].deadline)
						assert.Zero(t, calls["A,B"][0].began)

						bFinished := min(test.resumesAt+duration, test.timeout)
						assert.Equal(t, []call{{test.resumesAt, test.timeout, bFinished}}, calls["B"],
							"the unheld continuation gets the original request's remaining time")
						if test.resumesAt+duration < test.timeout {
							requireRecoveryCounter(t, series, "B", "edge", 5)
						} else {
							_, hasB := seriesValue(series, ".b_total", `resource_name="edge",`)
							assert.False(t, hasB, "the continuation cannot publish a late sample")
						}

						wantFinish := bFinished
						if test.supplemental {
							requireRecoveryCounter(t, series, "D", "edge", 5)
							assert.Equal(t, []call{{28 * time.Second, 54 * time.Second, 32 * time.Second}}, calls["D"])
							wantFinish = 32 * time.Second
						}
						assert.Equal(t, wantFinish, time.Since(start))
					})
				})
			}
		})
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
