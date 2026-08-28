// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"maps"
	"net/netip"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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

func consumeTopologySemanticEvent(builder *topologyBuilder, recorder *topologySemanticRecorder, event topologySemanticEvent) {
	applyTopologySemanticEvent(builder, event)
	if recorder != nil {
		recorder.record(event)
	}
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

type topologySemanticLimits struct {
	maxRecords      uint64
	maxLogicalBytes uint64
}

var defaultTopologySemanticLimits = topologySemanticLimits{
	maxRecords:      100_000,
	maxLogicalBytes: 32 << 20,
}

var defaultTopologyDiagnosticGlobalLimits = topologySemanticLimits{
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

type topologySemanticCapture struct {
	state        diagnosticCaptureState
	reason       diagnosticCaptureReason
	recordCount  uint64
	logicalBytes uint64
	evidence     *topologySemanticEvidence
}

type topologySemanticEvidence struct {
	device      topologySemanticDeviceInput
	targets     []netip.Addr
	collectedAt time.Time
	freshFor    time.Duration
	events      []topologySemanticEventEvidence
}

type topologySemanticEventEvidence struct {
	kind      topologySemanticEventKind
	sysUptime int64
	profiles  []topologySemanticProfileEvidence
	vlanID    string
	vlanName  string
}

type topologySemanticProfileEvidence struct {
	metadata  map[string]ddsnmp.MetaTag
	tags      map[string]string
	metrics   []topologySemanticMetricEvidence
	bgpRows   []topologySemanticBGPRowEvidence
	bgpFailed bool
}

type topologySemanticMetricEvidence struct {
	kind ddsnmp.TopologyKind
	tags map[string]string
}

type topologySemanticBGPRowEvidence struct {
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

type topologySemanticEventProjector func(*topologySemanticRecorder, topologySemanticEvent) error

type topologySemanticRecorder struct {
	limits       topologySemanticLimits
	state        diagnosticCaptureState
	reason       diagnosticCaptureReason
	recordCount  uint64
	logicalBytes uint64
	evidence     *topologySemanticEvidence
	projectEvent topologySemanticEventProjector
}

func newTopologySemanticRecorder(
	device topologySemanticDeviceInput,
	targets []netip.Addr,
	collectedAt time.Time,
	freshFor time.Duration,
	limits topologySemanticLimits,
) (r *topologySemanticRecorder) {
	r = &topologySemanticRecorder{
		limits:       limits,
		state:        diagnosticCaptureAvailable,
		projectEvent: projectTopologySemanticEvent,
	}
	defer func() {
		if recover() != nil {
			r.fail(diagnosticCaptureReasonProjectionPanic)
		}
	}()
	records := uint64(1 + len(targets))
	logicalBytes := topologySemanticDeviceLogicalBytes(device)
	for _, target := range targets {
		logicalBytes += uint64(len(target.String()))
	}
	if !r.admit(records, logicalBytes) {
		return r
	}
	r.evidence = &topologySemanticEvidence{
		device:      cloneTopologySemanticDeviceInput(device),
		targets:     slices.Clone(targets),
		collectedAt: collectedAt,
		freshFor:    freshFor,
	}
	return r
}

func (r *topologySemanticRecorder) record(event topologySemanticEvent) {
	if r == nil || r.state != diagnosticCaptureAvailable {
		return
	}
	defer func() {
		if recover() != nil {
			r.fail(diagnosticCaptureReasonProjectionPanic)
		}
	}()
	if r.projectEvent == nil {
		r.fail(diagnosticCaptureReasonProjectionError)
		return
	}
	if err := r.projectEvent(r, event); err != nil {
		r.fail(diagnosticCaptureReasonProjectionError)
	}
}

func (r *topologySemanticRecorder) finish() topologySemanticCapture {
	if r == nil {
		return topologySemanticCapture{state: diagnosticCaptureUnavailable}
	}
	return topologySemanticCapture{
		state:        r.state,
		reason:       r.reason,
		recordCount:  r.recordCount,
		logicalBytes: r.logicalBytes,
		evidence:     r.evidence,
	}
}

func (r *topologySemanticRecorder) admit(records, logicalBytes uint64) bool {
	if records > r.limits.maxRecords-r.recordCount {
		r.limit(diagnosticCaptureReasonRecordLimit)
		return false
	}
	if logicalBytes > r.limits.maxLogicalBytes-r.logicalBytes {
		r.limit(diagnosticCaptureReasonByteLimit)
		return false
	}
	r.recordCount += records
	r.logicalBytes += logicalBytes
	return true
}

func (r *topologySemanticRecorder) limit(reason diagnosticCaptureReason) {
	r.state = diagnosticCaptureLimitExceeded
	r.reason = reason
	r.evidence = nil
}

func (r *topologySemanticRecorder) fail(reason diagnosticCaptureReason) {
	r.state = diagnosticCaptureUnavailable
	r.reason = reason
	r.evidence = nil
}

func projectTopologySemanticEvent(r *topologySemanticRecorder, event topologySemanticEvent) error {
	records, logicalBytes, err := topologySemanticEventShape(event)
	if err != nil {
		return err
	}
	if !r.admit(records, logicalBytes) {
		return nil
	}

	projected := topologySemanticEventEvidence{
		kind:      event.kind,
		sysUptime: event.sysUptime,
		vlanID:    event.vlanID,
		vlanName:  event.vlanName,
		profiles:  make([]topologySemanticProfileEvidence, 0, len(event.profiles)),
	}
	for _, profile := range event.profiles {
		projected.profiles = append(projected.profiles, projectTopologySemanticProfile(event.kind, profile))
	}
	r.evidence.events = append(r.evidence.events, projected)
	return nil
}

func topologySemanticEventShape(event topologySemanticEvent) (uint64, uint64, error) {
	switch event.kind {
	case topologySemanticEventSysUptime,
		topologySemanticEventProfileTags,
		topologySemanticEventTopologyMetrics,
		topologySemanticEventBGPPeers,
		topologySemanticEventVLANContext:
	default:
		return 0, 0, errors.New("unknown semantic event kind")
	}
	records := uint64(1)
	logicalBytes := uint64(len(event.vlanID) + len(event.vlanName) + 8)
	for _, profile := range event.profiles {
		records++
		if profile == nil {
			continue
		}
		switch event.kind {
		case topologySemanticEventProfileTags:
			logicalBytes += topologySemanticFilteredMetaTagMapBytes(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed)
			logicalBytes += topologySemanticFilteredStringMapBytes(profile.Tags, topologySemanticProfileTagAllowed)
		case topologySemanticEventTopologyMetrics, topologySemanticEventVLANContext:
			if event.kind == topologySemanticEventVLANContext {
				logicalBytes += topologySemanticFilteredMetaTagMapBytes(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed)
				logicalBytes += topologySemanticFilteredStringMapBytes(profile.Tags, topologySemanticProfileTagAllowed)
			}
			for _, metric := range profile.TopologyMetrics {
				if !topologySemanticMetricConsumed(event.kind, metric.TopologyKind) {
					continue
				}
				records++
				logicalBytes += uint64(len(metric.TopologyKind)) + topologySemanticFilteredStringMapBytes(
					metric.Tags,
					func(key string) bool { return topologySemanticMetricTagAllowed(metric.TopologyKind, key) },
				)
			}
		case topologySemanticEventBGPPeers:
			if profile.BGPCollectError != nil {
				continue
			}
			records += uint64(len(profile.BGPRows))
			for _, row := range profile.BGPRows {
				if !portableTopologySemanticOrigin(row.OriginProfileID) {
					return 0, 0, errors.New("non-portable BGP origin profile ID")
				}
				logicalBytes += topologySemanticBGPRowLogicalBytes(row)
			}
		}
	}
	return records, logicalBytes, nil
}

func projectTopologySemanticProfile(kind topologySemanticEventKind, profile *ddsnmp.ProfileMetrics) topologySemanticProfileEvidence {
	if profile == nil {
		return topologySemanticProfileEvidence{}
	}
	result := topologySemanticProfileEvidence{}
	switch kind {
	case topologySemanticEventProfileTags:
		result.metadata = cloneTopologySemanticMetaTags(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed)
		result.tags = cloneTopologySemanticStringTags(profile.Tags, topologySemanticProfileTagAllowed)
	case topologySemanticEventTopologyMetrics:
		result.metrics = projectTopologySemanticMetrics(kind, profile.TopologyMetrics)
	case topologySemanticEventBGPPeers:
		result.bgpFailed = profile.BGPCollectError != nil
		if !result.bgpFailed {
			result.bgpRows = projectTopologySemanticBGPRows(profile.BGPRows)
		}
	case topologySemanticEventVLANContext:
		result.metadata = cloneTopologySemanticMetaTags(profile.DeviceMetadata, topologySemanticProfileMetadataAllowed)
		result.tags = cloneTopologySemanticStringTags(profile.Tags, topologySemanticProfileTagAllowed)
		result.metrics = projectTopologySemanticMetrics(kind, profile.TopologyMetrics)
	}
	return result
}

func projectTopologySemanticMetrics(
	eventKind topologySemanticEventKind,
	metrics []ddsnmp.Metric,
) []topologySemanticMetricEvidence {
	result := make([]topologySemanticMetricEvidence, 0, len(metrics))
	for _, metric := range metrics {
		if !topologySemanticMetricConsumed(eventKind, metric.TopologyKind) {
			continue
		}
		result = append(result, topologySemanticMetricEvidence{
			kind: metric.TopologyKind,
			tags: cloneTopologySemanticStringTags(
				metric.Tags,
				func(key string) bool { return topologySemanticMetricTagAllowed(metric.TopologyKind, key) },
			),
		})
	}
	return result
}

func projectTopologySemanticBGPRows(rows []ddsnmp.BGPRow) []topologySemanticBGPRowEvidence {
	result := make([]topologySemanticBGPRowEvidence, 0, len(rows))
	for _, row := range rows {
		result = append(result, topologySemanticBGPRowEvidence{
			originProfileID: row.OriginProfileID,
			table:           row.Table,
			rowKey:          row.RowKey,
			structuralID:    row.StructuralID,
			kind:            row.Kind,
			routingInstance: row.Identity.RoutingInstance,
			neighbor:        row.Identity.Neighbor,
			remoteAS:        row.Identity.RemoteAS,
			localAddress:    row.Descriptors.LocalAddress,
			localAS:         row.Descriptors.LocalAS,
			localIdentifier: row.Descriptors.LocalIdentifier,
			peerIdentifier:  row.Descriptors.PeerIdentifier,
			peerType:        row.Descriptors.PeerType,
			bgpVersion:      row.Descriptors.BGPVersion,
			description:     row.Descriptors.Description,
			adminHas:        row.Admin.Enabled.Has,
			adminEnabled:    row.Admin.Enabled.Value,
			stateHas:        row.State.Has,
			state:           row.State.State,
			stateRaw:        row.State.Raw,
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
	value.vnodeLabels = maps.Clone(value.vnodeLabels)
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
		result[key] = value
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
	case ddsnmp.KindLldpRemManAddr, ddsnmp.KindLldpRemManAddrCompat:
		switch key {
		case
			tagLldpLocPortNum, tagLldpRemIndex, tagLldpRemMgmtAddr, tagLldpRemMgmtAddrSubtype,
			tagLldpRemMgmtAddrLen, tagLldpRemMgmtAddrIfSubtype, tagLldpRemMgmtAddrIfID,
			tagLldpRemMgmtAddrOID:
			return true
		}
		return topologySemanticLldpRemOctetTagAllowed(key)
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

func topologySemanticLldpRemOctetTagAllowed(key string) bool {
	suffix := strings.TrimPrefix(key, tagLldpRemMgmtAddrOctetPref)
	index, err := strconv.Atoi(suffix)
	return err == nil && index >= 1 && index <= 16 && strconv.Itoa(index) == suffix
}

type topologySemanticUsage struct {
	limits       topologySemanticLimits
	recordCount  uint64
	logicalBytes uint64
}

func newTopologySemanticUsage(
	entries []ddsnmp.DeviceEntry,
	seen map[ddsnmp.DeviceRegistrationID]bool,
	selected map[ddsnmp.DeviceRegistrationID]bool,
	previousStates map[ddsnmp.DeviceRegistrationID]deviceRefreshState,
	states map[ddsnmp.DeviceRegistrationID]deviceRefreshState,
	limits topologySemanticLimits,
) topologySemanticUsage {
	removed := 0
	for registrationID := range previousStates {
		if !seen[registrationID] {
			removed++
		}
	}
	rows := uint64(len(entries) + removed)
	usage := topologySemanticUsage{
		limits:       limits,
		recordCount:  1 + rows,
		logicalBytes: topologyDiagnosticCutLogicalBytes + rows*topologyDiagnosticRowLogicalBytes,
	}

	for _, entry := range entries {
		registrationID := entry.RegistrationID
		if selected[registrationID] {
			continue
		}
		state := states[registrationID]
		states[registrationID] = usage.includeRetained(state)
	}
	return usage
}

func (u *topologySemanticUsage) availableLimits(perDevice topologySemanticLimits) topologySemanticLimits {
	return topologySemanticLimits{
		maxRecords:      min(perDevice.maxRecords, topologySemanticRemaining(u.limits.maxRecords, u.recordCount)),
		maxLogicalBytes: min(perDevice.maxLogicalBytes, topologySemanticRemaining(u.limits.maxLogicalBytes, u.logicalBytes)),
	}
}

func (u *topologySemanticUsage) include(capture topologySemanticCapture) topologySemanticCapture {
	if capture.state != diagnosticCaptureAvailable {
		return capture
	}
	if u.recordCount > u.limits.maxRecords || capture.recordCount > u.limits.maxRecords-u.recordCount {
		return limitTopologySemanticCapture(capture, diagnosticCaptureReasonGlobalRecordLimit)
	}
	if u.logicalBytes > u.limits.maxLogicalBytes || capture.logicalBytes > u.limits.maxLogicalBytes-u.logicalBytes {
		return limitTopologySemanticCapture(capture, diagnosticCaptureReasonGlobalByteLimit)
	}
	u.recordCount += capture.recordCount
	u.logicalBytes += capture.logicalBytes
	return capture
}

func (u *topologySemanticUsage) includeRetained(state deviceRefreshState) deviceRefreshState {
	if state.generation == nil || state.generation.semantic.state != diagnosticCaptureAvailable {
		return state
	}
	capture := u.include(state.generation.semantic)
	if capture.state == diagnosticCaptureAvailable {
		return state
	}
	generation := *state.generation
	generation.semantic = capture
	state.generation = &generation
	return state
}

func topologySemanticRemaining(limit, used uint64) uint64 {
	if used >= limit {
		return 0
	}
	return limit - used
}

func classifyTopologySemanticCaptureLimit(
	capture topologySemanticCapture,
	effective topologySemanticLimits,
	perDevice topologySemanticLimits,
) topologySemanticCapture {
	if capture.state != diagnosticCaptureLimitExceeded {
		return capture
	}
	switch capture.reason {
	case diagnosticCaptureReasonRecordLimit:
		if effective.maxRecords < perDevice.maxRecords {
			capture.reason = diagnosticCaptureReasonGlobalRecordLimit
		}
	case diagnosticCaptureReasonByteLimit:
		if effective.maxLogicalBytes < perDevice.maxLogicalBytes {
			capture.reason = diagnosticCaptureReasonGlobalByteLimit
		}
	}
	return capture
}

func limitTopologySemanticCapture(capture topologySemanticCapture, reason diagnosticCaptureReason) topologySemanticCapture {
	capture.state = diagnosticCaptureLimitExceeded
	capture.reason = reason
	capture.evidence = nil
	return capture
}
