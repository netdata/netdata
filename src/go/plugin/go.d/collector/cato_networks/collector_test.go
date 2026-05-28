// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
	catoscalars "github.com/catonetworks/cato-go-sdk/scalars"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

type fakeAPIClient struct {
	lookup          *catosdk.EntityLookup
	lookupErr       error
	lookupPages     map[int64]*catosdk.EntityLookup
	snapshot        *catosdk.AccountSnapshot
	snapshotErr     error
	metrics         *catosdk.AccountMetrics
	bgp             map[string][]*catosdk.SiteBgpStatusResult
	metricsErrSites map[string]error
	bgpErrSites     map[string]error
	groupInterfaces []*bool
	probeErr        error
	probeCalls      int
	lookupCalls     int
	snapshotCalls   int
	metricsCalls    int
	bgpCalls        int
}

func (f *fakeAPIClient) Probe(context.Context, string) error {
	f.probeCalls++
	return f.probeErr
}

func (f *fakeAPIClient) LookupSites(_ context.Context, _ string, _ int64, from int64) (*catosdk.EntityLookup, error) {
	f.lookupCalls++
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.lookupPages != nil {
		return f.lookupPages[from], nil
	}
	return f.lookup, nil
}

func (f *fakeAPIClient) AccountSnapshot(context.Context, string, []string) (*catosdk.AccountSnapshot, error) {
	f.snapshotCalls++
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return f.snapshot, nil
}

func (f *fakeAPIClient) AccountMetrics(_ context.Context, _ string, siteIDs []string, _ string, _ int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error) {
	f.metricsCalls++
	if groupInterfaces == nil {
		f.groupInterfaces = append(f.groupInterfaces, nil)
	} else {
		v := *groupInterfaces
		f.groupInterfaces = append(f.groupInterfaces, &v)
	}
	if len(siteIDs) > 0 && f.metricsErrSites != nil {
		if err := f.metricsErrSites[siteIDs[0]]; err != nil {
			return nil, err
		}
	}
	return f.metrics, nil
}

func (f *fakeAPIClient) SiteBgpStatus(_ context.Context, _ string, siteID string) ([]*catosdk.SiteBgpStatusResult, error) {
	f.bgpCalls++
	if f.bgpErrSites != nil {
		if err := f.bgpErrSites[siteID]; err != nil {
			return nil, err
		}
	}
	return f.bgp[siteID], nil
}

func (f *fakeAPIClient) APIStats() apiStats {
	return apiStats{}
}

func fixedCatoTestNow() time.Time {
	return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
}

func collectOnce(t *testing.T, c *Collector) {
	t.Helper()

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()
}

func TestCollectorCollectsMetricsAndTopology(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "site_connectivity_connected", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireValue(t, reader, "site_connectivity_degraded", metrix.Labels{
		"site_id":   "1002",
		"site_name": "Toulouse Office",
		"pop_name":  "POP-Toulouse",
	}, 1)
	requireValue(t, reader, "interface_bytes_upstream_max", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "",
		"interface_name": "all",
	}, 7168)
	requireValue(t, reader, "site_bytes_upstream_max", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 7168)
	requireValue(t, reader, "site_packets_discarded_upstream", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 2)
	requireValue(t, reader, "interface_packets_discarded_upstream", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "",
		"interface_name": "all",
	}, 2)
	requireValue(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)

	topo, ok := c.currentTopology()
	require.True(t, ok)
	require.Equal(t, topologySource, topo.Source)
	require.Len(t, topo.Actors, 5)
	require.Len(t, topo.Links, 3)
}

func TestCollectorOmitsTrafficMetricsWhenAccountMetricsDisabled(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = confopt.AutoBoolDisabled
	c.BGP.Enabled = confopt.AutoBoolDisabled
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "site_connectivity_connected", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireMetricMissing(t, reader, "site_bytes_upstream_max", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	})
	requireMetricMissing(t, reader, "interface_bytes_upstream_max", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "wan1",
		"interface_name": "WAN 1",
	})
}

func TestWriteMetricsDistinguishesDuplicateInterfaceNames(t *testing.T) {
	c := New()
	sites := map[string]*siteState{
		"1001": {
			ID:         "1001",
			Name:       "Paris Office",
			Interfaces: make(map[string]*interfaceState),
		},
	}
	sites["1001"].Interfaces[interfaceKey("wan1", "WAN")] = &interfaceState{
		ID:   "wan1",
		Name: "WAN",
		Metrics: trafficMetrics{
			present:          trafficMetricBytesUpstreamMax,
			BytesUpstreamMax: 100,
		},
	}
	sites["1001"].Interfaces[interfaceKey("wan2", "WAN")] = &interfaceState{
		ID:   "wan2",
		Name: "WAN",
		Metrics: trafficMetrics{
			present:          trafficMetricBytesUpstreamMax,
			BytesUpstreamMax: 200,
		},
	}

	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	c.writeMetrics(sites, []string{"1001"})
	cc.CommitCycleSuccess()

	reader := c.store.Read()
	requireValue(t, reader, "interface_bytes_upstream_max", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "wan1",
		"interface_name": "WAN",
	}, 100)
	requireValue(t, reader, "interface_bytes_upstream_max", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "wan2",
		"interface_name": "WAN",
	}, 200)
}

func TestCollectorDecodesRawCentreonFixtureThroughSDK(t *testing.T) {
	server := newRawCatoFixtureServer(t)
	defer server.Close()

	c := New()
	c.URL = server.URL
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 1)
	requireValue(t, reader, "collector_discovered_sites", nil, 3)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationDiscovery}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationSnapshot}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationMetrics}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationBGP}, 1)

	requireValue(t, reader, "site_connectivity_connected", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireValue(t, reader, "site_connectivity_disconnected", metrix.Labels{
		"site_id":   "1002",
		"site_name": "Toulouse Office",
		"pop_name":  "POP-Toulouse",
	}, 1)
	requireValue(t, reader, "site_connectivity_degraded", metrix.Labels{
		"site_id":   "1003",
		"site_name": "Saint Girons Office",
		"pop_name":  "POP-Ariege",
	}, 1)
	requireValue(t, reader, "site_bytes_upstream_max", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 4684)
	requireValue(t, reader, "site_packets_discarded_downstream", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireValue(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)
	requireValue(t, reader, "bgp_routes", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 12)
	requireValue(t, reader, "bgp_routes_limit", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 100)
	requireValue(t, reader, "bgp_rib_out_routes", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)
}

func TestConfigValidation(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	err := cfg.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "'account_id' is required")
	require.Contains(t, err.Error(), "'api_key' is required")

	cfg.AccountID = "12345"
	cfg.APIKey = "secret"
	require.NoError(t, cfg.validate())
	require.Equal(t, defaultEndpoint, cfg.URL)
	require.Equal(t, defaultUpdateEvery, cfg.UpdateEvery)

	cfg.Metrics.TimeFrame = "garbage"
	err = cfg.validate()
	require.ErrorContains(t, err, "'metrics.time_frame'")

	cfg.Metrics.TimeFrame = "utc.2020-02-11/{04:50:15--16:50:15}"
	require.NoError(t, cfg.validate())

	cfg.URL = "ftp://api.catonetworks.com/api/v1/graphql2"
	err = cfg.validate()
	require.ErrorContains(t, err, "'url' scheme")

	cfg.URL = "http://api.catonetworks.com/api/v1/graphql2"
	err = cfg.validate()
	require.ErrorContains(t, err, "'url' scheme")

	cfg.URL = "http://127.0.0.1:8080/api/v1/graphql2"
	require.NoError(t, cfg.validate())
}

func TestConfigApplyDefaultsNormalizesStringInputs(t *testing.T) {
	cfg := Config{
		AccountID: " 12345 ",
		APIKey:    " secret ",
		Limits: LimitsConfig{
			MaxSites:             new(0),
			MaxInterfacesPerSite: new(0),
		},
		BGP: BGPConfig{
			MaxPeersPerSite: new(0),
		},
	}
	cfg.URL = " https://api.catonetworks.com/api/v1/graphql2 "
	cfg.SiteSelector = " !lab-* * "
	cfg.InterfaceSelector = " wan* "
	cfg.Metrics.TimeFrame = " last.PT5M "
	cfg.BGP.PeerSelector = " 64512 "

	cfg.applyDefaults()

	require.Equal(t, "12345", cfg.AccountID)
	require.Equal(t, "secret", cfg.APIKey)
	require.Equal(t, "https://api.catonetworks.com/api/v1/graphql2", cfg.URL)
	require.Equal(t, "!lab-* *", cfg.SiteSelector)
	require.Equal(t, "wan*", cfg.InterfaceSelector)
	require.Equal(t, "last.PT5M", cfg.Metrics.TimeFrame)
	require.Equal(t, "64512", cfg.BGP.PeerSelector)
	require.Zero(t, cfg.maxSitesLimit())
	require.Zero(t, cfg.maxInterfacesPerSiteLimit())
	require.Zero(t, cfg.bgpMaxPeersPerSiteLimit())
	require.NoError(t, cfg.validate())
}

func TestGroupInterfacesAutoUsesNilSDKArgument(t *testing.T) {
	cfg := Config{Metrics: MetricsConfig{GroupInterfaces: confopt.AutoBoolAuto}}
	require.Nil(t, cfg.groupInterfaces())

	cfg.Metrics.GroupInterfaces = confopt.AutoBoolEnabled
	require.NotNil(t, cfg.groupInterfaces())
	require.True(t, *cfg.groupInterfaces())

	cfg.Metrics.GroupInterfaces = confopt.AutoBoolDisabled
	require.NotNil(t, cfg.groupInterfaces())
	require.False(t, *cfg.groupInterfaces())
}

func TestCheckUsesOnlyCheapProbe(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	fake := newFixtureAPIClient()
	fake.metricsErrSites = map[string]error{"1001": errors.New("metrics should not be called")}
	fake.bgpErrSites = map[string]error{"1001": errors.New("BGP should not be called")}
	c.client = fake
	c.now = fixedCatoTestNow

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Equal(t, 1, fake.probeCalls)
	require.Zero(t, fake.lookupCalls)
	require.Zero(t, fake.snapshotCalls)
	require.Zero(t, fake.metricsCalls)
	require.Zero(t, fake.bgpCalls)
	require.Empty(t, c.discovery.siteIDs)
	require.Empty(t, c.bgp.bySite)
	_, ok := c.currentTopology()
	require.False(t, ok)
	require.Empty(t, c.health.OperationFailures)
	require.Empty(t, c.health.CollectionFailureTotals)
}

func TestCheckDoesNotPopulateBGPCache(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Empty(t, c.bgp.bySite)
	require.True(t, c.bgp.nextRefresh.IsZero())
}

func TestCheckPreservesProbeErrorCause(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	fake := newFixtureAPIClient()
	fake.probeErr = context.Canceled
	c.client = fake

	require.NoError(t, c.Init(context.Background()))
	err := c.Check(context.Background())

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "canceled", classifyCatoError(err))
	require.Contains(t, err.Error(), "Cato API probe failed")
}

func TestCollectorReportsUnknownTimeseriesLabels(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.BGP.Enabled = "no"
	fake := newFixtureAPIClient()
	iface := fake.metrics.GetAccountMetrics().GetSites()[0].GetInterfaces()[0]
	iface.Timeseries = append(iface.Timeseries, &catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{
		Label: "renamedByVendor",
		Data:  [][]float64{{1, 42}},
	})
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceMetrics,
		"issue":   normalizationIssueUnknownTimeseriesLabel,
	}, 1)
}

func TestCollectorContinuesOnPartialMetricsAndBGPFailures(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.MaxSitesPerQuery = 1
	fake := newFixtureAPIClient()
	fake.metricsErrSites = map[string]error{"1002": errors.New("site metrics failed")}
	fake.bgpErrSites = map[string]error{"1002": errors.New("site bgp failed")}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "site_bytes_upstream_max", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 7168)
	requireValue(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)
	requireValue(t, reader, "collector_collection_success", nil, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationMetrics}, 0)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationBGP}, 0)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationMetrics,
		"error_class": "error",
	}, 1)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationBGP,
		"error_class": "error",
	}, 1)
	requireValue(t, reader, "collector_operation_affected_sites_total", metrix.Labels{
		"operation":   operationMetrics,
		"error_class": "error",
	}, 1)
	requireValue(t, reader, "collector_operation_affected_sites_total", metrix.Labels{
		"operation":   operationBGP,
		"error_class": "error",
	}, 1)
}

func TestCollectorMapsUnrecognizedStatusesToUnknown(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	fake := newFixtureAPIClient()
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
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "site_connectivity_unknown", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireValue(t, reader, "site_operational_unknown", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceSiteConnectivity,
		"issue":   normalizationIssueUnknownStatus,
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceSiteOperational,
		"issue":   normalizationIssueUnknownStatus,
	}, 1)
}

func TestCollectorReportsEmptyDiscovery(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	fake := newFixtureAPIClient()
	total := int64(0)
	fake.lookup = &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{Total: &total}}
	c.client = fake
	c.now = func() time.Time { return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	reader := c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 0)
	requireValue(t, reader, "collector_collection_failures_total", metrix.Labels{"error_class": "empty"}, 1)
}

func TestCollectorDiscoversMultiplePages(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Discovery.PageLimit = 2
	fake := newFixtureAPIClient()
	fake.lookupPages = map[int64]*catosdk.EntityLookup{
		0: fixtureLookupPage(5, "1001", "1002"),
		2: fixtureLookupPage(5, "1003", "1004"),
		4: fixtureLookupPage(5, "1005"),
	}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	require.Equal(t, []string{"1001", "1002", "1003", "1004", "1005"}, c.discovery.siteIDs)
	reader := c.store.Read()
	requireValue(t, reader, "collector_discovered_sites", nil, 5)
}

func TestCollectorAppliesSiteSelectorAndLimit(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.SiteSelector = "!Toulouse* *"
	c.Limits.MaxSites = new(1)
	fake := newFixtureAPIClient()
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
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	require.Equal(t, []string{"1001"}, c.discovery.siteIDs)
	reader := c.store.Read()
	requireValue(t, reader, "collector_discovered_sites", nil, 3)
	requireValue(t, reader, "collector_selected_entities", metrix.Labels{"entity": selectionEntitySite}, 1)
	requireValue(t, reader, "collector_skipped_entities", metrix.Labels{"entity": selectionEntitySite, "reason": selectionSkipSelector}, 1)
	requireValue(t, reader, "collector_skipped_entities", metrix.Labels{"entity": selectionEntitySite, "reason": selectionSkipLimit}, 1)
	requireValue(t, reader, "collector_cardinality_limit_hit", metrix.Labels{"entity": selectionEntitySite}, 1)
	requireValue(t, reader, "site_connectivity_connected", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"pop_name":  "POP-Paris",
	}, 1)
	requireMetricMissing(t, reader, "site_connectivity_degraded", metrix.Labels{
		"site_id":   "1002",
		"site_name": "Toulouse Office",
		"pop_name":  "POP-Toulouse",
	})
}

func TestCollectorAppliesInterfaceAndBGPPeerSelectorsAndLimits(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.InterfaceSelector = "wan*"
	c.Limits.MaxInterfacesPerSite = new(1)
	c.BGP.MaxPeersPerSite = new(1)
	c.BGP.MaxSitesPerCollection = 1
	fake := newFixtureAPIClient()
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
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_selected_entities", metrix.Labels{"entity": selectionEntityInterface}, 1)
	requireValue(t, reader, "collector_skipped_entities", metrix.Labels{"entity": selectionEntityInterface, "reason": selectionSkipSelector}, 1)
	requireValue(t, reader, "collector_skipped_entities", metrix.Labels{"entity": selectionEntityInterface, "reason": selectionSkipLimit}, 1)
	requireValue(t, reader, "collector_cardinality_limit_hit", metrix.Labels{"entity": selectionEntityInterface}, 1)
	requireValue(t, reader, "interface_connected", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "wan1",
		"interface_name": "WAN 1",
	}, 1)
	requireMetricMissing(t, reader, "interface_connected", metrix.Labels{
		"site_id":        "1001",
		"site_name":      "Paris Office",
		"interface_id":   "wan2",
		"interface_name": "WAN 2",
	})

	requireValue(t, reader, "collector_selected_entities", metrix.Labels{"entity": selectionEntityBGPPeer}, 1)
	requireValue(t, reader, "collector_skipped_entities", metrix.Labels{"entity": selectionEntityBGPPeer, "reason": selectionSkipLimit}, 1)
	requireValue(t, reader, "collector_cardinality_limit_hit", metrix.Labels{"entity": selectionEntityBGPPeer}, 1)
	requireValue(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)
	requireMetricMissing(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.11",
		"peer_asn":  "64513",
	})
}

func TestCollectorUsesCachedDiscoveryWhenRefreshFailsAfterBootstrap(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Discovery.RefreshEvery = 300
	fake := newFixtureAPIClient()
	c.client = fake
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	now = now.Add(301 * time.Second)
	fake.lookupErr = errors.New("connection refused")

	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	require.Equal(t, []string{"1001", "1002"}, c.discovery.siteIDs)
	require.Equal(t, now, c.discovery.fetchedAt)
	reader := c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationDiscovery}, 0)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationDiscovery,
		"error_class": "network",
	}, 1)
}

func TestCollectorDoesNotAdvanceBGPRotationWhenAllRequestsFail(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.MaxSitesPerCollection = 1
	fake := newFixtureAPIClient()
	fake.bgpErrSites = map[string]error{"1001": errors.New("rate limit exceeded")}
	c.client = fake
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	require.Zero(t, c.bgp.nextIndex)
	require.True(t, c.bgp.nextRefresh.IsZero())
	reader := c.store.Read()
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationBGP,
		"error_class": "rate_limit",
	}, 1)
	requireValue(t, reader, "collector_operation_affected_sites_total", metrix.Labels{
		"operation":   operationBGP,
		"error_class": "rate_limit",
	}, 1)
}

func TestCollectorFiltersEmptyBGPPeers(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.MaxSitesPerCollection = 1
	fake := newFixtureAPIClient()
	fake.bgp["1001"] = []*catosdk.SiteBgpStatusResult{{}}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceBGP,
		"issue":   normalizationIssueEmptyPeer,
	}, 1)
	_, ok := reader.Value("bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "",
		"peer_asn":  "",
	})
	require.False(t, ok)
}

func TestNormalizeBGPDropsPeersWithoutRemoteIdentity(t *testing.T) {
	peers, issues := normalizeBGP([]*catosdk.SiteBgpStatusResult{
		{IncomingConnection: catosdk.IncomingConnection{State: "Established"}},
	})

	require.Equal(t, []string{normalizationIssueEmptyPeer}, issues)
	require.Empty(t, peers)
}

func TestNormalizeBGPDeduplicatesPeerMetricLabels(t *testing.T) {
	peers, issues := normalizeBGP([]*catosdk.SiteBgpStatusResult{
		{RemoteIP: "192.0.2.10", RemoteASN: "64512", RoutesCount: "1"},
		{RemoteIP: "192.0.2.10", RemoteASN: "64512", RoutesCount: "2"},
	})

	require.Empty(t, issues)
	require.Len(t, peers, 1)
	require.Equal(t, int64(2), peers[0].RoutesCount)
}

func TestRecoverableWarningGateTracksErrorClass(t *testing.T) {
	c := New()

	c.warnRecoverable(warningKeyCollection, "network", "network failure")
	require.Equal(t, "network", c.warningStates[warningKeyCollection])

	c.warnRecoverable(warningKeyCollection, "network", "network failure")
	require.Equal(t, "network", c.warningStates[warningKeyCollection])

	c.warnRecoverable(warningKeyCollection, "auth", "auth failure")
	require.Equal(t, "auth", c.warningStates[warningKeyCollection])

	c.clearRecoverableWarning(warningKeyCollection)
	require.NotContains(t, c.warningStates, warningKeyCollection)
}

func TestCollectorKeepsLastOperationStatusForSkippedOperations(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	fake := newFixtureAPIClient()
	c.client = fake
	now := fixedCatoTestNow()
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	reader := c.store.Read()
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationDiscovery}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationBGP}, 1)

	now = now.Add(time.Second)
	fake.lookupErr = errors.New("unexpected discovery refresh")
	fake.bgpErrSites = map[string]error{
		"1001": errors.New("unexpected bgp refresh"),
		"1002": errors.New("unexpected bgp refresh"),
		"1003": errors.New("unexpected bgp refresh"),
	}
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	reader = c.store.Read()
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationDiscovery}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationBGP}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationSnapshot}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationMetrics}, 1)
	requireValue(t, reader, "collector_bgp_sites_per_collection", nil, 2)
	requireValue(t, reader, "collector_bgp_full_scan_seconds", nil, 300)
	requireValue(t, reader, "collector_bgp_cached_sites", nil, 2)
}

func TestMergeMetricsMergesAllInterfaceIntoSiteMetrics(t *testing.T) {
	siteBytesUpstream := float64(100)
	siteRTT := int64(42)
	metrics := &catosdk.AccountMetrics{AccountMetrics: &catosdk.AccountMetrics_AccountMetrics{
		Sites: []*catosdk.AccountMetrics_AccountMetrics_Sites{
			{
				ID: new("1001"),
				Metrics: &catosdk.AccountMetrics_AccountMetrics_Sites_Metrics{
					BytesUpstream: &siteBytesUpstream,
					Rtt:           &siteRTT,
				},
				Interfaces: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{
					{
						Name: new("all"),
						Timeseries: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{
							{Label: "bytesDownstreamMax", Data: [][]float64{{1, 200}}},
						},
					},
				},
			},
		},
	}}
	sites := map[string]*siteState{
		"1001": {ID: "1001", Interfaces: make(map[string]*interfaceState)},
	}

	require.Empty(t, mergeMetrics(metrics, sites))
	require.Equal(t, float64(100), sites["1001"].Metrics.BytesUpstreamMax)
	require.Equal(t, float64(200), sites["1001"].Metrics.BytesDownstreamMax)
	require.Equal(t, float64(42), sites["1001"].Metrics.RTTMS)
}

func TestBGPPollingRotatesAcrossSites(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.BGP.RefreshEvery = 60
	c.BGP.MaxSitesPerCollection = 1
	fake := newFixtureAPIClient()
	fake.bgp["1002"] = []*catosdk.SiteBgpStatusResult{
		{
			RemoteIP:   "192.0.2.20",
			RemoteASN:  "64513",
			BGPSession: "Established",
		},
	}
	c.client = fake
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()
	require.Len(t, c.bgp.bySite, 1)
	require.Contains(t, c.bgp.bySite, "1001")

	now = now.Add(61 * time.Second)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()
	require.Len(t, c.bgp.bySite, 2)
	require.Contains(t, c.bgp.bySite, "1002")
	reader := c.store.Read()
	requireValue(t, reader, "collector_bgp_sites_per_collection", nil, 1)
	requireValue(t, reader, "collector_bgp_full_scan_seconds", nil, 120)
	requireValue(t, reader, "collector_bgp_cached_sites", nil, 2)
}

func TestCollectorResetsBGPHealthWhenCollectionFailsBeforeBGP(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.RefreshEvery = 60
	c.BGP.MaxSitesPerCollection = 1
	fake := newFixtureAPIClient()
	fake.bgp["1002"] = []*catosdk.SiteBgpStatusResult{
		{
			RemoteIP:   "192.0.2.20",
			RemoteASN:  "64513",
			BGPSession: "Established",
		},
	}
	c.client = fake
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	now = now.Add(61 * time.Second)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()
	reader := c.store.Read()
	requireValue(t, reader, "collector_bgp_sites_per_collection", nil, 1)
	requireValue(t, reader, "collector_bgp_full_scan_seconds", nil, 120)
	requireValue(t, reader, "collector_bgp_cached_sites", nil, 2)

	fake.snapshotErr = errors.New("snapshot unavailable")
	now = now.Add(61 * time.Second)
	cc.BeginCycle()
	err := c.Collect(context.Background())
	cc.CommitCycleSuccess()

	require.NoError(t, err)
	reader = c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 0)
	requireValue(t, reader, "collector_collection_failures_total", metrix.Labels{"error_class": "error"}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationSnapshot}, 0)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationSnapshot,
		"error_class": "error",
	}, 1)
	requireValue(t, reader, "collector_bgp_sites_per_collection", nil, 0)
	requireValue(t, reader, "collector_bgp_full_scan_seconds", nil, 0)
	requireValue(t, reader, "collector_bgp_cached_sites", nil, 0)
}

func TestPruneBGPStateRemovesMissingSites(t *testing.T) {
	bySite := map[string][]bgpPeerState{
		"1001": {{RemoteIP: "192.0.2.10"}},
		"1002": {{RemoteIP: "192.0.2.20"}},
	}

	pruneBGPState(bySite, []string{"1002"})

	require.NotContains(t, bySite, "1001")
	require.Contains(t, bySite, "1002")
}

func TestTopologyFunctionReturnsCurrentTopology(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.client = newFixtureAPIClient()
	c.now = func() time.Time { return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) }

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	handler := &funcTopology{collector: c}
	resp := handler.Handle(context.Background(), topologyMethodID, nil)

	require.Equal(t, 200, resp.Status)
	require.Equal(t, "topology", resp.ResponseType)
	data, ok := resp.Data.(*topology.Data)
	require.True(t, ok)
	require.Equal(t, topologySource, data.Source)
	require.NotEmpty(t, data.Actors)
	require.NotEmpty(t, data.Links)
}

func TestTopologyOmitsUnavailableTunnelMetrics(t *testing.T) {
	data := buildTopology("12345", map[string]*siteState{
		"1001": {
			ID:                 "1001",
			Name:               "Paris Office",
			ConnectivityStatus: "connected",
			PopName:            "POP-Paris",
			Interfaces:         make(map[string]*interfaceState),
		},
	}, []string{"1001"}, fixedCatoTestNow())

	require.Len(t, data.Links, 1)
	require.Equal(t, linkTypeTunnel, data.Links[0].LinkType)
	require.Empty(t, data.Links[0].Metrics)
}

func TestTopologyFunctionRequiresJobSelection(t *testing.T) {
	cfg := catoTopologyMethodConfig()

	require.False(t, cfg.AgentWide)
}

func TestBuildTopologyOmitsEmptyBGPPeerIPMatch(t *testing.T) {
	site := &siteState{
		ID:   "1001",
		Name: "Paris Office",
		BGPPeers: []bgpPeerState{
			{
				RemoteASN:  "64512",
				BGPSession: "Established",
			},
		},
	}

	data := buildTopology("12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))

	var peerActor *topology.Actor
	for i := range data.Actors {
		if data.Actors[i].ActorType == actorTypeBGPPeer {
			peerActor = &data.Actors[i]
			break
		}
	}
	require.NotNil(t, peerActor)
	require.Empty(t, peerActor.Match.IPAddresses)

	var bgpLink *topology.Link
	for i := range data.Links {
		if data.Links[i].LinkType == linkTypeBGP {
			bgpLink = &data.Links[i]
			break
		}
	}
	require.NotNil(t, bgpLink)
	require.Empty(t, bgpLink.Dst.Match.IPAddresses)
}

func TestBuildTopologyDeduplicatesBGPPeers(t *testing.T) {
	site := &siteState{
		ID:   "1001",
		Name: "Paris Office",
		BGPPeers: []bgpPeerState{
			{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
			{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
		},
	}

	data := buildTopology("12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))

	actorIDs := make(map[string]bool)
	var peerActors int
	for _, actor := range data.Actors {
		require.False(t, actorIDs[actor.ActorID], "duplicate actor_id %q", actor.ActorID)
		actorIDs[actor.ActorID] = true
		if actor.ActorType == actorTypeBGPPeer {
			peerActors++
		}
	}
	var bgpLinks int
	for _, link := range data.Links {
		if link.LinkType == linkTypeBGP {
			bgpLinks++
		}
	}
	require.Equal(t, 1, peerActors)
	require.Equal(t, 1, bgpLinks)
}

func TestSiteTopologyTablesAreDeterministic(t *testing.T) {
	site := &siteState{
		Interfaces: map[string]*interfaceState{
			"z": {Name: "WAN 2"},
			"a": {Name: "WAN 1"},
		},
		Devices: []deviceState{
			{ID: "z", Name: "Socket 2"},
			{ID: "a", Name: "Socket 1"},
		},
	}

	tables := siteTopologyTables(site)

	require.Equal(t, "WAN 1", tables["interfaces"][0]["name"])
	require.Equal(t, "WAN 2", tables["interfaces"][1]["name"])
	require.Equal(t, "Socket 1", tables["devices"][0]["name"])
	require.Equal(t, "Socket 2", tables["devices"][1]["name"])
}

func TestRetryableCatoErrors(t *testing.T) {
	require.True(t, isRetryableCatoError(context.Background(), errors.New("GraphQL rate limit exceeded")))
	require.True(t, isRetryableCatoError(context.Background(), errors.New("HTTP 429 Too Many Requests")))
	require.True(t, isRetryableCatoError(context.Background(), errors.New("HTTP 503 Service Unavailable")))
	require.True(t, isRetryableCatoError(context.Background(), fmt.Errorf("client timeout: %w", context.DeadlineExceeded)))
	require.False(t, isRetryableCatoError(context.Background(), errors.New("invalid API key")))
	require.False(t, isRetryableCatoError(context.Background(), context.Canceled))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, isRetryableCatoError(ctx, fmt.Errorf("caller timeout: %w", context.DeadlineExceeded)))
}

func TestClassifyCatoErrors(t *testing.T) {
	require.Equal(t, "auth", classifyCatoError(errors.New("HTTP 403 forbidden")))
	require.Equal(t, "rate_limit", classifyCatoError(errors.New("GraphQL rate limit exceeded")))
	require.Equal(t, "timeout", classifyCatoError(context.DeadlineExceeded))
	require.Equal(t, "canceled", classifyCatoError(fmt.Errorf("wrapped: %w", context.Canceled)))
	require.Equal(t, "timeout", classifyCatoError(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)))
	require.Equal(t, "decode", classifyCatoError(errors.New("json: cannot unmarshal number into Go struct field")))
	require.Equal(t, "network", classifyCatoError(&net.DNSError{Err: "no such host", Name: "api.invalid"}))
	require.Equal(t, "tls", classifyCatoError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")))
	require.Equal(t, "proxy", classifyCatoError(errors.New("proxyconnect tcp: dial tcp: connection refused")))
	require.Equal(t, "empty", classifyCatoError(errors.New("no Cato sites discovered")))
}

func TestSDKClientClassifiesHTTPAndGraphQLErrors(t *testing.T) {
	tests := map[string]struct {
		operation     string
		response      rawCatoResponse
		wantOperation string
		wantClass     string
	}{
		"auth failure during discovery": {
			operation:     operationDiscovery,
			response:      rawCatoResponse{status: http.StatusUnauthorized, body: `{"errors":[{"message":"Unauthorized"}]}`},
			wantOperation: operationDiscovery,
			wantClass:     "auth",
		},
		"rate limit during metrics": {
			operation:     operationMetrics,
			response:      rawCatoResponse{status: http.StatusTooManyRequests, body: `{"errors":[{"message":"rate limit exceeded"}]}`},
			wantOperation: operationMetrics,
			wantClass:     "rate_limit",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server := newRawCatoFixtureServerWithResponses(t, map[string]rawCatoResponse{tt.operation: tt.response})
			defer server.Close()

			c := New()
			c.URL = server.URL
			c.AccountID = "12345"
			c.APIKey = "secret"
			c.Retry.Attempts = 1
			c.now = func() time.Time { return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) }

			require.NoError(t, c.Init(context.Background()))
			cc := mustCycleController(t, c.store)
			cc.BeginCycle()
			err := c.Collect(context.Background())
			require.NoError(t, err)
			cc.CommitCycleSuccess()

			reader := c.store.Read()
			requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
				"operation":   tt.wantOperation,
				"error_class": tt.wantClass,
			}, 1)
		})
	}
}

func TestRawGraphQLAccountSnapshotUsesMethodAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "argument-account", r.Header.Get("x-account-id"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"accountID":"argument-account"`)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatusSiteSnapshot":"connected"}]}}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := rawGraphQLClient{
		url:        server.URL,
		apiKey:     "secret",
		httpClient: server.Client(),
	}

	snapshot, err := client.AccountSnapshot(context.Background(), "argument-account", []string{"1001"})

	require.NoError(t, err)
	require.Len(t, snapshot.GetAccountSnapshot().GetSites(), 1)
}

func TestRawGraphQLAccountSnapshotDoesNotOverrideReservedHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "secret", r.Header.Get("x-api-key"))
		require.Equal(t, "argument-account", r.Header.Get("x-account-id"))
		require.Equal(t, "custom-value", r.Header.Get("x-custom-header"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[]}}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := rawGraphQLClient{
		url:        server.URL,
		apiKey:     "secret",
		httpClient: server.Client(),
		headers: map[string]string{
			"Content-Type":    "text/plain",
			"x-api-key":       "wrong-key",
			"X-Account-Id":    "wrong-account",
			"X-Custom-Header": "custom-value",
		},
	}

	_, err := client.AccountSnapshot(context.Background(), "argument-account", nil)

	require.NoError(t, err)
}

func TestCatoRequestHeadersFiltersReservedHeaders(t *testing.T) {
	headers := catoRequestHeaders(map[string]string{
		"Content-Type":    "text/plain",
		"x-api-key":       "wrong-key",
		"X-Account-Id":    "wrong-account",
		"user-agent":      "custom-agent",
		"X-Custom-Header": "custom-value",
	})

	require.False(t, hasCatoHeader(headers, "Content-Type"))
	require.False(t, hasCatoHeader(headers, "x-api-key"))
	require.False(t, hasCatoHeader(headers, "x-account-id"))
	require.True(t, hasCatoHeader(headers, "User-Agent"))
	require.Equal(t, "custom-agent", headers["user-agent"])
	require.Equal(t, "custom-value", headers["X-Custom-Header"])
}

func TestSDKClientClassifiesHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	c := New()
	c.URL = server.URL
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Timeout = confopt.Duration(time.Millisecond)
	c.Retry.Attempts = 1

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	err := c.Collect(context.Background())
	cc.CommitCycleSuccess()

	require.NoError(t, err)
	reader := c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 0)
	requireValue(t, reader, "collector_collection_failures_total", metrix.Labels{"error_class": "timeout"}, 1)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationDiscovery,
		"error_class": "timeout",
	}, 1)
}

func TestCollectReturnsContextCancellationWithoutHealthFailure(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	fake := newFixtureAPIClient()
	fake.lookupErr = context.Canceled
	c.client = fake

	require.NoError(t, c.Init(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	err := c.Collect(ctx)
	cc.AbortCycle()

	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, c.health.CollectionFailureTotals)
	reader := c.store.Read()
	requireMetricMissing(t, reader, "collector_collection_success", nil)
}

func TestCollectorSanitizesReturnedProviderErrors(t *testing.T) {
	server := newRawCatoFixtureServerWithResponses(t, map[string]rawCatoResponse{
		operationDiscovery: {
			status: http.StatusUnauthorized,
			body:   `{"errors":[{"message":"Unauthorized secret token leaked by upstream"}]}`,
		},
	})
	defer server.Close()

	c := New()
	c.URL = server.URL
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Retry.Attempts = 1

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	err := c.collect(context.Background())
	cc.CommitCycleSuccess()

	require.Error(t, err)
	require.Contains(t, err.Error(), "site discovery failed")
	require.Contains(t, err.Error(), "error_class=auth")
	require.Equal(t, "auth", classifyCatoError(err))
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "Unauthorized")
}

func TestSDKClientAccountSnapshotFallsBackOnEnumDecodeError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data":{"accountSnapshot":{"sites":[{"id":"1001","connectivityStatusSiteSnapshot":"degraded","operationalStatusSiteSnapshot":"active"}]}}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	cfg := Config{
		AccountID: "12345",
		APIKey:    "secret",
		Retry: RetryConfig{
			Attempts: 1,
		},
	}
	cfg.URL = server.URL
	client, err := newSDKAPIClient(cfg, server.Client())
	require.NoError(t, err)

	snapshot, err := client.AccountSnapshot(context.Background(), "12345", []string{"1001"})

	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Len(t, snapshot.GetAccountSnapshot().GetSites(), 1)
	require.Equal(t, "degraded", connectivityStatusString(snapshot.GetAccountSnapshot().GetSites()[0].GetConnectivityStatusSiteSnapshot()))
}

func TestRetryWaitCapsAtMaximum(t *testing.T) {
	require.Equal(t, 2*time.Second, retryWait(1*time.Second, 2*time.Second, 4))
}

func TestBGPSessionUpRequiresExactEstablishedStatus(t *testing.T) {
	require.True(t, isBGPSessionUp("Established"))
	require.True(t, isBGPSessionUp("up"))
	require.False(t, isBGPSessionUp("not_established"))
	require.False(t, isBGPSessionUp("idle"))
}

func TestSDKClientRecordsRetryStats(t *testing.T) {
	client := &sdkAPIClient{
		retry: RetryConfig{
			Attempts: 2,
			WaitMin:  confopt.Duration(time.Millisecond),
			WaitMax:  confopt.Duration(time.Millisecond),
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}

	var calls int
	err := client.withRetry(context.Background(), "accountMetrics", func() error {
		calls++
		if calls == 1 {
			return errors.New("GraphQL Rate Limit Exceeded")
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, int64(1), client.APIStats().Retries["accountMetrics"].RateLimit)
	require.Zero(t, client.APIStats().Retries["accountMetrics"].Transient)
}

func TestWriteAPIStatsWritesRetryCounterTotalsAndDeltas(t *testing.T) {
	c := New()
	cc := mustCycleController(t, c.store)
	labels := metrix.Labels{"query": operationMetrics}

	cc.BeginCycle()
	writeAPIStats(c.metrics.api, apiStats{Retries: map[string]apiRetryStats{
		operationMetrics: {RateLimit: 2, Transient: 3},
	}})
	cc.CommitCycleSuccess()
	reader := c.store.Read()
	requireValue(t, reader, "api_rate_limit_retries_total", labels, 2)
	requireValue(t, reader, "api_transient_retries_total", labels, 3)
	requireNoDelta(t, reader, "api_rate_limit_retries_total", labels)
	requireNoDelta(t, reader, "api_transient_retries_total", labels)

	cc.BeginCycle()
	writeAPIStats(c.metrics.api, apiStats{Retries: map[string]apiRetryStats{
		operationMetrics: {RateLimit: 5, Transient: 7},
	}})
	cc.CommitCycleSuccess()
	reader = c.store.Read()
	requireValue(t, reader, "api_rate_limit_retries_total", labels, 5)
	requireValue(t, reader, "api_transient_retries_total", labels, 7)
	requireDelta(t, reader, "api_rate_limit_retries_total", labels, 3)
	requireDelta(t, reader, "api_transient_retries_total", labels, 4)
}

func TestSDKClientRetriesClientDeadlineExceeded(t *testing.T) {
	client := &sdkAPIClient{
		retry: RetryConfig{
			Attempts: 2,
			WaitMin:  confopt.Duration(time.Millisecond),
			WaitMax:  confopt.Duration(time.Millisecond),
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}

	var calls int
	err := client.withRetry(context.Background(), "accountSnapshot", func() error {
		calls++
		if calls == 1 {
			return fmt.Errorf("http client timeout: %w", context.DeadlineExceeded)
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, int64(1), client.APIStats().Retries["accountSnapshot"].Transient)
}

func TestNormalizeSnapshotDefaultsNilInfoAndStatuses(t *testing.T) {
	snapshot := &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
		Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
			{ID: new("1001")},
		},
	}}

	sites, order := normalizeSnapshot(snapshot, map[string]string{"1001": "Site One"})

	require.Equal(t, []string{"1001"}, order)
	require.Equal(t, "Site One", sites["1001"].Name)
	require.Empty(t, sites["1001"].Description)
	require.Empty(t, sites["1001"].CountryCode)
	require.Empty(t, sites["1001"].CountryName)
	require.Empty(t, sites["1001"].Region)
	require.Empty(t, sites["1001"].SiteType)
	require.Empty(t, sites["1001"].ConnectionType)
	require.Equal(t, "unknown", sites["1001"].ConnectivityStatus)
	require.Equal(t, "unknown", sites["1001"].OperationalStatus)
}

func TestChartTemplateCompiles(t *testing.T) {
	spec, err := charttpl.DecodeYAML([]byte(chartTemplate))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestChartTemplateDynamicChartsDeclareLifecycle(t *testing.T) {
	spec, err := charttpl.DecodeYAML([]byte(chartTemplate))
	require.NoError(t, err)

	for _, group := range spec.Groups {
		for _, chart := range group.Charts {
			if chart.Instances == nil {
				continue
			}
			require.NotNil(t, chart.Lifecycle, "chart %s must declare lifecycle", chart.ID)
			require.Equal(t, 5, chart.Lifecycle.ExpireAfterCycles, "chart %s lifecycle", chart.ID)
		}
	}
}

func TestConfigSchemaParses(t *testing.T) {
	require.True(t, json.Valid([]byte(configSchema)))
}

type rawCatoResponse struct {
	status int
	body   string
}

func newRawCatoFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	return newRawCatoFixtureServerWithResponses(t, nil)
}

func newRawCatoFixtureServerWithResponses(t *testing.T, overrides map[string]rawCatoResponse) *httptest.Server {
	t.Helper()

	responses := map[string]rawCatoResponse{
		operationDiscovery: {body: loadSDKCompatibleMockoonResponseBody(t, "entityLookup")},
		operationSnapshot:  {body: loadSDKCompatibleMockoonResponseBody(t, "accountSnapshot")},
		operationMetrics:   {body: loadSDKCompatibleMockoonResponseBody(t, "accountMetrics")},
		operationBGP:       {body: loadTestdata(t, "cato-site-bgp-status.schema-shaped.json")},
	}
	maps.Copy(responses, overrides)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("x-api-key") != "secret" || r.Header.Get("x-account-id") != "12345" {
			t.Errorf("unexpected auth headers: x-api-key present=%v x-account-id present=%v", r.Header.Get("x-api-key") != "", r.Header.Get("x-account-id") != "")
			http.Error(w, "unexpected auth headers", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		request := string(body)

		var operation string
		switch {
		case strings.Contains(request, "siteBgpStatus"):
			operation = operationBGP
		case strings.Contains(request, "entityLookup"):
			operation = operationDiscovery
		case strings.Contains(request, "accountSnapshot"):
			operation = operationSnapshot
		case strings.Contains(request, "accountMetrics"):
			operation = operationMetrics
		default:
			t.Errorf("unexpected Cato GraphQL request: %s", request)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}

		response := responses[operation]
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(response.body)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
}

func loadMockoonResponseBody(t *testing.T, label string) string {
	t.Helper()

	var env struct {
		Routes []struct {
			Responses []struct {
				Label string `json:"label"`
				Body  string `json:"body"`
			} `json:"responses"`
		} `json:"routes"`
	}
	require.NoError(t, json.Unmarshal([]byte(loadTestdata(t, "centreon-cato-api.mockoon.json")), &env))
	for _, route := range env.Routes {
		for _, response := range route.Responses {
			if response.Label == label {
				return response.Body
			}
		}
	}
	t.Fatalf("Mockoon response %q not found", label)
	return ""
}

func loadSDKCompatibleMockoonResponseBody(t *testing.T, label string) string {
	t.Helper()

	body := loadMockoonResponseBody(t, label)
	switch label {
	case "accountSnapshot":
		body = adaptAccountSnapshotFixtureForSDK(t, body)
	}
	return body
}

func adaptAccountSnapshotFixtureForSDK(t *testing.T, body string) string {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	sites := payload["data"].(map[string]any)["accountSnapshot"].(map[string]any)["sites"].([]any)
	for _, rawSite := range sites {
		site := rawSite.(map[string]any)
		if v, ok := site["connectivityStatus"]; ok {
			site["connectivityStatusSiteSnapshot"] = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
			delete(site, "connectivityStatus")
		}
		if v, ok := site["operationalStatus"]; ok {
			site["operationalStatusSiteSnapshot"] = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
			delete(site, "operationalStatus")
		}
		if v, ok := site["info"]; ok {
			site["infoSiteSnapshot"] = v
			delete(site, "info")
		}
		delete(site, "operationalStats")
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func loadTestdata(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(data)
}

func newFixtureAPIClient() *fakeAPIClient {
	return &fakeAPIClient{
		lookup:   fixtureLookup(),
		snapshot: fixtureSnapshot(),
		metrics:  fixtureMetrics(),
		bgp: map[string][]*catosdk.SiteBgpStatusResult{
			"1001": {
				{
					RemoteIP:         "192.0.2.10",
					RemoteASN:        "64512",
					LocalIP:          "198.51.100.10",
					LocalASN:         "65000",
					BGPSession:       "Established",
					RoutesCount:      "12",
					RoutesCountLimit: "100",
					RIBOut:           []catosdk.RIBOut{{Subnet: "10.0.0.0/8"}},
				},
			},
			"1002": nil,
		},
	}
}

func fixtureLookup() *catosdk.EntityLookup {
	total := int64(2)
	siteType := catomodels.EntityTypeSite
	return &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{
		Total: &total,
		Items: []*catosdk.EntityLookup_EntityLookup_Items{
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1001", Name: new("Paris Office"), Type: siteType}},
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1002", Name: new("Toulouse Office"), Type: siteType}},
		},
	}}
}

func fixtureLookupPage(total int64, ids ...string) *catosdk.EntityLookup {
	siteType := catomodels.EntityTypeSite
	items := make([]*catosdk.EntityLookup_EntityLookup_Items, 0, len(ids))
	for _, id := range ids {
		items = append(items, &catosdk.EntityLookup_EntityLookup_Items{
			Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: id, Name: new("Site " + id), Type: siteType},
		})
	}
	return &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{
		Total: &total,
		Items: items,
	}}
}

func fixtureSnapshot() *catosdk.AccountSnapshot {
	return &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
		Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
			{
				ID:                             new("1001"),
				ConnectivityStatusSiteSnapshot: connectivityPtr("connected"),
				OperationalStatusSiteSnapshot:  operationalPtr("active"),
				PopName:                        new("POP-Paris"),
				HostCount:                      new(int64(42)),
				ConnectedSince:                 new("2026-05-01T10:00:00Z"),
				InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
					Name:        new("Paris Office"),
					Description: new("Main site"),
					CountryCode: new("FR"),
					CountryName: new("France"),
				},
				Devices: []*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices{
					{
						ID:        new("dev-1"),
						Name:      new("Socket 1"),
						Type:      new("socket"),
						Connected: new(true),
						Interfaces: []*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces{
							{
								ID:             new("wan1"),
								Name:           new("WAN 1"),
								Connected:      new(true),
								PopName:        new("POP-Paris"),
								TunnelRemoteIP: new("203.0.113.10"),
								TunnelUptime:   new(int64(3600)),
							},
						},
					},
				},
			},
			{
				ID:                             new("1002"),
				ConnectivityStatusSiteSnapshot: connectivityPtr("Degraded"),
				OperationalStatusSiteSnapshot:  operationalPtr("locked"),
				PopName:                        new("POP-Toulouse"),
				HostCount:                      new(int64(7)),
				InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
					Name: new("Toulouse Office"),
				},
			},
		},
	}}
}

func fixtureMetrics() *catosdk.AccountMetrics {
	return &catosdk.AccountMetrics{AccountMetrics: &catosdk.AccountMetrics_AccountMetrics{
		Sites: []*catosdk.AccountMetrics_AccountMetrics_Sites{
			{
				ID:   new("1001"),
				Name: new("Paris Office"),
				Interfaces: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{
					{
						Name: new("all"),
						Timeseries: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{
							{Label: "bytesUpstreamMax", Data: [][]float64{{1, 6008}, {2, 7168}}},
							{Label: "bytesDownstreamMax", Data: [][]float64{{1, 12008}, {2, 11168}}},
							{Label: "lostUpstreamPcnt", Data: [][]float64{{1, 0.2}}},
							{Label: "packetsDiscardedUpstream", Data: [][]float64{{1, 1}, {2, 2}}},
							{Label: "packetsDiscardedDownstream", Data: [][]float64{{1, 3}, {2, 4}}},
							{Label: "rtt", Data: [][]float64{{1, 15}}},
						},
					},
				},
			},
		},
	}}
}

func requireValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.True(t, ok, "missing metric %s labels %#v", name, labels)
	require.Equal(t, want, got)
}

func requireMetricMissing(t *testing.T, r metrix.Reader, name string, labels metrix.Labels) {
	t.Helper()
	_, ok := r.Value(name, labels)
	require.False(t, ok, "unexpected metric %s labels %#v", name, labels)
}

func requireDelta(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Delta(name, labels)
	require.True(t, ok, "missing delta for metric %s labels %#v", name, labels)
	require.Equal(t, want, got)
}

func requireNoDelta(t *testing.T, r metrix.Reader, name string, labels metrix.Labels) {
	t.Helper()
	_, ok := r.Delta(name, labels)
	require.False(t, ok, "unexpected delta for metric %s labels %#v", name, labels)
}

func mustCycleController(t *testing.T, s metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	return managed.CycleController()
}

//go:fix inline
func strPtr(v string) *string { return new(v) }

//go:fix inline
func boolPtr(v bool) *bool { return new(v) }

//go:fix inline
func int64Ptr(v int64) *int64 { return new(v) }

func connectivityPtr(v string) *catomodels.ConnectivityStatus {
	status := catomodels.ConnectivityStatus(v)
	return &status
}

func operationalPtr(v string) *catoscalars.OperationalStatus {
	status := catoscalars.OperationalStatus(v)
	return &status
}
