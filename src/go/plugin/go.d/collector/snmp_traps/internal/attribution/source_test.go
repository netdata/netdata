// SPDX-License-Identifier: GPL-3.0-or-later

package attribution

import (
	"encoding/hex"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func TestResolveUsesVnodeHostScope(t *testing.T) {
	entry := testEntry()
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"

	got, ok := Resolve(entry, "profile-job", DeviceSource, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	wantScope := metrix.HostScope{ScopeKey: "vnode-1", GUID: "vnode-1", Hostname: "switch-1"}
	if got.Scope.ScopeKey != wantScope.ScopeKey || got.Scope.GUID != wantScope.GUID || got.Scope.Hostname != wantScope.Hostname {
		t.Fatalf("scope = %#v, want %#v", got.Scope, wantScope)
	}
	if got.Key != (Key{ScopeKey: "vnode-1", SourceID: "vnode-1", SourceKind: "vnode"}) {
		t.Fatalf("key = %#v", got.Key)
	}
}

func TestResolveFallsBackForAmbiguousVnode(t *testing.T) {
	entry := testEntry()
	entry.SourceVnodeID = "vnode-1"
	entry.Enrichment.Topology = &model.TrapEnrichmentLookup{Status: "conflict", Reason: "vnode_mismatch"}

	got, ok := Resolve(entry, "profile-job", DeviceSource, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if got.Key.SourceKind != "udp_peer" || got.Key.SourceID == "192.0.2.10" {
		t.Fatalf("key = %#v, want hashed udp_peer identity", got.Key)
	}
	if got.Scope.ScopeKey != "" {
		t.Fatalf("scope = %#v, want default host", got.Scope)
	}
}

func TestResolveFallsBackForRejectedSourceCandidate(t *testing.T) {
	entry := testEntry()
	entry.SourceVnodeID = "vnode-1"
	entry.Enrichment.Source.RejectedCandidates = []string{"snmpTrapAddress.0:untrusted_relay_uses_udp_peer"}

	got, ok := Resolve(entry, "profile-job", DeviceSource, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if got.Key.SourceKind != "udp_peer" || got.Key.SourceID == "192.0.2.10" {
		t.Fatalf("key = %#v, want hashed udp_peer identity", got.Key)
	}
	if got.Scope.ScopeKey != "" {
		t.Fatalf("scope = %#v, want default host", got.Scope)
	}
}

func TestResolveSourceLabelIgnoresVnode(t *testing.T) {
	entry := testEntry()
	entry.SourceVnodeID = "vnode-1"

	got, ok := Resolve(entry, "profile-job", DeviceSourceLabel, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if got.Key.SourceKind != "udp_peer" || got.Key.SourceID == "vnode-1" {
		t.Fatalf("key = %#v, want hashed fallback", got.Key)
	}
}

func TestResolveListenerIdentity(t *testing.T) {
	got, ok := Resolve(testEntry(), "profile-job", DeviceListener, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if got.Key != (Key{SourceID: "profile-job", SourceKind: "listener"}) {
		t.Fatalf("key = %#v", got.Key)
	}
}

func TestResolveHashesFallbackIdentity(t *testing.T) {
	got, ok := Resolve(testEntry(), "profile-job", DeviceSource, "salt")
	if !ok {
		t.Fatal("Resolve returned false")
	}
	if got.Key.SourceID == "192.0.2.10" || len(got.Key.SourceID) != 16 {
		t.Fatalf("source ID = %q, want 16-character hash", got.Key.SourceID)
	}
	if _, err := hex.DecodeString(got.Key.SourceID); err != nil {
		t.Fatalf("source ID is not hex: %v", err)
	}
	if got.RawRouteKey != "udp_peer:192.0.2.10" {
		t.Fatalf("raw route key = %q", got.RawRouteKey)
	}
}

func TestRawFallbackMapsUnknownMethodToOther(t *testing.T) {
	entry := testEntry()
	entry.Enrichment.Source.Method = "future_method_name"

	sourceID, sourceKind := rawFallback(entry)
	if sourceID != "192.0.2.10" || sourceKind != "other" {
		t.Fatalf("raw fallback identity = %q/%q, want 192.0.2.10/other", sourceID, sourceKind)
	}
}

func TestResolveRejectsMissingSource(t *testing.T) {
	if _, ok := Resolve(&model.TrapEntry{}, "profile-job", DeviceSource, "salt"); ok {
		t.Fatal("Resolve returned true without source evidence")
	}
}

func testEntry() *model.TrapEntry {
	return &model.TrapEntry{
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
	}
}
