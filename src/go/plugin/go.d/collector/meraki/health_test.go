// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCollection(t *testing.T) {
	f, c, now := newHealthFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))
	assert.Equal(t, map[string]metrix.SampleValue{"A/wan1/198.51.100.1": 5.25, "B/wan1/198.51.100.1": 0}, healthPointsByKey(c, metricApplianceUplinkPacketLoss))
	assert.Equal(t, map[string]metrix.SampleValue{"A/wan1/198.51.100.1": 12.5, "B/wan1/198.51.100.1": 25.75}, healthPointsByKey(c, metricApplianceUplinkLatency))
	assert.Equal(t, map[string]metrix.SampleValue{"A//": 30, "B//": 60}, healthPointsByKey(c, metricApplianceReportAge))
	assert.Equal(t, map[string]metrix.SampleValue{"A/wan1/198.51.100.1": 180, "B/wan1/198.51.100.1": 180}, healthPointsByKey(c, metricApplianceUplinkSampleAge))
	for _, metric := range []string{metricApplianceUplinkStatus, metricApplianceReportAge, metricApplianceUplinkPacketLoss, metricApplianceUplinkLatency, metricApplianceUplinkSampleAge} {
		for _, p := range trafficPoints(c, metric) {
			assert.False(t, p.Scope.IsDefault())
			assert.Contains(t, []string{"A", "B"}, p.Scope.Labels["serial"])
			assert.Equal(t, p.Scope.Labels["serial"], p.Labels["serial"])
		}
	}
	calls := f.calls()
	require.Len(t, calls, 9)
	assert.Equal(t, applianceUplinkStatusesPath, calls[4].path)
	assert.Equal(t, url.Values{"perPage": {"1000"}}, calls[4].query)
	assert.Equal(t, uplinkLossAndLatencyPath, calls[5].path)
	assert.Equal(t, url.Values{"t0": {now.Add(-7 * time.Minute).Format(time.RFC3339)}, "t1": {now.Add(-2 * time.Minute).Format(time.RFC3339)}}, calls[5].query)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestHealthOptionAndEligibility(t *testing.T) {
	assert.False(t, New().CollectApplianceUplinkHealth)
	for _, tc := range []struct {
		name              string
		enabled, eligible bool
		calls             int
	}{{"disabled", false, true, 4}, {"health only", true, true, 6}, {"no appliances", true, false, 4}} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, _ := newHealthFixture(t)
			healthOnly(c)
			c.CollectApplianceUplinkHealth = tc.enabled
			if !tc.eligible {
				f.set("/organizations/org/devices", fixtureResponse{body: `[{"serial":"S1","networkId":"N1","productType":"switch"}]`})
			}
			require.NoError(t, c.Check(t.Context()))
			require.Len(t, f.calls(), 1)
			assert.Equal(t, "/organizations/org/devices", f.calls()[0].path)
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Len(t, f.calls(), 1+tc.calls)
			assert.Len(t, trafficPoints(c, metricApplianceUplinkLatency), 2*boolCount(tc.enabled && tc.eligible))
		})
	}
}

func TestHealthStatesAndReportMeaning(t *testing.T) {
	f, c, now := newHealthFixture(t)
	healthOnly(c)
	var uplinks []string
	for i, state := range []string{"active", "connecting", "failed", "not connected", "ready", "disabled", "unknown", ""} {
		uplinks = append(uplinks, fmt.Sprintf(`{"interface":"wan%d","status":%q}`, i+1, state))
	}
	f.set(applianceUplinkStatusesPath, fixtureResponse{body: "[" + healthStatusRow("A", "N1", now.Add(-24*time.Hour).Format(time.RFC3339), strings.Join(uplinks, ",")) + "]"})
	require.NoError(t, collectCycle(t.Context(), c))
	states := healthPointsByKey(c, metricApplianceUplinkStatus)
	require.Len(t, states, 25)
	for i, state := range []string{"active", "connecting", "failed", "not_connected", "ready"} {
		for _, other := range []string{"active", "connecting", "failed", "not_connected", "ready"} {
			want := metrix.SampleValue(0)
			if state == other {
				want = 1
			}
			assert.Equal(t, want, states[fmt.Sprintf("A/wan%d//%s", i+1, other)])
		}
	}
	assert.Equal(t, map[string]metrix.SampleValue{"A//": 86400}, healthPointsByKey(c, metricApplianceReportAge), "old heartbeat age is literal and does not relabel ready/active")
	for _, at := range []string{"", "bad", now.Add(time.Second).Format(time.RFC3339)} {
		f.set(applianceUplinkStatusesPath, fixtureResponse{body: "[" + healthStatusRow("A", "N1", at, `{"interface":"wan1","status":"ready"}`) + "]"})
		require.NoError(t, collectCycle(t.Context(), c))
		assert.Empty(t, trafficPoints(c, metricApplianceReportAge))
		assert.Equal(t, metrix.SampleValue(1), healthPointsByKey(c, metricApplianceUplinkStatus)["A/wan1//ready"])
	}
}

func TestHealthLatestSamplesAndBounds(t *testing.T) {
	f, c, now := newHealthFixture(t)
	healthOnly(c)
	fields := `"lossPercent":5.25,"latencyMs":12.5`
	tests := map[string]struct {
		samples            string
		loss, latency, age []metrix.SampleValue
	}{
		"zero at end":              {healthSample(now.Add(-2*time.Minute), `"lossPercent":0,"latencyMs":0`), []metrix.SampleValue{0}, []metrix.SampleValue{0}, []metrix.SampleValue{120}},
		"inclusive start":          {healthSample(now.Add(-7*time.Minute), fields), []metrix.SampleValue{5.25}, []metrix.SampleValue{12.5}, []metrix.SampleValue{420}},
		"before start":             {samples: healthSample(now.Add(-7*time.Minute-time.Nanosecond), fields)},
		"after end":                {samples: healthSample(now.Add(-2*time.Minute+time.Nanosecond), fields)},
		"future":                   {samples: healthSample(now.Add(time.Minute), fields)},
		"invalid time":             {samples: `{"ts":"bad","lossPercent":1,"latencyMs":1}`},
		"latest null":              {samples: healthSample(now.Add(-4*time.Minute), fields) + "," + healthSample(now.Add(-3*time.Minute), `"lossPercent":null,"latencyMs":null`)},
		"latest missing":           {samples: healthSample(now.Add(-4*time.Minute), fields) + "," + healthSample(now.Add(-3*time.Minute), "")},
		"latest partial unordered": {healthSample(now.Add(-3*time.Minute), `"latencyMs":0`) + "," + healthSample(now.Add(-4*time.Minute), fields), nil, []metrix.SampleValue{0}, []metrix.SampleValue{180}},
		"loss over100":             {healthSample(now.Add(-3*time.Minute), `"lossPercent":100.01,"latencyMs":1.5`), nil, []metrix.SampleValue{1.5}, []metrix.SampleValue{180}},
		"negative values":          {samples: healthSample(now.Add(-3*time.Minute), `"lossPercent":-1,"latencyMs":-1`)},
		"loss100":                  {healthSample(now.Add(-3*time.Minute), `"lossPercent":100`), []metrix.SampleValue{100}, nil, []metrix.SampleValue{180}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f.set(uplinkLossAndLatencyPath, fixtureResponse{body: "[" + healthProbeRow("A", "N1", "wan1", "198.51.100.1", tc.samples) + "]"})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.ElementsMatch(t, tc.loss, trafficValues(c, metricApplianceUplinkPacketLoss))
			assert.ElementsMatch(t, tc.latency, trafficValues(c, metricApplianceUplinkLatency))
			assert.ElementsMatch(t, tc.age, trafficValues(c, metricApplianceUplinkSampleAge))
		})
	}
}

func TestHealthIdentityDestinationsAndDuplicates(t *testing.T) {
	f, c, now := newHealthFixture(t)
	healthOnly(c)
	sample := healthSample(now.Add(-3*time.Minute), `"lossPercent":1.25,"latencyMs":2.5`)
	row := healthProbeRow("A", "N1", "wan1", "198.51.100.1", sample)
	conflict := healthProbeRow("A", "N1", "wan1", "198.51.100.1", healthSample(now.Add(-3*time.Minute), `"lossPercent":99,"latencyMs":2.5`))
	for name, tc := range map[string]struct {
		rows  string
		count int
	}{
		"same duplicate": {row + "," + row, 1}, "sticky disagreement": {row + "," + conflict + "," + conflict, 0},
		"two HA serials":                  {row + "," + healthProbeRow("B", "N1", "wan1", "198.51.100.1", sample), 2},
		"two destinations":                {row + "," + healthProbeRow("A", "N1", "wan1", "203.0.113.2", sample), 2},
		"two interfaces":                  {row + "," + healthProbeRow("A", "N1", "wan2", "198.51.100.1", sample), 2},
		"network conflict poisons serial": {row + "," + healthProbeRow("A", "OTHER", "wan2", "203.0.113.2", sample), 0},
		"unknown serial":                  {healthProbeRow("UNKNOWN", "N1", "wan1", "198.51.100.1", sample), 0},
		"switch serial":                   {healthProbeRow("S1", "N1", "wan1", "198.51.100.1", sample), 0},
		"missing destination":             {healthProbeRow("A", "N1", "wan1", "", sample), 0},
		"missing interface":               {healthProbeRow("A", "N1", "", "198.51.100.1", sample), 0},
	} {
		t.Run(name, func(t *testing.T) {
			f.set(uplinkLossAndLatencyPath, fixtureResponse{body: "[" + tc.rows + "]"})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Len(t, trafficPoints(c, metricApplianceUplinkLatency), tc.count)
			assert.Len(t, readDevices(c), 3)
		})
	}
	status := healthStatusRow("A", "N1", now.Add(-time.Minute).Format(time.RFC3339), `{"interface":"wan1","status":"ready"}`)
	bad := healthStatusRow("A", "N1", now.Add(-time.Minute).Format(time.RFC3339), `{"interface":"wan1","status":"failed"}`)
	for name, rows := range map[string]string{"sticky status": status + "," + bad + "," + bad, "network mismatch": status + "," + healthStatusRow("A", "OTHER", "", `{"interface":"wan2","status":"active"}`), "unknown serial": healthStatusRow("UNKNOWN", "N1", "", `{"interface":"wan1","status":"active"}`)} {
		t.Run(name, func(t *testing.T) {
			f.set(applianceUplinkStatusesPath, fixtureResponse{body: "[" + rows + "]"})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Empty(t, trafficPoints(c, metricApplianceUplinkStatus))
		})
	}
}

func TestHealthOptionalFailureAndRecovery(t *testing.T) {
	for _, status := range []int{403, 404, 500} {
		for _, path := range []string{applianceUplinkStatusesPath, uplinkLossAndLatencyPath} {
			t.Run(fmt.Sprintf("%d%s", status, path), func(t *testing.T) {
				f, c, _ := newHealthFixture(t)
				valid := f.response(path)
				f.set(path, fixtureResponse{status: status, body: `{"errors":["denied"]}`})
				require.NoError(t, collectCycle(t.Context(), c))
				assert.Len(t, readDevices(c), 3)
				assert.Len(t, trafficPoints(c, metricApplianceUplinkStatus), 10*boolCount(path != applianceUplinkStatusesPath))
				assert.Len(t, trafficPoints(c, metricApplianceUplinkLatency), 2*boolCount(path != uplinkLossAndLatencyPath))
				for _, metric := range trafficFamilies {
					assert.Len(t, trafficPoints(c, metric), 1)
				}
				f.set(path, valid)
				require.NoError(t, collectCycle(t.Context(), c))
				assert.Len(t, trafficPoints(c, metricApplianceUplinkLatency), 2)
				assert.Len(t, trafficPoints(c, metricApplianceUplinkStatus), 10)
			})
		}
	}
}

func TestHealth429StopsAllOptionalRequests(t *testing.T) {
	order := []string{applianceUplinkStatusesPath, uplinkLossAndLatencyPath, switchUsageHistoryPath, applianceUplinkUsagePath, switchPacketsPath}
	for i, path := range order {
		for _, delay := range []string{"", "600"} {
			t.Run(fmt.Sprintf("%s delay=%q", path, delay), func(t *testing.T) {
				f, c, now := newHealthFixture(t)
				f.set(path, fixtureResponse{status: http.StatusTooManyRequests, retryAfter: delay})
				require.NoError(t, collectCycle(t.Context(), c))
				require.Len(t, f.calls(), 5+i)
				for later := i + 1; later < len(order); later++ {
					for _, call := range f.calls() {
						assert.NotEqual(t, order[later], call.path)
					}
				}
				c.now = func() time.Time { return now.Add(5 * time.Minute) }
				if delay == "" {
					require.NoError(t, c.Check(t.Context()))
					assert.Len(t, f.calls(), 6+i)
					return
				}
				err := collectCycle(t.Context(), c)
				require.Error(t, err)
				assert.Equal(t, now.Add(10*time.Minute), collectorapi.RetryNotBefore(err))
				assert.Len(t, f.calls(), 5+i)
				c.now = func() time.Time { return now.Add(10 * time.Minute) }
				require.NoError(t, c.Check(t.Context()))
				assert.Len(t, f.calls(), 6+i)
			})
		}
	}
}

func TestHealthMalformedProbeAndContextAbort(t *testing.T) {
	for _, number := range []string{"NaN", "Infinity", "1e999"} {
		t.Run(number, func(t *testing.T) {
			f, c, now := newHealthFixture(t)
			healthOnly(c)
			f.set(uplinkLossAndLatencyPath, fixtureResponse{body: "[" + healthProbeRow("A", "N1", "wan1", "198.51.100.1", healthSample(now.Add(-3*time.Minute), `"lossPercent":`+number)) + "]"})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Empty(t, trafficPoints(c, metricApplianceUplinkPacketLoss))
			assert.Len(t, trafficPoints(c, metricApplianceUplinkStatus), 10)
		})
	}
	for _, mode := range []string{"deadline", "parent canceled", "health write canceled"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := newHealthFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := error(context.Canceled)
			switch mode {
			case "deadline":
				c.Timeout = confopt.Duration(450 * time.Millisecond)
				want = context.DeadlineExceeded
			case "parent canceled":
				cancel()
			case "health write canceled":
				c.metrics.meter = healthCancelMeter{SnapshotMeter: c.metrics.meter, cancel: cancel}
			}
			require.ErrorIs(t, collectCycle(ctx, c), want)
			assert.Empty(t, readOrganization(c))
			assert.Empty(t, readDevices(c))
			assert.Empty(t, trafficPoints(c, metricApplianceUplinkLatency))
			if mode == "parent canceled" {
				assert.Empty(t, f.calls())
			}
		})
	}
}

func TestHealthRenamesKeepDeviceIdentity(t *testing.T) {
	f, c, _ := newHealthFixture(t)
	healthOnly(c)
	require.NoError(t, collectCycle(t.Context(), c))
	before := readDevices(c)
	f.set("/organizations/org/devices", fixtureResponse{body: `[{"serial":"A","networkId":"N1","name":"Renamed appliance","model":"MX68","productType":"appliance"},{"serial":"B","networkId":"N1","productType":"appliance"}]`})
	f.set("/organizations/org/networks", fixtureResponse{body: `[{"id":"N1","organizationId":"org","name":"Renamed site"}]`})
	require.NoError(t, collectCycle(t.Context(), c))
	after := readDevices(c)
	assert.Equal(t, before["A"].Scope.GUID, after["A"].Scope.GUID)
	assert.Equal(t, before["B"].Scope.GUID, after["B"].Scope.GUID)
	for _, p := range trafficPoints(c, metricApplianceUplinkLatency) {
		assert.Equal(t, "Renamed site", p.Labels["network_name"])
		if p.Labels["serial"] == "A" {
			assert.Equal(t, "Renamed appliance", p.Labels["device_name"])
		}
	}
}

func TestHealthManagedPublicationAndPlacement(t *testing.T) {
	_, c, _ := newHealthFixture(t)
	healthOnly(c)
	out := &lockedBuffer{}
	job := startManagedJob(t, c, out, hostoutput.New())
	require.Eventually(t, func() bool {
		if strings.Contains(out.String(), "meraki.appliance_uplink_latency") {
			return true
		}
		job.Tick(0)
		return false
	}, 5*time.Second, 120*time.Millisecond)
	wire := out.String()
	host := ""
	hostLine := regexp.MustCompile(`^HOST '([^']*)'`)
	chartLine := regexp.MustCompile(`^CHART .* '(meraki\.appliance_(?:uplink_(?:status|packet_loss|latency|sample_age)|report_age))' `)
	hosts := map[string]map[string]bool{}
	for _, line := range strings.Split(wire, "\n") {
		if m := hostLine.FindStringSubmatch(line); m != nil {
			host = m[1]
		}
		if m := chartLine.FindStringSubmatch(line); m != nil {
			if hosts[m[1]] == nil {
				hosts[m[1]] = map[string]bool{}
			}
			hosts[m[1]][host] = true
		}
	}
	devices := readDevices(c)
	want := map[string]bool{devices["A"].Scope.GUID: true, devices["B"].Scope.GUID: true}
	require.NotEmpty(t, devices["A"].Scope.GUID)
	require.Len(t, hosts, 5)
	for chart, got := range hosts {
		assert.Equal(t, want, got, chart)
	}
	for _, value := range []string{"5.25", "12.5", "25.75"} {
		assert.Regexp(t, regexp.MustCompile(`(?m)^SET '[^']+' = `+regexp.QuoteMeta(value)+`$`), wire)
	}
}
