// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationSerialize(t *testing.T) {
	j, err := os.ReadFile("testdata/config.json")
	require.NoError(t, err)
	y, err := os.ReadFile("testdata/config.yaml")
	require.NoError(t, err)
	collecttest.TestConfigurationSerialize(t, New(), j, y)
}

func TestInit(t *testing.T) {
	tests := map[string]struct {
		change  func(*Collector)
		wantErr string
	}{
		"valid defaults": {},
		"no organization": {
			change:  func(c *Collector) { c.OrganizationID = "" },
			wantErr: "organization_id",
		},
		"spaced organization": {
			change:  func(c *Collector) { c.OrganizationID = " org " },
			wantErr: "organization_id",
		},
		"dot organization": {
			change:  func(c *Collector) { c.OrganizationID = ".." },
			wantErr: "organization_id",
		},
		"no credentials": {change: func(c *Collector) { c.APIKey = "" }, wantErr: "exactly one"},
		"two credentials": {
			change:  func(c *Collector) { c.BearerTokenFile = "unused" },
			wantErr: "exactly one",
		},
		"unsafe URL": {
			change:  func(c *Collector) { c.URL = "http://example.test/api/v1" },
			wantErr: "HTTPS",
		},
		"zero timeout":     {change: func(c *Collector) { c.Timeout = 0 }, wantErr: "timeout"},
		"negative timeout": {change: func(c *Collector) { c.Timeout = -1 }, wantErr: "timeout"},
		"partial TLS pair": {
			change:  func(c *Collector) { c.TLSCert = "unused" },
			wantErr: "tls_cert",
		},
		"bad proxy does not expose password": {
			change:  func(c *Collector) { c.ProxyURL = "http://user:synthetic-secret@bad%host" },
			wantErr: "check proxy and TLS",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.OrganizationID = "org"
			c.APIKey = "test-key"
			if tc.change != nil {
				tc.change(c)
			}
			err := c.Init(t.Context())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.NotContains(t, err.Error(), "synthetic-secret")
			} else {
				require.NoError(t, err)
			}
			c.Cleanup(t.Context())
			c.Cleanup(t.Context())
		})
	}
}

func TestCheckIsOneOrganizationScopedPage(t *testing.T) {
	f, server := newFixtureAPI(t)
	// Check must neither follow pagination nor require an item/metric.
	f.set(
		"/organizations/org/devices",
		fixtureResponse{
			body: `[]`,
			link: `</api/v1/organizations/org/devices?startingAfter=next>; rel="next"`,
		},
	)
	c := newTestCollector(t, server)
	require.NoError(t, c.Check(t.Context()))
	calls := f.calls()
	require.Len(t, calls, 1)
	assert.Equal(
		t,
		fixtureRequest{
			path:          "/organizations/org/devices",
			pageSize:      "3",
			authorization: "Bearer test-key",
			query:         url.Values{"perPage": {"3"}},
			at:            calls[0].at,
		},
		calls[0],
	)
	assert.Empty(t, c.store.Read().HostScopes())
	f.set("/organizations/org/devices", fixtureResponse{
		status: 403,
		body:   `{"errors":["denied"]}`,
	})
	require.ErrorContains(t, c.Check(t.Context()), "HTTP status 403")
}

func TestAvailabilityStatesAndPagination(t *testing.T) {
	f, server := newFixtureAPI(t)
	f.set(
		"/organizations/org/devices",
		fixtureResponse{
			body: `[{"serial":"S1","networkId":"N1"},{"serial":"S2"}]`,
			link: `</api/v1/organizations/org/devices?startingAfter=p2>; rel="next"`,
		},
	)
	f.responses["/organizations/org/devices:p2"] = fixtureResponse{
		body: `[{"serial":"S3"},{"serial":"S4"},{"serial":"S5"}]`,
	}
	f.set(
		"/organizations/org/devices/availabilities",
		fixtureResponse{
			body: `[{"serial":"S1","status":"online","network":{"id":"N1"}},{"serial":"S2","status":"alerting"}]`,
			link: `</api/v1/organizations/org/devices/availabilities?startingAfter=p2>; rel="next"`,
		},
	)
	f.responses["/organizations/org/devices/availabilities:p2"] = fixtureResponse{
		body: `[{"serial":"S3","status":"offline"},{"serial":"S4","status":"dormant"},{"serial":"OLD","status":"offline"}]`,
	}
	c := newTestCollector(t, server)
	require.NoError(t, collectCycle(t.Context(), c))
	assert.Equal(t, map[string]metrix.SampleValue{"organization_assigned_devices": 5}, readOrganization(c))
	got := readDevices(c)
	values := make(map[string]map[string]metrix.SampleValue)
	for serial, d := range got {
		values[serial] = d.Values
	}
	assert.Equal(
		t,
		map[string]map[string]metrix.SampleValue{
			"S1": wantStatus("online"),
			"S2": wantStatus("alerting"),
			"S3": wantStatus("offline"),
			"S4": wantStatus("dormant"),
			"S5": {"device_assigned": 1},
		},
		values,
	)
	assert.Equal(
		t,
		map[string]string{
			"organization_id":   "org",
			"organization_name": "Org",
			"serial":            "S1",
			"device_name":       "",
			"model":             "",
			"product_type":      "",
			"network_id":        "N1",
			"network_name":      "Office",
		},
		got["S1"].Labels,
	)
	assert.Equal(t, "meraki-S1", got["S1"].Scope.Hostname)
	assert.Equal(t, "meraki_device", got["S1"].Scope.Labels["_vnode_type"])
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	require.Len(t, f.calls(), 6)
}

func TestMissingOrAmbiguousAvailability(t *testing.T) {
	tests := map[string]string{
		"missing":               `[]`,
		"unknown":               `[{"serial":"S1","status":"future-state"}]`,
		"null":                  `[{"serial":"S1","status":null}]`,
		"network moved":         `[{"serial":"S1","status":"offline","network":{"id":"N2"}}]`,
		"conflicting duplicate": `[{"serial":"S1","status":"online"},{"serial":"S1","status":"offline"},{"serial":"S1","status":"online"}]`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			f, server := newFixtureAPI(t)
			c := newTestCollector(t, server)
			require.NoError(t, collectCycle(t.Context(), c))
			before := readDevices(c)["S1"]
			f.set("/organizations/org/devices/availabilities", fixtureResponse{
				body: body,
			})
			require.NoError(t, collectCycle(t.Context(), c))
			after := readDevices(c)
			require.Len(t, after, 1)
			assert.Equal(t, map[string]metrix.SampleValue{"device_assigned": 1}, after["S1"].Values)
			assert.Equal(t, before.Scope, after["S1"].Scope)
		})
	}
}

func TestDiscoveryFailureRecoveryAndRemoval(t *testing.T) {
	f, server := newFixtureAPI(t)
	c := newTestCollector(t, server)
	require.NoError(t, collectCycle(t.Context(), c))
	before := readDevices(c)["S1"]
	f.set(
		"/organizations/org/devices",
		fixtureResponse{
			body: `[{"serial":"S2"}]`,
			link: `</api/v1/organizations/org/devices?startingAfter=fail>; rel="next"`,
		},
	)
	f.responses["/organizations/org/devices:fail"] = fixtureResponse{
		status: 500,
		body:   `{"errors":["failed page"]}`,
	}
	require.ErrorContains(t, collectCycle(t.Context(), c), "HTTP status 500")
	assert.Empty(t, readDevices(c), "failed snapshot must not be freshly visible")
	f.set(
		"/organizations/org/devices",
		fixtureResponse{
			body: `[{"serial":"S1","networkId":"N2","name":"Renamed","model":"MR","productType":"wireless"}]`,
		},
	)
	f.set("/organizations/org/networks", fixtureResponse{
		body: `[{"id":"N2","name":"New Office"}]`,
	})
	f.set(
		"/organizations/org/devices/availabilities",
		fixtureResponse{
			body: `[{"serial":"S1","status":"offline","network":{"id":"N2"}}]`,
		},
	)
	require.NoError(t, collectCycle(t.Context(), c))
	after := readDevices(c)["S1"]
	assert.Equal(t, before.Scope.GUID, after.Scope.GUID)
	assert.Equal(t, "Renamed", after.Scope.Hostname)
	assert.Equal(t, "New Office", after.Labels["network_name"])
	assert.Equal(t, wantStatus("offline"), after.Values)
	f.set("/organizations/org/devices", fixtureResponse{
		body: `[]`,
	})
	require.NoError(t, collectCycle(t.Context(), c))
	assert.Empty(t, readDevices(c))
	assert.Equal(t, map[string]metrix.SampleValue{"organization_assigned_devices": 0}, readOrganization(c))
}

func TestInvalidDiscovery(t *testing.T) {
	tests := map[string]struct{ path, body, wantErr string }{
		"organization absent": {"/organizations", `[]`, "not visible"},
		"missing serial":      {"/organizations/org/devices", `[{}]`, "no serial"},
		"conflicting assignment": {
			"/organizations/org/devices",
			`[{"serial":"S1","networkId":"N1"},{"serial":"S1","networkId":"N2"}]`,
			"conflicting",
		},
		"wrong network organization": {
			"/organizations/org/networks",
			`[{"id":"N1","organizationId":"other"}]`,
			"different organization",
		},
		"malformed body": {"/organizations/org/devices/availabilities", `{}`, "invalid JSON array"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, server := newFixtureAPI(t)
			f.set(tc.path, fixtureResponse{
				body: tc.body,
			})
			c := newTestCollector(t, server)
			require.ErrorContains(t, collectCycle(t.Context(), c), tc.wantErr)
			assert.Empty(t, readDevices(c))
			assert.Empty(t, readOrganization(c))
		})
	}
}

func TestRetryAfterPreventsEarlyRequests(t *testing.T) {
	f, server := newFixtureAPI(t)
	c := newTestCollector(t, server)
	now := time.Now()
	c.now = func() time.Time { return now }
	f.set("/organizations", fixtureResponse{
		status:     429,
		retryAfter: "600",
		body:       `{"errors":["rate limit"]}`,
	})
	require.ErrorContains(t, collectCycle(t.Context(), c), "HTTP status 429")
	require.Len(t, f.calls(), 1)
	require.ErrorContains(t, collectCycle(t.Context(), c), "Retry-After")
	require.ErrorContains(t, c.Check(t.Context()), "Retry-After")
	require.Len(t, f.calls(), 1)
	assert.Empty(t, readOrganization(c))
	now = now.Add(600 * time.Second)
	f.set("/organizations", fixtureResponse{
		body: `[{"id":"org"}]`,
	})
	require.NoError(t, collectCycle(t.Context(), c))
	assert.Equal(t, wantStatus("online"), readDevices(c)["S1"].Values)
}

func TestOperationDeadlineAndCancellation(t *testing.T) {
	_, server := newFixtureAPI(t)
	c := newTestCollector(t, server)
	c.Timeout = confopt.Duration(20 * time.Millisecond)
	err := collectCycle(t.Context(), c) // Request pacing spans the complete operation budget.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, readDevices(c))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Check(ctx), context.Canceled)
	require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
	assert.Empty(t, readOrganization(c))
}

func TestBeforeInit(t *testing.T) {
	c := New()
	c.Cleanup(t.Context())
	// Configuration is usable and serializable before Init.
	data, err := json.Marshal(c.Configuration())
	require.NoError(t, err)
	var cfg Config
	require.NoError(t, json.Unmarshal(data, &cfg))
	assert.Equal(t, c.Config, cfg)
}
