// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func buildSNMPTopologyV1ActorDetails(
	actors []topologymodel.Actor,
	actorIndex map[string]int,
	stringsDict *topologyapi.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) (map[string]topologyapi.DetailTable, map[string]topologyapi.TableType, error) {
	details := make(map[string]topologyapi.DetailTable)
	tableTypes := make(map[string]topologyapi.TableType)

	labelsTable := buildSNMPTopologyV1ActorLabelsTable(actors, stringsDict)
	details["actor_labels"] = topologyapi.DetailTable{
		Type:  "actor_labels",
		Table: labelsTable,
	}
	tableTypes["actor_labels"] = snmpTopologyV1ActorLabelsTableType()
	usedTableIDs := map[string]struct{}{
		"actor_labels":     {},
		"actor_port_links": {},
	}

	metadataTable := buildSNMPTopologyV1ActorMetadataTable(actors)
	if metadataTable.Rows > 0 {
		tableID := "actor_metadata"
		details[tableID] = topologyapi.DetailTable{
			Type:  tableID,
			Table: metadataTable,
		}
		tableTypes[tableID] = topologyapi.TableType{
			Role:        "actor_detail",
			Owner:       "actor",
			Aggregation: "append",
			Columns:     metadataTable.Columns,
			Presentation: &topologyapi.TableTypePresentation{
				Label:             "Debug metadata",
				DefaultVisibility: "debug",
				Columns: []topologyapi.ModalColumn{
					modalDirectColumn("labels", "Labels", "labels", "debug_json"),
				},
			},
		}
		usedTableIDs[tableID] = struct{}{}
	}

	tableRowsByName := collectSNMPTopologyV1ActorTableRows(actors)
	for _, tableName := range topologyutil.SortedMapKeys(tableRowsByName) {
		rows := tableRowsByName[tableName]
		if len(rows) == 0 {
			continue
		}
		tableID := ""
		var table topologyapi.Table
		switch tableName {
		case "ports":
			tableID = "actor_ports"
			table = buildSNMPTopologyV1ActorPortsTable(rows, stringsDict, portNeighborSummaries)
		case "ospf_neighbors":
			tableID = "actor_ospf_neighbors"
			table = buildSNMPTopologyV1OSPFNeighborsTable(rows, actorIndex, stringsDict)
		case "bgp_peers":
			tableID = "actor_bgp_peers"
			table = buildSNMPTopologyV1BGPPeersTable(rows, actorIndex, stringsDict)
		default:
			continue
		}
		details[tableID] = topologyapi.DetailTable{
			Type:  tableID,
			Table: table,
		}
		switch tableID {
		case "actor_ports":
			tableTypes[tableID] = snmpTopologyV1ActorPortsTableType()
		case "actor_ospf_neighbors":
			tableTypes[tableID] = snmpTopologyV1OSPFNeighborsTableType()
		case "actor_bgp_peers":
			tableTypes[tableID] = snmpTopologyV1BGPPeersTableType()
		}
		usedTableIDs[tableID] = struct{}{}
	}
	return details, tableTypes, nil
}

func buildSNMPTopologyV1ActorMetadataTable(actors []topologymodel.Actor) topologyapi.Table {
	actorRefs := make([]any, 0, len(actors))
	labels := make([]any, 0, len(actors))
	for i, actor := range actors {
		if len(actor.Labels) == 0 {
			continue
		}
		actorRefs = append(actorRefs, i)
		labels = append(labels, nullableJSON(actor.Labels))
	}
	if len(actorRefs) == 0 {
		return topologyapi.EmptyTable()
	}
	return topologyapi.MustTable(len(actorRefs),
		[]topologyapi.Column{
			topologyapi.NewColumn("actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("labels", "json", topologyapi.WithNullable()),
		},
		[]topologyapi.ColumnEncoding{
			topologyapi.Values(actorRefs...),
			topologyapi.Values(labels...),
		},
	)
}

func buildSNMPTopologyV1ActorLabelsTable(
	actors []topologymodel.Actor,
	stringsDict *topologyapi.StringDictionary,
) topologyapi.Table {
	type labelRow struct {
		actor      int
		key        string
		value      string
		source     string
		kind       string
		valueIndex any
	}

	rows := make([]labelRow, 0, len(actors)*8)
	add := func(actor int, key, value, source, kind string, valueIndex any) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		rows = append(rows, labelRow{
			actor:      actor,
			key:        key,
			value:      value,
			source:     source,
			kind:       kind,
			valueIndex: valueIndex,
		})
	}
	addSlice := func(actor int, key string, values []string, source, kind string) {
		index := 0
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			add(actor, key, value, source, kind, index)
			index++
		}
	}

	for actorIndex, actor := range actors {
		add(actorIndex, "actor_type", snmpTopologyV1ActorType(actor.ActorType), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "layer", snmpTopologyV1ActorLayer(actor), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "source", topologyutil.FirstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "display_name", snmpTopologyV1DisplayName(actor), snmpTopologyV1ProducerSource, "attribute", nil)
		add(actorIndex, "sys_name", actor.Match.SysName, snmpTopologyV1ProducerSource, "match", nil)
		add(actorIndex, "sys_object_id", actor.Match.SysObjectID, snmpTopologyV1ProducerSource, "match", nil)
		addSlice(actorIndex, "chassis_id", actor.Match.ChassisIDs, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "mac_address", actor.Match.MacAddresses, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "ip_address", actor.Match.IPAddresses, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "hostname", actor.Match.Hostnames, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "dns_name", actor.Match.DNSNames, snmpTopologyV1ProducerSource, "match")

		for key, value := range actor.Labels {
			add(actorIndex, key, value, "producer_label", "label", nil)
		}
		scalarValues := topologymodel.ActorDetailScalarLabelValues(actor)
		for _, key := range topologyutil.SortedMapKeys(scalarValues) {
			add(actorIndex, key, scalarValues[key], snmpTopologyV1ProducerSource, "attribute", nil)
		}
		arrayValues := topologymodel.ActorDetailArrayLabelValues(actor)
		for _, key := range topologyutil.SortedMapKeys(arrayValues) {
			addSlice(actorIndex, key, arrayValues[key], snmpTopologyV1ProducerSource, "attribute")
		}
	}

	actorRefs := make([]any, len(rows))
	keys := make([]any, len(rows))
	values := make([]any, len(rows))
	sources := make([]any, len(rows))
	kinds := make([]any, len(rows))
	valueIndexes := make([]any, len(rows))
	for i, row := range rows {
		actorRefs[i] = row.actor
		keys[i] = stringsDict.Ref(row.key)
		values[i] = stringsDict.Ref(row.value)
		sources[i] = nullableStringRef(stringsDict, row.source)
		kinds[i] = nullableStringRef(stringsDict, row.kind)
		valueIndexes[i] = row.valueIndex
	}

	return topologyapi.MustTable(len(rows), snmpTopologyV1ActorLabelsTableType().Columns, []topologyapi.ColumnEncoding{
		topologyapi.Values(actorRefs...),
		topologyapi.Values(keys...),
		topologyapi.Values(values...),
		topologyapi.Values(sources...),
		topologyapi.Values(kinds...),
		topologyapi.Values(valueIndexes...),
	})
}
