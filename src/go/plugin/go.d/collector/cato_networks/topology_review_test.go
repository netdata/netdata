// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	catosdk "github.com/catonetworks/cato-go-sdk"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTopologyEmptySnapshotRecovery(t *testing.T) {
	for _, initiallyEmpty := range []bool{true, false} {
		t.Run(map[bool]string{true: "initial empty", false: "successful then empty"}[initiallyEmpty], func(t *testing.T) {
			c := New()
			c.AccountID = "account"
			c.APIKey = "secret"
			c.now = fixedCatoTestNow
			fake := newFixtureAPIClient()
			c.client = fake
			initCollector(t, c)
			original := fake.snapshot
			if !initiallyEmpty {
				collectReviewCycle(t, c)
			}
			fake.snapshot = &accountSnapshot{}
			collectReviewCycle(t, c)
			data, ok := c.topology.CurrentTopology()
			require.True(t, ok)
			actors := topologyTableRows(t, data.Actors, data.Dictionaries)
			labels := topologyTableRows(t, data.Tables.Actor["actor_labels"].Table, data.Dictionaries)
			count := 0
			for index, actor := range actors {
				if actor["type"] == catofunc.ActorTypeSite {
					count++
					require.Equal(t, "Unavailable", topologyLabelValue(t, labels, index, "snapshot_collection"))
				}
			}
			require.Equal(t, len(c.discovery.siteIDs), count)
			found := false
			for _, n := range data.Notifications {
				if n.Code == "cato_snapshot_coverage" {
					found = true
				}
			}
			require.True(t, found)
			fake.snapshot = original
			collectReviewCycle(t, c)
			recovered, ok := c.topology.CurrentTopology()
			require.True(t, ok)
			require.NotSame(t, data, recovered)
			for _, n := range recovered.Notifications {
				require.NotEqual(t, "cato_snapshot_coverage", n.Code)
			}
		})
	}
}

func topologyLabelValue(t *testing.T, rows []map[string]any, actor int, key string) any {
	t.Helper()
	var values []any
	for _, row := range rows {
		if row["actor"] == actor && row["key"] == key {
			values = append(values, row["value"])
		}
	}
	require.Len(t, values, 1, "actor=%d key=%s", actor, key)
	return values[0]
}

func TestTopologyDeviceNamesAndLabels(t *testing.T) {
	for _, tc := range []struct {
		name string
		dev  deviceState
		want string
	}{
		{"configured", deviceState{
			ID:           "1",
			Name:         " Edge appliance ",
			SocketSerial: "12345",
		}, "Edge appliance"},
		{"numeric configured", deviceState{
			ID:           "1",
			Name:         "42",
			SocketSerial: "12345",
		}, "42"},
		{"serial fallback", deviceState{
			ID:           "1",
			SocketSerial: "12345",
			HaRole:       "BACKUP",
		}, "serial 12345 · backup · Office"},
		{"identity as name", deviceState{
			ID:           "1",
			Name:         "12345",
			SocketSerial: "12345",
			HaRole:       "MASTER",
		}, "serial 12345 · master · Office"},
		{"id fallback", deviceState{
			ID:     "1",
			HaRole: "NONE",
		}, "ID 1 · device · Office"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &siteState{
				ID:      "site",
				Name:    " Office ",
				Devices: []deviceState{tc.dev},
			}
			metricName := deviceDisplayName(tc.dev)
			data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
			actors := topologyTableRows(t, data.Actors, data.Dictionaries)
			require.Equal(t, tc.want, actors[1]["display_name"])
			labels := topologyTableRows(t, data.Tables.Actor["actor_labels"].Table, data.Dictionaries)
			require.Equal(t, tc.want, topologyLabelValue(t, labels, 1, "name"))
			require.Equal(t, "Office", topologyLabelValue(t, labels, 1, "site_name"))
			if tc.dev.Name != "" {
				require.Equal(t, normalizeName(tc.dev.Name), topologyLabelValue(t, labels, 1, "source_name"))
			}
			require.Equal(t, metricName, deviceDisplayName(site.Devices[0]), "topology must not change metric label names")
		})
	}
}

func TestTopologyDuplicateDeviceIdentity(t *testing.T) {
	site := &siteState{
		ID:      "site",
		Name:    "Office",
		PopName: "PoP",
		Devices: []deviceState{
			{ID: "same", Name: "Zulu", SocketSerial: "222"}, {ID: "same", Name: "Alpha", SocketSerial: "111"},
		},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	require.Equal(t, 3, data.Actors.Rows)
	require.Equal(t, 2, data.Links.Rows)
	labels := topologyTableRows(t, data.Tables.Actor["actor_labels"].Table, data.Dictionaries)
	require.Equal(t, "Alpha", topologyLabelValue(t, labels, 1, "name"))
	require.Equal(t, "111", topologyLabelValue(t, labels, 1, "socket_serial"))
	require.Len(t, site.Devices, 2, "metric observations stay unchanged")
}

func TestTopologyMetricsCollectionStatus(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "observed", true: "partial"}[partial], func(t *testing.T) {
			c := New()
			fake := newFixtureAPIClient()
			c.client = fake
			raw := &catosdk.AccountMetrics_AccountMetrics_Sites{
				ID: new("1001"),
			}
			if partial {
				raw.Interfaces = []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces{{Name: new("WAN"), Timeseries: []*catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Timeseries{{Label: "unsupported", Data: [][]float64{{1, 2}}}}}}
			}
			fake.metrics = &catosdk.AccountMetrics{
				AccountMetrics: &catosdk.AccountMetrics_AccountMetrics{
					Sites: []*catosdk.AccountMetrics_AccountMetrics_Sites{raw},
				},
			}
			sites := map[string]*siteState{"1001": {ID: "1001", Interfaces: map[string]*interfaceState{}}}
			require.NoError(t, c.collectMetrics(t.Context(), sites))
			want := "observed"
			if partial {
				want = "partial"
			}
			require.Equal(t, want, sites["1001"].MetricsCollection)
			data := mustBuildTopology(t, "account", sites, []string{"1001"}, fixedCatoTestNow())
			labels := topologyTableRows(t, data.Tables.Actor["actor_labels"].Table, data.Dictionaries)
			require.Equal(t, collectionStatusLabel(want), topologyLabelValue(t, labels, 0, "metrics_collection"))
		})
	}
}

func collectReviewCycle(t *testing.T, c *Collector) {
	t.Helper()
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	if err := c.Collect(t.Context()); err != nil {
		cc.AbortCycle()
		require.NoError(t, err)
		return
	}
	require.NoError(t, cc.CommitCycleSuccess())
}

func TestTopologyDeviceIdentityPrecedesLongSiteContext(t *testing.T) {
	site := &siteState{ID: "site", Name: "Europe Greece Athens Headquarters Production Operations Centre"}
	first := topologyDeviceName(site, deviceState{ID: "1", SocketSerial: "123456789"})
	second := topologyDeviceName(site, deviceState{ID: "2", SocketSerial: "987654321"})
	require.Contains(t, first, site.Name)
	require.Contains(t, second, site.Name)
	require.NotEqual(t, []rune(first)[:79], []rune(second)[:79], "graph caption shortening must retain device identity")
	require.Contains(t, string([]rune(first)[:79]), "serial 123456789")
	require.Contains(t, string([]rune(second)[:79]), "serial 987654321")
}
