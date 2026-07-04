// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"

	"github.com/stretchr/testify/assert"
)

// TestControllerCommandActsParity drives SeqExec across the deterministic
// stage-gate matrix and asserts CommandActs agrees: a command mutates the
// vnode store (or reaches its claim-protected gates) exactly when
// CommandActs reports true, and every wantActs=false cell answers its
// rejection without touching the store. Claim scheduling relies on this
// predicate to skip the vnode write claim for rejection-only commands - a
// stage-gate change without a matching CommandActs update fails here as the
// starvation bug it would be. Two documented acts-but-rejects cells answer
// inline UNDER the claim by design: payload parse/GUID/uniqueness rejections
// (bounded by one effect window) and the remove path's referenced-by-configs
// 409 (a stopping dependent's read claim must be waited out, not answered
// around).
func TestControllerCommandActsParity(t *testing.T) {
	const guid = "b0b0b0b0-0000-4000-8000-0000000000aa"

	tests := map[string]struct {
		initial  map[string]*vnodes.VirtualNode
		affected []string
		fn       func(ctl *Controller) dyncfg.Function
		wantActs bool
		wantCode int
	}{
		"enable is unsupported and rejection-only": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandEnable)}})
			},
			wantActs: false,
			wantCode: 501,
		},
		"restart is unsupported and rejection-only": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandRestart)}})
			},
			wantActs: false,
			wantCode: 501,
		},
		"disable is unsupported and rejection-only": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandDisable)}})
			},
			wantActs: false,
			wantCode: 501,
		},
		"add with payload and name acts": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h1"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
			},
			wantActs: true,
			wantCode: 202,
		},
		"add without payload is rejection-only": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"}})
			},
			wantActs: false,
			wantCode: 400,
		},
		"add without a name is rejection-only": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h1"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd)},
				})
			},
			wantActs: false,
			wantCode: 400,
		},
		"add with an unacceptable name is rejection-only": {
			// JobName() normalizes plain spaces and colons to underscores;
			// a tab survives normalization and fails the name rule.
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h1"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "bad\tname"},
				})
			},
			wantActs: false,
			wantCode: 400,
		},
		"add with an invalid guid acts and rejects inline under the claim": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "not-a-guid", "hostname": "h1"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
			},
			wantActs: true,
			wantCode: 400,
		},
		"update of an existing vnode acts": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h2"}),
					Args:        []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
			},
			wantActs: true,
			wantCode: 202,
		},
		"update of a missing vnode is rejection-only": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h2"}),
					Args:        []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
			},
			wantActs: false,
			wantCode: 404,
		},
		"remove of a dyncfg vnode acts": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
			},
			wantActs: true,
			wantCode: 200,
		},
		"remove of a missing vnode is rejection-only": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
			},
			wantActs: false,
			wantCode: 404,
		},
		"remove of a non-dyncfg vnode is rejection-only": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "stock")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
			},
			wantActs: false,
			wantCode: 405,
		},
		"remove of a referenced vnode acts but answers 409 under the claim": {
			initial:  map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			affected: []string{"mysql:prod"},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
			},
			wantActs: true,
			wantCode: 409,
		},
		"test answers inline and never claims": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h2"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandTest), "db"},
				})
			},
			wantActs: false,
			wantCode: 202,
		},
		"get answers inline and never claims": {
			initial: map[string]*vnodes.VirtualNode{"db": testVnode("db", "h1", guid, "dyncfg")},
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.configID("db"), string(dyncfg.CommandGet)}})
			},
			wantActs: false,
			wantCode: 200,
		},
		"schema answers inline and never claims": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "parity", Args: []string{ctl.Prefix(), string(dyncfg.CommandSchema)}})
			},
			wantActs: false,
			wantCode: 200,
		},
		"userconfig answers inline and never claims": {
			fn: func(ctl *Controller) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "parity",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": guid, "hostname": "h1"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandUserconfig), "db"},
				})
			},
			wantActs: false,
			wantCode: 200,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, seams := newControllerTestSubject(tc.initial)
			for _, ref := range tc.affected {
				seams.affectedJobs["db"] = append(seams.affectedJobs["db"], ref)
			}

			fn := tc.fn(ctl)

			// Capture the predicate BEFORE running: execution mutates the
			// store, and the executor consults the predicate pre-execution.
			acts := ctl.CommandActs(fn)
			assert.Equal(t, tc.wantActs, acts, "CommandActs")

			ctl.SeqExec(fn)
			assert.Contains(t, out.String(), fmt.Sprintf("FUNCTION_RESULT_BEGIN parity %d", tc.wantCode),
				"the command must answer its expected outcome, output:\n%s", out.String())

			if !tc.wantActs {
				if tc.initial == nil {
					_, ok := ctl.Lookup("db")
					assert.False(t, ok, "a rejection-only command must not create store state")
				} else if orig := tc.initial["db"]; orig != nil {
					got, ok := ctl.Lookup("db")
					assert.True(t, ok, "a rejection-only command must not remove store state")
					assert.Equal(t, orig.Hostname, got.Hostname, "a rejection-only command must not mutate store state")
				}
			}
		})
	}
}
