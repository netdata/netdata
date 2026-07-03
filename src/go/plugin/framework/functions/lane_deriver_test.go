// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaneKeyDerivation(t *testing.T) {
	noop := func(Function) {}

	tests := map[string]struct {
		setup    func(mgr *Manager)
		fn       Function
		wantKey  string
		wantMeta any
	}{
		"prefix with deriver narrows the lane to the derived identity": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(fn Function) (string, any) {
					return "job-a", "meta-a"
				})
			},
			fn:       Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey:  "config|collector:\x00job-a",
			wantMeta: "meta-a",
		},
		"underivable request keeps the registration-wide lane": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(Function) (string, any) {
					return "", nil
				})
			},
			fn:      Function{Name: "config", Args: []string{"collector:garbled", "enable"}},
			wantKey: "config|collector:",
		},
		"panicking deriver falls back to the registration-wide lane": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(Function) (string, any) {
					panic("deriver bug")
				})
			},
			fn:      Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey: "config|collector:",
		},
		"prefix without deriver keeps the registration-wide lane": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
			},
			fn:      Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey: "config|collector:",
		},
		"nil deriver removes a previously attached one": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(Function) (string, any) {
					return "job-a", nil
				})
				mgr.RegisterPrefixLaneDeriver("config", "collector:", nil)
			},
			fn:      Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey: "config|collector:",
		},
		"deriver does not touch other prefixes of the same name": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefix("config", "vnode:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(Function) (string, any) {
					return "job-a", nil
				})
			},
			fn:      Function{Name: "config", Args: []string{"vnode:myhost", "update"}},
			wantKey: "config|vnode:",
		},
		"unregistering a prefix drops its deriver": {
			setup: func(mgr *Manager) {
				// A second prefix keeps the function set alive across the
				// unregister, so a stale deriver would survive in it.
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefix("config", "vnode:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "collector:", func(Function) (string, any) {
					return "stale", nil
				})
				mgr.UnregisterPrefix("config", "collector:")
				mgr.RegisterPrefix("config", "collector:", noop)
			},
			fn:      Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey: "config|collector:",
		},
		"deriver registration for an unknown prefix is a no-op": {
			setup: func(mgr *Manager) {
				mgr.RegisterPrefix("config", "collector:", noop)
				mgr.RegisterPrefixLaneDeriver("config", "ghost:", func(Function) (string, any) {
					return "x", nil
				})
			},
			fn:      Function{Name: "config", Args: []string{"collector:job-a", "enable"}},
			wantKey: "config|collector:",
		},
		"direct registration is unaffected by lane derivation": {
			setup: func(mgr *Manager) {
				mgr.Register("plainfn", noop)
			},
			fn:      Function{Name: "plainfn"},
			wantKey: "plainfn",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := NewManager()
			tc.setup(mgr)

			handler, key, meta, ok := mgr.lookupFunctionRoute(tc.fn)

			require.True(t, ok)
			require.NotNil(t, handler)
			assert.Equal(t, tc.wantKey, key)
			assert.Equal(t, tc.wantMeta, meta)
		})
	}
}

func TestLaneMetaFromContext(t *testing.T) {
	tests := map[string]struct {
		ctx  context.Context
		want any
	}{
		"returns attached meta": {
			ctx:  context.WithValue(context.Background(), laneMetaCtxKey{}, "meta"),
			want: "meta",
		},
		"nil context yields nil": {
			ctx:  nil,
			want: nil,
		},
		"context without meta yields nil": {
			ctx:  context.Background(),
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, LaneMetaFromContext(tc.ctx))
		})
	}
}

// Derived lanes are real scheduler lanes: same derived key serializes until
// terminal completion, different derived keys under one registration proceed
// independently.
func TestDerivedLaneScheduling(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, s *keyScheduler)
	}{
		"different derived lanes dispatch independently": {
			run: func(t *testing.T, s *keyScheduler) {
				require.NoError(t, s.enqueue(&invocationRequest{fn: &Function{UID: "a1"}, scheduleKey: "config|p|a"}))
				require.NoError(t, s.enqueue(&invocationRequest{fn: &Function{UID: "b1"}, scheduleKey: "config|p|b"}))

				first, ok := s.next()
				require.True(t, ok)
				second, ok := s.next()
				require.True(t, ok)
				assert.ElementsMatch(t,
					[]string{"a1", "b1"},
					[]string{first.fn.UID, second.fn.UID},
					"requests on distinct derived lanes must both dispatch without a completion in between")
			},
		},
		"same derived lane serializes until completion": {
			run: func(t *testing.T, s *keyScheduler) {
				require.NoError(t, s.enqueue(&invocationRequest{fn: &Function{UID: "a1"}, scheduleKey: "config|p|a"}))
				require.NoError(t, s.enqueue(&invocationRequest{fn: &Function{UID: "a2"}, scheduleKey: "config|p|a"}))

				first, ok := s.next()
				require.True(t, ok)
				assert.Equal(t, "a1", first.fn.UID)
				assert.Equal(t, 1, s.pendingCount(), "the second same-lane request must stay queued")

				s.complete("config|p|a", "a1")
				second, ok := s.next()
				require.True(t, ok)
				assert.Equal(t, "a2", second.fn.UID, "completion must promote the queued same-lane request")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t, newKeyScheduler(16))
		})
	}
}
