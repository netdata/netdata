// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"time"
)

const (
	snmpTopologyV1ProducerSource = "snmp-l2"
	snmpTopologyV1Instance       = "local"

	snmpTopologyV1ActorDevice   = "device"
	snmpTopologyV1ActorEndpoint = "endpoint"
	snmpTopologyV1ActorSegment  = "segment"

	snmpTopologyV1LinkObservation = "l2_observation"
	snmpTopologyV1LinkLLDP        = "lldp"
	snmpTopologyV1LinkCDP         = "cdp"
	snmpTopologyV1LinkBridge      = "bridge"
	snmpTopologyV1LinkFDB         = "fdb"
	snmpTopologyV1LinkSTP         = "stp"
	snmpTopologyV1LinkARP         = "arp"
	snmpTopologyV1LinkSNMP        = "snmp"
	snmpTopologyV1LinkProbable    = "probable"
	snmpTopologyV1LinkL3Subnet    = topologyL3SubnetLinkType
	snmpTopologyV1LinkOSPF        = topologyOSPFAdjacencyLinkType
)

func snmpTopologyToV1(data topologyData) (topologyv1.Data, error) {
	stringsDict := topologyv1.NewStringDictionary("")
	actorRows, actorIndex := buildSNMPTopologyV1Actors(data.Actors, stringsDict)

	linkRows, evidenceSections, err := buildSNMPTopologyV1Links(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyv1.Data{}, err
	}

	portNeighborSummaries := buildSNMPTopologyV1PortNeighborSummaries(data.Links, actorIndex)
	actorDetails, tableTypes, err := buildSNMPTopologyV1ActorDetails(data.Actors, actorIndex, stringsDict, portNeighborSummaries)
	if err != nil {
		return topologyv1.Data{}, err
	}
	if tableTypes == nil {
		tableTypes = make(map[string]topologyv1.TableType)
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
	portLinksTable, err := buildSNMPTopologyV1ActorPortLinksTable(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyv1.Data{}, err
	}
	if portLinksTable.Rows > 0 {
		if actorDetails == nil {
			actorDetails = make(map[string]topologyv1.DetailTable)
		}
		actorDetails["actor_port_links"] = topologyv1.DetailTable{
			Type:  "actor_port_links",
			Table: portLinksTable,
		}
	}

	types := topologyv1.TypeRegistry{
		ActorTypes:    snmpTopologyV1ActorTypes(),
		LinkTypes:     snmpTopologyV1LinkTypes(),
		PortTypes:     snmpTopologyV1PortTypes(),
		EvidenceTypes: snmpTopologyV1EvidenceTypes(),
		TableTypes:    tableTypes,
		AggregationScopes: map[string]topologyv1.AggregationScope{
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

	payload := topologyv1.Data{
		SchemaVersion: topologyv1.SchemaVersion,
		Producer: topologyv1.Producer{
			Source:   snmpTopologyV1ProducerSource,
			Instance: firstNonEmptyString(data.AgentID, snmpTopologyV1Instance),
			Plugin:   "go.d/snmp_topology",
			Capabilities: []string{
				"lldp",
				"cdp",
				"fdb",
				"stp",
				"l3_subnet",
				"ospf",
			},
		},
		CollectedAt: data.CollectedAt,
		View: &topologyv1.View{
			ID:    firstNonEmptyString(data.View, "summary"),
			Scope: "network",
			Mode:  "detailed",
		},
		Dictionaries: topologyv1.Dictionaries{
			"strings": stringsDict.Values(),
		},
		Types:        types,
		Presentation: snmpTopologyV1Presentation(),
		Actors:       actorRows,
		Links:        linkRows,
		Evidence:     evidenceSections,
		Stats:        cloneAnyMapForTopologyV1(data.Stats),
	}
	if payload.CollectedAt.IsZero() {
		payload.CollectedAt = time.Now().UTC()
	}
	if actorDetails != nil {
		payload.Tables = &topologyv1.DetailTables{
			Actor: actorDetails,
		}
	}
	return payload, nil
}
