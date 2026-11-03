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
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficValuesAndRequestContract(t *testing.T) {
	f, c, now := newTrafficFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))
	for metric, value := range map[string]float64{metricPortTrafficSent: 10.5, metricPortTrafficReceived: 20.25, metricPortTrafficIntervalAge: 60, metricPortPacketsSent: 2.5, metricPortPacketsReceived: 3.25, metricUplinkTrafficSent: 8, metricUplinkTrafficReceived: 16} {
		points := trafficPoints(c, metric)
		require.Len(t, points, 1, metric)
		p := points[0]
		assert.Equal(t, value, p.Value, metric)
		if strings.HasPrefix(metric, "appliance_") {
			assert.True(t, p.Scope.IsDefault())
			assert.Equal(t, "N1", p.Labels["network_id"])
			assert.Equal(t, "wan1", p.Labels["interface"])
			assert.NotContains(t, p.Labels, "serial")
		} else {
			assert.Equal(t, "S1", p.Scope.Labels["serial"])
			assert.Equal(t, "1/0", p.Labels["port_id"])
		}
	}
	require.Len(t, readDevices(c), 3)
	calls := f.calls()
	require.Len(t, calls, 7)
	expectedWindow := url.Values{"t0": {now.Add(-5 * time.Minute).Format(time.RFC3339)}, "t1": {now.Format(time.RFC3339)}}
	switchQuery := expectedWindow.Clone()
	switchQuery.Set("perPage", "50")
	assert.Equal(t, trafficHistoryPath, calls[4].path)
	assert.Equal(t, switchQuery, calls[4].query)
	assert.Equal(t, trafficUplinkPath, calls[5].path)
	assert.Equal(t, expectedWindow, calls[5].query)
	assert.Equal(t, trafficPacketPath, calls[6].path)
	assert.Equal(t, url.Values{"timespan": {"300"}}, calls[6].query)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestTrafficIndependentOptionsAndCheapCheck(t *testing.T) {
	for _, family := range []string{"default", "traffic", "packets", "uplinks"} {
		t.Run(family, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			defaults := New()
			assert.False(t, defaults.CollectSwitchPortTraffic)
			assert.False(t, defaults.CollectSwitchPortPackets)
			assert.False(t, defaults.CollectApplianceUplinkUsage)
			c.CollectSwitchPortTraffic = family == "traffic"
			c.CollectSwitchPortPackets = family == "packets"
			c.CollectApplianceUplinkUsage = family == "uplinks"
			require.NoError(t, c.Check(t.Context()))
			require.Len(t, f.calls(), 1)
			assert.Equal(t, "3", f.calls()[0].pageSize)
			require.NoError(t, collectCycle(t.Context(), c))
			want := 5
			if family != "default" {
				want++
			}
			require.Len(t, f.calls(), want)
			assert.Len(t, trafficPoints(c, metricPortTrafficSent), boolCount(family == "traffic"))
			assert.Len(t, trafficPoints(c, metricPortPacketsSent), boolCount(family == "packets"))
			assert.Len(t, trafficPoints(c, metricUplinkTrafficSent), boolCount(family == "uplinks"))
		})
	}
}
func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestTrafficMissingZeroAndPacketCategories(t *testing.T) {
	f, c, now := newTrafficFixture(t)
	f.set(trafficHistoryPath, fixtureResponse{body: trafficHistoryBody("S1", "N1", "1/0", trafficInterval(now.Add(-time.Minute), 300, `{"upstream":0,"downstream":null}`))})
	f.set(trafficPacketPath, fixtureResponse{body: `[{"portId":"1/0","packets":[{"desc":"Total","ratePerSec":{"sent":0,"recv":null}},{"desc":"Broadcast","ratePerSec":{"sent":123,"recv":123}}]},{"portId":"2","packets":[{"desc":"Unicast","ratePerSec":{"sent":999,"recv":999}}]}]`})
	f.set(trafficUplinkPath, fixtureResponse{body: `[{"networkId":"N1","byUplink":[{"interface":"wan1","sent":0},{"interface":"wan2","received":null}]}]`})
	require.NoError(t, collectCycle(t.Context(), c))
	for _, metric := range []string{metricPortTrafficSent, metricPortPacketsSent, metricUplinkTrafficSent} {
		points := trafficPoints(c, metric)
		require.Len(t, points, 1)
		assert.Zero(t, points[0].Value)
	}
	for _, metric := range []string{metricPortTrafficReceived, metricPortPacketsReceived, metricUplinkTrafficReceived} {
		assert.Empty(t, trafficPoints(c, metric))
	}
}

func TestTrafficPortObservationEligibility(t *testing.T) {
	for _, kind := range []string{"stale", "future", "wrong duration", "network mismatch", "unknown serial", "missing port", "duplicate", "conflicting duplicate", "latest missing", "latest direction missing"} {
		t.Run(kind, func(t *testing.T) {
			f, c, now := newTrafficFixture(t)
			sample := trafficInterval(now.Add(-time.Minute), 300, `{"upstream":10.5,"downstream":20.25}`)
			serial, network, port := "S1", "N1", "1/0"
			switch kind {
			case "stale":
				sample = trafficInterval(now.Add(-5*time.Minute), 300, `{"upstream":99,"downstream":99}`)
			case "future":
				sample = trafficInterval(now.Add(time.Second), 300, `{"upstream":99,"downstream":99}`)
			case "wrong duration":
				sample = trafficInterval(now.Add(-time.Minute), 60, `{"upstream":99,"downstream":99}`)
			case "network mismatch":
				network = "OTHER"
			case "unknown serial":
				serial = "OLD"
			case "missing port":
				port = ""
			case "duplicate":
				sample += "," + sample
			case "conflicting duplicate":
				sample += "," + trafficInterval(now.Add(-time.Minute), 300, `{"upstream":99,"downstream":20.25}`)
			case "latest missing":
				sample += "," + trafficInterval(now, 300, `{}`)
			case "latest direction missing":
				sample += "," + trafficInterval(now, 300, `{"downstream":0}`)
			}
			f.set(trafficHistoryPath, fixtureResponse{body: trafficHistoryBody(serial, network, port, sample)})
			require.NoError(t, collectCycle(t.Context(), c))
			if kind == "duplicate" {
				require.Len(t, trafficPoints(c, metricPortTrafficSent), 1)
				assert.Equal(t, 10.5, trafficPoints(c, metricPortTrafficSent)[0].Value)
			} else {
				assert.Empty(t, trafficPoints(c, metricPortTrafficSent))
			}
			if kind == "latest direction missing" {
				p := trafficPoints(c, metricPortTrafficReceived)
				require.Len(t, p, 1)
				assert.Zero(t, p[0].Value)
				assert.Zero(t, trafficPoints(c, metricPortTrafficIntervalAge)[0].Value)
			}
			assert.Len(t, trafficPoints(c, metricPortPacketsSent), 1)
			assert.Len(t, trafficPoints(c, metricUplinkTrafficSent), 1)
		})
	}
}

func TestTrafficDuplicatesAndNetworkAttribution(t *testing.T) {
	for _, kind := range []string{"identical", "conflicting packets", "conflicting HA", "unknown network", "negative"} {
		t.Run(kind, func(t *testing.T) {
			f, c, _ := newTrafficFixture(t)
			packets := `{"desc":"Total","ratePerSec":{"sent":2.5,"recv":3.25}}`
			uplink := `{"interface":"wan1","serial":"A","sent":300000,"received":600000}`
			secondPackets := packets
			secondUplink := strings.ReplaceAll(uplink, `"A"`, `"B"`)
			network := "N1"
			if kind == "conflicting packets" {
				secondPackets = `{"desc":"Total","ratePerSec":{"sent":99,"recv":3.25}}`
			}
			if kind == "conflicting HA" {
				secondUplink = `{"interface":"wan1","serial":"B","sent":400000,"received":600000}`
			}
			if kind == "unknown network" {
				network = "UNKNOWN"
			}
			if kind == "negative" {
				packets = `{"desc":"Total","ratePerSec":{"sent":-1,"recv":-1}}`
				secondPackets = packets
				uplink = `{"interface":"wan1","sent":-1,"received":-1}`
				secondUplink = uplink
			}
			f.set(trafficPacketPath, fixtureResponse{body: fmt.Sprintf(`[{"portId":"1/0","packets":[%s,%s]}]`, packets, secondPackets)})
			f.set(trafficUplinkPath, fixtureResponse{body: fmt.Sprintf(`[{"networkId":%q,"byUplink":[%s,%s]}]`, network, uplink, secondUplink)})
			require.NoError(t, collectCycle(t.Context(), c))
			assert.Len(t, trafficPoints(c, metricPortPacketsSent), boolCount(kind != "conflicting packets" && kind != "negative"))
			assert.Len(t, trafficPoints(c, metricUplinkTrafficSent), boolCount(kind != "conflicting HA" && kind != "unknown network" && kind != "negative"))
		})
	}
}

func TestTrafficOptionalFailureRecovery(t *testing.T) {
	for _, code := range []int{403, 404, 500} {
		for _, family := range []string{trafficHistoryPath, trafficUplinkPath, trafficPacketPath} {
			t.Run(fmt.Sprintf("%d/%s", code, family), func(t *testing.T) {
				f, c, _ := newTrafficFixture(t)
				f.mu.Lock()
				valid := f.responses[family+":"]
				f.mu.Unlock()
				require.NoError(t, collectCycle(t.Context(), c))
				f.set(family, fixtureResponse{status: code, body: `{"errors":["synthetic denied"]}`})
				require.NoError(t, collectCycle(t.Context(), c))
				assert.Len(t, readDevices(c), 3)
				assert.Equal(t, float64(3), readOrganization(c)[metricOrganizationAssignedDevices])
				for path, metric := range map[string]string{trafficHistoryPath: metricPortTrafficSent, trafficUplinkPath: metricUplinkTrafficSent, trafficPacketPath: metricPortPacketsSent} {
					assert.Len(t, trafficPoints(c, metric), boolCount(path != family))
				}
				f.set(family, valid)
				require.NoError(t, collectCycle(t.Context(), c))
				for _, metric := range []string{metricPortTrafficSent, metricPortPacketsSent, metricUplinkTrafficSent} {
					assert.Len(t, trafficPoints(c, metric), 1)
				}
			})
		}
	}
}

func TestTraffic429StopsAndKeepsOriginalCooldown(t *testing.T) {
	for _, family := range []string{trafficHistoryPath, trafficUplinkPath, trafficPacketPath} {
		for _, header := range []string{"", "600"} {
			t.Run(family+"/"+header, func(t *testing.T) {
				f, c, now := newTrafficFixture(t)
				f.set(family, fixtureResponse{status: http.StatusTooManyRequests, retryAfter: header})
				require.NoError(t, collectCycle(t.Context(), c))
				want := map[string]int{trafficHistoryPath: 5, trafficUplinkPath: 6, trafficPacketPath: 7}[family]
				require.Len(t, f.calls(), want, "429 must stop later optional requests")
				assert.Len(t, readDevices(c), 3)
				if header != "" {
					deadline := now.Add(600 * time.Second)
					now = now.Add(300 * time.Second)
					c.now = func() time.Time { return now }
					err := c.Check(t.Context())
					require.Error(t, err)
					assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
					err = collectCycle(t.Context(), c)
					require.Error(t, err)
					assert.Equal(t, deadline, collectorapi.RetryNotBefore(err))
					assert.Len(t, f.calls(), want)
				} else {
					require.NoError(t, c.Check(t.Context()), "missing Retry-After must not invent a cooldown")
					assert.Len(t, f.calls(), want+1)
				}
			})
		}
	}
}

func TestTrafficRenameKeepsIdentity(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))
	before := trafficPoints(c, metricPortTrafficSent)[0]
	f.set("/organizations/org/devices", fixtureResponse{body: `[{"serial":"S1","networkId":"N1","name":"Renamed","model":"MS120","productType":"switch"},{"serial":"A","networkId":"N1","productType":"appliance"},{"serial":"B","networkId":"N1","productType":"appliance"}]`})
	f.set("/organizations/org/networks", fixtureResponse{body: `[{"id":"N1","organizationId":"org","name":"Renamed office"}]`})
	require.NoError(t, collectCycle(t.Context(), c))
	points := trafficPoints(c, metricPortTrafficSent)
	require.Len(t, points, 1)
	assert.Equal(t, before.Scope.GUID, points[0].Scope.GUID)
	assert.Equal(t, "1/0", points[0].Labels["port_id"])
	assert.Equal(t, "Renamed", points[0].Labels["device_name"])
	uplinks := trafficPoints(c, metricUplinkTrafficSent)
	require.Len(t, uplinks, 1)
	assert.Equal(t, "N1", uplinks[0].Labels["network_id"])
	assert.Equal(t, "Renamed office", uplinks[0].Labels["network_name"])
}

func TestTrafficCancellationAbortsRealWrites(t *testing.T) {
	_, c, _ := newTrafficFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wrote := false
	c.metrics.meter = trafficWriteMeter{SnapshotMeter: c.metrics.meter, after: func(name string) {
		if name == metricPortTrafficSent {
			wrote = true
			cancel()
		}
	}}
	require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
	require.True(t, wrote, "cancel after a real traffic observation")
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
	assert.Empty(t, trafficPoints(c, metricPortTrafficSent))
}

func TestTrafficDeadlineAbortsCycle(t *testing.T) {
	_, c, _ := newTrafficFixture(t)
	// Four paced core reads finish in about300ms; deadline expires while the
	// optional requests are still being read, so core metrics cannot commit.
	c.Timeout = confopt.Duration(450 * time.Millisecond)
	require.ErrorIs(t, collectCycle(t.Context(), c), context.DeadlineExceeded)
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
}

func TestTrafficParentCancellationMakesNoRequests(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
	assert.Empty(t, f.calls())
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
}

func TestTrafficPacketsOnlyAssignedSwitchesInSerialOrder(t *testing.T) {
	f, c, _ := newTrafficFixture(t)
	f.set("/organizations/org/devices", fixtureResponse{body: `[{"serial":"S2","productType":"switch"},{"serial":"S1","productType":"switch"},{"serial":"AP","productType":"wireless"},{"serial":"MX","productType":"appliance"}]`})
	f.set("/devices/S2/switch/ports/statuses/packets", fixtureResponse{body: `[]`})
	c.CollectSwitchPortTraffic = false
	c.CollectApplianceUplinkUsage = false
	require.NoError(t, collectCycle(t.Context(), c))
	calls := f.calls()
	require.Len(t, calls, 6)
	assert.Equal(t, trafficPacketPath, calls[4].path)
	assert.Equal(t, "/devices/S2/switch/ports/statuses/packets", calls[5].path)
}

func TestTrafficManagedHostPlacementAndFloatEmission(t *testing.T) {
	_, c, _ := newTrafficFixture(t)
	out := &lockedBuffer{}
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{PluginName: "go.d", Name: "traffic", ModuleName: "meraki", FullName: "meraki_traffic", Module: c, Out: out, UpdateEvery: 1, Publication: hostoutput.New()})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("traffic managed job did not stop")
		}
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("traffic managed startup blocked")
	}
	require.NoError(t, run.StartupErr())
	require.Eventually(t, func() bool {
		if strings.Contains(out.String(), "meraki.appliance_uplink_traffic") {
			return true
		}
		job.Tick(0)
		return false
	}, 5*time.Second, 120*time.Millisecond)
	wire := out.String()
	guid := readDevices(c)["S1"].Scope.GUID
	require.NotEmpty(t, guid)
	host := ""
	scopes := map[string]string{}
	contexts := []string{"meraki.switch_port_traffic", "meraki.switch_port_traffic_interval_age", "meraki.switch_port_packets", "meraki.appliance_uplink_traffic"}
	hostLine := regexp.MustCompile(`^HOST '([^']*)'`)
	for _, line := range strings.Split(wire, "\n") {
		if match := hostLine.FindStringSubmatch(line); match != nil {
			host = match[1]
		}
		if strings.HasPrefix(line, "CHART ") {
			for _, ctx := range contexts {
				if strings.Contains(line, "'"+ctx+"'") {
					scopes[ctx] = host
				}
			}
		}
	}
	for _, ctx := range contexts {
		require.Contains(t, scopes, ctx)
		if ctx == "meraki.appliance_uplink_traffic" {
			assert.Empty(t, scopes[ctx])
		} else {
			assert.Equal(t, guid, scopes[ctx])
		}
	}
	for _, value := range []string{"10.5", "20.25", "2.5", "3.25"} {
		assert.Regexp(t, regexp.MustCompile(`(?m)^SET '[^']+' = `+regexp.QuoteMeta(value)+`$`), wire)
	}
}
