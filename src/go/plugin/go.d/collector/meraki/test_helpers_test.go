// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

type fixtureResponse struct {
	body, link, retryAfter string
	status                 int
}

type fixtureRequest struct {
	path, cursor, pageSize, authorization string
	at                                    time.Time
	query                                 url.Values
}

type fixtureAPI struct {
	mu        sync.Mutex
	responses map[string]fixtureResponse
	requests  []fixtureRequest
}

func newFixtureAPI(t *testing.T) (*fixtureAPI, *httptest.Server) {
	t.Helper()
	f := &fixtureAPI{
		responses: map[string]fixtureResponse{
			"/organizations:":              {body: `[{"id":"org","name":"Org"}]`},
			"/organizations/org/networks:": {body: `[{"id":"N1","organizationId":"org","name":"Office"}]`},
			"/organizations/org/devices:": {
				body: `[{"serial":"S1","networkId":"N1","name":"Access Point","model":"MR","productType":"wireless"}]`,
			},
			"/organizations/org/devices/availabilities:": {
				body: `[{"serial":"S1","status":"online","network":{"id":"N1"}}]`,
			},
		},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		cursor := r.URL.Query().Get("startingAfter")
		f.mu.Lock()
		f.requests = append(
			f.requests,
			fixtureRequest{
				path:          path,
				cursor:        cursor,
				pageSize:      r.URL.Query().Get("perPage"),
				authorization: r.Header.Get("Authorization"),
				at:            time.Now(),
				query:         r.URL.Query(),
			},
		)
		response, ok := f.responses[path+":"+cursor]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "unexpected fixture operation", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if response.link != "" {
			w.Header().Set("Link", response.link)
		}
		if response.retryAfter != "" {
			w.Header().Set("Retry-After", response.retryAfter)
		}
		if response.status != 0 {
			w.WriteHeader(response.status)
		}
		_, _ = w.Write([]byte(response.body))
	}))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fixtureAPI) set(path string, response fixtureResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[path+":"] = response
}

func (f *fixtureAPI) response(path string) fixtureResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.responses[path+":"]
}

func (f *fixtureAPI) calls() []fixtureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fixtureRequest(nil), f.requests...)
}

func configuredCollector(server *httptest.Server) *Collector {
	c := New()
	c.URL = server.URL + "/api/v1"
	c.OrganizationID = "org"
	c.APIKey = "test-key"
	c.InsecureSkipVerify = true
	return c
}

func newTestCollector(t *testing.T, server *httptest.Server) *Collector {
	t.Helper()
	c := configuredCollector(server)
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c
}

// collectCycle runs one metric-store cycle and reports the commit result too.
func collectCycle(ctx context.Context, c *Collector) error {
	managed, _ := metrix.AsCycleManagedStore(c.MetricStore())
	cc := managed.CycleController()
	cc.BeginCycle()
	if err := c.Collect(ctx); err != nil {
		cc.AbortCycle()
		return err
	}
	return cc.CommitCycleSuccess()
}

type observedDevice struct {
	Scope  metrix.HostScope
	Labels map[string]string
	Values map[string]metrix.SampleValue
}

func readDevices(c *Collector) map[string]observedDevice {
	result := make(map[string]observedDevice)
	reader := c.store.Read(metrix.ReadFlatten())
	for _, scope := range reader.(metrix.FreshVisibleHostScopesReader).FreshVisibleHostScopes() {
		if scope.IsDefault() {
			continue
		}
		d := observedDevice{
			Scope:  scope,
			Values: make(map[string]metrix.SampleValue),
		}
		c.store.Read(metrix.ReadFlatten(), metrix.ReadHostScope(scope.ScopeKey)).
			ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
				if name == "device_assigned" {
					d.Labels = labels.CloneMap()
				}
				if state, ok := labels.Get("device_availability_status"); ok {
					name += ":" + state
				}
				d.Values[name] = value
			})
		result[scope.Labels["serial"]] = d
	}
	return result
}

func readOrganization(c *Collector) map[string]metrix.SampleValue {
	result := make(map[string]metrix.SampleValue)
	c.store.Read(metrix.ReadFlatten()).
		ForEachSeries(func(name string, _ metrix.LabelView, value metrix.SampleValue) { result[name] = value })
	return result
}

func wantStatus(status string) map[string]metrix.SampleValue {
	want := map[string]metrix.SampleValue{
		"device_assigned":                     1,
		"device_availability_status:online":   0,
		"device_availability_status:alerting": 0,
		"device_availability_status:offline":  0,
		"device_availability_status:dormant":  0,
	}
	want["device_availability_status:"+status] = 1
	return want
}

const (
	switchUsageHistoryPath   = "/organizations/org/switch/ports/usage/history/byDevice/byInterval"
	applianceUplinkUsagePath = "/organizations/org/appliance/uplinks/usage/byNetwork"
	switchPacketsPath        = "/devices/S1/switch/ports/statuses/packets"
)

// newTrafficFixture returns a collector with every traffic family enabled, over
// an organization with one switch (S1, port 1/0) and an MX pair (A, B) in N1. Its
// clock is fixed at the returned time.
func newTrafficFixture(t *testing.T) (*fixtureAPI, *Collector, time.Time) {
	t.Helper()
	f, server := newFixtureAPI(t)
	f.set("/organizations/org/devices", fixtureResponse{
		body: `[{"serial":"S1","networkId":"N1","name":"Switch","model":"MS120","productType":"switch"},` +
			`{"serial":"A","networkId":"N1","productType":"appliance"},` +
			`{"serial":"B","networkId":"N1","productType":"appliance"}]`,
	})
	f.set("/organizations/org/devices/availabilities", fixtureResponse{
		body: `[{"serial":"S1","status":"online","network":{"id":"N1"}},` +
			`{"serial":"A","status":"online"},{"serial":"B","status":"dormant"}]`,
	})
	now := time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC)
	f.set(switchUsageHistoryPath, fixtureResponse{
		body: usageHistoryBody(
			"S1",
			"N1",
			"1/0",
			usageInterval(now.Add(-time.Minute), 300, `{"upstream":10.5,"downstream":20.25}`),
		),
	})
	f.set(switchPacketsPath, fixtureResponse{
		body: `[{"portId":"1/0","packets":[` +
			`{"desc":"Total","sent":99999999,"recv":88888888,"ratePerSec":{"sent":2.5,"recv":3.25}},` +
			`{"desc":"Broadcast","ratePerSec":{"sent":999,"recv":999}}]}]`,
	})
	f.set(applianceUplinkUsagePath, fixtureResponse{
		body: `[{"networkId":"N1","byUplink":[{"interface":"wan1","serial":"A","sent":300000,"received":600000}]}]`,
	})

	c := newTestCollector(t, server)
	c.now = func() time.Time { return now }
	c.CollectSwitchPortTraffic = true
	c.CollectSwitchPortPackets = true
	c.CollectApplianceUplinkTraffic = true
	return f, c, now
}

// usageInterval is one switch port usage interval that ended at end.
func usageInterval(end time.Time, seconds int, usage string) string {
	start := end.Add(-time.Duration(seconds) * time.Second)
	return fmt.Sprintf(
		`{"startTs":%q,"endTs":%q,"bandwidth":{"usage":%s}}`,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		usage,
	)
}

func usageHistoryBody(serial, network, port, intervals string) string {
	return fmt.Sprintf(
		`{"items":[{"serial":%q,"network":{"id":%q},"ports":[{"portId":%q,"intervals":[%s]}]}]}`,
		serial,
		network,
		port,
		intervals,
	)
}

type metricPoint struct {
	Scope  metrix.HostScope
	Labels map[string]string
	Value  metrix.SampleValue
}

// metricPoints returns every series of the metric, from every host scope.
func metricPoints(c *Collector, metric string) []metricPoint {
	var points []metricPoint
	read := func(scope metrix.HostScope) {
		opts := []metrix.ReadOption{metrix.ReadFlatten()}
		if !scope.IsDefault() {
			opts = append(opts, metrix.ReadHostScope(scope.ScopeKey))
		}
		c.store.Read(opts...).ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
			if name == metric {
				points = append(points, metricPoint{
					Scope:  scope,
					Labels: labels.CloneMap(),
					Value:  value,
				})
			}
		})
	}
	read(metrix.HostScope{})
	for _, scope := range c.store.Read().HostScopes() {
		if !scope.IsDefault() {
			read(scope)
		}
	}
	return points
}

// metricValues returns the values of every series of the metric.
func metricValues(c *Collector, metric string) []metrix.SampleValue {
	values := []metrix.SampleValue{}
	for _, p := range metricPoints(c, metric) {
		values = append(values, p.Value)
	}
	return values
}
