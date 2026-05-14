// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import "github.com/netdata/netdata/go/plugins/pkg/topology"

func vsphereTopologyPresentation() *topology.Presentation {
	return &topology.Presentation{
		ActorTypes: map[string]topology.PresentationActorType{
			"vsphere_datacenter":        vsphereActorType("Datacenter", "blue", "name", "vsphere_id"),
			"vsphere_cluster":           vsphereActorType("Cluster", "green", "name", "overall_status", "drs_enabled", "ha_enabled", "vsan_enabled"),
			"vsphere_host":              vsphereActorType("ESXi Host", "orange", "name", "connection_state", "power_state", "overall_status"),
			"vsphere_vm":                vsphereActorType("VM", "purple", "name", "connection_state", "power_state", "snapshot_count"),
			"vsphere_datastore":         vsphereActorType("Datastore", "cyan", "name", "type", "accessible", "maintenance_mode"),
			"vsphere_network":           vsphereActorType("Network", "yellow", "name", "type", "accessible", "hosts", "vms"),
			"vsphere_datastore_cluster": vsphereActorType("Datastore Cluster", "teal", "name", "storage_drs_enabled"),
			"vsphere_resource_pool":     vsphereActorType("Resource Pool", "gray", "name", "overall_status"),
		},
		LinkTypes: map[string]topology.PresentationLinkType{
			"contains": {Label: "Contains", ColorSlot: "gray", Width: 1},
			"connects": {Label: "Connects",
				ColorSlot: "green", Width: 1},
			"runs": {Label: "Runs", ColorSlot: "blue", Width: 1},
		},
		Legend: topology.PresentationLegend{
			Actors: []topology.PresentationLegendEntry{
				{Type: "vsphere_datacenter", Label: "Datacenter"},
				{Type: "vsphere_cluster", Label: "Cluster"},
				{Type: "vsphere_host", Label: "ESXi Host"},
				{Type: "vsphere_vm", Label: "VM"},
				{Type: "vsphere_datastore", Label: "Datastore"},
				{Type: "vsphere_network", Label: "Network"},
				{Type: "vsphere_datastore_cluster", Label: "Datastore Cluster"},
				{Type: "vsphere_resource_pool", Label: "Resource Pool"},
			},
			Links: []topology.PresentationLegendEntry{
				{Type: "contains", Label: "Contains"},
				{Type: "connects", Label: "Connects"},
				{Type: "runs", Label: "Runs"},
			},
		},
		ActorClickBehavior: "highlight_connections",
	}
}

func vsphereActorType(label, colorSlot string, summaryKeys ...string) topology.PresentationActorType {
	return topology.PresentationActorType{
		Label:         label,
		ColorSlot:     colorSlot,
		Border:        true,
		SummaryFields: vsphereSummaryFields(summaryKeys...),
	}
}

func vsphereSummaryFields(keys ...string) []topology.PresentationSummaryField {
	fields := make([]topology.PresentationSummaryField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, topology.PresentationSummaryField{
			Key:     key,
			Label:   vsphereSummaryFieldLabels[key],
			Sources: []string{"attributes"},
		})
	}
	return fields
}

var vsphereSummaryFieldLabels = map[string]string{
	"accessible":          "Accessible",
	"connection_state":    "Connection State",
	"drs_enabled":         "DRS Enabled",
	"ha_enabled":          "HA Enabled",
	"hosts":               "Hosts",
	"maintenance_mode":    "Maintenance Mode",
	"name":                "Name",
	"overall_status":      "Overall Status",
	"power_state":         "Power State",
	"snapshot_count":      "Snapshots",
	"storage_drs_enabled": "Storage DRS Enabled",
	"type":                "Type",
	"vms":                 "VMs",
	"vsan_enabled":        "vSAN Enabled",
	"vsphere_id":          "vSphere ID",
}
