// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControllerCommandActsParity drives StepExec across the deterministic
// stage-gate matrix and asserts CommandActs agrees: blocking work runs (the
// step runner is invoked) exactly when CommandActs reports true. Claim
// scheduling relies on this predicate to skip claims for rejection-only
// store commands - a stage-gate change without a matching CommandActs update
// fails here as the starvation bug it would be. The deliberate exception
// class: remove of any EXISTING store "acts" even when it will reject -
// its affected-jobs check reads the dependency index, which must be
// excluded by the granted store write claim, so that 409 AND the handler's
// source/type 405s ordered behind it answer under the claim without
// blocking work - those cells assert the exception explicitly via wantRan.
func TestControllerCommandActsParity(t *testing.T) {
	const storeName = "vault_prod"
	storeKey := secretstore.StoreKey(secretstore.KindVault, storeName)
	garbagePayload := []byte(`{"value": `)

	newFn := func(t *testing.T, ctl *Controller, cmd dyncfg.Command, id string, name string, payload []byte) dyncfg.Function {
		t.Helper()
		args := []string{id, string(cmd)}
		if name != "" {
			args = append(args, name)
		}
		f := functions.Function{UID: "parity", Args: args, Payload: payload}
		if payload != nil {
			f.ContentType = "application/json"
		}
		return dyncfg.NewFunction(f)
	}
	boolPtr := func(b bool) *bool { return &b }

	tests := map[string]struct {
		seedDyncfg bool // seed a dyncfg-sourced store via the custom add
		seedFile   bool // seed a file-sourced store (the conversion source)
		affected   bool // seed dependent refs (the remove 409 arm)
		fn         func(t *testing.T, ctl *Controller) dyncfg.Function
		wantActs   bool
		wantRan    *bool // nil: strict parity (ran == wantActs); non-nil: documented exception
	}{
		"enable is unsupported and rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandEnable, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"restart is unsupported and rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRestart, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"disable is unsupported and rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandDisable, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"add of a new store acts": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandAdd, ctl.templateID(secretstore.KindVault), storeName, mustJSON(t, map[string]any{"value": "good"}))
			},
			wantActs: true,
		},
		"add of an existing store is rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandAdd, ctl.templateID(secretstore.KindVault), storeName, mustJSON(t, map[string]any{"value": "good"}))
			},
			wantActs: false,
		},
		"add without payload is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandAdd, ctl.templateID(secretstore.KindVault), storeName, nil)
			},
			wantActs: false,
		},
		"add with unparsable payload is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandAdd, ctl.templateID(secretstore.KindVault), storeName, garbagePayload)
			},
			wantActs: false,
		},
		"update of a dyncfg store acts": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", mustJSON(t, map[string]any{"value": "rotated"}))
			},
			wantActs: true,
		},
		"update of a missing store is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", mustJSON(t, map[string]any{"value": "rotated"}))
			},
			wantActs: false,
		},
		"update without payload is rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"update of a dyncfg store with unparsable payload is rejection-only": {
			// The PayloadParser stage gate in the shared handler's
			// updateRejection: the parse prefix answers 400 before any claim
			// or effect.
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", garbagePayload)
			},
			wantActs: false,
		},
		"conversion update of a file store acts": {
			seedFile: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", mustJSON(t, map[string]any{"value": "override"}))
			},
			wantActs: true,
		},
		"conversion update without payload is rejection-only": {
			seedFile: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"conversion update with unparsable payload is rejection-only": {
			seedFile: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUpdate, ctl.configID(storeKey), "", garbagePayload)
			},
			wantActs: false,
		},
		"remove of an unreferenced store acts": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRemove, ctl.configID(storeKey), "", nil)
			},
			wantActs: true,
		},
		"remove of a missing store is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRemove, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"remove of a referenced store acts but answers 409 under the claim": {
			seedDyncfg: true,
			affected:   true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRemove, ctl.configID(storeKey), "", nil)
			},
			wantActs: true,
			wantRan:  boolPtr(false),
		},
		"remove of a file store acts but answers 405 under the claim": {
			// The source gate sits BEHIND the affected-jobs read in the
			// remove's execution order, so it cannot be answered claimlessly.
			seedFile: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRemove, ctl.configID(storeKey), "", nil)
			},
			wantActs: true,
			wantRan:  boolPtr(false),
		},
		"remove of a referenced file store acts but answers 409 under the claim": {
			seedFile: true,
			affected: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandRemove, ctl.configID(storeKey), "", nil)
			},
			wantActs: true,
			wantRan:  boolPtr(false),
		},
		"payloadless test of an existing store acts": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandTest, ctl.configID(storeKey), "", nil)
			},
			wantActs: true,
		},
		"payloadless test of a missing store is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandTest, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"payload test of an existing store acts": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandTest, ctl.configID(storeKey), "", mustJSON(t, map[string]any{"value": "candidate"}))
			},
			wantActs: true,
		},
		"payload test of a missing store is rejection-only": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandTest, ctl.configID(storeKey), "", mustJSON(t, map[string]any{"value": "candidate"}))
			},
			wantActs: false,
		},
		"payload test with unparsable payload is rejection-only": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandTest, ctl.configID(storeKey), "", garbagePayload)
			},
			wantActs: false,
		},
		"schema answers inline and never claims": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandSchema, ctl.templateID(secretstore.KindVault), "", nil)
			},
			wantActs: false,
		},
		"get answers inline and never claims": {
			seedDyncfg: true,
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandGet, ctl.configID(storeKey), "", nil)
			},
			wantActs: false,
		},
		"userconfig answers inline and never claims": {
			fn: func(t *testing.T, ctl *Controller) dyncfg.Function {
				return newFn(t, ctl, dyncfg.CommandUserconfig, ctl.templateID(secretstore.KindVault), "", mustJSON(t, map[string]any{"value": "good"}))
			},
			wantActs: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var opts Options
			if tc.seedFile {
				opts.Initial = []secretstore.Config{
					newSecretStoreConfigWithSource(t, secretstore.KindVault, storeName, map[string]any{"value": "one"}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser),
				}
			}
			ctl, _, seams := newControllerTestSubjectWithOptions(opts)
			if tc.seedFile {
				ctl.PublishExisting()
				_, ok := ctl.Lookup(storeKey)
				require.True(t, ok, "seeding the file store failed")
			}
			if tc.seedDyncfg {
				ctl.SeqExec(dyncfg.NewFunction(functions.Function{
					UID:         "seed-add",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "good"}),
					Args:        []string{ctl.templateID(secretstore.KindVault), string(dyncfg.CommandAdd), storeName},
				}))
				_, ok := ctl.Lookup(storeKey)
				require.True(t, ok, "seeding the dyncfg store failed")
			}
			if tc.affected {
				seams.affectedJobs[storeKey] = []secretstore.JobRef{{ID: "mysql:prod", Display: "mysql:prod"}}
			}

			fn := tc.fn(t, ctl)

			// Capture the predicate BEFORE running: execution mutates the
			// caches, and the executor consults the predicate pre-execution.
			acts := ctl.CommandActs(fn)
			assert.Equal(t, tc.wantActs, acts, "CommandActs")

			ran := false
			ctl.StepExec(fn, func(effect func(context.Context) error, commit func(error)) {
				ran = true
				dyncfg.RunStepSync(effect, commit)
			})

			wantRan := acts
			if tc.wantRan != nil {
				wantRan = *tc.wantRan
			}
			assert.Equal(t, wantRan, ran, "blocking work must run exactly when the command acts")
		})
	}
}
