// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCandidateTrustWithdrawalRetiresRuntimeDespiteMetadataQuarantine(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			state := &continuationCollectorState{}
			trusted := confgroup.Config{"module": "module", "name": "local2", "update_every": 1, "option_str": "incumbent"}.
				SetSource("sd-target").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
				SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(true)
			fixture := newContinuationFixture(t, version, true, state, []confgroup.Config{trusted})
			fixture.waitStatus(dyncfg.StatusRunning)
			if version == "v2" {
				require.EqualValues(t, 1, state.active.Load())
			}
			// The unknown module collides with the incumbent's public full name.
			untrusted := confgroup.Config{"module": "module_local2", "name": "module_local2", "update_every": 1, "option_str": "literal"}.
				SetSource("sd-target").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
				SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(false)
			require.Equal(t, trusted.FullName(), untrusted.FullName())
			fixture.observe([]confgroup.Config{untrusted})
			// Graph retirement is a positive, synchronized oracle that the real
			// inventory was applied, rather than a sleep followed by a private read.
			require.Eventually(t, func() bool {
				_, exists := fixture.generation.vnodes.graph.Lookup(trusted.FullName())
				return !exists
			}, 4*time.Second, time.Millisecond, "owning withdrawal must retire the binding despite metadata rejection")
			require.Eventually(t, func() bool { return state.active.Load() == 0 }, time.Second, time.Millisecond)
			fixture.command("withdrawn-get", "get", 404)
			fixture.command("withdrawn-enable", "enable", 404)
			require.Equal(t, []string{"incumbent"}, state.checked(), "quarantined metadata must never initialize a replacement")
			require.EqualValues(t, 1, state.inits.Load())
			require.NoError(t, fixture.generation.run.DirtyCause())
		})
	}
}
