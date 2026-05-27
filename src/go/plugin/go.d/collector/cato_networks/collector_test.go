// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	events          *catosdk.EventsFeed
	eventsByMarker  map[string]*catosdk.EventsFeed
	eventMarkers    []string
	bgp             map[string][]*catosdk.SiteBgpStatusResult
	metricsErrSites map[string]error
	bgpErrSites     map[string]error
	groupInterfaces []*bool
}

type flakyMarkerStore struct {
	readMarker string
	writeErrs  []error
	writes     []string
}

type temporaryMarkerError struct{ error }

func (temporaryMarkerError) Temporary() bool { return true }

func (s *flakyMarkerStore) read() (string, error) {
	return s.readMarker, nil
}

func (s *flakyMarkerStore) write(marker string) error {
	s.writes = append(s.writes, marker)
	if len(s.writeErrs) == 0 {
		return nil
	}
	err := s.writeErrs[0]
	s.writeErrs = s.writeErrs[1:]
	return err
}

func (f *fakeAPIClient) LookupSites(_ context.Context, _ string, _ int64, from int64) (*catosdk.EntityLookup, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.lookupPages != nil {
		return f.lookupPages[from], nil
	}
	return f.lookup, nil
}

func (f *fakeAPIClient) AccountSnapshot(context.Context, string, []string) (*catosdk.AccountSnapshot, error) {
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return f.snapshot, nil
}

func (f *fakeAPIClient) AccountMetrics(_ context.Context, _ string, siteIDs []string, _ string, _ int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error) {
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

func (f *fakeAPIClient) EventsFeed(_ context.Context, _ string, marker *string) (*catosdk.EventsFeed, error) {
	key := ""
	if marker != nil {
		key = *marker
	}
	f.eventMarkers = append(f.eventMarkers, key)
	if f.eventsByMarker != nil {
		return f.eventsByMarker[key], nil
	}
	return f.events, nil
}

func (f *fakeAPIClient) SiteBgpStatus(_ context.Context, _ string, siteID string) ([]*catosdk.SiteBgpStatusResult, error) {
	if f.bgpErrSites != nil {
		if err := f.bgpErrSites[siteID]; err != nil {
			return nil, err
		}
	}
	return f.bgp[siteID], nil
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

func TestCollectorCollectsMetricsEventsAndTopology(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}, 1)
	requireValue(t, reader, "bgp_session_up", metrix.Labels{
		"site_id":   "1001",
		"site_name": "Paris Office",
		"peer_ip":   "192.0.2.10",
		"peer_asn":  "64512",
	}, 1)

	marker, err := os.ReadFile(c.Events.MarkerFile)
	require.NoError(t, err)
	require.Equal(t, "marker-2\n", string(marker))

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
	c.Events.Enabled = confopt.AutoBoolDisabled
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
	c.writeMetrics(sites, []string{"1001"}, nil)
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
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_collection_success", nil, 1)
	requireValue(t, reader, "collector_discovered_sites", nil, 3)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationDiscovery}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationSnapshot}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationMetrics}, 1)
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationEvents}, 1)
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
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}, 1)
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Reopened",
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

	marker, err := os.ReadFile(c.Events.MarkerFile)
	require.NoError(t, err)
	require.Equal(t, "eyJmZXRjaGVkQ291bnQiOjIwfQ==\n", string(marker))
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
			MaxSites:             intPtr(0),
			MaxInterfacesPerSite: intPtr(0),
		},
		BGP: BGPConfig{
			MaxPeersPerSite: intPtr(0),
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

func TestDefaultEventsMarkerPathIncludesEndpointAndVnode(t *testing.T) {
	base := t.TempDir()

	first := defaultEventsMarkerPath(base, "12345", "https://api.catonetworks.com/api/v1/graphql2", "site-a")
	same := defaultEventsMarkerPath(base, " 12345 ", " https://api.catonetworks.com/api/v1/graphql2 ", " site-a ")
	differentEndpoint := defaultEventsMarkerPath(base, "12345", "https://api.us1.catonetworks.com/api/v1/graphql2", "site-a")
	differentVnode := defaultEventsMarkerPath(base, "12345", "https://api.catonetworks.com/api/v1/graphql2", "site-b")

	require.Equal(t, same, first)
	require.NotEqual(t, differentEndpoint, first)
	require.NotEqual(t, differentVnode, first)
	require.Contains(t, first, filepath.Join(base, "cato_networks"))
	require.Contains(t, filepath.Base(first), ".events.marker")
}

func TestCheckDoesNotAdvanceEventsMarker(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	c.client = newFixtureAPIClient()
	c.now = func() time.Time { return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) }

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Empty(t, c.eventMarker)
	_, err := os.Stat(c.Events.MarkerFile)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCheckDoesNotPublishDryRunHealth(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow

	require.NoError(t, c.Init(context.Background()))
	fake := c.client.(*fakeAPIClient)
	fake.metricsErrSites = map[string]error{"1001": errors.New("dry-run metrics failed")}
	require.NoError(t, c.Check(context.Background()))

	fake.metricsErrSites = nil
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	reader := c.store.Read()
	requireValue(t, reader, "collector_operation_success", metrix.Labels{"operation": operationMetrics}, 1)
	_, ok := reader.Value("collector_operation_failures_total", metrix.Labels{
		"operation":   operationMetrics,
		"error_class": "error",
	})
	require.False(t, ok)
}

func TestCheckDoesNotPopulateBGPCache(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.Events.Enabled = "no"
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Empty(t, c.bgp.bySite)
	require.True(t, c.bgp.nextRefresh.IsZero())
}

func TestCollectorDrainsEventsFeedPagesAndCapsCardinality(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	c.Events.MaxPagesPerCycle = 5
	c.Events.MaxCardinality = 2
	fake := newFixtureAPIClient()
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"": eventsFeedPage("marker-1", eventsFeedMaxFetchSize, []map[string]any{
			{"event_type": "Security", "event_sub_type": "Threat Prevention", "severity": "HIGH", "status": "Closed"},
			{"event_type": "Connectivity", "event_sub_type": "Tunnel", "severity": "INFO", "status": "Open"},
		}),
		"marker-1": eventsFeedPage("marker-2", 1, []map[string]any{
			{"event_type": "Audit", "event_sub_type": "Admin Login", "severity": "LOW", "status": "Closed"},
		}),
	}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	require.Equal(t, []string{"", "marker-1"}, fake.eventMarkers)
	require.Equal(t, "marker-2", c.eventMarker)
	data, err := os.ReadFile(c.Events.MarkerFile)
	require.NoError(t, err)
	require.Equal(t, "marker-2", strings.TrimSpace(string(data)))

	reader := c.store.Read()
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}, 1)
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Connectivity",
		"event_sub_type": "Tunnel",
		"severity":       "INFO",
		"status":         "Open",
	}, 1)
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "other",
		"event_sub_type": "other",
		"severity":       "other",
		"status":         "other",
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueCardinalityLimit,
	}, 1)
}

func TestCollectorDeduplicatesEventsByEventID(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	fake.events = eventsFeedPage("", 0, []map[string]any{
		{"event_id": "evt-1", "event_type": "Security", "event_sub_type": "Threat Prevention", "severity": "HIGH", "status": "Closed"},
		{"event_id": "evt-1", "event_type": "Security", "event_sub_type": "Threat Prevention", "severity": "HIGH", "status": "Closed"},
		{"event_id": "evt-2", "event_type": "Security", "event_sub_type": "Threat Prevention", "severity": "HIGH", "status": "Closed"},
	})
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}, 2)
}

func TestAddEventCountAggregatesByNormalizedEventKey(t *testing.T) {
	counts := make(map[eventKey]int64)
	key := eventKey{
		EventType:    " Security ",
		EventSubType: " Threat Prevention ",
		Severity:     " HIGH ",
		Status:       " Closed ",
	}

	require.False(t, addEventCount(counts, key, 10))
	require.False(t, addEventCount(counts, key, 10))

	require.Equal(t, map[eventKey]int64{
		{
			EventType:    "Security",
			EventSubType: "Threat Prevention",
			Severity:     "HIGH",
			Status:       "Closed",
		}: 2,
	}, counts)
}

func TestAddEventCountAllowsConfiguredRealSeriesBeforeOther(t *testing.T) {
	counts := make(map[eventKey]int64)

	require.False(t, addEventCount(counts, eventKey{EventType: "first"}, 2))
	require.False(t, addEventCount(counts, eventKey{EventType: "second"}, 2))
	require.True(t, addEventCount(counts, eventKey{EventType: "third"}, 2))

	require.Equal(t, int64(1), counts[eventKey{EventType: "first", EventSubType: "unknown", Severity: "unknown", Status: "unknown"}])
	require.Equal(t, int64(1), counts[eventKey{EventType: "second", EventSubType: "unknown", Severity: "unknown", Status: "unknown"}])
	require.Equal(t, int64(1), counts[eventKey{EventType: "other", EventSubType: "other", Severity: "other", Status: "other"}])
}

func TestCollectorReportsUnknownTimeseriesLabels(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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

func TestCollectorNormalizesEventFieldAliasesAndReportsBadFields(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	fake.events = eventsFeedPage("marker-1", 0, []map[string]any{
		{"eventType": "Security", "eventSubType": "Threat Prevention", "severity": "HIGH", "status": "Closed"},
		{"event_type": map[string]any{"unexpected": "shape"}, "event_sub_type": "", "severity": nil},
	})
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}, 1)
	requireValue(t, reader, "events_total", metrix.Labels{
		"event_type":     "unknown",
		"event_sub_type": "unknown",
		"severity":       "unknown",
		"status":         "unknown",
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueComplexEventField,
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueEmptyEventSubType,
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueEmptyEventSeverity,
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueEmptyEventStatus,
	}, 1)
}

func TestCollectorUsesPersistedEventsMarker(t *testing.T) {
	markerFile := filepath.Join(t.TempDir(), "marker")
	require.NoError(t, os.WriteFile(markerFile, []byte("persisted-marker"), 0o600))

	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = markerFile
	fake := newFixtureAPIClient()
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"persisted-marker": eventsFeedPage("next-marker", 0, nil),
	}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	require.Equal(t, []string{"persisted-marker"}, fake.eventMarkers)
	require.Equal(t, "next-marker", c.eventMarker)
}

func TestCollectorDoesNotAdvanceMarkerOnEventsAccountError(t *testing.T) {
	markerFile := filepath.Join(t.TempDir(), "marker")
	require.NoError(t, os.WriteFile(markerFile, []byte("persisted-marker\n"), 0o600))

	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = markerFile
	fake := newFixtureAPIClient()
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"persisted-marker": eventsFeedAccountErrorPage("next-marker"),
	}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	data, err := os.ReadFile(markerFile)
	require.NoError(t, err)
	require.Equal(t, []string{"persisted-marker"}, fake.eventMarkers)
	require.Equal(t, "persisted-marker", c.eventMarker)
	require.Equal(t, "persisted-marker\n", string(data))
	reader := c.store.Read()
	requireValue(t, reader, "collector_operation_success", metrix.Labels{
		"operation": operationEvents,
	}, 0)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationEvents,
		"error_class": "error",
	}, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueAccountError,
	}, 1)
}

func TestCollectorReportsEventsPageCap(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	c.Events.MaxPagesPerCycle = 1
	fake := newFixtureAPIClient()
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"": eventsFeedPage("marker-1", eventsFeedMaxFetchSize, []map[string]any{
			{"event_type": "Security"},
		}),
	}
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssuePageCap,
	}, 1)
}

func TestCollectorStopsWhenEventsMarkerDoesNotAdvance(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	c.eventMarker = "stalled-marker"
	fake := newFixtureAPIClient()
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"stalled-marker": eventsFeedPage("stalled-marker", eventsFeedMaxFetchSize, []map[string]any{
			{"event_type": "Security"},
		}),
	}
	c.client = fake
	c.now = func() time.Time { return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC) }

	require.NoError(t, c.Init(context.Background()))
	c.eventMarker = "stalled-marker"
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	require.Equal(t, []string{"stalled-marker"}, fake.eventMarkers)
	reader := c.store.Read()
	requireMetricMissing(t, reader, "events_total", metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "unknown",
		"severity":       "unknown",
		"status":         "unknown",
	})
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueMarkerStalled,
	}, 1)
	_, err := os.Stat(c.Events.MarkerFile)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCollectorDoesNotDoubleCountEventsWhenMarkerStalls(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	record := map[string]any{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}
	fake.eventsByMarker = map[string]*catosdk.EventsFeed{
		"":         eventsFeedPage("marker-1", 1, []map[string]any{record}),
		"marker-1": eventsFeedPage("marker-1", eventsFeedMaxFetchSize, []map[string]any{record}),
	}
	c.client = fake
	c.now = fixedCatoTestNow

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	labels := metrix.Labels{
		"event_type":     "Security",
		"event_sub_type": "Threat Prevention",
		"severity":       "HIGH",
		"status":         "Closed",
	}
	reader := c.store.Read()
	requireValue(t, reader, "events_total", labels, 1)

	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()

	require.Equal(t, []string{"", "marker-1"}, fake.eventMarkers)
	reader = c.store.Read()
	requireValue(t, reader, "events_total", labels, 1)
	requireValue(t, reader, "collector_normalization_issues_total", metrix.Labels{
		"surface": normalizationSurfaceEvents,
		"issue":   normalizationIssueMarkerStalled,
	}, 1)
	data, err := os.ReadFile(c.Events.MarkerFile)
	require.NoError(t, err)
	require.Equal(t, "marker-1\n", string(data))
}

func TestCollectorContinuesOnPartialMetricsAndBGPFailures(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Events.Enabled = "no"
	c.Metrics.MaxSitesPerQuery = 1
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	fake.snapshot = &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
		Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
			{
				ID:                             strPtr("1001"),
				ConnectivityStatusSiteSnapshot: connectivityPtr("Initializing"),
				OperationalStatusSiteSnapshot:  operationalPtr("Maintenance"),
				PopName:                        strPtr("POP-Paris"),
				InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
					Name: strPtr("Paris Office"),
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
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Discovery.PageLimit = 2
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.SiteSelector = "!Toulouse* *"
	c.Limits.MaxSites = intPtr(1)
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	total := int64(3)
	siteType := catomodels.EntityTypeSite
	fake.lookup = &catosdk.EntityLookup{EntityLookup: catosdk.EntityLookup_EntityLookup{
		Total: &total,
		Items: []*catosdk.EntityLookup_EntityLookup_Items{
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1001", Name: strPtr("Paris Office"), Type: siteType}},
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1002", Name: strPtr("Toulouse Office"), Type: siteType}},
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1003", Name: strPtr("Madrid Office"), Type: siteType}},
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
	c.Events.Enabled = "no"
	c.InterfaceSelector = "wan*"
	c.Limits.MaxInterfacesPerSite = intPtr(1)
	c.BGP.MaxPeersPerSite = intPtr(1)
	c.BGP.MaxSitesPerCollection = 1
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
	fake := newFixtureAPIClient()
	snapshot := fixtureSnapshot()
	snapshot.GetAccountSnapshot().GetSites()[0].GetDevices()[0].Interfaces = append(
		snapshot.GetAccountSnapshot().GetSites()[0].GetDevices()[0].GetInterfaces(),
		&catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces{
			ID:        strPtr("wan2"),
			Name:      strPtr("WAN 2"),
			Connected: boolPtr(true),
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
	c.Events.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Discovery.RefreshEvery = 300
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
	c.BGP.MaxSitesPerCollection = 1
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
	c.BGP.MaxSitesPerCollection = 1
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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

func TestCollectorReportsMarkerWriteFailure(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	blockedPath := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blockedPath, []byte("blocked"), 0o600))
	c.Events.MarkerFile = filepath.Join(blockedPath, "marker")
	c.client = newFixtureAPIClient()
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	require.Equal(t, "marker-2", c.eventMarker)
	requireValue(t, reader, "collector_events_marker_persistence_available", nil, 0)
	requireValue(t, reader, "collector_operation_failures_total", metrix.Labels{
		"operation":   operationEventMarker,
		"error_class": "error",
	}, 1)
}

func TestCollectorRetriesMarkerWrite(t *testing.T) {
	c := New()
	c.Retry.Attempts = 2
	c.Retry.WaitMin = confopt.Duration(time.Nanosecond)
	c.Retry.WaitMax = confopt.Duration(time.Nanosecond)
	store := &flakyMarkerStore{writeErrs: []error{temporaryMarkerError{errors.New("temporary marker write failure")}}}
	c.markerStore = store
	c.markerStoreAvailable = true

	c.commitEventsMarker(context.Background(), "marker-1")

	require.Equal(t, []string{"marker-1", "marker-1"}, store.writes)
	require.True(t, c.markerStoreAvailable)
	require.True(t, c.health.MarkerPersistenceAvailable)
	require.Equal(t, operationHealth{Success: true, ErrorClass: "none"}, c.health.LastOperations[operationEventMarker])
}

func TestCollectorReportsMarkerReadFailureUnavailable(t *testing.T) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.Metrics.Enabled = "no"
	c.BGP.Enabled = "no"
	c.Events.MarkerFile = t.TempDir()
	fake := newFixtureAPIClient()
	fake.events = eventsFeedPage("", 0, nil)
	c.client = fake
	c.now = fixedCatoTestNow
	collectOnce(t, c)

	reader := c.store.Read()
	require.Empty(t, c.eventMarker)
	requireValue(t, reader, "collector_events_marker_persistence_available", nil, 0)
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
	c.Events.Enabled = "no"
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
				ID: strPtr("1001"),
				Metrics: &catosdk.AccountMetrics_AccountMetrics_Sites_Metrics{
					BytesUpstream: &siteBytesUpstream,
					Rtt:           &siteRTT,
				},
				Interfaces: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{
					{
						Name: strPtr("all"),
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
	c.Events.Enabled = "no"
	c.BGP.RefreshEvery = 60
	c.BGP.MaxSitesPerCollection = 1
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.Enabled = "no"
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
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
		"graphql rate limit during events": {
			operation:     operationEvents,
			response:      rawCatoResponse{status: http.StatusOK, body: `{"errors":[{"message":"rate limit exceeded"}],"data":{"eventsFeed":null}}`},
			wantOperation: operationEvents,
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
			c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")
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
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")

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
	c.Events.MarkerFile = filepath.Join(t.TempDir(), "marker")

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	err := c.collect(context.Background(), true)
	cc.CommitCycleSuccess()

	require.Error(t, err)
	require.Contains(t, err.Error(), "site discovery failed")
	require.Contains(t, err.Error(), "error_class=auth")
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
	writeAPIStats(c.store, apiStats{Retries: map[string]apiRetryStats{
		operationMetrics: {RateLimit: 2, Transient: 3},
	}})
	cc.CommitCycleSuccess()
	reader := c.store.Read()
	requireValue(t, reader, "api_rate_limit_retries_total", labels, 2)
	requireValue(t, reader, "api_transient_retries_total", labels, 3)
	requireNoDelta(t, reader, "api_rate_limit_retries_total", labels)
	requireNoDelta(t, reader, "api_transient_retries_total", labels)

	cc.BeginCycle()
	writeAPIStats(c.store, apiStats{Retries: map[string]apiRetryStats{
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
	err := client.withRetry(context.Background(), "eventsFeed", func() error {
		calls++
		if calls == 1 {
			return fmt.Errorf("http client timeout: %w", context.DeadlineExceeded)
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, int64(1), client.APIStats().Retries["eventsFeed"].Transient)
}

func TestNormalizeSnapshotDefaultsNilInfoAndStatuses(t *testing.T) {
	snapshot := &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{
		Sites: []*catosdk.AccountSnapshot_AccountSnapshot_Sites{
			{ID: strPtr("1001")},
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
		operationEvents:    {body: loadSDKCompatibleMockoonResponseBody(t, "eventsFeed")},
		operationBGP:       {body: loadTestdata(t, "cato-site-bgp-status.schema-shaped.json")},
	}
	for operation, response := range overrides {
		responses[operation] = response
	}

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
		case strings.Contains(request, "eventsFeed"):
			operation = operationEvents
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
	case "eventsFeed":
		body = adaptEventsFeedFixtureForSDK(t, body)
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

func adaptEventsFeedFixtureForSDK(t *testing.T, body string) string {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	accounts := payload["data"].(map[string]any)["eventsFeed"].(map[string]any)["accounts"].([]any)
	for _, rawAccount := range accounts {
		account := rawAccount.(map[string]any)
		if id, ok := account["id"]; ok {
			account["id"] = fmt.Sprint(id)
		}
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
		events:   fixtureEvents(),
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
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1001", Name: strPtr("Paris Office"), Type: siteType}},
			{Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: "1002", Name: strPtr("Toulouse Office"), Type: siteType}},
		},
	}}
}

func fixtureLookupPage(total int64, ids ...string) *catosdk.EntityLookup {
	siteType := catomodels.EntityTypeSite
	items := make([]*catosdk.EntityLookup_EntityLookup_Items, 0, len(ids))
	for _, id := range ids {
		items = append(items, &catosdk.EntityLookup_EntityLookup_Items{
			Entity: catosdk.EntityLookup_EntityLookup_Items_Entity{ID: id, Name: strPtr("Site " + id), Type: siteType},
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
				ID:                             strPtr("1001"),
				ConnectivityStatusSiteSnapshot: connectivityPtr("connected"),
				OperationalStatusSiteSnapshot:  operationalPtr("active"),
				PopName:                        strPtr("POP-Paris"),
				HostCount:                      int64Ptr(42),
				ConnectedSince:                 strPtr("2026-05-01T10:00:00Z"),
				InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
					Name:        strPtr("Paris Office"),
					Description: strPtr("Main site"),
					CountryCode: strPtr("FR"),
					CountryName: strPtr("France"),
				},
				Devices: []*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices{
					{
						ID:        strPtr("dev-1"),
						Name:      strPtr("Socket 1"),
						Type:      strPtr("socket"),
						Connected: boolPtr(true),
						Interfaces: []*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces{
							{
								ID:             strPtr("wan1"),
								Name:           strPtr("WAN 1"),
								Connected:      boolPtr(true),
								PopName:        strPtr("POP-Paris"),
								TunnelRemoteIP: strPtr("203.0.113.10"),
								TunnelUptime:   int64Ptr(3600),
							},
						},
					},
				},
			},
			{
				ID:                             strPtr("1002"),
				ConnectivityStatusSiteSnapshot: connectivityPtr("Degraded"),
				OperationalStatusSiteSnapshot:  operationalPtr("locked"),
				PopName:                        strPtr("POP-Toulouse"),
				HostCount:                      int64Ptr(7),
				InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
					Name: strPtr("Toulouse Office"),
				},
			},
		},
	}}
}

func fixtureMetrics() *catosdk.AccountMetrics {
	return &catosdk.AccountMetrics{AccountMetrics: &catosdk.AccountMetrics_AccountMetrics{
		Sites: []*catosdk.AccountMetrics_AccountMetrics_Sites{
			{
				ID:   strPtr("1001"),
				Name: strPtr("Paris Office"),
				Interfaces: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{
					{
						Name: strPtr("all"),
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

func fixtureEvents() *catosdk.EventsFeed {
	return &catosdk.EventsFeed{EventsFeed: &catosdk.EventsFeed_EventsFeed{
		Marker: strPtr("marker-2"),
		Accounts: []*catosdk.EventsFeed_EventsFeed_Accounts{
			{
				ID: strPtr("12345"),
				Records: []*catosdk.EventsFeed_EventsFeed_Accounts_Records{
					{FieldsMap: map[string]any{
						"event_type":     "Security",
						"event_sub_type": "Threat Prevention",
						"severity":       "HIGH",
						"status":         "Closed",
					}},
				},
			},
		},
	}}
}

func eventsFeedPage(marker string, fetchedCount int64, records []map[string]any) *catosdk.EventsFeed {
	account := &catosdk.EventsFeed_EventsFeed_Accounts{ID: strPtr("12345")}
	for _, fields := range records {
		account.Records = append(account.Records, &catosdk.EventsFeed_EventsFeed_Accounts_Records{FieldsMap: fields})
	}
	return &catosdk.EventsFeed{EventsFeed: &catosdk.EventsFeed_EventsFeed{
		Marker:       strPtr(marker),
		FetchedCount: fetchedCount,
		Accounts:     []*catosdk.EventsFeed_EventsFeed_Accounts{account},
	}}
}

func eventsFeedAccountErrorPage(marker string) *catosdk.EventsFeed {
	return &catosdk.EventsFeed{EventsFeed: &catosdk.EventsFeed_EventsFeed{
		Marker:       strPtr(marker),
		FetchedCount: eventsFeedMaxFetchSize,
		Accounts: []*catosdk.EventsFeed_EventsFeed_Accounts{
			{ID: strPtr("12345"), ErrorString: strPtr("account temporarily unavailable")},
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

func strPtr(v string) *string { return &v }

func boolPtr(v bool) *bool { return &v }

func int64Ptr(v int64) *int64 { return &v }

func connectivityPtr(v string) *catomodels.ConnectivityStatus {
	status := catomodels.ConnectivityStatus(v)
	return &status
}

func operationalPtr(v string) *catoscalars.OperationalStatus {
	status := catoscalars.OperationalStatus(v)
	return &status
}
