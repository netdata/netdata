// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
)

func TestTopologyFunction(t *testing.T) {
	tests := map[string]struct {
		setup func(*testing.T, *Collector)
		check func(*testing.T, *Collector)
	}{
		"returns current topology": {
			setup: func(t *testing.T, c *Collector) {
				c.client = newFixtureAPIClient()
				c.now = fixedCatoTestNow
				initCollector(t, c)
				collectOnce(t, c)
			},
			check: func(t *testing.T, c *Collector) {
				resp := c.funcRouter.Handle(context.Background(), catofunc.TopologyMethodID, nil)
				require.Equal(t, 200, resp.Status)
				require.Equal(t, "topology", resp.ResponseType)
				data, ok := resp.Data.(*topology.Data)
				require.True(t, ok)
				require.Equal(t, topologySource, data.Source)
				require.NotEmpty(t, data.Actors)
				require.NotEmpty(t, data.Links)
			},
		},
		"requires job selection": {
			check: func(t *testing.T, _ *Collector) {
				cfg := catofunc.Methods(defaultUpdateEvery)[0]
				require.False(t, cfg.AgentWide)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.AccountID = "12345"
			c.APIKey = "secret"
			if tc.setup != nil {
				tc.setup(t, c)
			}
			tc.check(t, c)
		})
	}
}

func TestBuildTopology(t *testing.T) {
	tests := map[string]struct {
		build func() *topology.Data
		check func(*testing.T, *topology.Data)
	}{
		"omits unavailable tunnel metrics": {
			build: func() *topology.Data {
				return buildTopology("12345", map[string]*siteState{
					"1001": {
						ID:                 "1001",
						Name:               "Paris Office",
						ConnectivityStatus: "connected",
						PopName:            "POP-Paris",
						Interfaces:         make(map[string]*interfaceState),
					},
				}, []string{"1001"}, fixedCatoTestNow())
			},
			check: func(t *testing.T, data *topology.Data) {
				require.Len(t, data.Links, 1)
				require.Equal(t, catofunc.LinkTypeTunnel, data.Links[0].LinkType)
				require.Empty(t, data.Links[0].Metrics)
			},
		},
		"omits empty BGP peer IP match": {
			build: func() *topology.Data {
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
				return buildTopology("12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
			},
			check: func(t *testing.T, data *topology.Data) {
				var peerActor *topology.Actor
				for i := range data.Actors {
					if data.Actors[i].ActorType == catofunc.ActorTypeBGPPeer {
						peerActor = &data.Actors[i]
						break
					}
				}
				require.NotNil(t, peerActor)
				require.Empty(t, peerActor.Match.IPAddresses)

				var bgpLink *topology.Link
				for i := range data.Links {
					if data.Links[i].LinkType == catofunc.LinkTypeBGP {
						bgpLink = &data.Links[i]
						break
					}
				}
				require.NotNil(t, bgpLink)
				require.Empty(t, bgpLink.Dst.Match.IPAddresses)
			},
		},
		"deduplicates BGP peers": {
			build: func() *topology.Data {
				site := &siteState{
					ID:   "1001",
					Name: "Paris Office",
					BGPPeers: []bgpPeerState{
						{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
						{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
					},
				}
				return buildTopology("12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
			},
			check: func(t *testing.T, data *topology.Data) {
				actorIDs := make(map[string]bool)
				var peerActors int
				for _, actor := range data.Actors {
					require.False(t, actorIDs[actor.ActorID], "duplicate actor_id %q", actor.ActorID)
					actorIDs[actor.ActorID] = true
					if actor.ActorType == catofunc.ActorTypeBGPPeer {
						peerActors++
					}
				}
				var bgpLinks int
				for _, link := range data.Links {
					if link.LinkType == catofunc.LinkTypeBGP {
						bgpLinks++
					}
				}
				require.Equal(t, 1, peerActors)
				require.Equal(t, 1, bgpLinks)
			},
		},
		"site topology tables are deterministic": {
			build: func() *topology.Data {
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
				return &topology.Data{Actors: []topology.Actor{{Tables: siteTopologyTables(site)}}}
			},
			check: func(t *testing.T, data *topology.Data) {
				tables := data.Actors[0].Tables
				require.Equal(t, "WAN 1", tables["interfaces"][0]["name"])
				require.Equal(t, "WAN 2", tables["interfaces"][1]["name"])
				require.Equal(t, "Socket 1", tables["devices"][0]["name"])
				require.Equal(t, "Socket 2", tables["devices"][1]["name"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.check(t, tc.build())
		})
	}
}
