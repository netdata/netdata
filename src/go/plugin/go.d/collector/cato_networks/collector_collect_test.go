// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_Collect(t *testing.T) {
	type collectStep struct {
		name        string
		setup       func(*testing.T, *Collector, *fakeAPIClient)
		collect     func(*testing.T, *Collector) (map[string]metrix.SampleValue, error)
		wantErr     string
		wantErrIs   error
		wantClass   string
		wantMetrics map[string]metrix.SampleValue
		wantMissing []string
		check       func(*testing.T, *Collector, *fakeAPIClient, map[string]metrix.SampleValue, error)
	}
	tests := map[string]struct {
		setup    func(*testing.T, *Collector, *fakeAPIClient)
		skipInit bool
		steps    []collectStep
	}{
		"fixture metrics and topology": {
			steps: []collectStep{{
				name: "collects metrics and topology",
				wantMetrics: map[string]metrix.SampleValue{
					stateMetricKey("site_connectivity_status", "connected", siteLabels("1001", "Paris Office", "POP-Paris")):           1,
					stateMetricKey("site_connectivity_status", "degraded", siteLabels("1002", "Toulouse Office", "POP-Toulouse")):      1,
					stateMetricKey("device_connection_status", "connected", deviceLabels("1001", "Paris Office", "dev-1", "Socket 1")): 1,
					metricKey("interface_bytes_upstream_max", interfaceLabels("1001", "Paris Office", "", "", "", "all")):              7168,
					metricKey("site_bytes_upstream_max", siteLabels("1001", "Paris Office", "POP-Paris")):                              7168,
					metricKey("site_packets_discarded_upstream", siteLabels("1001", "Paris Office", "POP-Paris")):                      2,
					metricKey("interface_packets_discarded_upstream", interfaceLabels("1001", "Paris Office", "", "", "", "all")):      2,
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):               1,
					stateMetricKey("bgp_routes_limit_status", "ok", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):          1,
					metricKey("bgp_routes", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                                  12,
					metricKey("bgp_routes_limit", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                            100,
					metricKey("bgp_rib_out_routes", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                          1,
				},
				check: func(t *testing.T, c *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
					topo, ok := c.topology.CurrentTopology()
					require.True(t, ok)
					require.Equal(t, topologySource, topo.Producer.Source)
					require.Equal(t, 6, topo.Actors.Rows)
					require.Equal(t, 3, topo.Links.Rows)
					collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
				},
			}},
		},
		"raw Centreon fixture through SDK": {
			setup: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				server := newRawCatoFixtureServer(t)
				t.Cleanup(server.Close)
				c.client = nil
				c.URL = server.URL
			},
			steps: []collectStep{{
				name: "decodes raw fixture",
				wantMetrics: map[string]metrix.SampleValue{
					stateMetricKey("site_connectivity_status", "connected", siteLabels("1001", "Paris Office", "POP-Paris")):          1,
					stateMetricKey("site_connectivity_status", "disconnected", siteLabels("1002", "Toulouse Office", "POP-Toulouse")): 1,
					stateMetricKey("site_connectivity_status", "degraded", siteLabels("1003", "Saint Girons Office", "POP-Ariege")):   1,
					metricKey("site_bytes_upstream_max", siteLabels("1001", "Paris Office", "POP-Paris")):                             4684,
					metricKey("site_packets_discarded_downstream", siteLabels("1001", "Paris Office", "POP-Paris")):                   1,
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):              1,
					metricKey("bgp_routes", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                                 12,
					metricKey("bgp_routes_limit", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                           100,
					metricKey("bgp_rib_out_routes", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                         1,
				},
			}},
		},
		"default accountMetrics SDK arguments": {
			steps: []collectStep{{
				name: "groupInterfaces is nil",
				check: func(t *testing.T, _ *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
					require.Len(t, fake.groupInterfaces, 1)
					require.Nil(t, fake.groupInterfaces[0])
				},
			}},
		},
		"unknown timeseries labels do not fail collection": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				iface := fake.metrics.GetAccountMetrics().GetSites()[0].GetInterfaces()[0]
				iface.Timeseries = append(iface.Timeseries, &catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{
					Label: "renamedByVendor",
					Data:  [][]float64{{1, 42}},
				})
			},
			steps: []collectStep{{
				name: "still emits known metrics",
				wantMetrics: map[string]metrix.SampleValue{
					metricKey("site_bytes_upstream_max", siteLabels("1001", "Paris Office", "POP-Paris")): 7168,
				},
			}},
		},
		"partial BGP failure is fail-soft": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				fake.bgpErrSites = map[string]error{"1002": errors.New("site bgp failed")}
			},
			steps: []collectStep{{
				name: "keeps successful site data",
				wantMetrics: map[string]metrix.SampleValue{
					metricKey("site_bytes_upstream_max", siteLabels("1001", "Paris Office", "POP-Paris")):                7168,
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")): 1,
				},
			}},
		},
		"partial BGP failure drops stale failed site state": func() struct {
			setup    func(*testing.T, *Collector, *fakeAPIClient)
			skipInit bool
			steps    []collectStep
		} {
			now := fixedCatoTestNow()
			stalePeerLabels := bgpLabels("1002", "Toulouse Office", "192.0.2.20", "64513")
			return struct {
				setup    func(*testing.T, *Collector, *fakeAPIClient)
				skipInit bool
				steps    []collectStep
			}{
				setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
					c.now = func() time.Time { return now }
					fake.bgp["1002"] = []*catosdk.SiteBgpStatusResult{
						{
							RemoteIP:   "192.0.2.20",
							RemoteASN:  "64513",
							BGPSession: "Established",
						},
					}
				},
				steps: []collectStep{
					{
						name: "emit initial peer",
						wantMetrics: map[string]metrix.SampleValue{
							stateMetricKey("bgp_session_status", "up", stalePeerLabels): 1,
						},
					},
					{
						name: "omit failed site peer next cycle",
						setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
							fake.bgpErrSites = map[string]error{"1002": errors.New("site bgp failed")}
						},
						wantMetrics: map[string]metrix.SampleValue{
							stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")): 1,
						},
						wantMissing: []string{
							stateMetricKey("bgp_session_status", "up", stalePeerLabels),
						},
					},
				},
			}
		}(),
		"unrecognized statuses map to unknown": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				fake.snapshot = &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
					Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
						{
							ID:                             new("1001"),
							ConnectivityStatusSiteSnapshot: connectivityPtr("Initializing"),
							OperationalStatusSiteSnapshot:  operationalPtr("Maintenance"),
							PopName:                        new("POP-Paris"),
							InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
								Name: new("Paris Office"),
							},
						},
					},
				}}
			},
			steps: []collectStep{{
				name: "emits unknown states",
				wantMetrics: map[string]metrix.SampleValue{
					stateMetricKey("site_connectivity_status", "unknown", siteLabels("1001", "Paris Office", "POP-Paris")): 1,
					stateMetricKey("site_operational_status", "unknown", siteLabels("1001", "Paris Office", "POP-Paris")):  1,
				},
			}},
		},
		"empty discovery fails collection": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				total := int64(0)
				fake.lookup = &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{Total: &total}}
			},
			steps: []collectStep{{
				name:      "returns empty error",
				wantErr:   "no Cato sites discovered",
				wantClass: "empty",
			}},
		},
		"discovers multiple pages": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				ids := numberedSiteIDs(1001, 205)
				fake.lookupPages = map[int64]*catosdk.EntityLookup{
					0:   fixtureLookupPage(205, ids[:100]...),
					100: fixtureLookupPage(205, ids[100:200]...),
					200: fixtureLookupPage(205, ids[200:]...),
				}
			},
			steps: []collectStep{{
				name: "stores discovered IDs in order",
				check: func(t *testing.T, c *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
					require.Len(t, c.discovery.siteIDs, 205)
					require.Equal(t, "1001", c.discovery.siteIDs[0])
					require.Equal(t, "1205", c.discovery.siteIDs[len(c.discovery.siteIDs)-1])
				},
			}},
		},
		"applies site selector": {
			setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
				c.SiteSelector = "!Toulouse* Paris*"
				total := int64(3)
				siteType := catomodels.EntityTypeSite
				fake.lookup = &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{
					Total: &total,
					Items: []*catosdk.EntityLookup_EntityLookup_Items{
						{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1001", Name: new("Paris Office"), Type: siteType}},
						{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1002", Name: new("Toulouse Office"), Type: siteType}},
						{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1003", Name: new("Madrid Office"), Type: siteType}},
					},
				}}
			},
			steps: []collectStep{{
				name: "collects selected site only",
				wantMetrics: map[string]metrix.SampleValue{
					stateMetricKey("site_connectivity_status", "connected", siteLabels("1001", "Paris Office", "POP-Paris")): 1,
				},
				wantMissing: []string{
					stateMetricKey("site_connectivity_status", "degraded", siteLabels("1002", "Toulouse Office", "POP-Toulouse")),
				},
				check: func(t *testing.T, c *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
					require.Equal(t, []string{"1001"}, c.discovery.siteIDs)
				},
			}},
		},
		"collects all interfaces and BGP peers": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				snapshot := fixtureSnapshot()
				snapshot.GetAccountSnapshot().GetSites()[0].GetDevices()[0].Interfaces = append(
					snapshot.GetAccountSnapshot().GetSites()[0].GetDevices()[0].GetInterfaces(),
					&catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces{
						ID:        new("wan2"),
						Name:      new("WAN 2"),
						Connected: new(true),
					},
				)
				fake.snapshot = snapshot
				fake.bgp["1001"] = append(fake.bgp["1001"], &catosdk.SiteBgpStatusResult{
					RemoteIP:   "192.0.2.11",
					RemoteASN:  "64513",
					BGPSession: "Established",
				})
			},
			steps: []collectStep{{
				name: "emits all entity label sets",
				wantMetrics: map[string]metrix.SampleValue{
					stateMetricKey("interface_connection_status", "connected", interfaceLabels("1001", "Paris Office", "dev-1", "Socket 1", "wan1", "WAN 1")): 1,
					stateMetricKey("interface_connection_status", "connected", interfaceLabels("1001", "Paris Office", "dev-1", "Socket 1", "wan2", "WAN 2")): 1,
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")):                                      1,
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.11", "64513")):                                      1,
				},
			}},
		},
		"uses cached discovery when refresh fails after bootstrap": {
			setup: func(_ *testing.T, c *Collector, _ *fakeAPIClient) {
				now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
				c.now = func() time.Time { return now }
			},
			steps: []collectStep{
				{name: "bootstrap"},
				{
					name: "uses cache after refresh failure",
					setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
						now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Add(seconds(defaultDiscoveryEvery) + time.Second)
						c.now = func() time.Time { return now }
						fake.lookupErr = errors.New("connection refused")
					},
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("site_connectivity_status", "connected", siteLabels("1001", "Paris Office", "POP-Paris")): 1,
					},
					check: func(t *testing.T, c *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, []string{"1001", "1002"}, c.discovery.siteIDs)
						require.Equal(t, time.Date(2026, 5, 1, 13, 0, 1, 0, time.UTC), c.discovery.fetchedAt)
					},
				},
			},
		},
		"BGP failures are not cached": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				fake.bgpErrSites = map[string]error{
					"1001": errors.New("rate limit exceeded"),
					"1002": errors.New("rate limit exceeded"),
				}
			},
			steps: []collectStep{
				{
					name: "first failed BGP cycle",
					wantMissing: []string{
						stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")),
					},
					check: func(t *testing.T, c *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 2, fake.bgpCalls)
						require.Empty(t, c.bgp.noBGPUntil)
					},
				},
				{
					name: "failed BGP sites are retried next cycle",
					check: func(t *testing.T, c *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 4, fake.bgpCalls)
						require.Empty(t, c.bgp.noBGPUntil)
					},
				},
			},
		},
		"filters empty BGP peers": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				fake.bgp["1001"] = []*catosdk.SiteBgpStatusResult{{}}
			},
			steps: []collectStep{{
				name: "omits empty peer label set",
				wantMissing: []string{
					stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "", "")),
				},
				check: func(t *testing.T, c *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
					require.NotContains(t, c.bgp.noBGPUntil, "1001")
				},
			}},
		},
		"caches sites without BGP peers for one hour with jitter": {
			setup: func(_ *testing.T, c *Collector, _ *fakeAPIClient) {
				now := fixedCatoTestNow()
				c.now = func() time.Time { return now }
			},
			steps: []collectStep{
				{
					name: "initial collection caches no-peer site",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")): 1,
					},
					check: func(t *testing.T, c *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 2, fake.bgpCalls)
						require.NotContains(t, c.bgp.noBGPUntil, "1001")
						require.Equal(t, c.noBGPCacheUntil("1002", fixedCatoTestNow()), c.bgp.noBGPUntil["1002"])
					},
				},
				{
					name: "no-peer site is skipped within TTL",
					setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
						now := fixedCatoTestNow().Add(time.Minute)
						c.now = func() time.Time { return now }
						fake.lookupErr = errors.New("unexpected discovery refresh")
						fake.bgpErrSites = map[string]error{
							"1002": errors.New("unexpected bgp refresh"),
						}
					},
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_session_status", "up", bgpLabels("1001", "Paris Office", "192.0.2.10", "64512")): 1,
					},
					check: func(t *testing.T, _ *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 3, fake.bgpCalls)
					},
				},
				{
					name: "expired no-peer site is polled again",
					setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
						now := c.noBGPCacheUntil("1002", fixedCatoTestNow()).Add(time.Second)
						c.now = func() time.Time { return now }
						fake.lookupErr = nil
						fake.bgpErrSites = nil
						fake.bgp["1002"] = []*catosdk.SiteBgpStatusResult{
							{
								RemoteIP:   "192.0.2.20",
								RemoteASN:  "64513",
								BGPSession: "Established",
							},
						}
					},
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_session_status", "up", bgpLabels("1002", "Toulouse Office", "192.0.2.20", "64513")): 1,
					},
					check: func(t *testing.T, c *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 5, fake.bgpCalls)
						require.NotContains(t, c.bgp.noBGPUntil, "1002")
					},
				},
			},
		},
		"collection fails before BGP polling": {
			setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
				fake.bgp["1002"] = []*catosdk.SiteBgpStatusResult{
					{
						RemoteIP:   "192.0.2.20",
						RemoteASN:  "64513",
						BGPSession: "Established",
					},
				}
				now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
				c.now = func() time.Time { return now }
			},
			steps: []collectStep{
				{name: "first collection"},
				{
					name: "snapshot failure aborts before BGP polling",
					setup: func(_ *testing.T, c *Collector, fake *fakeAPIClient) {
						bgpCalls := fake.bgpCalls
						fake.snapshotErr = errors.New("snapshot unavailable")
						now := time.Date(2026, 5, 1, 12, 1, 0, 0, time.UTC)
						c.now = func() time.Time { return now }
						fake.bgpCalls = bgpCalls
					},
					wantErr: "account snapshot failed",
					check: func(t *testing.T, _ *Collector, fake *fakeAPIClient, _ map[string]metrix.SampleValue, _ error) {
						require.Equal(t, 2, fake.bgpCalls)
					},
				},
			},
		},
		"provider errors are classified": {
			setup: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				server := newRawCatoFixtureServerWithResponses(t, map[string]rawCatoResponse{
					operationDiscovery: {
						status: http.StatusUnauthorized,
						body:   `{"errors":[{"message":"Unauthorized"}]}`,
					},
				})
				t.Cleanup(server.Close)
				c.client = nil
				c.URL = server.URL
			},
			steps: []collectStep{{
				name:      "auth failure",
				wantErr:   "site discovery failed",
				wantClass: "auth",
			}},
		},
		"rate limit errors are classified": {
			setup: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				server := newRawCatoFixtureServerWithResponses(t, map[string]rawCatoResponse{
					operationSnapshot: {
						status: http.StatusTooManyRequests,
						body:   `{"errors":[{"message":"rate limit exceeded"}]}`,
					},
				})
				t.Cleanup(server.Close)
				c.client = nil
				c.URL = server.URL
			},
			steps: []collectStep{{
				name:      "snapshot rate limit",
				wantErr:   "account snapshot failed",
				wantClass: "rate_limit",
			}},
		},
		"canceled context returns cancellation": {
			setup: func(_ *testing.T, _ *Collector, fake *fakeAPIClient) {
				fake.lookupErr = context.Canceled
			},
			steps: []collectStep{{
				name:      "context canceled",
				wantErrIs: context.Canceled,
				collect: func(t *testing.T, c *Collector) (map[string]metrix.SampleValue, error) {
					cc := mustCycleController(t, c.store)
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					cc.BeginCycle()
					err := c.Collect(ctx)
					cc.AbortCycle()
					return nil, err
				},
			}},
		},
		"provider error is sanitized": {
			setup: func(t *testing.T, c *Collector, _ *fakeAPIClient) {
				server := newRawCatoFixtureServerWithResponses(t, map[string]rawCatoResponse{
					operationDiscovery: {
						status: http.StatusUnauthorized,
						body:   `{"errors":[{"message":"Unauthorized secret token leaked by upstream"}]}`,
					},
				})
				t.Cleanup(server.Close)
				c.client = nil
				c.URL = server.URL
			},
			steps: []collectStep{{
				name:      "redacts provider detail",
				wantErr:   "site discovery failed",
				wantClass: "auth",
				check: func(t *testing.T, _ *Collector, _ *fakeAPIClient, _ map[string]metrix.SampleValue, err error) {
					require.Contains(t, err.Error(), "error_class=auth")
					require.NotContains(t, err.Error(), "secret")
					require.NotContains(t, err.Error(), "Unauthorized")
				},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, fake := newTestCollector()
			if tc.setup != nil {
				tc.setup(t, c, fake)
			}
			if !tc.skipInit {
				initCollector(t, c)
			}

			for _, step := range tc.steps {
				t.Run(step.name, func(t *testing.T) {
					if step.setup != nil {
						step.setup(t, c, fake)
					}
					collect := step.collect
					if collect == nil {
						collect = collectScalarSeries
					}

					mx, err := collect(t, c)
					if step.wantErr != "" || step.wantErrIs != nil {
						require.Error(t, err)
						if step.wantErr != "" {
							require.ErrorContains(t, err, step.wantErr)
						}
						if step.wantErrIs != nil {
							require.ErrorIs(t, err, step.wantErrIs)
						}
						if step.wantClass != "" {
							require.Equal(t, step.wantClass, classifyCatoError(err))
						}
						if step.check != nil {
							step.check(t, c, fake, mx, err)
						}
						return
					}

					require.NoError(t, err)
					requireMetricValues(t, mx, step.wantMetrics)
					requireMetricsMissing(t, mx, step.wantMissing)
					if step.check != nil {
						step.check(t, c, fake, mx, err)
					}
				})
			}
		})
	}
}

func TestCollector_WriteMetrics(t *testing.T) {
	tests := map[string]struct {
		sites map[string]*siteState
		order []string
		want  map[string]metrix.SampleValue
	}{
		"distinguishes duplicate interface names": {
			sites: map[string]*siteState{
				"1001": {
					ID:         "1001",
					Name:       "Paris Office",
					Interfaces: make(map[string]*interfaceState),
				},
			},
			order: []string{"1001"},
			want: map[string]metrix.SampleValue{
				metricKey("interface_bytes_upstream_max", interfaceLabels("1001", "Paris Office", "", "", "wan1", "WAN")): 100,
				metricKey("interface_bytes_upstream_max", interfaceLabels("1001", "Paris Office", "", "", "wan2", "WAN")): 200,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.sites["1001"].Interfaces[interfaceKey("wan1", "WAN")] = &interfaceState{
				ID:   "wan1",
				Name: "WAN",
				Metrics: trafficMetrics{
					present:          trafficMetricBytesUpstreamMax,
					BytesUpstreamMax: 100,
				},
			}
			tc.sites["1001"].Interfaces[interfaceKey("wan2", "WAN")] = &interfaceState{
				ID:   "wan2",
				Name: "WAN",
				Metrics: trafficMetrics{
					present:          trafficMetricBytesUpstreamMax,
					BytesUpstreamMax: 200,
				},
			}

			c := New()
			cc := mustCycleController(t, c.store)
			cc.BeginCycle()
			c.writeMetrics(tc.sites, tc.order)
			cc.CommitCycleSuccess()

			got := map[string]metrix.SampleValue{}
			c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
				got[metricKeyFromLabelView(name, labels)] = value
			})
			requireMetricValues(t, got, tc.want)
		})
	}
}

func TestNoBGPCache(t *testing.T) {
	tests := map[string]struct {
		setup func(map[string]time.Time, time.Time)
		want  []string
	}{
		"keeps active unexpired sites": {
			setup: func(noBGPUntil map[string]time.Time, now time.Time) {
				noBGPUntil["1002"] = now.Add(time.Hour)
			},
			want: []string{"1002"},
		},
		"prunes inactive sites": {
			setup: func(noBGPUntil map[string]time.Time, now time.Time) {
				noBGPUntil["1001"] = now.Add(time.Hour)
				noBGPUntil["1002"] = now.Add(time.Hour)
			},
			want: []string{"1002"},
		},
		"prunes expired sites": {
			setup: func(noBGPUntil map[string]time.Time, now time.Time) {
				noBGPUntil["1002"] = now.Add(-time.Second)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			now := fixedCatoTestNow()
			noBGPUntil := map[string]time.Time{}
			tc.setup(noBGPUntil, now)

			pruneNoBGPCache(noBGPUntil, []string{"1002"}, now)

			require.ElementsMatch(t, tc.want, mapKeys(noBGPUntil))
		})
	}
}

func TestNoBGPCacheJitter(t *testing.T) {
	c := New()
	c.UpdateEvery = defaultUpdateEvery

	first := c.noBGPCacheJitter("1001")
	require.Equal(t, first, c.noBGPCacheJitter("1001"))
	require.GreaterOrEqual(t, first, time.Duration(0))
	require.Less(t, first, seconds(defaultUpdateEvery)*5)
	require.Greater(t, seconds(defaultUpdateEvery)*5, seconds(defaultUpdateEvery))
}

func siteLabels(siteID, siteName, popName string) metrix.Labels {
	return metrix.Labels{"site_id": siteID, "site_name": siteName, "pop_name": popName}
}

func interfaceLabels(siteID, siteName, deviceID, deviceName, interfaceID, interfaceName string) metrix.Labels {
	return metrix.Labels{
		"site_id":        siteID,
		"site_name":      siteName,
		"device_id":      deviceID,
		"device_name":    deviceName,
		"interface_id":   interfaceID,
		"interface_name": interfaceName,
	}
}

func deviceLabels(siteID, siteName, deviceID, deviceName string) metrix.Labels {
	return metrix.Labels{"site_id": siteID, "site_name": siteName, "device_id": deviceID, "device_name": deviceName}
}

func bgpLabels(siteID, siteName, peerIP, peerASN string) metrix.Labels {
	return metrix.Labels{"site_id": siteID, "site_name": siteName, "peer_ip": peerIP, "peer_asn": peerASN}
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func metricKeyFromLabelView(name string, labels metrix.LabelView) string {
	out := make(metrix.Labels)
	labels.Range(func(key, value string) bool {
		out[key] = value
		return true
	})
	return metricKey(name, out)
}
