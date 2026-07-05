// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// A vnode mutation write-claims the vnode name: it must park behind an
// in-flight collector effect holding a read claim on the same vnode, and
// proceed once that command commits. Vnode wire output is otherwise
// unchanged (the commands stay stage+commit on the loop).
func TestVnodeClaims_WriteConflictsWithCollectorReadClaim(t *testing.T) {
	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-gate // the enable's detection holds its claims
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	h := startSecretStoreEffectHarness(t, reg, nil)
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	})

	const guid = "b0b0b0b0-0000-4000-8000-000000000001"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v1"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-one"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	// A collector job bound to the vnode: its enable holds a READ claim on
	// the vnode while the detection runs.
	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v1")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v1"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-add"), charWait, charTick)
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, func() bool { return inits.Load() >= 1 }, charWait, charTick,
		"the enable's detection did not start")

	// The vnode update's WRITE claim conflicts with the enable's READ claim:
	// it parks until the enable commits.
	h.dyncfg("vn-update", []string{h.mgr.dyncfgVnodePrefixValue() + ":v1", "update"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update"), charNeverWait, charTick,
		"the vnode update must wait out the in-flight collector effect's vnode read claim")

	close(gate)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick,
		"the enable must commit after release")
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update 202"), charWait, charTick,
		"the parked vnode update must proceed once the collector command commits")

	vnode, ok := h.mgr.vnodesCtl.Lookup("v1")
	require.True(t, ok)
	assert.Equal(t, "host-two", vnode.Hostname, "the parked update must apply after unparking")
}

// A stop-shaped collector command holds its OLD vnode reference: a vnode
// remove must wait out the dependent job's in-flight stop instead of racing
// its affected-job gate against the exposed-cache removal the stop already
// performed at stage.
func TestVnodeRemove_WaitsForDependentStopInFlight(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 {
					select {
					case entered <- struct{}{}:
					default:
					}
					<-release
					return map[string]int64{"d1": 1}
				},
			}
		},
	})

	h := startSecretStoreEffectHarness(t, reg, nil)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	const guid = "b0b0b0b0-0000-4000-8000-000000000002"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v2"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v2")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v2"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)
	select {
	case <-entered:
	case <-time.After(charWait):
		t.Fatal("no collection started")
	}

	// The job's removal clears the exposed entry at stage and blocks in its
	// stop; its vnode read claim keeps the dependency visible as "in flight".
	h.dyncfg("3-remove", []string{h.mgr.dyncfgJobID(cfg), "remove"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-remove"), charNeverWait, charTick,
		"the job's stop must be blocked in its final collection")

	// The vnode remove's write claim conflicts with the stopping job's read
	// claim: it must wait, not act on the already-cleared exposed cache.
	h.dyncfg("vn-remove", []string{h.mgr.dyncfgVnodePrefixValue() + ":v2", "remove"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-remove"), charNeverWait, charTick,
		"the vnode remove must wait out the dependent's in-flight stop")

	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-remove 200"), charWait, charTick)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-remove 200"), charWait, charTick,
		"the vnode remove proceeds once the dependent's removal committed")
	_, ok := h.mgr.vnodesCtl.Lookup("v2")
	assert.False(t, ok, "the vnode is removed once no dependent remains")
}

// An underivable command (here: an add without a job name) is
// rejection-only: it must answer 400 immediately even when its payload
// references a store whose key is write-held - claiming payload refs for it
// would park an invalid command behind that hold instead.
func TestUnderivableCommand_RejectsWithoutClaiming(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the rotation's dependent restart blocks
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	h := startSecretStoreEffectHarness(t, reg, nil)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)
	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	// pg stays ACCEPTED (added, never enabled) and references the store: its
	// exposed refs exist for a later restart's claim compute, but Accepted is
	// not restartable - the plan below must not claim it either.
	h.dyncfg("pg-add", []string{h.mgr.dyncfgModID("gated"), "add", "pg"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN pg-add"), charWait, charTick)

	// legacy is USER-SOURCED (discovered) and references the store: its
	// remove is source-rejected (405, only dyncfg configs can be removed).
	legacyCfg := prepareUserCfg("gated", "legacy").Set("option_str", "${store:vault:vault_prod:value}")
	h.in <- prepareCfgGroups(legacyCfg.Source(), legacyCfg)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:legacy create accepted"), charWait, charTick)

	// The rotation holds the store's WRITE claim while its dependent
	// restart blocks.
	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, func() bool { return inits.Load() >= 2 }, charWait, charTick,
		"the rotation's dependent restart did not start")

	// An add with NO job name is underivable; its payload references the
	// write-held store. It must be rejected immediately, not parked.
	h.dyncfg("bad-add", []string{h.mgr.dyncfgModID("gated"), "add"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN bad-add 400"), charWait, charTick,
		"an underivable command must reach its 400 rejection while the store is held")

	// A DERIVABLE update addressed to a config that is not exposed is
	// rejection-only too (the handler answers 404 before any effect): its
	// payload refs must not be claimed, or the doomed command would park
	// behind the write-held store instead of answering immediately.
	h.dyncfg("ghost-update", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("gated", "ghost")), "update"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ghost-update 404"), charWait, charTick,
		"an update of an unknown config must reach its 404 rejection while the store is held")

	// A STATE-rejected command is rejection-only as well: restart of an
	// Accepted config answers 405 before any work, so its exposed refs must
	// not be claimed either - pg was added (Accepted) before the rotation
	// took the store's write claim.
	h.dyncfg("pg-restart", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("gated", "pg")), "restart"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN pg-restart 405"), charWait, charTick,
		"a restart of an Accepted config must reach its 405 rejection while the store is held")

	// A SOURCE-rejected command too: remove of a user-sourced config answers
	// 405 before any stop work, so its exposed refs must not be claimed.
	h.dyncfg("legacy-remove", []string{h.mgr.dyncfgJobID(legacyCfg), "remove"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN legacy-remove 405"), charWait, charTick,
		"a remove of a non-dyncfg config must reach its 405 rejection while the store is held")

	// The PAYLOAD axis: update without a payload answers 400 before any
	// blocking work, so it must not claim its exposed refs - pg references
	// the write-held store and would otherwise park on the store's read
	// claim.
	h.dyncfg("pg-nopayload", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("gated", "pg")), "update"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN pg-nopayload 400"), charWait, charTick,
		"an update without a payload must reach its 400 rejection while the store is held")

	// The NAME-POLICY axis, a CALLBACK gate (ValidateConfigName /
	// JobNameRuleStrict): a dotted job name is derivable (non-empty) yet
	// answers 400 before any blocking work, so its payload refs - which
	// reference the write-held store - must not be claimed. The predicate
	// runs the handler's own addRejection gates, so callback gates cannot
	// drift out of it.
	h.dyncfg("bad-name-add", []string{h.mgr.dyncfgModID("gated"), "add", "bad.name"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN bad-name-add 400"), charWait, charTick,
		"an add with an invalid strict name must reach its 400 rejection while the store is held")

	// The PAYLOAD-PARSE axis (the PayloadParser stage gate): a present but
	// unparsable payload answers 400 from the cheap parse prefix, before
	// any claim or effect. An update of mysql would otherwise park at the
	// LANE (its job key is write-held by the rotation's plan), and an
	// add-over-pg would otherwise claim pg's exposed refs - the write-held
	// store - and park in the claim table.
	h.dyncfg("mysql-badpayload", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{ not json"))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN mysql-badpayload 400"), charWait, charTick,
		"an update with an unparsable payload must reach its 400 rejection while its job key is write-held")
	h.dyncfg("pg-badpayload", []string{h.mgr.dyncfgModID("gated"), "add", "pg"}, []byte("{ not json"))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN pg-badpayload 400"), charWait, charTick,
		"an add with an unparsable payload must reach its 400 rejection while the store is held")

	// The STATUS axis is HOLD-AWARE (user decision 1A): enable of a Running
	// config is a no-op only while its status is stable - mysql's key is
	// write-held by the rotation whose blocked restart is mid-mutating that
	// very status, so the enable must PARK and answer truthfully after the
	// hold resolves, not mint a 200 from mid-mutation state. Sent LAST so
	// the parked command does not block the same-key bypass pins above.
	h.dyncfg("mysql-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN mysql-enable"), charNeverWait, charTick,
		"a status-gated no-op must park while its job key is write-held")

	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN mysql-enable 200"), charWait, charTick,
		"the parked enable answers truthfully once the restart committed")
}

// Status commands that arrive while a store-driven dependent restart owns
// the job key's foreign write claim must park until the holder commits,
// then answer from the settled state. The table keeps the original
// false-success enable pin and adds restart/disable coverage for the same
// foreign-hold choreography.
func startDependentRestartHold(t *testing.T) (*charHarness, confgroup.Config, *atomic.Int32, func()) {
	t.Helper()

	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the rotation's dependent restart blocks, then FAILS
						return errors.New("rotated secret rejected")
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	h := startSecretStoreEffectHarness(t, reg, nil)
	closeRelease := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	t.Cleanup(closeRelease)

	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, func() bool { return inits.Load() >= 2 }, charWait, charTick,
		"the rotation's dependent restart did not start")

	return h, cfg, &inits, closeRelease
}

func TestStatusCommandsDuringDependentRestart_AnswerTruthfully(t *testing.T) {
	tests := map[string]struct {
		command string
		check   func(t *testing.T, h *charHarness, inits *atomic.Int32)
	}{
		"enable": {
			command: string(dyncfg.CommandEnable),
			check: func(t *testing.T, h *charHarness, inits *atomic.Int32) {
				t.Helper()
				require.Eventually(t, func() bool { return inits.Load() >= 3 }, charWait, charTick,
					"enable must run its own detection instead of answering a stale no-op")
				require.Eventually(t, func() bool {
					return strings.Count(h.out.String(), "CONFIG test:collector:gated:mysql status running") >= 2
				}, charWait, charTick, "enable must recover the job to running")
			},
		},
		"restart": {
			command: string(dyncfg.CommandRestart),
			check: func(t *testing.T, h *charHarness, inits *atomic.Int32) {
				t.Helper()
				require.Eventually(t, func() bool { return inits.Load() >= 3 }, charWait, charTick,
					"restart must run its own detection against the post-restart failed state")
				require.Eventually(t, func() bool {
					return strings.Count(h.out.String(), "CONFIG test:collector:gated:mysql status running") >= 2
				}, charWait, charTick, "restart must recover the job to running")
			},
		},
		"disable": {
			command: string(dyncfg.CommandDisable),
			check: func(t *testing.T, h *charHarness, inits *atomic.Int32) {
				t.Helper()
				require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status disabled"), charWait, charTick,
					"disable must act on the post-restart failed state")
				require.Never(t, func() bool { return inits.Load() >= 3 }, charNeverWait, charTick,
					"disable must not start a fresh detection")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h, cfg, inits, release := startDependentRestartHold(t)

			uid := "mysql-" + tc.command
			h.dyncfg(uid, []string{h.mgr.dyncfgJobID(cfg), tc.command}, nil)
			require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN "+uid), charNeverWait, charTick,
				"the command must park while the store restart owns the dependent write claim")

			release()

			require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
				"the failed store-driven restart must commit the dependent as failed")
			require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)
			require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN "+uid+" 200"), charWait, charTick,
				"the parked command must answer after the store-held mutation settles")
			tc.check(t, h, inits)
		})
	}
}

// Unsupported store/vnode commands (enable/disable/restart - the 501 arms)
// are rejection-only: they must answer immediately instead of reserving
// their domain claims first - a store restart would park its write claim
// behind an in-flight enable's store read claim, and a vnode restart would
// park behind the same enable's vnode read claim, both for the full effect
// window.
func TestUnsupportedDomainCommands_RejectWithoutClaiming(t *testing.T) {
	release := make(chan struct{})

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					<-release // the enable holds its claims for the whole detection
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	h := startSecretStoreEffectHarness(t, reg, nil)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	const guid = "b0b0b0b0-0000-4000-8000-0000000000bb"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v9"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-nine"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	// The enable blocks in its detection holding {job WRITE, store READ,
	// vnode READ}.
	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}", "vnode": "v9"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable"), charNeverWait, charTick,
		"the enable must be blocked in its detection")

	h.dyncfg("ss-restart", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "restart"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-restart 501"), charWait, charTick,
		"an unsupported store command must answer 501 while a collector holds the store's read claim")

	h.dyncfg("vn-restart", []string{h.mgr.dyncfgVnodePrefixValue() + ":v9", "restart"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-restart 501"), charWait, charTick,
		"an unsupported vnode command must answer 501 while a collector holds the vnode's read claim")

	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable 200"), charWait, charTick)
}

// The store remove's affected-jobs gate reads the dependency index, which
// the granted store write claim must exclude - and the handler's
// source/type gates sit BEHIND that read in the remove's execution order.
// A remove of a FILE-sourced store (whose eventual answer is a rejection
// either way: 409 while referenced, 405 otherwise) must therefore wait out
// an in-flight collector effect holding the store's read claim rather than
// answer claimlessly from an index snapshot that effect is about to
// change.
func TestStoreRemove_AffectedGateWaitsForStoreClaim(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					inits.Add(1)
					<-release // the enable holds its claims for the whole detection
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	fileStore := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod",
		map[string]any{"value": "good"}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)

	var out simOutput
	mgr := New(Config{
		PluginName:         testPluginName,
		SecretStores:       []secretstore.Config{fileStore},
		SecretStoreService: newTestSecretStoreService(),
	})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, func() bool { return inits.Load() >= 1 }, charWait, charTick,
		"the enable's detection did not start")

	h.dyncfg("ss-remove", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "remove"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-remove"), charNeverWait, charTick,
		"the remove must park behind the in-flight enable's store read claim")

	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable 200"), charWait, charTick)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-remove 409"), charWait, charTick,
		"the remove answers its 409 under the granted claim once the reference settles")
}

// The registration-time vnode reconcile is COMMITTED into the job, not
// queued: V1 applies queued updates only during collection, so a job
// stopped before its first tick would otherwise clean up with the stale
// creation-time vnode - HOST/HOSTINFO lines for a hostname the store no
// longer holds.
func TestWarmResume_CleanupEmitsReconciledVnodeBeforeFirstCollection(t *testing.T) {
	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-gate // the enable's detection wedges past the deadline
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService(), Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	mgr.effectDeadline = testEffectDeadline

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	const guid = "b0b0b0b0-0000-4000-8000-000000000006"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v7"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-one"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	// update_every keeps the warm job from collecting inside the test
	// window: only Cleanup can reveal which vnode the job carries.
	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v7")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v7", "update_every": 3600}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")

	h.dyncfg("vn-update", []string{h.mgr.dyncfgVnodePrefixValue() + ":v7", "update"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update 202"), charWait, charTick)

	close(gate)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick,
		"the late detection success must start the warm job")

	// Stop before any collection: Cleanup's HOST/HOSTINFO output must carry
	// the reconciled vnode, which only a COMMITTED baseline makes visible.
	h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status disabled"), charWait, charTick)
	require.Eventually(t, func() bool { return strings.Contains(jobOut.String(), "host-two") }, charWait, charTick,
		"cleanup must emit the vnode config reconciled at registration")
	assert.NotContains(t, jobOut.String(), "host-one",
		"the stale creation-time vnode must never reach the wire")
}

// A leaked detection that FAILS after shutdown finalized its command as
// 503-publish-nothing must dispose silently: the never-started vnode-bound
// job's Cleanup would otherwise emit HOST/HOSTINFO through the open gate -
// identity output on the wire after the one rule said nothing publishes.
func TestLateDetectionFailure_DisposesSilentlyAfterShutdown(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-gate // the enable's detection wedges past the deadline
						return fmt.Errorf("late detection failure")
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService(), Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	mgr.effectDeadline = testEffectDeadline
	mgr.drainWait = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	h := &charHarness{mgr: mgr, out: &out, in: in}

	const guid = "b0b0b0b0-0000-4000-8000-000000000007"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v8"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-silent"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v8")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v8"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")

	// Shutdown completes without unblocking the leaked detection.
	cancel()
	select {
	case <-done:
	case <-time.After(charWait):
		t.Fatal("shutdown did not complete")
	}

	// The leaked detection now FAILS - after cleanup. Its disposal must be
	// silent: nothing of the never-started job may reach the wire.
	close(gate)
	require.Eventually(t, func() bool { return mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the leaked detection must drain after release")
	assert.Empty(t, jobOut.String(),
		"a never-started job disposed after shutdown must not emit - its command answered 503-publish-nothing")
}

// A dropped warm continuation must dispose SILENTLY: V1 Cleanup emits
// HOST/HOSTINFO lines even for a job that never started, and publishing
// identity output for a suppressed start would mark its vnode active and
// leak stale job output (at shutdown it would breach the one rule). The
// disposal closes the job's emission gate before cleanup.
func TestWarmResume_DropDisposesSilently(t *testing.T) {
	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-gate // the enable's detection wedges past the deadline
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService(), Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	mgr.effectDeadline = testEffectDeadline

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	const guid = "b0b0b0b0-0000-4000-8000-000000000005"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v6"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-silent"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	// The vnode-bound job's detection wedges past the deadline; a disable
	// queued behind the wedge makes the eventual late success a DROP
	// (stop intent wins over the continuation).
	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v6")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v6"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")
	h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)

	// Late success: the warm job is dropped for the queued disable, which
	// then executes; the never-started job's disposal must reach the wire
	// with NOTHING (no HOSTINFO, no HOST lines).
	close(gate)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable 200"), charWait, charTick,
		"the queued disable must execute after the drop")
	assert.Empty(t, jobOut.String(),
		"a dropped warm job must dispose silently - its cleanup output is suppressed by the closed gate")
}

// A vnode update that commits inside a dependent restart's stop/start gap
// reaches no job (the dependent left runningJobs at its restart's stage, and
// the replacement is not registered yet): the replacement must still start
// on the CURRENT vnode config - registration reconciles the job's
// creation-time snapshot against the store.
func TestDependentRestart_ReceivesVnodeUpdatedDuringRestartGap(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			c := &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
			c.InitFunc = func(context.Context) error {
				inits.Add(1)
				if c.Config.OptionStr == "rotated" {
					// The dependent restart's detection: the replacement is
					// created (vnode snapshotted) but not yet registered.
					select {
					case entered <- struct{}{}:
					default:
					}
					<-release
				}
				return nil
			}
			return c
		},
	})

	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService(), Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	const guid = "b0b0b0b0-0000-4000-8000-000000000004"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v5"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-one"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)
	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v5")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v5", "option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	// The rotation restarts the dependent; the replacement blocks in its
	// detection with the OLD vnode snapshot already taken at creation.
	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	select {
	case <-entered:
	case <-time.After(charWait):
		t.Fatal("the dependent restart's detection did not start")
	}

	// Mid-gap vnode update: the store command holds no vnode claim, and
	// applyVnodeUpdate reaches no job (the dependent is between instances).
	h.dyncfg("vn-update", []string{h.mgr.dyncfgVnodePrefixValue() + ":v5", "update"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update 202"), charWait, charTick,
		"the vnode update must proceed during the restart gap")

	// The replacement registers AFTER the vnode update committed: it must
	// receive the current config at registration, not run on host-one.
	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)
	require.Eventually(t, func() bool { return strings.Contains(jobOut.String(), "host-two") }, charWait, charTick,
		"the restarted dependent's vnode host info must carry the config updated during the restart gap")
}

// A vnode update that commits while a dependent's key is wedged reaches only
// RUNNING jobs (applyVnodeUpdate): the dependent's late detection success
// must still start with the CURRENT vnode config - the resume re-delivers
// the update the warm job missed, exactly as the live update would have.
func TestWarmResume_RefreshesVnodeUpdatedWhileWedged(t *testing.T) {
	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-gate // the enable's detection wedges past the deadline
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	// The job's chart/host wire output is observed separately from the dyncfg
	// responder output: the refreshed vnode identity shows up as HOSTINFO on
	// the job writer.
	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService(), Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	mgr.effectDeadline = testEffectDeadline

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(in)
		mgr.Run(ctx, in)
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), charWait)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx))
	t.Cleanup(func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	const guid = "b0b0b0b0-0000-4000-8000-000000000003"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v3"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-one"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	// The job binds the vnode at creation (host-one) and its detection wedges
	// past the deadline; the abandon releases the vnode read claim.
	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v3")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v3"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-add"), charWait, charTick)
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")

	// The vnode update proceeds (the read claim was released at the abandon)
	// and reaches no jobs: the warm job is not running yet.
	h.dyncfg("vn-update", []string{h.mgr.dyncfgVnodePrefixValue() + ":v3", "update"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update 202"), charWait, charTick,
		"the vnode update must proceed once the wedged dependent's read claim released")

	// Late success: the warm job starts (no re-detection) and its first
	// emission carries the UPDATED vnode identity, not the stale snapshot.
	close(gate)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick,
		"the late detection success must start the warm job")
	require.Eventually(t, func() bool { return strings.Contains(jobOut.String(), "host-two") }, charWait, charTick,
		"the warm job's vnode host info must carry the config updated during the wedge")
	assert.NotContains(t, jobOut.String(), "host-one",
		"the stale vnode snapshot must never reach the wire")
	assert.Equal(t, int32(1), inits.Load(), "the warm start must not re-run detection")
}
