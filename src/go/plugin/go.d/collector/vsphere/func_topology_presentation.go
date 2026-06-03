// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

func vsphereTopologyPresentation() *topologyv1.Presentation {
	return &topologyv1.Presentation{
		ProfileVersion: "vsphere.inventory.v1",
		Selection: &topologyv1.SelectionPresentation{
			ActorClick: &topologyv1.ActorClickPresentation{
				Mode: "highlight_connections",
			},
		},
		Legend: &topologyv1.PresentationLegend{
			Actors: []topologyv1.LegendEntry{
				{Type: "vsphere_datacenter", Label: "Datacenter"},
				{Type: "vsphere_cluster", Label: "Cluster"},
				{Type: "vsphere_host", Label: "ESXi Host"},
				{Type: "vsphere_vm", Label: "VM"},
				{Type: "vsphere_datastore", Label: "Datastore"},
				{Type: "vsphere_network", Label: "Network"},
				{Type: "vsphere_datastore_cluster", Label: "Datastore Cluster"},
				{Type: "vsphere_resource_pool", Label: "Resource Pool"},
			},
			Links: []topologyv1.LegendEntry{
				{Type: vsphereTopologyOwnershipLink, Label: "Contains"},
				{Type: vsphereTopologyRunsOnLink, Label: "Runs on"},
				{Type: vsphereTopologyNetworkLink, Label: "Connected to"},
			},
		},
	}
}

func vsphereActorPresentation(label, icon, colorSlot string) *topologyv1.ActorPresentation {
	return &topologyv1.ActorPresentation{
		Label:     label,
		Role:      "actor",
		Icon:      icon,
		ColorSlot: colorSlot,
		Border: &topologyv1.BorderPresentation{
			Enabled: new(true),
			Style:   "solid",
		},
		LabelPolicy: &topologyv1.LabelPolicy{
			Columns:   []string{"name"},
			Fallback:  "type_label",
			MaxLength: 80,
			Array:     "reject",
		},
		Hover: &topologyv1.HoverPresentation{
			Fields: []topologyv1.PresentationField{
				{Key: "name", Label: "Name"},
				{Key: "object_type", Label: "Type"},
				{Key: "vsphere_moid", Label: "vSphere MoRef"},
				{Key: "overall_status", Label: "Status"},
				{Key: "power_state", Label: "Power"},
			},
		},
		Modal: vsphereActorModal(),
	}
}

func vsphereLinkPresentation(label, colorSlot, arrow, distance string) *topologyv1.LinkPresentation {
	return &topologyv1.LinkPresentation{
		Label:     label,
		ColorSlot: colorSlot,
		LineStyle: "solid",
		Width:     "normal",
		Curve:     "auto",
		Arrow:     arrow,
		Hover: &topologyv1.HoverPresentation{
			Fields: []topologyv1.PresentationField{
				{Key: "relationship", Label: "Relationship"},
				{Key: "evidence_count", Label: "Evidence"},
			},
		},
		Layout: &topologyv1.LinkLayoutPresentation{
			Strength: "normal",
			Distance: distance,
		},
	}
}

func vsphereActorModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		Enabled: new(true),
		Labels: &topologyv1.ModalLabelsPresentation{
			Enabled:          new(true),
			Table:            vsphereTopologyLabelsTable,
			ActorColumn:      "actor",
			KeyColumn:        "key",
			ValueColumn:      "value",
			SourceColumn:     "source",
			KindColumn:       "kind",
			ValueIndexColumn: "value_index",
			Identification: &topologyv1.ModalLabelIdentificationPresentation{
				Enabled: new(true),
				Fields: []topologyv1.ModalLabelIdentificationField{
					{Key: "object_type", Label: "Type", MaxValues: 1},
					{Key: "datacenter", Label: "Datacenter", MaxValues: 1},
					{Key: "cluster", Label: "Cluster", MaxValues: 1},
					{Key: "host", Label: "Host", MaxValues: 1},
					{Key: "resource_pool", Label: "Resource Pool", MaxValues: 1},
				},
			},
		},
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{
			Enabled: new(true),
			Depth:   1,
			IncludeLinkTypes: []string{
				vsphereTopologyOwnershipLink,
				vsphereTopologyRunsOnLink,
				vsphereTopologyNetworkLink,
			},
		},
		Sections: []topologyv1.ModalSection{
			{
				ID:    "details",
				Label: "Details",
				Order: 10,
				Source: topologyv1.ModalSource{
					Kind:  "actor_table",
					Table: vsphereTopologyDetailTable,
				},
				OwnerFilter: &topologyv1.ModalOwnerFilter{
					Mode:        "actor_column",
					ActorColumn: "actor",
				},
				Columns:    vsphereDetailModalColumns(),
				EmptyLabel: "No vSphere details",
			},
			{
				ID:    "relationships",
				Label: "Relationships",
				Order: 20,
				Source: topologyv1.ModalSource{
					Kind: "links",
				},
				OwnerFilter: &topologyv1.ModalOwnerFilter{
					Mode:           "incident_link",
					SrcActorColumn: "src_actor",
					DstActorColumn: "dst_actor",
				},
				Columns:    vsphereRelationshipModalColumns(),
				EmptyLabel: "No vSphere relationships",
			},
		},
	}
}

func vsphereDetailModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
		vsphereModalDirectColumn("object_type", "Type", "object_type", "badge"),
		vsphereModalDirectColumn("name", "Name", "name", "text"),
		vsphereModalDirectColumn("vsphere_moid", "vSphere MoRef", "vsphere_moid", "text"),
		vsphereModalDirectColumn("datacenter", "Datacenter", "datacenter", "text"),
		vsphereModalDirectColumn("cluster", "Cluster", "cluster", "text"),
		vsphereModalDirectColumn("host", "Host", "host", "text"),
		vsphereModalDirectColumn("resource_pool", "Resource Pool", "resource_pool", "text"),
		vsphereModalDirectColumn("connection_state", "Connection", "connection_state", "badge"),
		vsphereModalDirectColumn("power_state", "Power", "power_state", "badge"),
		vsphereModalDirectColumn("overall_status", "Status", "overall_status", "badge"),
		vsphereModalDirectColumn("accessible", "Accessible", "accessible", "badge"),
		vsphereModalDirectColumn("in_maintenance_mode", "In Maintenance", "in_maintenance_mode", "badge"),
		vsphereModalDirectColumn("maintenance_mode", "Maintenance", "maintenance_mode", "badge"),
		vsphereModalDirectColumn("datastore_type", "Datastore Type", "datastore_type", "badge"),
		vsphereModalDirectColumn("network_type", "Network Type", "network_type", "badge"),
		vsphereModalDirectColumnWithVisibility("ip_pool_name", "IP Pool", "ip_pool_name", "text", "expanded"),
		vsphereModalDirectColumn("snapshot_count", "Snapshots", "snapshot_count", "number"),
		vsphereModalDirectColumnWithVisibility("snapshot_chain_depth", "Snapshot Chain", "snapshot_chain_depth", "number", "expanded"),
		vsphereModalDirectColumn("configured_vcpus", "vCPUs", "configured_vcpus", "number"),
		vsphereModalDirectColumn("configured_memory_mib", "Memory MiB", "configured_memory_mib", "number"),
		vsphereModalDirectColumn("capacity_bytes", "Capacity", "capacity_bytes", "number"),
		vsphereModalDirectColumn("free_space_bytes", "Free Space", "free_space_bytes", "number"),
		vsphereModalDirectColumnWithVisibility("uncommitted_bytes", "Uncommitted", "uncommitted_bytes", "number", "expanded"),
		vsphereModalDirectColumn("cpu_capacity_mhz", "CPU Capacity MHz", "cpu_capacity_mhz", "number"),
		vsphereModalDirectColumn("memory_capacity_mib", "Memory Capacity MiB", "memory_capacity_mib", "number"),
		vsphereModalDirectColumn("cpu_limit_mhz", "CPU Limit MHz", "cpu_limit_mhz", "number"),
		vsphereModalDirectColumn("memory_limit_mib", "Memory Limit MiB", "memory_limit_mib", "number"),
		vsphereModalDirectColumnWithVisibility("cpu_reservation_mhz", "CPU Reservation MHz", "cpu_reservation_mhz", "number", "expanded"),
		vsphereModalDirectColumnWithVisibility("memory_reservation_mib", "Memory Reservation MiB", "memory_reservation_mib", "number", "expanded"),
		vsphereModalDirectColumn("network_hosts", "Hosts", "network_hosts", "number"),
		vsphereModalDirectColumn("network_vms", "VMs", "network_vms", "number"),
		vsphereModalDirectColumn("drs_enabled", "DRS", "drs_enabled", "badge"),
		vsphereModalDirectColumn("ha_enabled", "HA", "ha_enabled", "badge"),
		vsphereModalDirectColumn("vsan_enabled", "vSAN", "vsan_enabled", "badge"),
		vsphereModalDirectColumn("storage_drs_enabled", "Storage DRS", "storage_drs_enabled", "badge"),
		vsphereModalDirectColumn("multiple_host_access", "Multiple Host Access", "multiple_host_access", "badge"),
		vsphereModalDirectColumn("consolidation_needed", "Consolidation", "consolidation_needed", "badge"),
		vsphereModalDirectColumnWithVisibility("tools_running_status", "Tools Running", "tools_running_status", "badge", "expanded"),
		vsphereModalDirectColumnWithVisibility("tools_version_status", "Tools Version", "tools_version_status", "badge", "expanded"),
	}
}

func vsphereRelationshipModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
		vsphereModalDirectColumn("relationship", "Relationship", "relationship", "badge"),
		{
			ID:    "related_object",
			Label: "Related Object",
			Projection: topologyv1.ModalProjection{
				Kind:           "opposite_actor",
				SrcActorColumn: "src_actor",
				DstActorColumn: "dst_actor",
			},
			Cell: "actor_link",
		},
		vsphereModalDirectColumn("evidence_count", "Evidence", "evidence_count", "number"),
	}
}

func vsphereModalDirectColumn(id, label, sourceColumn, cell string) topologyv1.ModalColumn {
	return vsphereModalDirectColumnWithVisibility(id, label, sourceColumn, cell, "")
}

func vsphereModalDirectColumnWithVisibility(id, label, sourceColumn, cell, visibility string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyv1.ModalProjection{
			Kind:   "direct",
			Column: sourceColumn,
		},
		Cell:       cell,
		Visibility: visibility,
	}
}
