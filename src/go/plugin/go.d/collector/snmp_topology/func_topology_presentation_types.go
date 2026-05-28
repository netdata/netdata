// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/topology"

func topologyPresentationActorTypes(
	deviceSummaryFields []topology.PresentationSummaryField,
	deviceTables map[string]topology.PresentationTable,
	linkOnlyTables map[string]topology.PresentationTable,
	infoOnlyTabs []topology.PresentationModalTab,
	segmentSummaryFields []topology.PresentationSummaryField,
	endpointSummaryFields []topology.PresentationSummaryField,
) map[string]topology.PresentationActorType {
	deviceType := func(label, colorSlot string) topology.PresentationActorType {
		return topology.PresentationActorType{
			Label:           label,
			ColorSlot:       colorSlot,
			Border:          true,
			Role:            "actor",
			SizeByLinks:     true,
			ShowPortBullets: true,
			SummaryFields:   deviceSummaryFields,
			Tables:          deviceTables,
			ModalTabs:       infoOnlyTabs,
		}
	}

	return map[string]topology.PresentationActorType{
		"device":        deviceType("Device", "primary"),
		"router":        deviceType("Router", "primary"),
		"switch":        deviceType("Switch", "primary"),
		"firewall":      deviceType("Firewall", "warning"),
		"access_point":  deviceType("Access Point", "info"),
		"server":        deviceType("Server", "secondary"),
		"storage":       deviceType("Storage", "secondary"),
		"load_balancer": deviceType("Load Balancer", "info"),
		"printer":       deviceType("Printer", "neutral"),
		"phone":         deviceType("Phone", "neutral"),
		"ups":           deviceType("UPS", "structural"),
		"camera":        deviceType("Camera", "neutral"),
		"segment": {
			Label:         "Network segment",
			ColorSlot:     "dim",
			SummaryFields: segmentSummaryFields,
			Tables:        linkOnlyTables,
			ModalTabs:     infoOnlyTabs,
		},
		"endpoint": {
			Label:         "Inferred endpoint",
			ColorSlot:     "derived",
			Border:        true,
			Role:          "endpoint",
			SummaryFields: endpointSummaryFields,
			Tables:        linkOnlyTables,
			ModalTabs:     infoOnlyTabs,
		},
	}
}

func topologyPresentationLinkTypes() map[string]topology.PresentationLinkType {
	return map[string]topology.PresentationLinkType{
		"lldp":     {Label: "LLDP", ColorSlot: "accent", Width: 2},
		"cdp":      {Label: "CDP", ColorSlot: "accent", Width: 2},
		"bridge":   {Label: "Bridge", ColorSlot: "neutral"},
		"fdb":      {Label: "FDB", ColorSlot: "neutral"},
		"stp":      {Label: "STP", ColorSlot: "muted"},
		"arp":      {Label: "ARP", ColorSlot: "muted"},
		"snmp":     {Label: "SNMP", ColorSlot: "primary"},
		"probable": {Label: "Probable", ColorSlot: "dim"},
	}
}

func topologyPresentationPortFields() []topology.PresentationPortField {
	return []topology.PresentationPortField{
		{Key: "type", Label: "Type"},
		{Key: "role", Label: "Role"},
		{Key: "status", Label: "Status"},
		{Key: "mode", Label: "Mode"},
		{Key: "sources", Label: "Sources"},
	}
}

func topologyPresentationPortTypes() map[string]topology.PresentationPortType {
	return map[string]topology.PresentationPortType{
		"lldp":           {Label: "lldp/cdp", ColorSlot: "accent"},
		"switch_facing":  {Label: "switch-facing", ColorSlot: "primary"},
		"host_facing":    {Label: "host-facing", ColorSlot: "secondary"},
		"host_candidate": {Label: "host-candidate", ColorSlot: "info"},
		"trunk":          {Label: "trunk", ColorSlot: "warning"},
		"access":         {Label: "access", ColorSlot: "derived"},
		"topology":       {Label: "unclassified", ColorSlot: "neutral"},
		"idle":           {Label: "idle", ColorSlot: "muted"},
		"unknown":        {Label: "unknown", ColorSlot: "dim"},
	}
}

func topologyPresentationLegend() topology.PresentationLegend {
	return topology.PresentationLegend{
		Actors: []topology.PresentationLegendEntry{
			{Type: "router", Label: "Router"},
			{Type: "switch", Label: "Switch"},
			{Type: "firewall", Label: "Firewall"},
			{Type: "access_point", Label: "Access Point"},
			{Type: "server", Label: "Server"},
			{Type: "storage", Label: "Storage"},
			{Type: "load_balancer", Label: "Load Balancer"},
			{Type: "printer", Label: "Printer"},
			{Type: "phone", Label: "IP Phone"},
			{Type: "ups", Label: "UPS / PDU"},
			{Type: "camera", Label: "Camera / Media"},
			{Type: "device", Label: "Other device"},
			{Type: "endpoint", Label: "Inferred endpoint"},
			{Type: "segment", Label: "Network segment"},
		},
		Links: []topology.PresentationLegendEntry{
			{Type: "lldp", Label: "LLDP"},
			{Type: "cdp", Label: "CDP"},
			{Type: "snmp", Label: "SNMP"},
			{Type: "bridge", Label: "Bridge"},
			{Type: "probable", Label: "Probable"},
		},
		Ports: []topology.PresentationLegendEntry{
			{Type: "lldp", Label: "lldp/cdp"},
			{Type: "switch_facing", Label: "switch-facing"},
			{Type: "host_facing", Label: "host-facing"},
			{Type: "host_candidate", Label: "host-candidate"},
			{Type: "trunk", Label: "trunk"},
			{Type: "access", Label: "access"},
			{Type: "topology", Label: "unclassified"},
			{Type: "idle", Label: "idle"},
		},
	}
}
