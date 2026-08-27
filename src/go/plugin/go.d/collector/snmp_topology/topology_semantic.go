// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

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
	kind topologySemanticEventKind

	sysUptime int64
	profiles  []*ddsnmp.ProfileMetrics
	vlan      topologyVLANContextResult
}

// topologySemanticObserver borrows one live event synchronously. It must not
// retain maps, slices, or profile metrics. An asynchronous recorder first
// detaches its allowlisted DTO while this callback is active.
type topologySemanticObserver func(topologySemanticEvent)

type topologySemanticStream interface {
	next() (topologySemanticEvent, bool)
}

func applyTopologySemanticStream(builder *topologyBuilder, stream topologySemanticStream, observer topologySemanticObserver) {
	if builder == nil || stream == nil {
		return
	}
	for {
		event, ok := stream.next()
		if !ok {
			return
		}
		if observer != nil {
			observer(event)
		}
		applyTopologySemanticEvent(builder, event)
	}
}

func applyTopologySemanticEvent(builder *topologyBuilder, event topologySemanticEvent) {
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
		if event.vlan.state == topologyVLANContextSuccess {
			builder.ingestTopologyVLANContextMetrics(
				event.vlan.vlanID,
				event.vlan.vlanName,
				event.vlan.profiles,
			)
		}
	}
}

type topologyMainSemanticStream struct {
	position  uint8
	sysUptime int64
	profiles  []*ddsnmp.ProfileMetrics
}

func newTopologyMainSemanticStream(sysUptime int64, profiles []*ddsnmp.ProfileMetrics) *topologyMainSemanticStream {
	return &topologyMainSemanticStream{sysUptime: sysUptime, profiles: profiles}
}

func (s *topologyMainSemanticStream) next() (topologySemanticEvent, bool) {
	if s == nil {
		return topologySemanticEvent{}, false
	}
	s.position++
	switch s.position {
	case 1:
		return topologySemanticEvent{kind: topologySemanticEventSysUptime, sysUptime: s.sysUptime}, true
	case 2:
		return topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: s.profiles}, true
	case 3:
		return topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: s.profiles}, true
	case 4:
		return topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: s.profiles}, true
	default:
		return topologySemanticEvent{}, false
	}
}

type topologyVLANSemanticStream struct {
	position int
	results  []topologyVLANContextResult
}

func newTopologyVLANSemanticStream(results []topologyVLANContextResult) *topologyVLANSemanticStream {
	return &topologyVLANSemanticStream{results: results}
}

func (s *topologyVLANSemanticStream) next() (topologySemanticEvent, bool) {
	if s == nil || s.position >= len(s.results) {
		return topologySemanticEvent{}, false
	}
	result := s.results[s.position]
	s.position++
	return topologySemanticEvent{kind: topologySemanticEventVLANContext, vlan: result}, true
}
