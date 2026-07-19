// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/wiretest"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// startSecretStoreEffectHarness builds a running manager with the given
// registry and the vault test-store service.
func startSecretStoreEffectHarness(t *testing.T, reg collectorapi.Registry, tune func(*Manager)) *charHarness {
	t.Helper()

	var out simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService()})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	if tune != nil {
		tune(mgr)
	}

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
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	return &charHarness{mgr: mgr, out: &out, in: in}
}

// Two store commands whose effects overlap must never cross-attribute their
// terminal messages: each command's message rides its own effect contexts.
func TestSecretStoreCommands_ConcurrentMessagesStayPerCommand(t *testing.T) {
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
				if c.Config.OptionStr == "good" {
					return nil
				}
				// A rotated (bad) secret: hold the restart so both store
				// commands' effects overlap, then fail with the resolved
				// value so each terminal identifies its own dependent.
				<-release
				return fmt.Errorf("secret is not usable: %s", c.Config.OptionStr)
			}
			return c
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

	for _, store := range []string{"store_a", "store_b"} {
		h.dyncfg("ss-add-"+store, []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", store},
			mustJSON(t, map[string]any{"value": "good"}))
		require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add-"+store+" 200"), charWait, charTick)
	}
	for job, store := range map[string]string{"a1": "store_a", "b1": "store_b"} {
		h.dyncfg("add-"+job, []string{h.mgr.dyncfgModID("gated"), "add", job},
			mustJSON(t, map[string]any{"option_str": fmt.Sprintf("${store:vault:%s:value}", store)}))
		h.dyncfg("en-"+job, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("gated", job)), "enable"}, nil)
		require.Eventually(t, h.outputContains("CONFIG test:collector:gated:"+job+" status running"), charWait, charTick)
	}

	// Rotate both stores to bad values: the dependent restarts block, so both
	// store effects are in flight at once.
	h.dyncfg("ss-update-a", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:store_a", "update"},
		mustJSON(t, map[string]any{"value": "bad-a"}))
	h.dyncfg("ss-update-b", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:store_b", "update"},
		mustJSON(t, map[string]any{"value": "bad-b"}))
	require.Eventually(t, func() bool { return inits.Load() >= 4 }, charWait, charTick,
		"both dependents' restarts must be in flight concurrently")

	close(release)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update-a 200"), charWait, charTick)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update-b 200"), charWait, charTick)

	var respA, respB map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update-a", &respA)
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update-b", &respB)
	msgA, _ := respA["message"].(string)
	msgB, _ := respB["message"].(string)
	assert.Contains(t, msgA, "gated:a1", "store_a's terminal must report its own dependent")
	assert.NotContains(t, msgA, "gated:b1", "store_a's terminal must not carry store_b's dependent")
	assert.Contains(t, msgB, "gated:b1", "store_b's terminal must report its own dependent")
	assert.NotContains(t, msgB, "gated:a1", "store_b's terminal must not carry store_a's dependent")
}

// The store test's affected/restartable impact lists are point-in-time
// advisory, recomputed AT COMMIT: a dependency that appears while the test's
// validation runs shows up in the answer (the test holds only a read claim,
// so collector work on the store proceeds).
func TestSecretStoreTest_AdvisoryListsRecomputedAtCommit(t *testing.T) {
	validateGate := make(chan struct{})

	// A dedicated service whose store Init blocks on the marker value: the
	// test command's validation effect parks on it deterministically.
	svc := secretstore.NewService(secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
		Schema:      `{"jsonSchema":{"type":"object","properties":{"value":{"type":"string"}}},"uiSchema":[]}`,
		Create: func() secretstore.Store {
			return &gatedValidateStore{gate: validateGate}
		},
	})

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	var out simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: svc})
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
		case <-validateGate:
		default:
			close(validateGate)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	// The test's validation effect blocks on the marker value.
	h.dyncfg("ss-test", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "test"},
		mustJSON(t, map[string]any{"value": "slow-validate"}))
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-test"), charNeverWait, charTick,
		"the test's validation must be in flight")

	// A dependency change during the validation window: a collector job
	// referencing the store goes running (its store READ claim shares the
	// test's read claim).
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick,
		"collector work on the store must proceed during the advisory test")

	close(validateGate)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-test 202"), charWait, charTick)

	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-test", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "gated:mysql",
		"the impact lists must be recomputed at commit and include the dependency added during validation")
}

// gatedValidateStore blocks Init while validating the marker value, so a
// store test's validation effect can be held open deterministically.
type gatedValidateStore struct {
	testStoreConfig struct {
		Value string `yaml:"value" json:"value"`
	}
	gate <-chan struct{}
}

func (s *gatedValidateStore) Configuration() any { return &s.testStoreConfig }

func (s *gatedValidateStore) Init(context.Context) error {
	if s.testStoreConfig.Value == "slow-validate" {
		<-s.gate
	}
	if s.testStoreConfig.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func (s *gatedValidateStore) Publish() secretstore.PublishedStore {
	return &gatedValidatePublished{value: s.testStoreConfig.Value}
}

type gatedValidatePublished struct{ value string }

func (*gatedValidatePublished) RetainedBytes() int64 { return 512 }

func (s *gatedValidatePublished) Resolve(_ context.Context, req secretstore.ResolveRequest) (string, error) {
	if req.Operand != "value" {
		return "", fmt.Errorf("unexpected operand %q", req.Operand)
	}
	return s.value, nil
}

// A restart sequence cut by the deadline reports a non-nil error alongside
// the message, and wedged-only skips do not: the error is what makes the
// command's classification race-independent - without it, an effect
// returning just as the deadline fires could classify as SUCCESS (store
// running) or the timeout failure depending on which select arm the worker
// happens to pick for the same physical state.
func TestRunDependentRestarts_DeadlineCutReturnsError(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})

	plan := []secretStoreDependentPlan{
		{name: "gated:alpha", display: "gated:alpha", cfg: prepareDyncfgCfg("gated", "alpha")},
		{name: "gated:beta", display: "gated:beta", cfg: prepareDyncfgCfg("gated", "beta")},
	}

	t.Run("expired deadline cuts the sequence with an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // the deadline has already fired when the loop checks

		msg, err := mgr.runDependentRestarts(ctx, "vault:vault_prod", plan, &restartReplayBuffer{})
		require.Error(t, err, "a deadline-cut sequence must return an error, not just message text")
		assert.Contains(t, err.Error(), "timed out")
		assert.Contains(t, msg, "gated:alpha (skipped: the store operation timed out before this restart started)")
		assert.Contains(t, msg, "gated:beta (skipped: the store operation timed out before this restart started)")
	})

	t.Run("wedged-only skips stay a successful outcome", func(t *testing.T) {
		wedgedPlan := []secretStoreDependentPlan{
			{name: "gated:alpha", display: "gated:alpha", cfg: plan[0].cfg, wedged: true},
		}
		msg, err := mgr.runDependentRestarts(context.Background(), "vault:vault_prod", wedgedPlan, &restartReplayBuffer{})
		require.NoError(t, err, "skip-and-report on wedged dependents is the command's normal outcome")
		assert.Contains(t, msg, "gated:alpha (skipped: operation in progress)")
	})
}

// A store command abandoned at its deadline mid-restart-sequence: dependents
// already restarted still commit (their CONFIG STATUS replays before the
// terminal), no new restarts launch after the deadline, and the terminal
// carries the timeout failure.
func TestSecretStoreUpdate_DeadlineMidSequencePartialOutcome(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32
	var gammaRestarts atomic.Int32

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
				switch c.Config.OptionStr {
				case "rotated":
					switch c.Config.OptionInt {
					case 2: // beta: wedges the restart sequence past the deadline
						<-release
					case 3: // gamma: must never restart
						gammaRestarts.Add(1)
					}
				}
				return nil
			}
			return c
		},
	})

	h := startSecretStoreEffectHarness(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
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

	// Three dependents; restart order is the sorted job order:
	// alpha, beta, gamma.
	for job, idx := range map[string]int{"alpha": 1, "beta": 2, "gamma": 3} {
		h.dyncfg("add-"+job, []string{h.mgr.dyncfgModID("gated"), "add", job},
			mustJSON(t, map[string]any{
				"option_str": "${store:vault:vault_prod:value}",
				"option_int": idx,
			}))
		h.dyncfg("en-"+job, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("gated", job)), "enable"}, nil)
		require.Eventually(t, h.outputContains("CONFIG test:collector:gated:"+job+" status running"), charWait, charTick)
	}

	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))

	// The sequence wedges inside beta's restart and the command commits its
	// deadline outcome: alpha's completed restart still replays before the
	// terminal, the store goes failed.
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick,
		"the update must commit its deadline outcome while beta's restart is wedged")

	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "timed out", "the terminal must carry the abandonment")

	output := h.out.String()
	wiretest.RequireSubsequence(t, output, []wiretest.RecordWant{
		{
			Name:     "alpha's completed restart replays",
			Contains: []string{"CONFIG test:collector:gated:alpha status running"},
		},
		{
			Name:     "store update terminal",
			Contains: []string{"FUNCTION_RESULT_BEGIN ss-update 200"},
		},
		{
			Name:     "store failed status",
			Contains: []string{"CONFIG " + h.mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")) + " status failed"},
		},
	})
	assert.Equal(t, 2, strings.Count(output, "CONFIG test:collector:gated:alpha status running"),
		"alpha: one enable line plus one replayed restart line")

	// Unwedge: beta's leaked restart returns; gamma must never have been
	// launched (no new restarts after the deadline).
	close(release)
	require.Eventually(t, func() bool { return h.mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the leaked store effect must return after release")
	assert.Zero(t, gammaRestarts.Load(), "no new dependent restarts may launch after the deadline")
}

// A collector update that changes the job's store reference must finalize
// the dependency index when its outcome commits - INCLUDING the
// deadline-abandon commit, where the job's key stays wedged and no settle
// runs: a later rotation of the NEW store must see the job as a dependent
// (and report it as skipped while it is wedged), never silently miss it.
func TestSecretStoreRotation_SeesDependentRereferencedByAbandonedUpdate(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the re-reference update's detection wedges
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

	h := startSecretStoreEffectHarness(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	for _, store := range []string{"store_a", "store_b"} {
		h.dyncfg("ss-add-"+store, []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", store},
			mustJSON(t, map[string]any{"value": "good"}))
		require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add-"+store+" 200"), charWait, charTick)
	}

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:store_a:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	// The job update re-references store_b; its detection wedges past the
	// deadline. The update commits FAILED with the store_b config exposed,
	// releases its READ claim on store_b, and the job's key stays wedged.
	h.dyncfg("3-update", []string{h.mgr.dyncfgJobID(cfg), "update"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:store_b:value}"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-update 200"), charWait, charTick,
		"the update must commit its deadline outcome while its detection is wedged")

	// The store_b rotation proceeds (the read claim was released at the
	// abandon) and must SEE the failed job as a store_b dependent: wedged,
	// so skipped-and-reported - a stale dependency index would silently
	// miss it.
	h.dyncfg("ss-update-b", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:store_b", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update-b 200"), charWait, charTick,
		"the rotation must not wait on the wedged dependent")

	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update-b", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "gated:mysql",
		"the dependency index must reflect the abandoned update's re-reference before its claims release")
	assert.Contains(t, msg, "skipped: operation in progress")
}

// A store command abandoned mid-restart keeps its WRITE claims on the
// dependents until the leaked restart returns: a newer command on the
// dependent must wait it out (no resurrection of just-disabled work), and
// the leaked restart's status replay is delivered late, before the claims
// release.
func TestSecretStoreUpdate_AbandonedRestartHoldsDependentClaim(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the rotation's dependent restart wedges
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

	h := startSecretStoreEffectHarness(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
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

	// The rotation's dependent restart wedges past the deadline: the store
	// command commits its timeout outcome while the leaked restart runs on.
	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick,
		"the rotation must commit its deadline outcome while the restart is wedged")

	// The dependent's write claim is still held by the leaked restart: a
	// user disable must WAIT, not run concurrently with (or be resurrected
	// over by) the leaked Stop/Start.
	h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable"), charNeverWait, charTick,
		"the disable must wait out the leaked restart's write claim on the dependent")

	close(release)
	// The leaked restart finishes: its status replay is delivered late
	// (while the claim is still held), then the claims release and the
	// parked disable executes against the settled state.
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable 200"), charWait, charTick,
		"the parked disable must proceed once the leaked restart returns")
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status disabled"), charWait, charTick)

	wiretest.RequireSubsequence(t, h.out.String(), []wiretest.RecordWant{
		{Name: "rotation deadline terminal", Contains: []string{"FUNCTION_RESULT_BEGIN ss-update 200"}},
		{Name: "late restart replay", Contains: []string{"CONFIG test:collector:gated:mysql status running"}},
		{Name: "parked disable terminal", Contains: []string{"FUNCTION_RESULT_BEGIN 3-disable 200"}},
		{Name: "dependent disabled", Contains: []string{"CONFIG test:collector:gated:mysql status disabled"}},
	})

	h.mgr.runningJobs.lock()
	_, running := h.mgr.runningJobs.lookup(cfg.FullName())
	h.mgr.runningJobs.unlock()
	assert.False(t, running, "no resurrected instance may survive the disable")
}

// A store mutation parked behind a BUSY dependent must be re-attempted when
// that dependent WEDGES: the wedged key's write claim has no bounded
// release, so the parked command's recompute (which excludes wedged keys)
// must run at the wedge transition - skip-and-reporting the dependent -
// instead of waiting out the leaked collector call.
func TestSecretStoreUpdate_UnparksWhenParkedDependentWedges(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						// The user restart's detection wedges past the deadline.
						select {
						case entered <- struct{}{}:
						default:
						}
						<-release
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

	h := startSecretStoreEffectHarness(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
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

	// A user restart occupies the dependent's key (its detection is in
	// flight, the key merely BUSY); the rotation sent now claims the RUNNING
	// dependent for restart and parks behind that write claim.
	h.dyncfg("3-restart", []string{h.mgr.dyncfgJobID(cfg), "restart"}, nil)
	select {
	case <-entered:
	case <-time.After(charWait):
		t.Fatal("the restart's detection did not start")
	}
	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))

	// The restart wedges at its deadline WITHOUT the leaked call returning:
	// the parked rotation must be re-attempted at the wedge transition,
	// exclude the now-wedged dependent, and commit with the skip report.
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick,
		"the parked rotation must proceed once the dependent wedges, not wait out the leaked call")
	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "gated:mysql (skipped: operation in progress)",
		"the rotation must report the wedged dependent as skipped")

	// Late success after the committed rotation: the warm replacement carries
	// pre-rotation credentials and is dropped, never started.
	close(release)
	h.dyncfg("4-get", []string{h.mgr.dyncfgJobID(cfg), "get"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 4-get"), charWait, charTick,
		"the key must unwedge and settle after the late return")
	assert.Equal(t, 1, strings.Count(h.out.String(), "CONFIG test:collector:gated:mysql status running"),
		"the stale warm replacement must not publish a second running transition")
	h.mgr.runningJobs.lock()
	_, running := h.mgr.runningJobs.lookup(cfg.FullName())
	h.mgr.runningJobs.unlock()
	assert.False(t, running, "the stale warm replacement must be dropped, not registered")
}

// A store rotation that commits while a dependent's key is wedged reports
// that dependent as skipped ("skipped: operation in progress"); the
// dependent's late detection success must then be DROPPED, never started:
// the warm job resolved its secrets from the pre-rotation store, and with
// the rotation already committed nothing would ever restart it onto the new
// value.
func TestWarmResume_DroppedAfterStoreRotationCommitted(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-release // the enable's detection wedges past the deadline
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

	h := startSecretStoreEffectHarness(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
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
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")

	// The rotation commits fully: the wedged dependent is skipped-and-reported
	// and the store now holds the rotated value.
	h.dyncfg("ss-update", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)
	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "gated:mysql (skipped: operation in progress)",
		"the rotation must report the wedged dependent as skipped")

	// Late success: the store changed while the key was wedged - the warm job
	// carries pre-rotation credentials and must be dropped, not started.
	close(release)
	h.dyncfg("3-get", []string{h.mgr.dyncfgJobID(cfg), "get"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-get"), charWait, charTick,
		"the key must unwedge and settle after the late return")
	assert.NotContains(t, h.out.String(), "CONFIG test:collector:gated:mysql status running",
		"a warm job resolved from the pre-rotation store must never start")
	h.mgr.runningJobs.lock()
	_, running := h.mgr.runningJobs.lookup(cfg.FullName())
	h.mgr.runningJobs.unlock()
	assert.False(t, running, "the stale warm job must be dropped, not registered")
}

// A store mutation IN FLIGHT at the late return (granted, its effect
// possibly past activation but its loop-side commit not yet run) must also
// drop the warm continuation: here the mutation is wedged inside backend
// validation - the store's committed identity is provably unchanged, so the
// write-hold check is the only guard standing between the warm job and a
// start that could race the activation.
func TestWarmResume_DroppedWhileStoreMutationInFlight(t *testing.T) {
	wedgeGate := make(chan struct{})
	validateGate := make(chan struct{})
	var inits atomic.Int32

	svc := secretstore.NewService(secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
		Schema:      `{"jsonSchema":{"type":"object","properties":{"value":{"type":"string"}}},"uiSchema":[]}`,
		Create: func() secretstore.Store {
			return &gatedValidateStore{gate: validateGate}
		},
	})

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 1 {
						<-wedgeGate // the enable's detection wedges past the deadline
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

	var out simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: svc})
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
		for _, gate := range []chan struct{}{wedgeGate, validateGate} {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	h.dyncfg("ss-add", []string{mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status failed"), charWait, charTick,
		"the enable must commit its deadline outcome while the detection is wedged")

	// The store update's backend validation wedges past the deadline: the
	// command commits 400, its loop-side entry stays UNCHANGED, and its write
	// claim on the store key is held until the leaked validation returns.
	h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "slow-validate"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 400"), charWait, charTick,
		"the store update must commit its deadline outcome while its validation is wedged")

	// Late success under the in-flight mutation: identity is unchanged, so
	// only the write-hold check can (and must) drop the warm job.
	close(wedgeGate)
	h.dyncfg("3-get", []string{mgr.dyncfgJobID(cfg), "get"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-get"), charWait, charTick,
		"the key must unwedge and settle after the late return")
	assert.NotContains(t, h.out.String(), "CONFIG test:collector:gated:mysql status running",
		"a warm job must not start while a mutation of its store is in flight")
	h.mgr.runningJobs.lock()
	_, running := h.mgr.runningJobs.lookup(cfg.FullName())
	h.mgr.runningJobs.unlock()
	assert.False(t, running, "the warm job must be dropped, not registered")

	// Unwedge the store: its leaked validation returns and everything drains.
	close(validateGate)
	require.Eventually(t, func() bool { return mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the leaked store validation must return after release")
}

// A conversion whose dependent restarts are cut by the deadline is
// APPLIED-BUT-DEGRADED and must answer 200 with the failure text - exactly
// like the dyncfg-sourced update's partial outcome - never 400: the
// override was activated in place (activation failures are coded), so a
// bad-request terminal would misreport an applied mutation.
func TestDyncfgSecretStoreConversion_DeadlineCutAnswersAppliedOutcome(t *testing.T) {
	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the conversion's dependent restart wedges
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

	initial := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod",
		map[string]any{"value": "good"}, "file=/etc/netdata/go.d/ss/vault.conf", "user")

	var out simOutput
	mgr := New(Config{
		PluginName:         testPluginName,
		SecretStores:       []secretstore.Config{initial},
		SecretStoreService: newTestSecretStoreService(),
	})
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

	storeID := h.mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
	require.Eventually(t, h.outputContains("CONFIG "+storeID+" create"), charWait, charTick,
		"the file-sourced store must publish at startup")

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	// The update of a file/user-sourced store takes the CONVERSION path: it
	// activates the override in place, then the dependent restart wedges
	// past the deadline.
	h.dyncfg("ss-convert", []string{storeID, "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-convert 200"), charWait, charTick,
		"an applied-but-degraded conversion must answer the 200 partial outcome, not 400")
	assert.NotContains(t, h.out.String(), "FUNCTION_RESULT_BEGIN ss-convert 400",
		"a bad-request terminal would misreport an applied conversion")

	var resp map[string]any
	mustDecodeFunctionPayload(t, h.out.String(), "ss-convert", &resp)
	msg, _ := resp["message"].(string)
	assert.Contains(t, msg, "timed out", "the terminal must carry the degradation")
	require.Eventually(t, h.outputContains("CONFIG "+storeID+" status failed"), charWait, charTick,
		"the applied-but-degraded store commits failed")

	close(release)
	require.Eventually(t, func() bool { return h.mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the leaked conversion effect must drain after release")
}

// A store test shares the store's read claim with in-flight collector
// commands in EITHER arrival order: here the collector enable holds the
// read first and the advisory test must still complete alongside it.
func TestSecretStoreTest_SharesReadClaimWithInFlightCollector(t *testing.T) {
	gate := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					inits.Add(1)
					<-gate // the enable's detection holds the store read claim
					return nil
				},
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

	h.dyncfg("ss-add", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, func() bool { return inits.Load() >= 1 }, charWait, charTick,
		"the enable's detection did not start")

	// Read/read shares: the advisory test must complete while the enable is
	// still in flight.
	h.dyncfg("ss-test", []string{h.mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "test"}, nil)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-test 202"), charWait, charTick,
		"the advisory test must share the store read claim with the in-flight enable")

	close(gate)
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable"), charWait, charTick)
}

// Secretstore backend validation runs under the effect's context: a backend
// that honors cancellation returns at the deadline, so the leaked call
// drains instead of wedging the store key forever.
func TestSecretStoreAdd_ValidationHonorsEffectContext(t *testing.T) {
	svc := secretstore.NewService(secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
		Schema:      `{"jsonSchema":{"type":"object","properties":{"value":{"type":"string"}}},"uiSchema":[]}`,
		Create: func() secretstore.Store {
			return &ctxBlockingStore{}
		},
	})

	var out simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: svc})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = collectorapi.Registry{}
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
		cancel()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Errorf("manager did not stop after cancel")
		}
	})
	h := &charHarness{mgr: mgr, out: &out, in: in}

	// The backend blocks until its context ends: with the effect context
	// threaded through, the deadline both abandons the command AND unblocks
	// the leaked call, so the store key drains instead of wedging forever.
	h.dyncfg("ss-add", []string{mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "ctx-block"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add"), charWait, charTick,
		"the add must reach a terminal at the deadline")
	require.Eventually(t, func() bool { return mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the ctx-honoring backend call must return once the effect context ends")
}

// ctxBlockingStore blocks Init until its context ends when validating the
// marker value.
type ctxBlockingStore struct {
	cfg struct {
		Value string `yaml:"value" json:"value"`
	}
}

func (s *ctxBlockingStore) Configuration() any { return &s.cfg }

func (s *ctxBlockingStore) Init(ctx context.Context) error {
	if s.cfg.Value == "ctx-block" {
		<-ctx.Done()
		return ctx.Err()
	}
	if s.cfg.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func (s *ctxBlockingStore) Publish() secretstore.PublishedStore {
	return &gatedValidatePublished{value: s.cfg.Value}
}

// A dependent restart running inside a store command's effect is
// control-free and its context cause pins to DeadlineExceeded once that
// effect's deadline fires - its late detection success must still never
// start a job once the manager is shutting down (cleanup has already
// snapshotted the running set; a start here would leak a running collector).
func TestSecretStoreDependentRestart_LateSuccessNeverStartsAfterShutdown(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	release := make(chan struct{})
	var inits atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					if inits.Add(1) == 2 {
						<-release // the rotation's dependent restart wedges
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

	var out simOutput
	mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService()})
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

	h.dyncfg("ss-add", []string{mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
		mustJSON(t, map[string]any{"value": "good"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql")
	h.dyncfg("1-add", []string{mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
	h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)

	// The rotation's dependent restart wedges past the deadline: the store
	// command commits its timeout outcome while the restart runs on leaked.
	h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
		mustJSON(t, map[string]any{"value": "rotated"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick,
		"the rotation must commit its deadline outcome while the restart is wedged")

	// Shutdown completes without unblocking the restart.
	cancel()
	select {
	case <-done:
	case <-time.After(charWait):
		t.Fatal("shutdown did not complete")
	}

	// The leaked restart's detection now succeeds - after cleanup. The
	// start guard must drop it (cleanup, no start): a started job here
	// would run unmanaged forever.
	close(release)
	require.Eventually(t, func() bool { return mgr.leakedNow.Load() == 0 }, charWait, charTick,
		"the leaked store effect must drain after release")
	mgr.runningJobs.lock()
	_, running := mgr.runningJobs.lookup(cfg.FullName())
	mgr.runningJobs.unlock()
	assert.False(t, running, "no job may register after shutdown cleanup")
}
