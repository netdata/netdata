// SPDX-License-Identifier: GPL-3.0-or-later

package attribution

import (
	"testing"
	"time"
)

func TestRouteTrackerReportsTransitions(t *testing.T) {
	tracker := NewRouteTracker(10)
	now := time.Unix(1, 0)

	if tracker.Observe("udp_peer:192.0.2.10", "udp_peer:a", now) {
		t.Fatal("first observation reported a transition")
	}
	if tracker.Observe("udp_peer:192.0.2.10", "udp_peer:a", now.Add(time.Second)) {
		t.Fatal("unchanged route reported a transition")
	}
	if !tracker.Observe("udp_peer:192.0.2.10", "vnode:vnode-1", now.Add(2*time.Second)) {
		t.Fatal("changed route did not report a transition")
	}
}

func TestRouteTrackerPrunesOldestRoute(t *testing.T) {
	tracker := NewRouteTracker(1)
	now := time.Unix(1, 0)

	tracker.Observe("raw:a", "effective:a", now)
	tracker.Observe("raw:b", "effective:b", now.Add(time.Second))

	if tracker.Len() != 1 {
		t.Fatalf("route count = %d, want 1", tracker.Len())
	}
	if _, ok := tracker.routes["raw:b"]; !ok {
		t.Fatalf("newest route was pruned: %#v", tracker.routes)
	}
}

func TestRouteTrackerUsesStableTieBreak(t *testing.T) {
	tracker := NewRouteTracker(1)
	now := time.Unix(1, 0)

	tracker.Observe("raw:b", "effective:b", now)
	tracker.Observe("raw:a", "effective:a", now)

	if _, ok := tracker.routes["raw:b"]; !ok {
		t.Fatalf("lexicographically newest route should remain: %#v", tracker.routes)
	}
}
