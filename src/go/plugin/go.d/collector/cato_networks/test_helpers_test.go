// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
	catoscalars "github.com/catonetworks/cato-go-sdk/scalars"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type fakeAPIClient struct {
	mu sync.Mutex

	lookup          *catosdk.EntityLookup
	lookupErr       error
	lookupPages     map[int64]*catosdk.EntityLookup
	snapshot        *catosdk.AccountSnapshot
	snapshotErr     error
	metrics         *catosdk.AccountMetrics
	bgp             map[string][]*catosdk.SiteBgpStatusResult
	metricsErrSites map[string]error
	metricsHook     func(context.Context, []string)
	bgpErrSites     map[string]error
	groupInterfaces []*bool
	metricsSiteIDs  [][]string
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

func (f *fakeAPIClient) AccountMetrics(ctx context.Context, _ string, siteIDs []string, _ string, _ int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error) {
	f.mu.Lock()

	f.metricsCalls++
	siteIDs = append([]string(nil), siteIDs...)
	f.metricsSiteIDs = append(f.metricsSiteIDs, siteIDs)
	if groupInterfaces == nil {
		f.groupInterfaces = append(f.groupInterfaces, nil)
	} else {
		v := *groupInterfaces
		f.groupInterfaces = append(f.groupInterfaces, &v)
	}
	hook := f.metricsHook
	metrics := f.metrics
	var err error
	if len(siteIDs) > 0 && f.metricsErrSites != nil {
		err = f.metricsErrSites[siteIDs[0]]
	}
	f.mu.Unlock()

	if hook != nil {
		hook(ctx, siteIDs)
	}
	if err != nil {
		return nil, err
	}
	return metrics, nil
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

type rawCatoResponse struct {
	status int
	body   string
}

const (
	operationDiscovery = "entityLookup"
	operationSnapshot  = "accountSnapshot"
	operationMetrics   = "accountMetrics"
	operationBGP       = "siteBgpStatus"
)

func newTestCollector() (*Collector, *fakeAPIClient) {
	c := New()
	c.AccountID = "12345"
	c.APIKey = "secret"
	c.now = fixedCatoTestNow
	fake := newFixtureAPIClient()
	c.client = fake
	return c, fake
}

func fixedCatoTestNow() time.Time {
	return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
}

func initCollector(t *testing.T, c *Collector) {
	t.Helper()
	require.NoError(t, c.Init(context.Background()))
}

func collectOnce(t *testing.T, c *Collector) {
	t.Helper()
	_, err := collectScalarSeries(t, c)
	require.NoError(t, err)
}

func collectScalarSeries(t *testing.T, c *Collector) (map[string]metrix.SampleValue, error) {
	t.Helper()
	cc := mustCycleController(t, c.store)
	committed := false
	cc.BeginCycle()
	defer func() {
		if !committed {
			cc.AbortCycle()
		}
	}()
	if err := c.Collect(context.Background()); err != nil {
		return nil, err
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		return nil, err
	}
	committed = true
	return scalarSeriesFromAllHostScopes(c.store), nil
}

func collectError(t *testing.T, c *Collector) error {
	t.Helper()
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	err := c.Collect(context.Background())
	cc.AbortCycle()
	return err
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

func numberedSiteIDs(start, count int) []string {
	ids := make([]string, 0, count)
	for id := start; id < start+count; id++ {
		ids = append(ids, fmt.Sprint(id))
	}
	return ids
}

func fixtureSnapshotForSiteIDs(ids ...string) *catosdk.AccountSnapshot {
	sites := make([]*catosdk.AccountSnapshot_AccountSnapshot_Sites, 0, len(ids))
	for _, siteID := range ids {
		sites = append(sites, &catosdk.AccountSnapshot_AccountSnapshot_Sites{
			ID:                             new(siteID),
			ConnectivityStatusSiteSnapshot: connectivityPtr("connected"),
			OperationalStatusSiteSnapshot:  operationalPtr("active"),
			PopName:                        new("POP-" + siteID),
			InfoSiteSnapshot: &catosdk.AccountSnapshot_AccountSnapshot_Sites_InfoSiteSnapshot{
				Name: new("Site " + siteID),
			},
		})
	}
	return &catosdk.AccountSnapshot{AccountSnapshot: &catosdk.AccountSnapshot_AccountSnapshot{Sites: sites}}
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

func requireMetricValues(t *testing.T, got map[string]metrix.SampleValue, want map[string]metrix.SampleValue) {
	t.Helper()
	for key, wantValue := range want {
		gotValue, ok := got[key]
		require.True(t, ok, "missing metric %s", key)
		require.Equal(t, wantValue, gotValue, "metric %s", key)
	}
}

func requireMetricsMissing(t *testing.T, got map[string]metrix.SampleValue, keys []string) {
	t.Helper()
	for _, key := range keys {
		require.NotContains(t, got, key)
	}
}

func scalarSeriesFromAllHostScopes(store metrix.CollectorStore) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	reader := store.Read(metrix.ReadFlatten())
	for _, scope := range reader.HostScopes() {
		maps.Copy(out, scalarSeriesFromReader(store.Read(metrix.ReadFlatten(), metrix.ReadHostScope(scope.ScopeKey))))
	}
	return out
}

func scalarSeriesFromReader(reader metrix.Reader) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	if reader == nil {
		return out
	}
	reader.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
		out[metricKeyFromLabelView(name, labels)] = value
	})
	return out
}

func metricKey(name string, labels metrix.Labels) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(labels[key]))
	}
	b.WriteByte('}')
	return b.String()
}

func stateMetricKey(name, state string, labels metrix.Labels) string {
	out := make(metrix.Labels, len(labels)+1)
	maps.Copy(out, labels)
	out[name] = state
	return metricKey(name, out)
}

func mustCycleController(t *testing.T, s metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(s)
	require.True(t, ok)
	return managed.CycleController()
}

func connectivityPtr(v string) *catomodels.ConnectivityStatus {
	status := catomodels.ConnectivityStatus(v)
	return &status
}

func operationalPtr(v string) *catoscalars.OperationalStatus {
	status := catoscalars.OperationalStatus(v)
	return &status
}
