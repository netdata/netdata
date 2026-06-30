// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

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

func resolveTrapMetricSourceIdentity(entry *TrapEntry, jobName string, identity ProfileMetricIdentityConfig, sourceHashSalt string) (trapMetricSourceIdentity, bool) {
	if entry == nil {
		return trapMetricSourceIdentity{}, false
	}

	identity.Device = defaultString(strings.ToLower(strings.TrimSpace(identity.Device)), profileMetricIdentitySource)
	identity.UnresolvedSource = defaultString(strings.ToLower(strings.TrimSpace(identity.UnresolvedSource)), profileMetricUnresolvedSourceLabel)
	identity.SourceIDPrivacy = defaultString(strings.ToLower(strings.TrimSpace(identity.SourceIDPrivacy)), profileMetricSourceIDHash)

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
		if identity.UnresolvedSource == profileMetricDropMetricInstance {
			return trapMetricSourceIdentity{}, false
		}
		sourceID, sourceKind = fallbackTrapSourceIdentity(entry, jobName, identity.SourceIDPrivacy, sourceHashSalt)
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

func fallbackTrapSourceIdentity(entry *TrapEntry, jobName, privacy, sourceHashSalt string) (string, string) {
	source, kind := rawFallbackTrapSourceIdentity(entry)
	if source == "" {
		return "", ""
	}
	if privacy == profileMetricSourceIDHash {
		sum := sha256.Sum256([]byte(sourceHashSalt + ":" + jobName + ":" + source))
		return hex.EncodeToString(sum[:])[:16], kind
	}
	return source, kind
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

func profileMetricSourceHashSalt() string {
	if provider, err := defaultJournalHostProvider(); err == nil {
		return "machine-id:" + provider.MachineID().String()
	}
	return "netdata-agent"
}
