// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"time"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const (
	snmpTopologyV1ProducerSource = "snmp-l2"
	snmpTopologyV1Instance       = "local"

	snmpTopologyV1ActorDevice          = "device"
	snmpTopologyV1ActorEndpoint        = "endpoint"
	snmpTopologyV1ActorSegment         = "segment"
	snmpTopologyV1ActorL3SubnetSegment = topologymodel.L3SubnetSegmentActorType

	snmpTopologyV1LinkObservation        = "l2_observation"
	snmpTopologyV1LinkLLDP               = "lldp"
	snmpTopologyV1LinkCDP                = "cdp"
	snmpTopologyV1LinkBridge             = "bridge"
	snmpTopologyV1LinkFDB                = "fdb"
	snmpTopologyV1LinkSTP                = "stp"
	snmpTopologyV1LinkARP                = "arp"
	snmpTopologyV1LinkSNMP               = "snmp"
	snmpTopologyV1LinkProbable           = "probable"
	snmpTopologyV1LinkL3Subnet           = topologymodel.L3SubnetLinkType
	snmpTopologyV1LinkL3SubnetMembership = topologymodel.L3SubnetMembershipLinkType
	snmpTopologyV1LinkOSPF               = topologymodel.OSPFAdjacencyLinkType
	snmpTopologyV1LinkBGP                = topologymodel.BGPAdjacencyLinkType
)

func Render(data topologymodel.Data) (topologyapi.Data, error) {
	stringsDict := topologyapi.NewStringDictionary("")
	actorRows, actorIndex := buildSNMPTopologyV1Actors(data.Actors, stringsDict)

	linkRows, evidenceSections, err := buildSNMPTopologyV1Links(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyapi.Data{}, err
	}

	portNeighborSummaries := buildSNMPTopologyV1PortNeighborSummaries(data.Links, actorIndex)
	actorDetails, tableTypes, err := buildSNMPTopologyV1ActorDetails(data.Actors, actorIndex, stringsDict, portNeighborSummaries)
	if err != nil {
		return topologyapi.Data{}, err
	}
	if tableTypes == nil {
		tableTypes = make(map[string]topologyapi.TableType)
	}
	if _, ok := tableTypes["actor_labels"]; !ok {
		tableTypes["actor_labels"] = snmpTopologyV1ActorLabelsTableType()
	}
	if _, ok := tableTypes["actor_ports"]; !ok {
		tableTypes["actor_ports"] = snmpTopologyV1ActorPortsTableType()
	}
	tableTypes["actor_port_links"] = snmpTopologyV1ActorPortLinksTableType()
	if _, ok := tableTypes["actor_ospf_neighbors"]; !ok {
		tableTypes["actor_ospf_neighbors"] = snmpTopologyV1OSPFNeighborsTableType()
	}
	if _, ok := tableTypes["actor_bgp_peers"]; !ok {
		tableTypes["actor_bgp_peers"] = snmpTopologyV1BGPPeersTableType()
	}
	portLinksTable, err := buildSNMPTopologyV1ActorPortLinksTable(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyapi.Data{}, err
	}
	if portLinksTable.Rows > 0 {
		if actorDetails == nil {
			actorDetails = make(map[string]topologyapi.DetailTable)
		}
		actorDetails["actor_port_links"] = topologyapi.DetailTable{
			Type:  "actor_port_links",
			Table: portLinksTable,
		}
	}

	types := topologyapi.TypeRegistry{
		ActorTypes:    snmpTopologyV1ActorTypes(),
		LinkTypes:     snmpTopologyV1LinkTypes(),
		PortTypes:     snmpTopologyV1PortTypes(),
		EvidenceTypes: snmpTopologyV1EvidenceTypes(),
		TableTypes:    tableTypes,
		AggregationScopes: map[string]topologyapi.AggregationScope{
			"device": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
			"network": {
				Columns:        []string{"type"},
				EvidencePolicy: "preserve",
			},
			"segment": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
			"endpoint": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
		},
	}

	if len(types.TableTypes) == 0 {
		types.TableTypes = nil
	}

	payload := topologyapi.Data{
		SchemaVersion: topologyapi.SchemaVersion,
		Producer: topologyapi.Producer{
			Source:   snmpTopologyV1ProducerSource,
			Instance: topologyutil.FirstNonEmptyString(data.AgentID, snmpTopologyV1Instance),
			Plugin:   "go.d/snmp_topology",
			Capabilities: []string{
				"lldp",
				"cdp",
				"fdb",
				"stp",
				"l3_subnet",
				"l3_subnet_membership",
				"ospf",
				"bgp",
			},
		},
		CollectedAt: data.CollectedAt,
		View: &topologyapi.View{
			ID:    topologyutil.FirstNonEmptyString(data.View, "summary"),
			Scope: "network",
			Mode:  "detailed",
		},
		Dictionaries: topologyapi.Dictionaries{
			"strings": stringsDict.Values(),
		},
		Types:        types,
		Presentation: snmpTopologyV1Presentation(),
		Actors:       actorRows,
		Links:        linkRows,
		Evidence:     evidenceSections,
		Stats:        topologyStatsToV1(data.Stats),
	}
	if payload.CollectedAt.IsZero() {
		payload.CollectedAt = time.Now().UTC()
	}
	if actorDetails != nil {
		payload.Tables = &topologyapi.DetailTables{
			Actor: actorDetails,
		}
	}
	return payload, nil
}
