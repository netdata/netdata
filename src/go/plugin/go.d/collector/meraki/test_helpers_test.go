// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// Unlike the older generic helper, check the commit result as well as Collect.
func collectCycle(c *Collector, ctx context.Context) error {
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
