// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net/netip"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

type topologySemanticEventKind uint8

const (
	topologySemanticEventUnknown topologySemanticEventKind = iota
	topologySemanticEventSysUptime
	topologySemanticEventProfileTags
	topologySemanticEventTopologyMetrics
	topologySemanticEventBGPPeers
	topologySemanticEventVLANContext
)

type topologySemanticEvent struct {
	kind      topologySemanticEventKind
	sysUptime int64
	profiles  []*ddsnmp.ProfileMetrics
	vlanID    string
	vlanName  string
}

func applyTopologySemanticEvent(builder *topologyBuilder, event topologySemanticEvent) {
	if builder == nil {
		return
	}
	switch event.kind {
	case topologySemanticEventSysUptime:
		builder.updateTopologySysUptime(event.sysUptime)
	case topologySemanticEventProfileTags:
		builder.updateTopologyProfileTags(event.profiles)
	case topologySemanticEventTopologyMetrics:
		builder.ingestTopologyProfileMetrics(event.profiles)
	case topologySemanticEventBGPPeers:
		builder.ingestTopologyBGPPeers(event.profiles)
	case topologySemanticEventVLANContext:
		builder.ingestTopologyVLANContextMetrics(event.vlanID, event.vlanName, event.profiles)
	}
}

func topologyDeviceInputFromConnection(dev ddsnmp.DeviceConnectionInfo) topologydiag.DeviceInput {
	return topologydiag.DeviceInput{
		Hostname:    dev.Hostname,
		SysObjectID: dev.SysObjectID,
		SysName:     dev.SysName,
		SysDescr:    dev.SysDescr,
		SysContact:  dev.SysContact,
		SysLocation: dev.SysLocation,
		Vendor:      dev.Vendor,
		Model:       dev.Model,
		VnodeGUID:   dev.VnodeGUID,
		VnodeLabels: dev.VnodeLabels,
	}
}

func newTopologyBuilderFromSemanticInput(
	device topologydiag.DeviceInput,
	targets []netip.Addr,
	collectedAt time.Time,
	freshFor time.Duration,
) *topologyBuilder {
	builder := newTopologyBuilder()
	builder.updateTime = collectedAt
	builder.staleAfter = freshFor
	builder.agentID = device.Hostname
	builder.localDevice = buildLocalTopologyDevice(device.ConnectionInfo())
	builder.targetManagementIPs = slices.Clone(targets)
	return builder
}

func projectTopologyAcquisitionMetrics(
	eventKind topologySemanticEventKind,
	metrics []ddsnmp.Metric,
	references []ddsnmpcollector.AcquisitionValueReference,
) []topologydiag.AcquisitionMetricValue {
	result := make([]topologydiag.AcquisitionMetricValue, 0, len(metrics))
	for i, metric := range metrics {
		if !topologySemanticMetricConsumed(eventKind, metric.TopologyKind) {
			continue
		}
		result = append(result, topologydiag.AcquisitionMetricValue{
			RowIndex:     strings.Clone(references[i].RowIndex),
			Field:        strings.Clone(references[i].Field),
			RouteOrdinal: references[i].RouteOrdinal,
			RowOrdinal:   references[i].RowOrdinal,
			ValueOrdinal: references[i].ValueOrdinal,
			Kind:         metric.TopologyKind,
			Tags: cloneTopologySemanticStringTags(
				metric.Tags,
				func(key string) bool { return topologySemanticMetricTagAllowed(metric.TopologyKind, key) },
			),
		})
	}
	return result
}

func projectTopologyAcquisitionBGPRows(
	rows []ddsnmp.BGPRow,
	references []ddsnmpcollector.AcquisitionValueReference,
) []topologydiag.AcquisitionBGPRowValue {
	result := make([]topologydiag.AcquisitionBGPRowValue, 0, len(rows))
	for i, row := range rows {
		result = append(result, topologydiag.AcquisitionBGPRowValue{
			RouteOrdinal:    references[i].RouteOrdinal,
			RowOrdinal:      references[i].RowOrdinal,
			ValueOrdinal:    references[i].ValueOrdinal,
			OriginProfileID: strings.Clone(row.OriginProfileID),
			Table:           strings.Clone(row.Table),
			RowKey:          strings.Clone(row.RowKey),
			StructuralID:    strings.Clone(row.StructuralID),
			Kind:            row.Kind,
			RoutingInstance: strings.Clone(row.Identity.RoutingInstance),
			Neighbor:        strings.Clone(row.Identity.Neighbor),
			RemoteAS:        strings.Clone(row.Identity.RemoteAS),
			LocalAddress:    strings.Clone(row.Descriptors.LocalAddress),
			LocalAS:         strings.Clone(row.Descriptors.LocalAS),
			LocalIdentifier: strings.Clone(row.Descriptors.LocalIdentifier),
			PeerIdentifier:  strings.Clone(row.Descriptors.PeerIdentifier),
			PeerType:        strings.Clone(row.Descriptors.PeerType),
			BGPVersion:      strings.Clone(row.Descriptors.BGPVersion),
			Description:     strings.Clone(row.Descriptors.Description),
			AdminHas:        row.Admin.Enabled.Has,
			AdminEnabled:    row.Admin.Enabled.Value,
			StateHas:        row.State.Has,
			State:           ddprofiledefinition.BGPPeerState(strings.Clone(string(row.State.State))),
			StateRaw:        strings.Clone(row.State.Raw),
			EstablishedHas:  row.Connection.EstablishedUptime.Has,
			Established:     row.Connection.EstablishedUptime.Value,
			UpdateAgeHas:    row.Connection.LastReceivedUpdateAge.Has,
			UpdateAge:       row.Connection.LastReceivedUpdateAge.Value,
			Tags:            cloneTopologySemanticStringTags(row.Tags, topologySemanticBGPTagAllowed),
		})
	}
	return result
}

func portableTopologySemanticOrigin(value string) bool {
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, `\:`) || path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for component := range strings.SplitSeq(value, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func cloneTopologyDeviceInput(value topologydiag.DeviceInput) topologydiag.DeviceInput {
	value.Hostname = strings.Clone(value.Hostname)
	value.SysObjectID = strings.Clone(value.SysObjectID)
	value.SysName = strings.Clone(value.SysName)
	value.SysDescr = strings.Clone(value.SysDescr)
	value.SysContact = strings.Clone(value.SysContact)
	value.SysLocation = strings.Clone(value.SysLocation)
	value.Vendor = strings.Clone(value.Vendor)
	value.Model = strings.Clone(value.Model)
	value.VnodeGUID = strings.Clone(value.VnodeGUID)
	value.VnodeLabels = cloneTopologySemanticStringMap(value.VnodeLabels)
	return value
}

func topologySemanticDeviceLogicalBytes(value topologydiag.DeviceInput) uint64 {
	return uint64(len(value.Hostname)+len(value.SysObjectID)+len(value.SysName)+len(value.SysDescr)+
		len(value.SysContact)+len(value.SysLocation)+len(value.Vendor)+len(value.Model)+len(value.VnodeGUID)) +
		topologySemanticStringMapBytes(value.VnodeLabels)
}

func topologySemanticStringMapBytes(values map[string]string) uint64 {
	var total uint64
	for key, value := range values {
		total += uint64(len(key) + len(value))
	}
	return total
}

func topologySemanticFilteredStringMapBytes(values map[string]string, allowed func(string) bool) uint64 {
	var total uint64
	for key, value := range values {
		if allowed(key) {
			total += uint64(len(key) + len(value))
		}
	}
	return total
}

func topologySemanticFilteredMetaTagMapBytes(values map[string]ddsnmp.MetaTag, allowed func(string) bool) uint64 {
	var total uint64
	for key, value := range values {
		if allowed(key) {
			total += uint64(len(key) + len(value.Value) + 1)
		}
	}
	return total
}

func topologySemanticBGPRowLogicalBytes(row ddsnmp.BGPRow) uint64 {
	return uint64(len(row.OriginProfileID)+len(row.Table)+len(row.RowKey)+len(row.StructuralID)+len(row.Kind)+
		len(row.Identity.RoutingInstance)+len(row.Identity.Neighbor)+len(row.Identity.RemoteAS)+
		len(row.Descriptors.LocalAddress)+len(row.Descriptors.LocalAS)+len(row.Descriptors.LocalIdentifier)+
		len(row.Descriptors.PeerIdentifier)+len(row.Descriptors.PeerType)+len(row.Descriptors.BGPVersion)+
		len(row.Descriptors.Description)+len(row.State.State)+len(row.State.Raw)+19) +
		topologySemanticFilteredStringMapBytes(row.Tags, topologySemanticBGPTagAllowed)
}

func cloneTopologySemanticStringTags(values map[string]string, allowed func(string) bool) map[string]string {
	var result map[string]string
	for key, value := range values {
		if !allowed(key) {
			continue
		}
		if result == nil {
			result = make(map[string]string)
		}
		result[key] = strings.Clone(value)
	}
	return result
}

func cloneTopologySemanticStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[strings.Clone(key)] = strings.Clone(value)
	}
	return result
}

func cloneTopologySemanticMetaTags(values map[string]ddsnmp.MetaTag, allowed func(string) bool) map[string]ddsnmp.MetaTag {
	var result map[string]ddsnmp.MetaTag
	for key, value := range values {
		if !allowed(key) {
			continue
		}
		if result == nil {
			result = make(map[string]ddsnmp.MetaTag)
		}
		value.Value = strings.Clone(value.Value)
		result[key] = value
	}
	return result
}

func topologySemanticProfileMetadataAllowed(key string) bool {
	if key == "vendor" || key == "model" {
		return true
	}
	return topologySemanticProfileTagAllowed(key)
}

func topologySemanticProfileTagAllowed(key string) bool {
	switch key {
	case tagLldpLocChassisID, tagLldpLocChassisIDSubtype,
		tagLldpLocSysName, tagLldpLocSysDesc,
		tagLldpLocSysCapSupported, tagLldpLocSysCapEnabled,
		tagBridgeBaseAddress, tagOSPFRouterID:
		return true
	default:
		return false
	}
}

func topologySemanticMetricConsumed(eventKind topologySemanticEventKind, kind ddsnmp.TopologyKind) bool {
	if eventKind == topologySemanticEventVLANContext {
		return isTopologyVLANContextMetric(kind)
	}
	return eventKind == topologySemanticEventTopologyMetrics
}

func topologySemanticMetricTagAllowed(kind ddsnmp.TopologyKind, key string) bool {
	switch kind {
	case ddsnmp.KindIfName, ddsnmp.KindIfStatus, ddsnmp.KindIfDuplex:
		switch key {
		case
			tagTopoIfIndex, tagTopoIfName, tagTopoIfType, tagTopoIfAdmin, tagTopoIfOper,
			tagTopoIfPhys, tagTopoIfDescr, tagTopoIfAlias, tagTopoIfSpeed, tagTopoIfHigh,
			tagTopoIfLast, tagTopoIfDuplex:
			return true
		}
	case ddsnmp.KindIpIfIndex:
		switch key {
		case tagTopoIfIndex, tagTopoIPAddr, tagTopoIPMask, tagTopoIPSource,
			tagTopoIPType, tagTopoIPPrefix, tagTopoIPStatus, tagTopoIPRow:
			return true
		}
	case ddsnmp.KindBridgePortIfIndex:
		switch key {
		case tagBridgeBasePort, tagBridgeIfIndex:
			return true
		}
	case ddsnmp.KindLldpLocPort:
		switch key {
		case tagLldpLocPortNum, tagLldpLocPortID, tagLldpLocPortIDSubtype, tagLldpLocPortDesc:
			return true
		}
	case ddsnmp.KindLldpLocManAddr:
		switch key {
		case tagLldpLocMgmtAddr, tagLldpLocMgmtAddrSubtype,
			tagLldpLocMgmtAddrIfSubtype, tagLldpLocMgmtAddrIfID, tagLldpLocMgmtAddrOID:
			return true
		}
	case ddsnmp.KindLldpRem:
		switch key {
		case
			tagLldpLocPortNum, tagLldpRemIndex, tagLldpRemChassisID, tagLldpRemChassisIDSubtype,
			tagLldpRemPortID, tagLldpRemPortIDSubtype, tagLldpRemPortDesc, tagLldpRemSysName,
			tagLldpRemSysDesc, tagLldpRemSysCapSupported, tagLldpRemSysCapEnabled,
			tagLldpRemMgmtAddr, tagLldpRemMgmtAddrSubtype:
			return true
		}
	case ddsnmp.KindLldpRemManAddr:
		switch key {
		case
			tagLldpLocPortNum, tagLldpRemIndex, tagLldpRemMgmtAddr, tagLldpRemMgmtAddrSubtype,
			tagLldpRemMgmtAddrLen, tagLldpRemMgmtAddrIfSubtype, tagLldpRemMgmtAddrIfID,
			tagLldpRemMgmtAddrOID:
			return true
		}
	case ddsnmp.KindCdpCache:
		switch key {
		case
			tagCdpIfIndex, tagCdpIfName, tagCdpDeviceIndex, tagCdpDeviceID, tagCdpAddressType,
			tagCdpDevicePort, tagCdpVersion, tagCdpPlatform, tagCdpCaps, tagCdpAddress,
			tagCdpVTPDomain, tagCdpNativeVLAN, tagCdpDuplex, tagCdpPower, tagCdpMTU,
			tagCdpSysName, tagCdpSysObjectID, tagCdpPrimaryMgmtAddrType, tagCdpPrimaryMgmtAddr,
			tagCdpSecondaryMgmtAddrType, tagCdpSecondaryMgmtAddr, tagCdpPhysicalLocation,
			tagCdpLastChange:
			return true
		}
	case ddsnmp.KindFdbEntry, ddsnmp.KindQbridgeFdbEntry:
		switch key {
		case
			tagFdbMac, tagFdbBridgePort, tagFdbStatus, tagDot1qFdbID, tagDot1qFdbMac,
			tagDot1qFdbPort, tagDot1qFdbStatus, tagTopologyContextVLANID, tagTopologyContextVLANName:
			return true
		}
	case ddsnmp.KindQbridgeVlanEntry:
		switch key {
		case tagDot1qVlanID, tagDot1qVlanID1, tagDot1qVlanFdbID:
			return true
		}
	case ddsnmp.KindStpPort:
		switch key {
		case
			tagStpPort, tagStpPortPriority, tagStpPortState, tagStpPortEnable,
			tagStpPortPathCost, tagStpPortDesignatedRoot, tagStpPortDesignatedCost,
			tagStpPortDesignatedBridge, tagStpPortDesignatedPort,
			tagTopologyContextVLANID, tagTopologyContextVLANName:
			return true
		}
	case ddsnmp.KindVtpVlan:
		switch key {
		case tagVtpVlanIndex, tagVtpVlanState, tagVtpVlanType, tagVtpVlanName:
			return true
		}
	case ddsnmp.KindArpEntry, ddsnmp.KindArpLegacyEntry:
		switch key {
		case tagArpIfIndex, tagArpIfName, tagArpIP, tagArpMac, tagArpType, tagArpState, tagArpAddrType:
			return true
		}
	case ddsnmp.KindOSPFNeighbor:
		switch key {
		case tagOSPFNeighborIP, tagOSPFNeighborAddresslessIndex, tagOSPFNeighborRouterID, tagOSPFNeighborState:
			return true
		}
	}
	return false
}

func topologySemanticBGPTagAllowed(key string) bool {
	switch key {
	case "neighbor", "remote_as", "routing_instance":
		return true
	default:
		return false
	}
}
