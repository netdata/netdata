// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// missedCollectionsBeforeStale is how many collections in a row a host can miss
// before the Agent marks its generated node stale.
const missedCollectionsBeforeStale = 3

// nodeStaleAfter is how long the generated node can go without data before the
// Agent marks it stale. A collection runs for up to timeout, so two successful
// collections are at most update_every + timeout apart; the timeout term keeps
// a slow host from flapping. The Agent counts whole seconds, so it rounds up.
func nodeStaleAfter(updateEvery int, timeout time.Duration) time.Duration {
	d := time.Duration(missedCollectionsBeforeStale*updateEvery)*time.Second + timeout
	return (d + time.Second - 1).Truncate(time.Second)
}

// canonicalEndpoint is the configured host in one spelling per target: an
// unmapped IP address, or a lowercase DNS name without the trailing dot.
func canonicalEndpoint(host string) string {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// newVirtualNode creates the job's generated node. Its GUID derives from the
// endpoint, so a Windows rename keeps the node and its history; the inventory
// later replaces the hostname with the Windows computer name. A failed
// collection publishes nothing on the node, so the Agent marks it stale once
// staleAfter passes without a successful collection.
func newVirtualNode(host string, staleAfter time.Duration) *vnodes.VirtualNode {
	endpoint := canonicalEndpoint(host)
	stale := confopt.Duration(staleAfter)
	return &vnodes.VirtualNode{
		GUID:       uuid.NewSHA1(uuid.NameSpaceDNS, []byte("wmi:"+endpoint)).String(),
		Hostname:   endpoint,
		Labels:     map[string]string{"_vnode_type": "wmi", "_os": "windows", "address": host},
		StaleAfter: &stale,
	}
}
