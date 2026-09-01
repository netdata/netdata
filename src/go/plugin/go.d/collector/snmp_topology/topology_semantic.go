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

type topologySemanticDeviceInput struct {
	hostname    string
	sysObjectID string
	sysName     string
	sysDescr    string
	sysContact  string
	sysLocation string
	vendor      string
	model       string
	vnodeGUID   string
	vnodeLabels map[string]string
}

func topologySemanticDeviceInputFromConnection(dev ddsnmp.DeviceConnectionInfo) topologySemanticDeviceInput {
	return topologySemanticDeviceInput{
		hostname:    dev.Hostname,
		sysObjectID: dev.SysObjectID,
		sysName:     dev.SysName,
		sysDescr:    dev.SysDescr,
		sysContact:  dev.SysContact,
		sysLocation: dev.SysLocation,
		vendor:      dev.Vendor,
		model:       dev.Model,
		vnodeGUID:   dev.VnodeGUID,
		vnodeLabels: dev.VnodeLabels,
	}
}

func (d topologySemanticDeviceInput) connectionInfo() ddsnmp.DeviceConnectionInfo {
	return ddsnmp.DeviceConnectionInfo{
		Hostname:    d.hostname,
		SysObjectID: d.sysObjectID,
		SysName:     d.sysName,
		SysDescr:    d.sysDescr,
		SysContact:  d.sysContact,
		SysLocation: d.sysLocation,
		Vendor:      d.vendor,
		Model:       d.model,
		VnodeGUID:   d.vnodeGUID,
		VnodeLabels: d.vnodeLabels,
	}
}

func newTopologyBuilderFromSemanticInput(
	device topologySemanticDeviceInput,
	targets []netip.Addr,
	collectedAt time.Time,
	freshFor time.Duration,
) *topologyBuilder {
	builder := newTopologyBuilder()
	builder.updateTime = collectedAt
	builder.staleAfter = freshFor
	builder.agentID = device.hostname
	builder.localDevice = buildLocalTopologyDevice(device.connectionInfo())
	builder.targetManagementIPs = slices.Clone(targets)
	return builder
}

type topologyAcquisitionLimits struct {
	maxRecords      uint64
	maxLogicalBytes uint64
}

var defaultTopologyAcquisitionLimits = topologyAcquisitionLimits{
	maxRecords:      100_000,
	maxLogicalBytes: 32 << 20,
}

var defaultTopologyDiagnosticGlobalLimits = topologyAcquisitionLimits{
	maxRecords:      250_000,
	maxLogicalBytes: 64 << 20,
}

type diagnosticCaptureState uint8

const (
	diagnosticCaptureUnknown diagnosticCaptureState = iota
	diagnosticCaptureAvailable
	diagnosticCaptureLimitExceeded
	diagnosticCaptureUnavailable
)

type diagnosticCaptureReason uint8

const (
	diagnosticCaptureReasonNone diagnosticCaptureReason = iota
	diagnosticCaptureReasonRecordLimit
	diagnosticCaptureReasonByteLimit
	diagnosticCaptureReasonProjectionError
	diagnosticCaptureReasonProjectionPanic
	diagnosticCaptureReasonGlobalRecordLimit
	diagnosticCaptureReasonGlobalByteLimit
)

type topologyAcquisitionProfileValues struct {
	metadata  map[string]ddsnmp.MetaTag
	tags      map[string]string
	metrics   []topologyAcquisitionMetricValue
	bgpRows   []topologyAcquisitionBGPRowValue
	bgpFailed bool
}

type topologyAcquisitionMetricValue struct {
	routeOrdinal uint32
	rowOrdinal   uint32
	valueOrdinal uint32
	kind         ddsnmp.TopologyKind
	tags         map[string]string
}

type topologyAcquisitionBGPRowValue struct {
	routeOrdinal uint32
	rowOrdinal   uint32
	valueOrdinal uint32

	originProfileID string
	table           string
	rowKey          string
	structuralID    string
	kind            ddprofiledefinition.BGPRowKind

	routingInstance string
	neighbor        string
	remoteAS        string
	localAddress    string
	localAS         string
	localIdentifier string
	peerIdentifier  string
	peerType        string
	bgpVersion      string
	description     string

	adminHas       bool
	adminEnabled   bool
	stateHas       bool
	state          ddprofiledefinition.BGPPeerState
	stateRaw       string
	establishedHas bool
	established    int64
	updateAgeHas   bool
	updateAge      int64
	tags           map[string]string
}

func projectTopologyAcquisitionMetrics(
	eventKind topologySemanticEventKind,
	metrics []ddsnmp.Metric,
	references []ddsnmpcollector.AcquisitionValueReference,
) []topologyAcquisitionMetricValue {
	result := make([]topologyAcquisitionMetricValue, 0, len(metrics))
	for i, metric := range metrics {
		if !topologySemanticMetricConsumed(eventKind, metric.TopologyKind) {
			continue
		}
		result = append(result, topologyAcquisitionMetricValue{
			routeOrdinal: references[i].RouteOrdinal,
			rowOrdinal:   references[i].RowOrdinal,
			valueOrdinal: references[i].ValueOrdinal,
			kind:         metric.TopologyKind,
			tags: cloneTopologySemanticStringTags(
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
) []topologyAcquisitionBGPRowValue {
	result := make([]topologyAcquisitionBGPRowValue, 0, len(rows))
	for i, row := range rows {
		result = append(result, topologyAcquisitionBGPRowValue{
			routeOrdinal:    references[i].RouteOrdinal,
			rowOrdinal:      references[i].RowOrdinal,
			valueOrdinal:    references[i].ValueOrdinal,
			originProfileID: strings.Clone(row.OriginProfileID),
			table:           strings.Clone(row.Table),
			rowKey:          strings.Clone(row.RowKey),
			structuralID:    strings.Clone(row.StructuralID),
			kind:            row.Kind,
			routingInstance: strings.Clone(row.Identity.RoutingInstance),
			neighbor:        strings.Clone(row.Identity.Neighbor),
			remoteAS:        strings.Clone(row.Identity.RemoteAS),
			localAddress:    strings.Clone(row.Descriptors.LocalAddress),
			localAS:         strings.Clone(row.Descriptors.LocalAS),
			localIdentifier: strings.Clone(row.Descriptors.LocalIdentifier),
			peerIdentifier:  strings.Clone(row.Descriptors.PeerIdentifier),
			peerType:        strings.Clone(row.Descriptors.PeerType),
			bgpVersion:      strings.Clone(row.Descriptors.BGPVersion),
			description:     strings.Clone(row.Descriptors.Description),
			adminHas:        row.Admin.Enabled.Has,
			adminEnabled:    row.Admin.Enabled.Value,
			stateHas:        row.State.Has,
			state:           ddprofiledefinition.BGPPeerState(strings.Clone(string(row.State.State))),
			stateRaw:        strings.Clone(row.State.Raw),
			establishedHas:  row.Connection.EstablishedUptime.Has,
			established:     row.Connection.EstablishedUptime.Value,
			updateAgeHas:    row.Connection.LastReceivedUpdateAge.Has,
			updateAge:       row.Connection.LastReceivedUpdateAge.Value,
			tags:            cloneTopologySemanticStringTags(row.Tags, topologySemanticBGPTagAllowed),
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

func cloneTopologySemanticDeviceInput(value topologySemanticDeviceInput) topologySemanticDeviceInput {
	value.hostname = strings.Clone(value.hostname)
	value.sysObjectID = strings.Clone(value.sysObjectID)
	value.sysName = strings.Clone(value.sysName)
	value.sysDescr = strings.Clone(value.sysDescr)
	value.sysContact = strings.Clone(value.sysContact)
	value.sysLocation = strings.Clone(value.sysLocation)
	value.vendor = strings.Clone(value.vendor)
	value.model = strings.Clone(value.model)
	value.vnodeGUID = strings.Clone(value.vnodeGUID)
	value.vnodeLabels = cloneTopologySemanticStringMap(value.vnodeLabels)
	return value
}

func topologySemanticDeviceLogicalBytes(value topologySemanticDeviceInput) uint64 {
	return uint64(len(value.hostname)+len(value.sysObjectID)+len(value.sysName)+len(value.sysDescr)+
		len(value.sysContact)+len(value.sysLocation)+len(value.vendor)+len(value.model)+len(value.vnodeGUID)) +
		topologySemanticStringMapBytes(value.vnodeLabels)
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
		case tagTopoIfIndex, tagTopoIPAddr, tagTopoIPMask:
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
