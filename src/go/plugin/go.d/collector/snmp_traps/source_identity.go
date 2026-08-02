// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type trapMetricSourceIdentityKey struct {
	scopeKey   string
	sourceID   string
	sourceKind string
}

type trapMetricSourceIdentity struct {
	key         trapMetricSourceIdentityKey
	scope       metrix.HostScope
	labels      []metrix.Label
	rawRouteKey string
	routeKey    string
}

func trapEntryHasAmbiguousSourceEvidence(entry *TrapEntry) bool {
	if entry == nil || entry.Enrichment == nil {
		return false
	}
	if src := entry.Enrichment.Source; src != nil && len(src.RejectedCandidates) > 0 {
		return true
	}
	return trapLookupIsAmbiguous(entry.Enrichment.Registry) || trapLookupIsAmbiguous(entry.Enrichment.Topology)
}

func trapLookupIsAmbiguous(lookup *TrapEnrichmentLookup) bool {
	if lookup == nil {
		return false
	}
	switch lookup.Status {
	case "ambiguous", "conflict":
		return true
	default:
		return lookup.Reason == "ambiguous_source" || lookup.Reason == "vnode_mismatch"
	}
}

func resolveTrapMetricSourceIdentity(entry *TrapEntry, jobName string, identity profileMetricIdentityPolicy, sourceHashSalt string) (trapMetricSourceIdentity, bool) {
	if entry == nil {
		return trapMetricSourceIdentity{}, false
	}

	sourceID, sourceKind := rawFallbackTrapSourceIdentity(entry)
	rawRouteKey := ""
	if sourceID != "" {
		rawRouteKey = sourceKind + ":" + sourceID
	}

	key := trapMetricSourceIdentityKey{}
	var scope metrix.HostScope
	switch {
	case identity.Device == profileMetricIdentityListener:
		key.sourceKind = "listener"
		key.sourceID = jobName
	case identity.Device == profileMetricIdentitySource && entry.SourceVnodeID != "" && !trapEntryHasAmbiguousSourceEvidence(entry):
		hostname := entry.DeviceHostname
		if hostname == "" {
			hostname = entry.SourceVnodeID
		}
		scope = metrix.HostScope{
			ScopeKey: entry.SourceVnodeID,
			GUID:     entry.SourceVnodeID,
			Hostname: hostname,
		}
		key.scopeKey = entry.SourceVnodeID
		key.sourceKind = "vnode"
		key.sourceID = entry.SourceVnodeID
	default:
		sourceID, sourceKind = fallbackTrapSourceIdentity(entry, jobName, sourceHashSalt)
		if sourceID == "" {
			return trapMetricSourceIdentity{}, false
		}
		key.sourceID = sourceID
		key.sourceKind = sourceKind
	}

	labels := []metrix.Label{
		{Key: "job_name", Value: jobName},
		{Key: "source_id", Value: key.sourceID},
		{Key: "source_kind", Value: key.sourceKind},
	}

	return trapMetricSourceIdentity{
		key:         key,
		scope:       scope,
		labels:      labels,
		rawRouteKey: rawRouteKey,
		routeKey:    key.sourceKind + ":" + key.sourceID,
	}, true
}

func fallbackTrapSourceIdentity(entry *TrapEntry, jobName, sourceHashSalt string) (string, string) {
	source, kind := rawFallbackTrapSourceIdentity(entry)
	if source == "" {
		return "", ""
	}
	sum := sha256.Sum256([]byte(sourceHashSalt + ":" + jobName + ":" + source))
	return hex.EncodeToString(sum[:])[:16], kind
}

func rawFallbackTrapSourceIdentity(entry *TrapEntry) (string, string) {
	source := ""
	kind := "source"
	if entry != nil && entry.Enrichment != nil && entry.Enrichment.Source != nil {
		source = entry.Enrichment.Source.Selected
		kind = fallbackTrapSourceKind(entry.Enrichment.Source.Method)
	}
	if entry != nil && source == "" {
		source = entry.SourceIP
		kind = "entry_source"
	}
	if entry != nil && source == "" {
		source = entry.SourceUDPPeer
		kind = "udp_peer"
	}
	if source == "" {
		return "", ""
	}
	return source, kind
}

func fallbackTrapSourceKind(method string) string {
	switch method {
	case "":
		return "source"
	case "trusted_relay_snmpTrapAddress.0":
		return "trusted_trap_address"
	case "udp_peer", "entry_source", "hostname_or_ip", "trap_varbind", "topology_ifindex":
		return method
	default:
		return "other"
	}
}
