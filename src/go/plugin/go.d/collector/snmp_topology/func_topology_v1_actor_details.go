// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"strings"
)

func buildSNMPTopologyV1ActorDetails(
	actors []topologyActor,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) (map[string]topologyv1.DetailTable, map[string]topologyv1.TableType, error) {
	details := make(map[string]topologyv1.DetailTable)
	tableTypes := make(map[string]topologyv1.TableType)

	labelsTable := buildSNMPTopologyV1ActorLabelsTable(actors, stringsDict)
	details["actor_labels"] = topologyv1.DetailTable{
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
		details[tableID] = topologyv1.DetailTable{
			Type:  tableID,
			Table: metadataTable,
		}
		tableTypes[tableID] = topologyv1.TableType{
			Role:        "actor_detail",
			Owner:       "actor",
			Aggregation: "append",
			Columns:     metadataTable.Columns,
			Presentation: &topologyv1.TableTypePresentation{
				Label:             "Debug metadata",
				DefaultVisibility: "debug",
				Columns: []topologyv1.ModalColumn{
					modalDirectColumn("attributes", "Attributes", "attributes", "debug_json"),
					modalDirectColumn("labels", "Labels", "labels", "debug_json"),
				},
			},
		}
		usedTableIDs[tableID] = struct{}{}
	}

	tableRowsByName := collectSNMPTopologyV1ActorTableRows(actors)
	tableNames := sortedMapKeys(tableRowsByName)
	reservedCustomTableIDs := snmpTopologyV1ReservedCustomTableIDs(tableNames)
	for _, tableName := range tableNames {
		rows := tableRowsByName[tableName]
		if len(rows) == 0 {
			continue
		}
		tableID := snmpTopologyV1ActorDetailTableID(tableName, usedTableIDs, reservedCustomTableIDs)
		var table topologyv1.Table
		var err error
		if tableID == "actor_ports" {
			table = buildSNMPTopologyV1ActorPortsTable(rows, stringsDict, portNeighborSummaries)
		} else if tableID == "actor_ospf_neighbors" {
			table = buildSNMPTopologyV1OSPFNeighborsTable(rows, actorIndex, stringsDict)
		} else if tableID == "actor_bgp_peers" {
			table = buildSNMPTopologyV1BGPPeersTable(rows, actorIndex, stringsDict)
		} else {
			table, err = buildSNMPTopologyV1DynamicTable(rows, stringsDict)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("build actor detail table %q: %w", tableName, err)
		}
		details[tableID] = topologyv1.DetailTable{
			Type:  tableID,
			Table: table,
		}
		if tableID == "actor_ports" {
			tableTypes[tableID] = snmpTopologyV1ActorPortsTableType()
		} else if tableID == "actor_ospf_neighbors" {
			tableTypes[tableID] = snmpTopologyV1OSPFNeighborsTableType()
		} else if tableID == "actor_bgp_peers" {
			tableTypes[tableID] = snmpTopologyV1BGPPeersTableType()
		} else {
			tableTypes[tableID] = topologyv1.TableType{
				Role:        "actor_detail",
				Owner:       "actor",
				Aggregation: "append",
				Columns:     table.Columns,
			}
		}
		usedTableIDs[tableID] = struct{}{}
	}
	return details, tableTypes, nil
}

func snmpTopologyV1ReservedCustomTableIDs(tableNames []string) map[string]struct{} {
	reserved := make(map[string]struct{})
	for _, tableName := range tableNames {
		tableID := topologyID("actor_"+tableName, "actor_detail")
		switch tableID {
		case "actor_labels", "actor_metadata", "actor_port_links":
			reserved[topologyID("actor_custom_"+tableName, "actor_detail")] = struct{}{}
		}
	}
	return reserved
}

func snmpTopologyV1ActorDetailTableID(tableName string, usedTableIDs, reservedCustomTableIDs map[string]struct{}) string {
	tableID := topologyID("actor_"+tableName, "actor_detail")
	switch tableID {
	case "actor_labels", "actor_metadata", "actor_port_links":
		return snmpTopologyV1UniqueActorDetailTableID(topologyID("actor_custom_"+tableName, "actor_detail"), usedTableIDs)
	default:
		if _, reserved := reservedCustomTableIDs[tableID]; reserved {
			return snmpTopologyV1UniqueActorDetailTableID(topologyID("actor_detail_"+tableName, "actor_detail"), usedTableIDs)
		}
		return snmpTopologyV1UniqueActorDetailTableID(tableID, usedTableIDs)
	}
}

func snmpTopologyV1UniqueActorDetailTableID(tableID string, usedTableIDs map[string]struct{}) string {
	if _, ok := usedTableIDs[tableID]; !ok {
		return tableID
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", tableID, suffix)
		if _, ok := usedTableIDs[candidate]; !ok {
			return candidate
		}
	}
}

func buildSNMPTopologyV1ActorMetadataTable(actors []topologyActor) topologyv1.Table {
	actorRefs := make([]any, 0, len(actors))
	attributes := make([]any, 0, len(actors))
	labels := make([]any, 0, len(actors))
	for i, actor := range actors {
		if len(actor.Attributes) == 0 && len(actor.Labels) == 0 {
			continue
		}
		actorRefs = append(actorRefs, i)
		attributes = append(attributes, nullableJSON(actor.Attributes))
		labels = append(labels, nullableJSON(actor.Labels))
	}
	if len(actorRefs) == 0 {
		return topologyv1.EmptyTable()
	}
	return topologyv1.MustTable(len(actorRefs),
		[]topologyv1.Column{
			topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("attributes", "json", topologyv1.WithNullable()),
			topologyv1.NewColumn("labels", "json", topologyv1.WithNullable()),
		},
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(actorRefs...),
			topologyv1.Values(attributes...),
			topologyv1.Values(labels...),
		},
	)
}

func buildSNMPTopologyV1ActorLabelsTable(
	actors []topologyActor,
	stringsDict *topologyv1.StringDictionary,
) topologyv1.Table {
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

	scalarAttributeKeys := []string{
		"vendor", "vendor_derived", "model", "sys_descr", "sys_location", "sys_contact",
		"management_ip", tagOSPFRouterID, "display_name", "display_source", "chart_id_prefix", "chart_context_prefix",
		"netdata_host_id", "ports_total", "ports_up", "ports_down", "vlan_count", "fdb_total_macs",
		"lldp_neighbor_count", "cdp_neighbor_count", "endpoints_total", "if_admin_status_counts",
		"if_oper_status_counts", "if_link_mode_counts", "if_topology_role_counts",
	}
	arrayAttributeKeys := []string{
		"protocols", "protocols_collected", "learned_sources", "capabilities",
		"capabilities_supported", "capabilities_enabled", "if_names", "if_indexes",
	}

	for actorIndex, actor := range actors {
		add(actorIndex, "actor_type", snmpTopologyV1ActorType(actor.ActorType), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "layer", snmpTopologyV1ActorLayer(actor), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "source", firstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource), snmpTopologyV1ProducerSource, "identity", nil)
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
		for _, key := range scalarAttributeKeys {
			if value := topologyV1ScalarLabelValue(actor.Attributes[key]); value != "" {
				add(actorIndex, key, value, snmpTopologyV1ProducerSource, "attribute", nil)
			}
		}
		for _, key := range arrayAttributeKeys {
			addSlice(actorIndex, key, anyStringSlice(actor.Attributes[key]), snmpTopologyV1ProducerSource, "attribute")
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

	return topologyv1.MustTable(len(rows), snmpTopologyV1ActorLabelsTableType().Columns, []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(keys...),
		topologyv1.Values(values...),
		topologyv1.Values(sources...),
		topologyv1.Values(kinds...),
		topologyv1.Values(valueIndexes...),
	})
}
