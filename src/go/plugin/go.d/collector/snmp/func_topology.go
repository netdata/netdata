// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopology)(nil)

type funcTopology struct {
	router *funcRouter
}

func newFuncTopology(r *funcRouter) *funcTopology {
	return &funcTopology{router: r}
}

const topologyMethodID = "topology:snmp"

const (
	topologyParamNodesIdentity      = "nodes_identity"
	topologyParamMapType            = "map_type"
	topologyParamInferenceStrategy  = "inference_strategy"
	topologyParamManagedDeviceFocus = "managed_snmp_device_focus"
	topologyParamDepth              = "depth"

	topologyNodesIdentityIP  = "ip"
	topologyNodesIdentityMAC = "mac"

	topologyMapTypeLLDPCDPManaged                = "lldp_cdp_managed"
	topologyMapTypeHighConfidenceInferred        = "high_confidence_inferred"
	topologyMapTypeAllDevicesLowConfidence       = "all_devices_low_confidence"
	topologyInferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	topologyInferenceStrategySTPParentTree       = "stp_parent_tree"
	topologyInferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	topologyInferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	topologyInferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
	topologyInferenceStrategyFDBOverlapWeighted  = "fdb_overlap_weighted"
	topologyInferenceStrategyExperimentalFull    = "experimental_full"
	topologyManagedFocusAllDevices               = "all_devices"
	topologyManagedFocusIPPrefix                 = "ip:"
	topologyDepthAll                             = "all"
	topologyDepthMin                             = 0
	topologyDepthMax                             = 10
	topologyDepthAllInternal                     = -1
)

func topologyNodesIdentityParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamNodesIdentity,
		Name:      "Nodes Identity",
		Help:      "Choose actor identity strategy: ip (collapse by IP, remove non-IP inferred) or mac",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: topologyNodesIdentityIP, Name: "IP", Default: true},
			{ID: topologyNodesIdentityMAC, Name: "MAC"},
		},
	}
}

func topologyMapTypeParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamMapType,
		Name:      "Map",
		Help:      "Choose topology map mode",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      topologyMapTypeLLDPCDPManaged,
				Name:    "LLDP/CDP/Managed Devices Map",
				Default: true,
			},
			{ID: topologyMapTypeHighConfidenceInferred, Name: "High Confidence Inferred Map"},
			{
				ID:   topologyMapTypeAllDevicesLowConfidence,
				Name: "All Devices (Low Confidence)",
			},
		},
	}
}

func topologyInferenceStrategyParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamInferenceStrategy,
		Name:      "Infer Strategy",
		Help:      "Choose the topology inference strategy (experimental modes are internal-testing only)",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      topologyInferenceStrategyFDBMinimumKnowledge,
				Name:    "FDB Minimum-Knowledge (Baseline)",
				Default: true,
			},
			{
				ID:   topologyInferenceStrategySTPParentTree,
				Name: "STP Parent Tree",
			},
			{
				ID:   topologyInferenceStrategyFDBPairwise,
				Name: "FDB Pairwise Minimum-Knowledge",
			},
			{
				ID:   topologyInferenceStrategySTPFDBCorrelated,
				Name: "STP + FDB Correlated",
			},
			{
				ID:   topologyInferenceStrategyCDPFDBHybrid,
				Name: "CDP + FDB Hybrid",
			},
			{
				ID:   topologyInferenceStrategyFDBOverlapWeighted,
				Name: "FDB Overlap Weighted",
			},
			{
				ID:   topologyInferenceStrategyExperimentalFull,
				Name: "Experimental Full (Internal Only)",
			},
		},
	}
}

func topologyManagedFocusParamConfig(extraOptions []funcapi.ParamOption) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+len(extraOptions))
	options = append(options, funcapi.ParamOption{
		ID:      topologyManagedFocusAllDevices,
		Name:    "All Devices",
		Default: true,
	})
	options = append(options, extraOptions...)

	return funcapi.ParamConfig{
		ID:        topologyParamManagedDeviceFocus,
		Name:      "Focus On",
		Help:      "Choose focus root set for depth filtering",
		Selection: funcapi.ParamMultiSelect,
		Options:   options,
	}
}

func topologyDepthParamConfig() funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+(topologyDepthMax-topologyDepthMin+1))
	options = append(options, funcapi.ParamOption{
		ID:      topologyDepthAll,
		Name:    "All",
		Default: true,
	})
	for depth := topologyDepthMin; depth <= topologyDepthMax; depth++ {
		value := strconv.Itoa(depth)
		options = append(options, funcapi.ParamOption{
			ID:   value,
			Name: value,
		},
		)
	}

	return funcapi.ParamConfig{
		ID:        topologyParamDepth,
		Name:      "Focus Depth",
		Help:      "Limit topology expansion hops from focus roots",
		Selection: funcapi.ParamSelect,
		Options:   options,
	}
}

func snmpTopologyPresentation() *topology.Presentation {
	deviceSummaryFields := []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "vendor", Label: "Vendor", Sources: []string{"attributes.vendor", "attributes.vendor_derived"}},
		{Key: "model", Label: "Model", Sources: []string{"attributes.model"}},
		{Key: "sys_descr", Label: "Description", Sources: []string{"attributes.sys_descr", "match.sys_name"}},
		{Key: "sys_location", Label: "Location", Sources: []string{"attributes.sys_location"}},
		{Key: "sys_contact", Label: "Contact", Sources: []string{"attributes.sys_contact"}},
		{Key: "protocols", Label: "Protocols", Sources: []string{"attributes.protocols", "attributes.learned_sources"}},
		{Key: "capabilities", Label: "Capabilities", Sources: []string{"attributes.capabilities"}},
		{Key: "ports_total", Label: "Ports", Sources: []string{"attributes.ports_total"}},
		{Key: "vlan_count", Label: "VLANs", Sources: []string{"attributes.vlan_count"}},
		{Key: "fdb_total_macs", Label: "FDB MACs", Sources: []string{"attributes.fdb_total_macs"}},
		{Key: "lldp_neighbor_count", Label: "LLDP Neighbors", Sources: []string{"attributes.lldp_neighbor_count"}},
		{Key: "cdp_neighbor_count", Label: "CDP Neighbors", Sources: []string{"attributes.cdp_neighbor_count"}},
		{Key: "chart_id_prefix", Label: "Chart Prefix", Sources: []string{"attributes.chart_id_prefix"}},
		{Key: "netdata_host_id", Label: "Netdata Host", Sources: []string{"attributes.netdata_host_id"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}
	deviceTables := map[string]topology.PresentationTable{
		"ports": {
			Label:        "Ports",
			Source:       "data",
			BulletSource: true,
			Order:        1,
			Columns: []topology.PresentationTableColumn{
				{Key: "name", Label: "Port"},
				{Key: "oper_status", Label: "Status", Type: "badge"},
				{Key: "admin_status", Label: "Admin"},
				{Key: "port_type", Label: "Type", Type: "badge"},
				{Key: "link_mode", Label: "Mode", Type: "badge"},
				{Key: "topology_role", Label: "Role", Type: "badge"},
				{Key: "stp_state", Label: "STP", Type: "badge"},
				{Key: "vlan_ids", Label: "VLANs", Type: "count"},
				{Key: "fdb_mac_count", Label: "FDB", Type: "number"},
				{Key: "link_count", Label: "Links", Type: "number"},
				{Key: "neighbor_count", Label: "Neighbors", Type: "number"},
			},
		},
		"links": {
			Label:  "Links",
			Source: "links",
			Order:  2,
			Columns: []topology.PresentationTableColumn{
				{Key: "localPort", Label: "Local Port"},
				{Key: "remoteLabel", Label: "Remote Actor", Type: "actor_link"},
				{Key: "remotePort", Label: "Remote Port"},
				{Key: "protocol", Label: "Protocol"},
				{Key: "direction", Label: "Direction"},
			},
		},
	}
	deviceModalTabs := []topology.PresentationModalTab{
		{ID: "info", Label: "Info"},
		{ID: "flows", Label: "Flows", Type: "flows"},
	}

	segmentSummaryFields := []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "ports_total", Label: "Ports", Sources: []string{"attributes.ports_total"}},
		{Key: "endpoints_total", Label: "Endpoints", Sources: []string{"attributes.endpoints_total"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}

	endpointSummaryFields := []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "vendor", Label: "Vendor", Sources: []string{"attributes.vendor", "attributes.vendor_derived"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}

	infoOnlyTabs := []topology.PresentationModalTab{
		{ID: "info", Label: "Info"},
	}

	linkOnlyTable := map[string]topology.PresentationTable{
		"links": deviceTables["links"],
	}

	deviceType := func(label, colorSlot string) topology.PresentationActorType {
		return topology.PresentationActorType{
			Label:           label,
			ColorSlot:       colorSlot,
			Border:          true,
			SizeByLinks:     true,
			ShowPortBullets: true,
			SummaryFields:   deviceSummaryFields,
			Tables:          deviceTables,
			ModalTabs:       deviceModalTabs,
		}
	}

	return &topology.Presentation{
		ActorTypes: map[string]topology.PresentationActorType{
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
				Label:     "Network segment",
				ColorSlot: "dim",
				SummaryFields: segmentSummaryFields,
				Tables:        linkOnlyTable,
				ModalTabs:     infoOnlyTabs,
			},
			"endpoint": {
				Label:     "Inferred endpoint",
				ColorSlot: "derived",
				Border:    true,
				SummaryFields: endpointSummaryFields,
				Tables:        linkOnlyTable,
				ModalTabs:     infoOnlyTabs,
			},
		},
		LinkTypes: map[string]topology.PresentationLinkType{
			"lldp":    {Label: "LLDP", ColorSlot: "accent", Width: 2},
			"cdp":     {Label: "CDP", ColorSlot: "accent", Width: 2},
			"bridge":  {Label: "Bridge", ColorSlot: "neutral"},
			"fdb":     {Label: "FDB", ColorSlot: "neutral"},
			"stp":     {Label: "STP", ColorSlot: "muted"},
			"arp":     {Label: "ARP", ColorSlot: "muted"},
			"snmp":    {Label: "SNMP", ColorSlot: "primary"},
			"probable": {Label: "Probable", ColorSlot: "dim"},
		},
		PortFields: []topology.PresentationPortField{
			{Key: "type", Label: "Type"},
			{Key: "role", Label: "Role"},
			{Key: "status", Label: "Status"},
			{Key: "mode", Label: "Mode"},
			{Key: "sources", Label: "Sources"},
		},
		PortTypes: map[string]topology.PresentationPortType{
			"lldp":           {Label: "lldp/cdp", ColorSlot: "accent"},
			"switch_facing":  {Label: "switch-facing", ColorSlot: "primary"},
			"host_facing":    {Label: "host-facing", ColorSlot: "secondary"},
			"host_candidate": {Label: "host-candidate", ColorSlot: "info"},
			"trunk":          {Label: "trunk", ColorSlot: "warning"},
			"access":         {Label: "access", ColorSlot: "derived"},
			"topology":       {Label: "unclassified", ColorSlot: "neutral"},
			"idle":           {Label: "idle", ColorSlot: "muted"},
			"unknown":        {Label: "unknown", ColorSlot: "dim"},
		},
		Legend: topology.PresentationLegend{
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
		},
		ActorClickBehavior: "highlight_connections",
	}
}

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP Layer-2 topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: "topology",
		AgentWide:    true,
		RequiredParams: []funcapi.ParamConfig{
			topologyNodesIdentityParamConfig(),
			topologyMapTypeParamConfig(),
			topologyInferenceStrategyParamConfig(),
			topologyManagedFocusParamConfig(nil),
			topologyDepthParamConfig(),
		},
		Presentation: snmpTopologyPresentation(),
	}
}

func (f *funcTopology) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topologyMethodID {
		return nil, nil
	}
	managedFocusOptions := []funcapi.ParamOption(nil)
	if snmpTopologyRegistry != nil {
		for _, target := range snmpTopologyRegistry.managedDeviceFocusTargets() {
			if strings.TrimSpace(target.Value) == "" {
				continue
			}
			managedFocusOptions = append(managedFocusOptions, funcapi.ParamOption{
				ID:   target.Value,
				Name: target.Name,
			})
		}
	}
	return []funcapi.ParamConfig{
		topologyNodesIdentityParamConfig(),
		topologyMapTypeParamConfig(),
		topologyInferenceStrategyParamConfig(),
		topologyManagedFocusParamConfig(managedFocusOptions),
		topologyDepthParamConfig(),
	}, nil
}

func (f *funcTopology) Cleanup(_ context.Context) {}

func (f *funcTopology) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}

	options := topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
	}
	if identity := normalizeTopologyNodesIdentity(params.GetOne(topologyParamNodesIdentity)); identity != "" {
		if identity == topologyNodesIdentityMAC {
			options.CollapseActorsByIP = false
			options.EliminateNonIPInferred = false
		}
	}
	if mapType := normalizeTopologyMapType(params.GetOne(topologyParamMapType)); mapType != "" {
		options.MapType = mapType
	}
	if strategy := normalizeTopologyInferenceStrategy(params.GetOne(topologyParamInferenceStrategy)); strategy != "" {
		options.InferenceStrategy = strategy
	}
	if focuses := normalizeTopologyManagedFocuses(params.Get(topologyParamManagedDeviceFocus)); len(focuses) > 0 {
		options.ManagedDeviceFocus = formatTopologyManagedFocuses(focuses)
	}
	options.Depth = normalizeTopologyDepth(params.GetOne(topologyParamDepth))

	if snmpTopologyRegistry == nil {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	data, ok := snmpTopologyRegistry.snapshotWithOptions(options)

	if !ok {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "SNMP topology and neighbor discovery data",
		ResponseType: "topology",
		Data:         data,
	}
}

func normalizeTopologyNodesIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyNodesIdentityIP:
		return topologyNodesIdentityIP
	case topologyNodesIdentityMAC:
		return topologyNodesIdentityMAC
	default:
		return ""
	}
}

func normalizeTopologyMapType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyMapTypeLLDPCDPManaged:
		return topologyMapTypeLLDPCDPManaged
	case topologyMapTypeHighConfidenceInferred:
		return topologyMapTypeHighConfidenceInferred
	case topologyMapTypeAllDevicesLowConfidence:
		return topologyMapTypeAllDevicesLowConfidence
	default:
		return ""
	}
}

func normalizeTopologyInferenceStrategy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyInferenceStrategyFDBMinimumKnowledge:
		return topologyInferenceStrategyFDBMinimumKnowledge
	case topologyInferenceStrategySTPParentTree:
		return topologyInferenceStrategySTPParentTree
	case topologyInferenceStrategyFDBPairwise:
		return topologyInferenceStrategyFDBPairwise
	case topologyInferenceStrategySTPFDBCorrelated:
		return topologyInferenceStrategySTPFDBCorrelated
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyCDPFDBHybrid
	case topologyInferenceStrategyFDBOverlapWeighted:
		return topologyInferenceStrategyFDBOverlapWeighted
	case topologyInferenceStrategyExperimentalFull:
		return topologyInferenceStrategyExperimentalFull
	default:
		return ""
	}
}

func normalizeTopologyManagedFocus(v string) string {
	value := strings.TrimSpace(v)
	if value == "" {
		return topologyManagedFocusAllDevices
	}
	return normalizeTopologyManagedFocusValue(value)
}

func normalizeTopologyManagedFocusValue(v string) string {
	value := strings.TrimSpace(v)
	switch strings.ToLower(value) {
	case topologyManagedFocusAllDevices:
		return topologyManagedFocusAllDevices
	}
	if len(value) > len(topologyManagedFocusIPPrefix) &&
		strings.EqualFold(value[:len(topologyManagedFocusIPPrefix)], topologyManagedFocusIPPrefix) {
		ip := normalizeIPAddress(strings.TrimSpace(value[len(topologyManagedFocusIPPrefix):]))
		if ip == "" {
			return ""
		}
		return topologyManagedFocusIPPrefix + ip
	}
	return ""
}

func normalizeTopologyManagedFocuses(values []string) []string {
	expanded := splitTopologyManagedFocusValues(values)
	if len(expanded) == 0 {
		return []string{topologyManagedFocusAllDevices}
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, raw := range expanded {
		normalized := normalizeTopologyManagedFocusValue(raw)
		if normalized == "" {
			continue
		}
		if normalized == topologyManagedFocusAllDevices {
			return []string{topologyManagedFocusAllDevices}
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return []string{topologyManagedFocusAllDevices}
	}
	sort.Strings(out)
	return out
}

func splitTopologyManagedFocusValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, raw := range values {
		for _, token := range strings.Split(raw, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

func parseTopologyManagedFocuses(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{topologyManagedFocusAllDevices}
	}
	return normalizeTopologyManagedFocuses(strings.Split(value, ","))
}

func formatTopologyManagedFocuses(values []string) string {
	normalized := normalizeTopologyManagedFocuses(values)
	if len(normalized) == 0 {
		return topologyManagedFocusAllDevices
	}
	return strings.Join(normalized, ",")
}

func isTopologyManagedFocusAllDevices(value string) bool {
	normalized := parseTopologyManagedFocuses(value)
	return len(normalized) == 1 && normalized[0] == topologyManagedFocusAllDevices
}

func normalizeTopologyDepth(v string) int {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" || value == topologyDepthAll {
		return topologyDepthAllInternal
	}
	depth, err := strconv.Atoi(value)
	if err != nil {
		return topologyDepthAllInternal
	}
	if depth < topologyDepthMin {
		return topologyDepthMin
	}
	if depth > topologyDepthMax {
		return topologyDepthMax
	}
	return depth
}

func isTopologyMapTypeProbable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyMapTypeAllDevicesLowConfidence:
		return true
	default:
		return false
	}
}
