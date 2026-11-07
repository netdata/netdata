// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/ownedjobs"
	"github.com/stretchr/testify/require"
)

func TestOwnedJobsOwnerOnlyRemovalReleasesIdentitiesForAnotherParent(t *testing.T) {
	_, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	owner, err := registry.Bind(t.Context(), "first-site")
	require.NoError(t, err)
	children := ownedTestChildren(3)
	applyOwned(t, owner, 1, children)
	waitOwnedRunning(t, owner, 3)
	applyOwned(t, owner, 2, nil)
	require.Empty(t, owner.Status())
	other, err := registry.Bind(t.Context(), "second-site")
	require.NoError(t, err)
	var replacement ownedjobs.Snapshot
	// A different owner cannot reuse the names while old graph/candidate claims
	// survive. Public admission proves their eventual reclamation after removal.
	require.Eventually(t, func() bool {
		replacement, err = other.Prepare(t.Context(), 1, children)
		return err == nil
	}, 3*time.Second, time.Millisecond)
	require.NoError(t, replacement.Apply(t.Context()))
	waitOwnedRunning(t, other, 3)
	require.Empty(t, owner.Status())
}
