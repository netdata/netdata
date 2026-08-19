// SPDX-License-Identifier: GPL-3.0-or-later

package attribution

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

// DeviceMode selects the device identity used by a profile metric.
type DeviceMode string

const (
	DeviceSource      DeviceMode = "source"
	DeviceSourceLabel DeviceMode = "source_label"
	DeviceListener    DeviceMode = "listener"
)

// Key is the stable identity portion of a resolved metric source.
type Key struct {
	ScopeKey   string
	SourceID   string
	SourceKind string
}

// Source contains the host scope and labels projected for a resolved metric source.
type Source struct {
	Key         Key
	Scope       metrix.HostScope
	Labels      []metrix.Label
	RawRouteKey string
	RouteKey    string
}

// Resolve maps a committed trap entry to its profile-metric source identity.
func Resolve(entry *model.TrapEntry, jobName string, mode DeviceMode, sourceHashSalt string) (Source, bool) {
	if entry == nil {
		return Source{}, false
	}

	sourceID, sourceKind := rawFallback(entry)
	rawRouteKey := ""
	if sourceID != "" {
		rawRouteKey = sourceKind + ":" + sourceID
	}

	key := Key{}
	var scope metrix.HostScope
	switch {
	case mode == DeviceListener:
		key.SourceKind = "listener"
		key.SourceID = jobName
	case mode == DeviceSource && entry.SourceVnodeID != "" && !hasAmbiguousSourceEvidence(entry):
		hostname := entry.DeviceHostname
		if hostname == "" {
			hostname = entry.SourceVnodeID
		}
		scope = metrix.HostScope{
			ScopeKey: entry.SourceVnodeID,
			GUID:     entry.SourceVnodeID,
			Hostname: hostname,
		}
		key.ScopeKey = entry.SourceVnodeID
		key.SourceKind = "vnode"
		key.SourceID = entry.SourceVnodeID
	default:
		sourceID, sourceKind = fallback(entry, jobName, sourceHashSalt)
		if sourceID == "" {
			return Source{}, false
		}
		key.SourceID = sourceID
		key.SourceKind = sourceKind
	}

	labels := []metrix.Label{
		{Key: "job_name", Value: jobName},
		{Key: "source_id", Value: key.SourceID},
		{Key: "source_kind", Value: key.SourceKind},
	}

	return Source{
		Key:         key,
		Scope:       scope,
		Labels:      labels,
		RawRouteKey: rawRouteKey,
		RouteKey:    key.SourceKind + ":" + key.SourceID,
	}, true
}

func hasAmbiguousSourceEvidence(entry *model.TrapEntry) bool {
	if entry == nil || entry.Enrichment == nil {
		return false
	}
	if src := entry.Enrichment.Source; src != nil && len(src.RejectedCandidates) > 0 {
		return true
	}
	return lookupIsAmbiguous(entry.Enrichment.Registry) || lookupIsAmbiguous(entry.Enrichment.Topology)
}

func lookupIsAmbiguous(lookup *model.TrapEnrichmentLookup) bool {
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

func fallback(entry *model.TrapEntry, jobName, sourceHashSalt string) (string, string) {
	source, kind := rawFallback(entry)
	if source == "" {
		return "", ""
	}
	sum := sha256.Sum256([]byte(sourceHashSalt + ":" + jobName + ":" + source))
	return hex.EncodeToString(sum[:])[:16], kind
}

func rawFallback(entry *model.TrapEntry) (string, string) {
	source := ""
	kind := "source"
	if entry != nil && entry.Enrichment != nil && entry.Enrichment.Source != nil {
		source = entry.Enrichment.Source.Selected
		kind = fallbackSourceKind(entry.Enrichment.Source.Method)
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

func fallbackSourceKind(method string) string {
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
