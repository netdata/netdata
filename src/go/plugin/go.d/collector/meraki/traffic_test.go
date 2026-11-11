// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficCollection(t *testing.T) {
	f, c, now := newTrafficFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))

	for metric, value := range map[string]metrix.SampleValue{
		metricSwitchPortTrafficSent:          10.5,
		metricSwitchPortTrafficReceived:      20.25,
		metricSwitchPortTrafficIntervalAge:   60,
		metricSwitchPortPacketsSent:          2.5,
		metricSwitchPortPacketsReceived:      3.25,
		metricApplianceUplinkTrafficSent:     8,
		metricApplianceUplinkTrafficReceived: 16,
	} {
		points := metricPoints(c, metric)
		require.Len(t, points, 1, metric)
		p := points[0]
		assert.Equal(t, value, p.Value, metric)
		switch metric {
		case metricApplianceUplinkTrafficSent, metricApplianceUplinkTrafficReceived:
			assert.True(t, p.Scope.IsDefault(), metric)
			assert.Equal(t, map[string]string{
				"organization_id":   "org",
				"organization_name": "Org",
				"network_id":        "N1",
				"network_name":      "Office",
				"interface":         "wan1",
			}, p.Labels, metric)
		default:
			assert.Equal(t, "S1", p.Scope.Labels["serial"], metric)
			assert.Equal(t, "1/0", p.Labels["port_id"], metric)
		}
	}
	assert.Len(t, readDevices(c), 3)

	type request struct {
		path  string
		query url.Values
	}
	var traffic []request
	for _, call := range f.calls()[4:] {
		traffic = append(traffic, request{call.path, call.query})
	}
	// Usage history is searched over the last 15 minutes; uplinks read the 5
	// minutes that ended 5 minutes ago.
	ago := func(d time.Duration) []string { return []string{now.Add(-d).Format(time.RFC3339)} }
	assert.Equal(t, []request{
		{switchUsageHistoryPath, url.Values{
			"perPage": {"50"},
			"t0":      ago(15 * time.Minute),
			"t1":      ago(0),
		}},
		{applianceUplinkUsagePath, url.Values{
			"t0": ago(10 * time.Minute),
			"t1": ago(5 * time.Minute),
		}},
		{switchPacketsPath, url.Values{
			"timespan": {"300"},
		}},
	}, traffic)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestTrafficOptionsAreIndependent(t *testing.T) {
	defaults := New()
	assert.False(t, defaults.CollectSwitchPortTraffic)
	assert.False(t, defaults.CollectSwitchPortPackets)
	assert.False(t, defaults.CollectApplianceUplinkTraffic)

	tests := map[string]struct {
		portTraffic, portPackets, uplinks bool
		wantRequests                      int
	}{
		"all disabled":             {wantRequests: 4},
		"switch port traffic":      {portTraffic: true, wantRequests: 5},
		"switch port packets":      {portPackets: true, wantRequests: 5},
		"appliance uplink traffic": {uplinks: true, wantRequests: 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			c.CollectSwitchPortTraffic = tc.portTraffic
			c.CollectSwitchPortPackets = tc.portPackets
			c.CollectApplianceUplinkTraffic = tc.uplinks

			require.NoError(t, c.Check(t.Context()))
			require.Len(t, f.calls(), 1, "Check stays one probe")
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Len(t, f.calls(), 1+tc.wantRequests)
			assert.Len(t, metricPoints(c, metricSwitchPortTrafficSent), boolCount(tc.portTraffic))
			assert.Len(t, metricPoints(c, metricSwitchPortPacketsSent), boolCount(tc.portPackets))
			assert.Len(t, metricPoints(c, metricApplianceUplinkTrafficSent), boolCount(tc.uplinks))
		})
	}
}

func TestTrafficZeroIsAValueAndMissingIsAGap(t *testing.T) {
	f, c, now := newTrafficFixture(t)
	f.set(switchUsageHistoryPath, fixtureResponse{
		body: usageHistoryBody(
			"S1",
			"N1",
			"1/0",
			usageInterval(now.Add(-time.Minute), 300, `{"upstream":0,"downstream":null}`),
		),
	})
	f.set(switchPacketsPath, fixtureResponse{
		body: `[{"portId":"1/0","packets":[{"desc":"Total","ratePerSec":{"sent":0,"recv":null}},` +
			`{"desc":"Broadcast","ratePerSec":{"sent":123,"recv":123}}]},` +
			`{"portId":"2","packets":[{"desc":"Unicast","ratePerSec":{"sent":999,"recv":999}}]}]`,
	})
	f.set(applianceUplinkUsagePath, fixtureResponse{
		body: `[{"networkId":"N1","byUplink":[{"interface":"wan1","sent":0},{"interface":"wan2","received":null}]}]`,
	})
	require.NoError(t, collectCycle(t.Context(), c))

	zero := []metrix.SampleValue{0}
	assert.Equal(t, zero, metricValues(c, metricSwitchPortTrafficSent))
	assert.Equal(t, zero, metricValues(c, metricSwitchPortPacketsSent))
	assert.Equal(t, zero, metricValues(c, metricApplianceUplinkTrafficSent))
	assert.Empty(t, metricValues(c, metricSwitchPortTrafficReceived))
	assert.Empty(t, metricValues(c, metricSwitchPortPacketsReceived))
	assert.Empty(t, metricValues(c, metricApplianceUplinkTrafficReceived))
}

func TestSwitchPortTrafficIntervalSelection(t *testing.T) {
	now := time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC)
	current := usageInterval(now.Add(-time.Minute), 300, `{"upstream":10.5,"downstream":20.25}`)
	type want struct {
		sent, received, age []metrix.SampleValue
	}
	accepted := want{
		sent:     []metrix.SampleValue{10.5},
		received: []metrix.SampleValue{20.25},
		age:      []metrix.SampleValue{60},
	}
	none := want{
		sent:     []metrix.SampleValue{},
		received: []metrix.SampleValue{},
		age:      []metrix.SampleValue{},
	}
	tests := map[string]struct {
		history string
		want    want
	}{
		"interval ending in the window": {
			history: usageHistoryBody("S1", "N1", "1/0", current),
			want:    accepted,
		},
		"interval ending 10 minutes ago": {
			history: usageHistoryBody(
				"S1",
				"N1",
				"1/0",
				usageInterval(now.Add(-10*time.Minute), 300, `{"upstream":10.5,"downstream":20.25}`),
			),
			want: want{
				sent:     []metrix.SampleValue{10.5},
				received: []metrix.SampleValue{20.25},
				age:      []metrix.SampleValue{600},
			},
		},
		"20-minute interval": {
			history: usageHistoryBody(
				"S1",
				"N1",
				"1/0",
				usageInterval(now.Add(-time.Minute), 1200, `{"upstream":10.5,"downstream":20.25}`),
			),
			want: accepted,
		},
		"interval ending at the window start": {
			history: usageHistoryBody(
				"S1",
				"N1",
				"1/0",
				usageInterval(now.Add(-15*time.Minute), 300, `{"upstream":99,"downstream":99}`),
			),
			want: none,
		},
		"interval ending in the future": {
			history: usageHistoryBody(
				"S1",
				"N1",
				"1/0",
				usageInterval(now.Add(time.Second), 300, `{"upstream":99,"downstream":99}`),
			),
			want: none,
		},
		"interval of another length": {
			history: usageHistoryBody(
				"S1",
				"N1",
				"1/0",
				usageInterval(now.Add(-time.Minute), 60, `{"upstream":99,"downstream":99}`),
			),
			want: none,
		},
		"switch reported in another network": {
			history: usageHistoryBody("S1", "OTHER", "1/0", current),
			want:    none,
		},
		"switch not assigned": {
			history: usageHistoryBody("OLD", "N1", "1/0", current),
			want:    none,
		},
		"port without ID": {
			history: usageHistoryBody("S1", "N1", "", current),
			want:    none,
		},
		"identical duplicate": {
			history: usageHistoryBody("S1", "N1", "1/0", current+","+current),
			want:    accepted,
		},
		"conflicting duplicate": {
			history: usageHistoryBody("S1", "N1", "1/0",
				current+","+usageInterval(now.Add(-time.Minute), 300, `{"upstream":99,"downstream":20.25}`)),
			want: none,
		},
		"later interval without values": {
			history: usageHistoryBody("S1", "N1", "1/0", current+","+usageInterval(now, 300, `{}`)),
			want:    none,
		},
		"later interval with one direction": {
			history: usageHistoryBody("S1", "N1", "1/0", current+","+usageInterval(now, 300, `{"downstream":0}`)),
			want: want{
				sent:     []metrix.SampleValue{},
				received: []metrix.SampleValue{0},
				age:      []metrix.SampleValue{0},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			f.set(switchUsageHistoryPath, fixtureResponse{
				body: tc.history,
			})
			require.NoError(t, collectCycle(t.Context(), c))

			assert.Equal(t, tc.want, want{
				sent:     metricValues(c, metricSwitchPortTrafficSent),
				received: metricValues(c, metricSwitchPortTrafficReceived),
				age:      metricValues(c, metricSwitchPortTrafficIntervalAge),
			})
			assert.Len(t, metricPoints(c, metricSwitchPortPacketsSent), 1, "packets are independent")
			assert.Len(t, metricPoints(c, metricApplianceUplinkTrafficSent), 1, "uplinks are independent")
		})
	}
}

func TestSwitchPortPacketsDuplicates(t *testing.T) {
	total := `{"desc":"Total","ratePerSec":{"sent":2.5,"recv":3.25}}`
	tests := map[string]struct {
		categories string
		wantSent   []metrix.SampleValue
	}{
		"identical duplicate": {
			categories: total + "," + total,
			wantSent:   []metrix.SampleValue{2.5},
		},
		"conflicting duplicate": {
			categories: total + `,{"desc":"Total","ratePerSec":{"sent":99,"recv":3.25}}`,
			wantSent:   []metrix.SampleValue{},
		},
		"agreeing duplicates after a conflict": {
			categories: total + strings.Repeat(`,{"desc":"Total","ratePerSec":{"sent":99,"recv":3.25}}`, 2),
			wantSent:   []metrix.SampleValue{},
		},
		"negative rates": {
			categories: `{"desc":"Total","ratePerSec":{"sent":-1,"recv":-1}}`,
			wantSent:   []metrix.SampleValue{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			f.set(
				switchPacketsPath,
				fixtureResponse{
					body: fmt.Sprintf(`[{"portId":"1/0","packets":[%s]}]`, tc.categories),
				},
			)
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Equal(t, tc.wantSent, metricValues(c, metricSwitchPortPacketsSent))
		})
	}
}

func TestApplianceUplinkTrafficAttribution(t *testing.T) {
	tests := map[string]struct {
		network  string
		uplinks  string
		wantSent []metrix.SampleValue
	}{
		"identical rows of two appliances": {
			uplinks: `{"interface":"wan1","serial":"A","sent":300000,"received":600000},` +
				`{"interface":"wan1","serial":"B","sent":300000,"received":600000}`,
			wantSent: []metrix.SampleValue{8},
		},
		"conflicting rows of two appliances": {
			uplinks: `{"interface":"wan1","serial":"A","sent":300000,"received":600000},` +
				`{"interface":"wan1","serial":"B","sent":400000,"received":600000}`,
			wantSent: []metrix.SampleValue{},
		},
		"network not in the organization's list": {
			network:  "UNKNOWN",
			uplinks:  `{"interface":"wan1","sent":300000,"received":600000}`,
			wantSent: []metrix.SampleValue{},
		},
		"negative bytes": {
			uplinks:  `{"interface":"wan1","sent":-1,"received":-1}`,
			wantSent: []metrix.SampleValue{},
		},
		"fractional bytes": {
			uplinks:  `{"interface":"wan1","sent":300000.5,"received":-1}`,
			wantSent: []metrix.SampleValue{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			network := "N1"
			if tc.network != "" {
				network = tc.network
			}
			f.set(applianceUplinkUsagePath, fixtureResponse{
				body: fmt.Sprintf(`[{"networkId":%q,"byUplink":[%s]}]`, network, tc.uplinks),
			})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Equal(t, tc.wantSent, metricValues(c, metricApplianceUplinkTrafficSent))
		})
	}
}

// trafficFamilies maps each family's request path to one of its metrics.
var trafficFamilies = map[string]string{
	switchUsageHistoryPath:   metricSwitchPortTrafficSent,
	applianceUplinkUsagePath: metricApplianceUplinkTrafficSent,
	switchPacketsPath:        metricSwitchPortPacketsSent,
}

func TestTrafficFailureLeavesOnlyItsGaps(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		for failing := range trafficFamilies {
			t.Run(fmt.Sprintf("%d %s", status, failing), func(t *testing.T) {
				f, c, _ := newTrafficFixture(t)
				valid := f.response(failing)
				require.NoError(t, collectCycle(t.Context(), c))

				f.set(failing, fixtureResponse{
					status: status,
					body:   `{"errors":["synthetic denied"]}`,
				})
				require.NoError(t, collectCycle(t.Context(), c))
				assert.Len(t, readDevices(c), 3)
				assert.Equal(t, metrix.SampleValue(3), readOrganization(c)[metricOrganizationAssignedDevices])
				for path, metric := range trafficFamilies {
					assert.Len(t, metricPoints(c, metric), boolCount(path != failing), metric)
				}

				f.set(failing, valid)
				require.NoError(t, collectCycle(t.Context(), c))
				for _, metric := range trafficFamilies {
					assert.Len(t, metricPoints(c, metric), 1, metric)
				}
			})
		}
	}
}

func TestTraffic429StopsTrafficRequests(t *testing.T) {
	// Requests sent up to and including the throttled one, in family order.
	sentUntil := map[string]int{
		switchUsageHistoryPath:   5,
		applianceUplinkUsagePath: 6,
		switchPacketsPath:        7,
	}
	for throttled, wantRequests := range sentUntil {
		for _, retryAfter := range []string{"", "600"} {
			t.Run(fmt.Sprintf("%s Retry-After=%q", throttled, retryAfter), func(t *testing.T) {
				f, c, now := newTrafficFixture(t)
				f.set(throttled, fixtureResponse{
					status:     http.StatusTooManyRequests,
					retryAfter: retryAfter,
				})
				require.NoError(t, collectCycle(t.Context(), c))
				require.Len(t, f.calls(), wantRequests, "a 429 stops later traffic requests")
				assert.Len(t, readDevices(c), 3)

				c.now = func() time.Time { return now.Add(300 * time.Second) }
				if retryAfter == "" {
					require.NoError(t, c.Check(t.Context()), "a missing Retry-After must not invent a wait")
					assert.Len(t, f.calls(), wantRequests+1)
					return
				}
				deadline := now.Add(600 * time.Second)
				err := c.Check(t.Context())
				require.Error(t, err)
				assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
				err = collectCycle(t.Context(), c)
				require.Error(t, err)
				assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
				assert.Len(t, f.calls(), wantRequests)
			})
		}
	}
}

func TestTrafficSurvivesRenames(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))
	before := metricPoints(c, metricSwitchPortTrafficSent)[0]

	f.set("/organizations/org/devices", fixtureResponse{
		body: `[{"serial":"S1","networkId":"N1","name":"Renamed","model":"MS120","productType":"switch"},` +
			`{"serial":"A","networkId":"N1","productType":"appliance"},` +
			`{"serial":"B","networkId":"N1","productType":"appliance"}]`,
	})
	f.set("/organizations/org/networks", fixtureResponse{
		body: `[{"id":"N1","organizationId":"org","name":"Renamed office"}]`,
	})
	require.NoError(t, collectCycle(t.Context(), c))

	ports := metricPoints(c, metricSwitchPortTrafficSent)
	require.Len(t, ports, 1)
	assert.Equal(t, before.Scope.GUID, ports[0].Scope.GUID)
	assert.Equal(t, "1/0", ports[0].Labels["port_id"])
	assert.Equal(t, "Renamed", ports[0].Labels["device_name"])
	uplinks := metricPoints(c, metricApplianceUplinkTrafficSent)
	require.Len(t, uplinks, 1)
	assert.Equal(t, "N1", uplinks[0].Labels["network_id"])
	assert.Equal(t, "Renamed office", uplinks[0].Labels["network_name"])
}

func TestTrafficDeadlineAbortsCycle(t *testing.T) {
	_, c, _ := newTrafficFixture(t)
	// The four paced core reads take about 300ms; the deadline expires during the
	// traffic reads, so the core metrics must not commit either.
	c.Timeout = confopt.Duration(450 * time.Millisecond)
	require.ErrorIs(t, collectCycle(t.Context(), c), context.DeadlineExceeded)
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
}

func TestTrafficCanceledCycleSendsNoRequests(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
	assert.Empty(t, f.calls())
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
}

func TestSwitchPortPacketsRequestEverySwitchInSerialOrder(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	f.set("/organizations/org/devices", fixtureResponse{
		body: `[{"serial":"S2","productType":"switch"},{"serial":"S1","productType":"switch"},` +
			`{"serial":"AP","productType":"wireless"},{"serial":"MX","productType":"appliance"}]`,
	})
	f.set("/devices/S2/switch/ports/statuses/packets", fixtureResponse{
		body: `[]`,
	})
	c.CollectSwitchPortTraffic = false
	c.CollectApplianceUplinkTraffic = false
	require.NoError(t, collectCycle(t.Context(), c))

	var paths []string
	for _, call := range f.calls()[4:] {
		paths = append(paths, call.path)
	}
	assert.Equal(t, []string{switchPacketsPath, "/devices/S2/switch/ports/statuses/packets"}, paths)
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
