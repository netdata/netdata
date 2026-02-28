// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupFunction_SnapshotScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager)
	}{
		"uses direct snapshot": {
			run: func(t *testing.T, mgr *Manager) {
				called := make(chan struct{}, 1)
				mgr.Register("fn", func(Function) { called <- struct{}{} })

				handler, ok := mgr.lookupFunction("fn")
				require.True(t, ok)

				mgr.Unregister("fn")
				handler(Function{Name: "fn"})

				select {
				case <-called:
				default:
					t.Fatal("snapshot handler should still invoke the originally resolved direct function")
				}
			},
		},
		"uses prefix snapshot": {
			run: func(t *testing.T, mgr *Manager) {
				called := make(chan struct{}, 1)
				mgr.RegisterPrefix("config", "collector:", func(Function) { called <- struct{}{} })

				handler, ok := mgr.lookupFunction("config")
				require.True(t, ok)

				mgr.UnregisterPrefix("config", "collector:")
				handler(Function{Name: "config", Args: []string{"collector:job"}})

				select {
				case <-called:
				default:
					t.Fatal("snapshot handler should still route using the prefix set captured at lookup time")
				}
			},
		},
		"prefix routing prefers longest match": {
			run: func(t *testing.T, mgr *Manager) {
				longHits := 0
				shortHits := 0

				mgr.RegisterPrefix("config", "collector:", func(Function) { shortHits++ })
				mgr.RegisterPrefix("config", "collector:job:", func(Function) { longHits++ })

				handler, ok := mgr.lookupFunction("config")
				require.True(t, ok)

				for range 32 {
					handler(Function{Name: "config", Args: []string{"collector:job:alpha"}})
				}

				assert.Equal(t, 32, longHits)
				assert.Equal(t, 0, shortHits)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t, NewManager())
		})
	}
}
