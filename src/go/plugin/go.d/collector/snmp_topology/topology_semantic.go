// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"maps"
	"net/netip"
	"path"
	"slices"
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
		vnodeLabels: maps.Clone(dev.VnodeLabels),
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
		VnodeLabels: maps.Clone(d.vnodeLabels),
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

type topologySemanticCaptureState uint8

const (
	topologySemanticCaptureUnknown topologySemanticCaptureState = iota
	topologySemanticCaptureAvailable
	topologySemanticCaptureLimitExceeded
	topologySemanticCaptureUnavailable
)

type topologySemanticCaptureReason uint8

const (
	topologySemanticCaptureReasonNone topologySemanticCaptureReason = iota
	topologySemanticCaptureReasonRecordLimit
	topologySemanticCaptureReasonByteLimit
	topologySemanticCaptureReasonProjectionError
	topologySemanticCaptureReasonProjectionPanic
)

type topologySemanticCapture struct {
	state        topologySemanticCaptureState
	reason       topologySemanticCaptureReason
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
	state        topologySemanticCaptureState
	reason       topologySemanticCaptureReason
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
		state:        topologySemanticCaptureAvailable,
		projectEvent: projectTopologySemanticEvent,
	}
	defer func() {
		if recover() != nil {
			r.fail(topologySemanticCaptureReasonProjectionPanic)
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
	if r == nil || r.state != topologySemanticCaptureAvailable {
		return
	}
	defer func() {
		if recover() != nil {
			r.fail(topologySemanticCaptureReasonProjectionPanic)
		}
	}()
	if r.projectEvent == nil {
		r.fail(topologySemanticCaptureReasonProjectionError)
		return
	}
	if err := r.projectEvent(r, event); err != nil {
		r.fail(topologySemanticCaptureReasonProjectionError)
	}
}

func (r *topologySemanticRecorder) finish() topologySemanticCapture {
	if r == nil {
		return topologySemanticCapture{state: topologySemanticCaptureUnavailable}
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
		r.limit(topologySemanticCaptureReasonRecordLimit)
		return false
	}
	if logicalBytes > r.limits.maxLogicalBytes-r.logicalBytes {
		r.limit(topologySemanticCaptureReasonByteLimit)
		return false
	}
	r.recordCount += records
	r.logicalBytes += logicalBytes
	return true
}

func (r *topologySemanticRecorder) limit(reason topologySemanticCaptureReason) {
	r.state = topologySemanticCaptureLimitExceeded
	r.reason = reason
	r.evidence = nil
}

func (r *topologySemanticRecorder) fail(reason topologySemanticCaptureReason) {
	r.state = topologySemanticCaptureUnavailable
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
			logicalBytes += topologySemanticMetaTagMapBytes(profile.DeviceMetadata)
			logicalBytes += topologySemanticStringMapBytes(profile.Tags)
		case topologySemanticEventTopologyMetrics, topologySemanticEventVLANContext:
			if event.kind == topologySemanticEventVLANContext {
				logicalBytes += topologySemanticMetaTagMapBytes(profile.DeviceMetadata)
				logicalBytes += topologySemanticStringMapBytes(profile.Tags)
			}
			records += uint64(len(profile.TopologyMetrics))
			for _, metric := range profile.TopologyMetrics {
				logicalBytes += uint64(len(metric.TopologyKind)) + topologySemanticStringMapBytes(metric.Tags)
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
		result.metadata = maps.Clone(profile.DeviceMetadata)
		result.tags = maps.Clone(profile.Tags)
	case topologySemanticEventTopologyMetrics:
		result.metrics = projectTopologySemanticMetrics(profile.TopologyMetrics)
	case topologySemanticEventBGPPeers:
		result.bgpFailed = profile.BGPCollectError != nil
		if !result.bgpFailed {
			result.bgpRows = projectTopologySemanticBGPRows(profile.BGPRows)
		}
	case topologySemanticEventVLANContext:
		result.metadata = maps.Clone(profile.DeviceMetadata)
		result.tags = maps.Clone(profile.Tags)
		result.metrics = projectTopologySemanticMetrics(profile.TopologyMetrics)
	}
	return result
}

func projectTopologySemanticMetrics(metrics []ddsnmp.Metric) []topologySemanticMetricEvidence {
	result := make([]topologySemanticMetricEvidence, 0, len(metrics))
	for _, metric := range metrics {
		result = append(result, topologySemanticMetricEvidence{
			kind: metric.TopologyKind,
			tags: maps.Clone(metric.Tags),
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
			tags:            maps.Clone(row.Tags),
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
	for _, component := range strings.Split(value, "/") {
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

func topologySemanticMetaTagMapBytes(values map[string]ddsnmp.MetaTag) uint64 {
	var total uint64
	for key, value := range values {
		total += uint64(len(key) + len(value.Value) + 1)
	}
	return total
}

func topologySemanticBGPRowLogicalBytes(row ddsnmp.BGPRow) uint64 {
	return uint64(len(row.OriginProfileID)+len(row.Table)+len(row.RowKey)+len(row.StructuralID)+len(row.Kind)+
		len(row.Identity.RoutingInstance)+len(row.Identity.Neighbor)+len(row.Identity.RemoteAS)+
		len(row.Descriptors.LocalAddress)+len(row.Descriptors.LocalAS)+len(row.Descriptors.LocalIdentifier)+
		len(row.Descriptors.PeerIdentifier)+len(row.Descriptors.PeerType)+len(row.Descriptors.BGPVersion)+
		len(row.Descriptors.Description)+len(row.State.State)+len(row.State.Raw)+19) + topologySemanticStringMapBytes(row.Tags)
}
