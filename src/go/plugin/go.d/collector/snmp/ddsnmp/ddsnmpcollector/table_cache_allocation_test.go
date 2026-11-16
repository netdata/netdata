// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !race

package ddsnmpcollector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Race instrumentation randomly discards sync.Pool entries used by JSON encoding.
// Measure the production allocation envelope separately from behavioral race checks.
func TestIndexIdentityCacheConfigAllocationEnvelopes(t *testing.T) {
	cache := &tableCache{}
	for name, config := range indexIdentityCacheConfigs() {
		t.Run(name, func(t *testing.T) {
			var identity string
			allocs := testing.AllocsPerRun(100, func() { identity = cache.generateConfigID(config) })
			require.NotEmpty(t, identity)
			limit := float64(11)
			if name == "legacy-three-tags" {
				limit = 2
			}
			require.LessOrEqual(
				t,
				allocs,
				limit,
				"config identity allocations are bounded by config size, independent of table row count",
			)
			t.Logf("config key: %.0f allocations; %d encoded bytes", allocs, len(identity))
		})
	}
}
