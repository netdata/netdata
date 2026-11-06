// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyenrich"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
)

func TestTopologyBGPSemanticIdentifiersFromAcquisitionAndReplay(t *testing.T) {
	for _, field := range []string{"process_id", "routing_instance"} {
		t.Run(field, func(t *testing.T) {
			for name, values := range map[string][]string{
				"space boundaries":            {"edge", " edge "},
				"other whitespace boundaries": {"edge", "\tedge\r\n", "\r\nedge\t"},
				"punctuation and controls":    {"edge'a", "edgea", "edge\x00a", "edge\\", "edge\r\na"},
			} {
				t.Run(name, func(t *testing.T) {
					const root = "1.3.6.1.4.1.99999.2991"
					definition := &ddprofiledefinition.ProfileDefinition{}
					require.NoError(t, yaml.Unmarshal([]byte(`bgp:
- id: semantic-identity
  kind: peer
  table: {OID: `+root+`, name: peers}
  identity:
    process_id: {value: "0"}
    routing_instance: {value: vrf}
    neighbor: {value: 192.0.2.1}
  state:
    value: "6"
    mapping: {1: idle, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: established}
  traffic:
    messages:
      received:
        symbol: {OID: `+root+`.1, name: received}
`), definition))
					column := ddprofiledefinition.BGPValueConfig{
						Symbol: ddprofiledefinition.SymbolConfig{
							OID:  root + ".2",
							Name: "identity",
						},
					}
					if field == "process_id" {
						definition.BGP[0].Identity.ProcessID = column
					} else {
						definition.BGP[0].Identity.RoutingInstance = column
					}
					definition.Normalize()
					require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(definition))
					profile := &ddsnmp.Profile{
						SourceFile: "semantic-identity.yaml",
						Definition: definition,
					}
					require.NoError(t, ddsnmp.CompileTransforms(profile))
					ddsnmp.FinalizeProfiles([]*ddsnmp.Profile{profile})
					handler := snmpmock.NewMockHandler(gomock.NewController(t))
					handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
					var pdus []gosnmp.SnmpPDU
					for i, value := range values {
						pdus = append(
							pdus,
							gosnmp.SnmpPDU{
								Name:  fmt.Sprintf("%s.1.%d", root, i+1),
								Type:  gosnmp.Counter32,
								Value: uint(i + 100),
							},
							gosnmp.SnmpPDU{
								Name:  fmt.Sprintf("%s.2.%d", root, i+1),
								Type:  gosnmp.OctetString,
								Value: []byte(value),
							},
						)
					}
					handler.EXPECT().BulkWalkAll(root).Return(pdus, nil)
					input := topologydiag.DeviceInput{
						Hostname: "192.0.2.2",
					}
					collectedAt := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
					recorder := newTopologyAcquisitionRecorder(
						topologydiag.AcquisitionAttemptID{
							RegistrationID: 1,
							Ordinal:        1,
						},
						input,
						topologydiag.TargetResolutionEvidence{
							Outcome: topologydiag.TargetResolutionEmpty,
						},
					)
					collector := ddsnmpcollector.New(ddsnmpcollector.Config{
						SnmpClient:          handler,
						Profiles:            []*ddsnmp.Profile{profile},
						Log:                 logger.NewWithWriter(&bytes.Buffer{}),
						AcquisitionObserver: recorder.beginContext(0, "", ""),
					})
					pms, err := collector.Collect()
					require.NoError(t, err)
					require.Len(t, pms, 1)
					require.Len(t, pms[0].BGPRows, len(values))
					var acquired []string
					for _, row := range pms[0].BGPRows {
						value := row.Identity.ProcessID
						if field == "routing_instance" {
							value = row.Identity.RoutingInstance
						}
						acquired = append(acquired, value)
					}
					require.ElementsMatch(t, values, acquired)
					builder := newTopologyBuilderFromSemanticInput(input, nil, collectedAt, time.Hour)
					builder.ingestTopologyBGPPeers(pms)
					builder.ingestTopologyBGPPeers(pms)
					live, _ := freezeTopologyBuilder(builder)
					require.NotNil(t, live)
					recorder.completeContext(0, successfulAcquisitionPhase())
					recorder.setCollectedShape(collectedAt, time.Hour, 0)
					capture := recorder.finish()
					require.Equal(t, topologydiag.CaptureAvailable, capture.State)
					require.NotNil(t, capture.Evidence)
					replayed, present := replayTopologyAcquisitionEvidence(capture.Evidence)
					require.True(t, present)
					require.Equal(t, live.observation.BGPPeers, replayed.BGPPeers)
					for source, peers := range map[string][]topologymodel.BGPPeer{"live": live.observation.BGPPeers, "replay": replayed.BGPPeers} {
						t.Run(source, func(t *testing.T) {
							require.Len(t, peers, len(values))
							data := topologyBGPSemanticData(t, peers[0].DeviceID)
							aggregate := topologymodel.ObservationAggregate{
								BGPPeers: peers,
							}
							wantLinks := 1
							if field == "routing_instance" {
								wantLinks = len(values)
							}
							stats := topologyenrich.ApplyBGPAdjacency(&data, aggregate)
							require.Equal(t, wantLinks, stats.EmittedLinks)
							require.Len(t, data.Links, wantLinks)
							stats = topologyenrich.ApplyBGPAdjacency(&data, aggregate)
							require.Zero(t, stats.EmittedLinks, "existing-link keys preserve exact routing identity")
							require.Len(t, data.Links, wantLinks)
							payload, err := topologyv1renderer.Render(data)
							require.NoError(t, err)
							require.NoError(t, topologyv1test.ValidateData(payload))
							table := payload.Tables.Actor["actor_bgp_peers"].Table
							require.ElementsMatch(t, values, topologyV1StringColumnValues(t, payload, table, field))
							if field == "routing_instance" {
								require.ElementsMatch(
									t,
									values,
									topologyV1StringColumnValues(
										t,
										payload,
										payload.Evidence["bgp_adjacency"].Table,
										field,
									),
								)
							}
						})
					}
				})
			}
		})
	}
}

func topologyBGPSemanticData(t testing.TB, localDeviceID string) topologymodel.Data {
	t.Helper()
	data := topologymodel.Data{
		AgentID: "semantic-test",
		View:    "summary",
		Actors: []topologymodel.Actor{
			{
				ActorID:   localDeviceID,
				ActorType: "router",
				Source:    "snmp",
				Match: topologymodel.Match{
					IPAddresses: []string{"192.0.2.2"},
				},
			},
			{
				ActorID:   "router-b",
				ActorType: "router",
				Source:    "snmp",
				Match: topologymodel.Match{
					IPAddresses: []string{"192.0.2.1"},
				},
			},
		},
	}
	assignSNMPTopologyTestHandles(t, &data)
	return data
}

// Each peer remains an inventory row; graph links scale with distinct routing instances.
func BenchmarkTopologyBGPSemanticProjection(b *testing.B) {
	for _, count := range []int{128, 1024} {
		b.Run(fmt.Sprintf("peers=%d", count), func(b *testing.B) {
			pm := &ddsnmp.ProfileMetrics{}
			for i := range count {
				pm.BGPRows = append(pm.BGPRows, ddsnmp.BGPRow{
					Kind: ddprofiledefinition.BGPRowKindPeer,
					Identity: ddsnmp.BGPIdentity{
						ProcessID:       fmt.Sprint(i),
						RoutingInstance: fmt.Sprintf("vrf-%d", i%8),
						Neighbor:        "192.0.2.1",
					},
					State: ddsnmp.BGPState{
						Has:   true,
						State: ddprofiledefinition.BGPPeerStateEstablished,
					},
				})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				builder := newTopologyBuilder()
				builder.ingestTopologyBGPPeers([]*ddsnmp.ProfileMetrics{pm})
				peers := builder.snapshotBGPPeers("router-a")
				data := topologyBGPSemanticData(b, "router-a")
				topologyenrich.ApplyBGPAdjacency(&data, topologymodel.ObservationAggregate{
					BGPPeers: peers,
				})
				if _, err := topologyv1renderer.Render(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
