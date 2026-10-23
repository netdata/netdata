// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
)

func TestTopologyObservedConnections(t *testing.T) {
	for name, connected := range map[string]*bool{"connected": new(true), "disconnected": new(false), "unknown": nil} {
		t.Run(name, func(t *testing.T) {
			site := &siteState{
				ID:      "site",
				Name:    "Office",
				PopName: "Old PoP",
				Devices: []deviceState{
					{ID: "socket", Name: "Socket", Connected: new(true), LastPopName: "Old PoP"},
					{ID: "backup", Name: "Backup", Connected: new(false), LastPopName: "Fallback PoP"},
				},
				Interfaces: map[string]*interfaceState{
					"wan1": {ID: "wan1", Name: "WAN 1", DeviceID: "socket", PopName: "New PoP", Connected: connected, LinkUp: new(true)},
					"wan2": {ID: "wan2", Name: "WAN 2", DeviceID: "socket", PopName: "Other PoP", Connected: new(true)},
				},
			}
			data := mustBuildTopology(t, "account", map[string]*siteState{site.ID: site}, []string{site.ID}, fixedCatoTestNow())
			validateCatoTopologyV1Data(t, data)
			actors := topologyTableRows(t, data.Actors, data.Dictionaries)
			links := topologyTableRows(t, data.Links, data.Dictionaries)
			got := map[string]string{}
			for _, l := range links {
				src := actors[l["src_actor"].(int)]["display_name"].(string)
				dst := actors[l["dst_actor"].(int)]["display_name"].(string)
				got[src+" -> "+dst] = l["type"].(string) + ":" + l["state"].(string)
			}
			require.Equal(t, map[string]string{
				"Office -> Socket": "ownership:known", "Office -> Backup": "ownership:known",
				"Socket -> New PoP": "cato_tunnel:" + name, "Socket -> Other PoP": "cato_tunnel:connected",
				"Backup -> Fallback PoP": "cato_tunnel:disconnected",
			}, got)
			interfaces := topologyTableRows(t, data.Tables.Actor[catofunc.ActorTableInterfaces].Table, data.Dictionaries)
			require.Equal(t, optionalBool(connected), interfaces[0]["tunnel_connected"])
			require.Equal(t, true, interfaces[0]["link_up"])
			link := links[interfaces[0]["link"].(int)]
			require.Equal(t, "wan1", link["interface_id"])
			require.Nil(t, link["bytes_upstream_max"])
		})
	}
}

func TestTopologyActorLabels(t *testing.T) {
	site := &siteState{
		ID:          "site",
		Name:        "Office",
		Description: "Main office",
		CountryCode: "FR",
		CountryName: "France",
		HostCount:   new(int64(0)),
		Devices: []deviceState{
			{ID: "socket", Name: "Socket", SocketSerial: "SERIAL", SocketVersion: "1.0", HaRole: "primary", Connected: new(false), LastPopName: "PoP"},
		},
		BGPPeers: []bgpPeerState{{RemoteIP: "192.0.2.10", RemoteASN: "64512"}},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{site.ID: site}, []string{site.ID}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	labels, ok := data.Tables.Actor["actor_labels"]
	require.True(t, ok, "known actor metadata must be exposed as labels")
	actors := topologyTableRows(t, data.Actors, data.Dictionaries)
	rows := topologyTableRows(t, labels.Table, data.Dictionaries)
	byActor := map[int]map[string]string{}
	for _, r := range rows {
		i := r["actor"].(int)
		if byActor[i] == nil {
			byActor[i] = map[string]string{}
		}
		require.NotEmpty(t, r["value"])
		byActor[i][r["key"].(string)] = r["value"].(string)
	}
	for _, a := range actors {
		require.NotEmpty(t, byActor[a["_row"].(int)])
		modal := data.Types.ActorTypes[a["type"].(string)].Presentation.Modal
		require.NotNil(t, modal)
		require.NotNil(t, modal.Labels)
		require.Equal(t, "actor_labels", modal.Labels.Table)
		require.NotEmpty(t, modal.Labels.Identification.Fields)
	}
	device := requireTopologyRow(t, actors, "type", catofunc.ActorTypeDevice)
	dl := byActor[device["_row"].(int)]
	require.Equal(t, "Office", dl["site_name"])
	require.Equal(t, "France", dl["country_name"])
	require.Equal(t, "false", dl["connected"])
	require.Equal(t, "SERIAL", dl["socket_serial"])
	require.Equal(t, "0", byActor[0]["host_count"])
	require.NotContains(t, dl, "internal_ip")
}

// Comparable to a few hundred sites with redundant sockets and WAN circuits.
func catoTopologyScaleFixture() (map[string]*siteState, []string) {
	sites := make(map[string]*siteState, 300)
	order := make([]string, 0, 300)
	for i := 0; i < 300; i++ {
		id := fmt.Sprint(i)
		pop := fmt.Sprintf("PoP-%d", i%48)
		site := &siteState{
			ID:                 id,
			Name:               "Office " + id,
			Description:        "Regional office",
			CountryCode:        "FR",
			CountryName:        "France",
			ConnectivityStatus: "connected",
			SiteType:           "branch",
			ConnectionType:     "socket",
			PopName:            pop,
			Interfaces:         map[string]*interfaceState{},
		}
		for j := 0; j < 2; j++ {
			dev := fmt.Sprintf("socket-%d", j)
			site.Devices = append(site.Devices, deviceState{
				ID:            dev,
				Name:          dev,
				Connected:     new(true),
				SocketSerial:  fmt.Sprintf("serial-%s-%d", id, j),
				SocketVersion: "1.0",
				LastPopName:   pop,
			})
			for k := 0; k < 2; k++ {
				key := fmt.Sprintf("%s-wan-%d", dev, k)
				site.Interfaces[key] = &interfaceState{
					ID:        key,
					Name:      key,
					DeviceID:  dev,
					PopName:   pop,
					Connected: new(true),
				}
			}
		}
		sites[id] = site
		order = append(order, id)
	}
	return sites, order
}

func BenchmarkCatoTopologyScale(b *testing.B) {
	sites, order := catoTopologyScaleFixture()
	now := fixedCatoTestNow()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := buildTopology("account", sites, order, now); err != nil {
			b.Fatal(err)
		}
	}
}

func TestTopologyScale(t *testing.T) {
	sites, order := catoTopologyScaleFixture()
	data := mustBuildTopology(t, "account", sites, order, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	payload, err := json.Marshal(data)
	require.NoError(t, err)
	t.Logf("actors=%d links=%d payload_bytes=%d", data.Actors.Rows, data.Links.Rows, len(payload))
}

func TestTopologyInterfaceOwnersAndFallback(t *testing.T) {
	tests := map[string]struct {
		devices    []deviceState
		interfaces map[string]*interfaceState
		want       map[string]string
	}{
		"site fallback":                      {want: map[string]string{"Office -> Summary": "unknown"}},
		"unresolved owner":                   {interfaces: map[string]*interfaceState{"wan": {ID: "wan", Name: "WAN", DeviceID: "missing", PopName: "Observed", Connected: new(false)}}, want: map[string]string{"Office -> Observed": "disconnected"}},
		"unknown owner alongside device":     {devices: []deviceState{{ID: "socket", Name: "Socket"}}, interfaces: map[string]*interfaceState{"wan": {ID: "wan", DeviceID: "missing", PopName: "Observed"}}, want: map[string]string{"Office -> Observed": "unknown", "Socket -> Summary": "unknown"}},
		"nil and destinationless interfaces": {devices: []deviceState{{ID: "socket", Name: "Socket"}}, interfaces: map[string]*interfaceState{"nil": nil, "wan": {ID: "wan", DeviceID: "socket", Connected: new(true)}}, want: map[string]string{"Socket -> Summary": "unknown"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			site := &siteState{
				ID:                 "site",
				Name:               "Office",
				PopName:            "Summary",
				ConnectivityStatus: "unknown",
				Devices:            tc.devices,
				Interfaces:         tc.interfaces,
			}
			data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
			validateCatoTopologyV1Data(t, data)
			actors := topologyTableRows(t, data.Actors, data.Dictionaries)
			links := topologyTableRows(t, data.Links, data.Dictionaries)
			got := map[string]string{}
			for _, l := range links {
				if l["type"] != catofunc.LinkTypeTunnel {
					continue
				}
				src := actors[l["src_actor"].(int)]["display_name"].(string)
				dst := actors[l["dst_actor"].(int)]["display_name"].(string)
				got[src+" -> "+dst] = l["state"].(string)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTopologyParallelInterfacesPreserveMeasurements(t *testing.T) {
	site := &siteState{
		ID:      "site",
		Name:    "Office",
		PopName: "Summary",
		Metrics: trafficMetrics{
			present: trafficMetricRTTMS,
			RTTMS:   999,
		},
		Devices: []deviceState{{ID: "socket", Name: "Socket"}},
		Interfaces: map[string]*interfaceState{
			"a": {ID: "a", Name: "WAN A", DeviceID: "socket", PopName: "PoP", Connected: new(true), Metrics: trafficMetrics{
				present: trafficMetricRTTMS,
				RTTMS:   0,
			}},
			"b": {ID: "b", Name: "WAN B", DeviceID: "socket", PopName: "PoP", Connected: new(false)},
		},
	}
	data := mustBuildTopology(t, "account", map[string]*siteState{"site": site}, []string{"site"}, fixedCatoTestNow())
	validateCatoTopologyV1Data(t, data)
	links := topologyTableRows(t, data.Links, data.Dictionaries)
	a := requireTopologyRow(t, links, "interface_id", "a")
	b := requireTopologyRow(t, links, "interface_id", "b")
	require.Equal(t, a["src_actor"], b["src_actor"])
	require.Equal(t, a["dst_actor"], b["dst_actor"])
	require.Equal(t, "connected", a["state"])
	require.Equal(t, "disconnected", b["state"])
	require.Equal(t, float64(0), a["rtt_ms"])
	require.Nil(t, b["rtt_ms"])
	require.Len(t, links, 3)
}
