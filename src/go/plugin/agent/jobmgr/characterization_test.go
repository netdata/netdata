// SPDX-License-Identifier: GPL-3.0-or-later

// Characterization tests: executable pins of the pipeline's behavior
// contracts, written before the concurrency redesign so later changes diff
// against an explicit specification instead of implicit behavior.
//
// Two classes, marked per test:
//
//   - FLIPS-BY-DESIGN: pins current global-serialization behavior that the
//     per-key concurrency redesign intentionally inverts. A redesign PR must
//     update such a test deliberately (invert the assertion), never delete it
//     as "broken".
//   - MUST-NOT-FLIP: load-bearing contracts that hold in both the serialized
//     and the concurrent designs (never-drop, terminal-once, per-command wire
//     order, dyncfg-commands-execute-while-waiting).

package jobmgr

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/wiretest"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	charNeverWait = 200 * time.Millisecond
	charWait      = 5 * time.Second
	charTick      = 10 * time.Millisecond
)

type charHarness struct {
	mgr *Manager
	out *simOutput
	in  chan []*confgroup.Group
}

func startCharManager(t *testing.T, modules collectorapi.Registry) *charHarness {
	t.Helper()

	var out simOutput
	mgr := New(Config{PluginName: testPluginName})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = modules

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
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

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

func (h *charHarness) dyncfg(uid string, args []string, payload []byte) {
	fn := functions.Function{UID: uid, Args: args}
	if payload != nil {
		fn.Payload = payload
		fn.ContentType = "application/json"
	}
	h.mgr.dyncfgConfig(dyncfg.NewFunction(fn))
}

func (h *charHarness) outputContains(substr string) func() bool {
	return func() bool { return strings.Contains(h.out.String(), substr) }
}

func charSuccessCreator() collectorapi.Creator {
	return collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	}
}

// INVERTED FLIPS-BY-DESIGN pin (previously pinned global serialization):
// with per-key lanes an unrelated config's commands MUST complete while
// another config's slow detection is in flight. Regressing this back to
// cross-key blocking is a defect.
func TestCharacterization_RunLoopSerialization(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"slow enable blocks unrelated dyncfg command": {
			run: func(t *testing.T) {
				started := make(chan struct{}, 4)
				release := make(chan struct{})

				reg := collectorapi.Registry{}
				reg.Register("slow", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								select {
								case started <- struct{}{}:
								default:
								}
								<-release
								return nil
							},
							ChartsFunc: func() *collectorapi.Charts {
								return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
							},
							CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
						}
					},
				})
				reg.Register("success", charSuccessCreator())

				h := startCharManager(t, reg)
				defer close(release)

				h.dyncfg("1-add-a", []string{h.mgr.dyncfgModID("slow"), "add", "a"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-add-a"), charWait, charTick)

				h.dyncfg("2-enable-a", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("slow", "a")), "enable"}, nil)
				select {
				case <-started:
				case <-time.After(charWait):
					t.Fatal("slow module Init did not start")
				}

				// The unrelated add runs on its own key lane: its terminal must
				// arrive while the slow enable is still blocked in AutoDetection.
				h.dyncfg("3-add-b", []string{h.mgr.dyncfgModID("success"), "add", "b"}, []byte("{}"))

				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-add-b"), charWait, charTick,
					"unrelated command did not complete while another config's enable was blocked in AutoDetection; "+
						"per-key concurrency regressed to cross-key blocking")
				assert.False(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable-a")(),
					"the slow enable must still be in flight")

				release <- struct{}{}

				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable-a"), charWait, charTick)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// Mixed classes, marked per case:
//   - INVERTED FLIPS-BY-DESIGN: waiting is scoped to the affected key -
//     while one discovered config awaits its enable/disable decision,
//     unrelated adds AND removals MUST proceed (previously the global gate
//     froze all discovery).
//   - MUST-NOT-FLIP: dyncfg commands execute immediately while a key is
//     wait-parked, with state-machine outcomes - load-bearing under per-key
//     lanes too (a parked command would deadlock its functions lane against
//     the gate-clearing enable).
func TestCharacterization_WaitGateBehavior(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, h *charHarness)
	}{
		"wait-parked key does not block unrelated discovery": {
			run: func(t *testing.T, h *charHarness) {
				cfgA := prepareUserCfg("success", "a")
				cfgB := prepareUserCfg("success", "b")

				h.in <- prepareCfgGroups(cfgA.Source(), cfgA)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:a create accepted"), charWait, charTick,
					"first discovered config was not exposed")

				h.in <- prepareCfgGroups(cfgB.Source(), cfgB)

				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return ok
				}, charWait, charTick,
					"second discovered config was not exposed while the first awaited its decision; "+
						"waiting regressed to a global freeze")

				// The parked key still takes its decision.
				h.dyncfg("1-enable-a", []string{h.mgr.dyncfgJobID(cfgA), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:a status running"), charWait, charTick)
			},
		},
		"wait-parked key does not block unrelated removal": {
			run: func(t *testing.T, h *charHarness) {
				cfgA := prepareUserCfg("success", "a")
				cfgB := prepareUserCfg("success", "b")

				// Establish B as a running job first, then park A awaiting its decision.
				h.in <- prepareCfgGroups(cfgB.Source(), cfgB)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:b create accepted"), charWait, charTick)
				h.dyncfg("1-enable-b", []string{h.mgr.dyncfgJobID(cfgB), "enable"}, nil)
				// Entry.Status is loop-owned (dyncfg cache contract) - observe the
				// transition through the wire output instead of reading the field.
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:b status running"), charWait, charTick)

				h.in <- prepareCfgGroups(cfgA.Source(), cfgA)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:a create accepted"), charWait, charTick,
					"second discovered config was not exposed")

				// B vanishes from discovery while A awaits its decision: the
				// removal MUST be consumed (waiting is per-key).
				h.in <- prepareCfgGroups(cfgB.Source())

				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return !ok
				}, charWait, charTick,
					"unrelated config's removal was not processed while another key awaited a decision; "+
						"waiting regressed to a global freeze")

				// A's decision still lands.
				h.dyncfg("2-enable-a", []string{h.mgr.dyncfgJobID(cfgA), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:a status running"), charWait, charTick)
			},
		},
		"dyncfg commands execute while a key is wait-parked": {
			run: func(t *testing.T, h *charHarness) {
				cfg := prepareUserCfg("success", "waity")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:waity create accepted"), charWait, charTick)

				h.dyncfg("1-update", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-update"), charWait, charTick)
				var updateResp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "1-update", &updateResp)
				assert.Equal(t, float64(403), updateResp["status"], "premature update must be rejected with 403")

				h.dyncfg("2-restart", []string{h.mgr.dyncfgJobID(cfg), "restart"}, nil)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-restart"), charWait, charTick)
				var restartResp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "2-restart", &restartResp)
				assert.Equal(t, float64(405), restartResp["status"], "premature restart must be rejected with 405")

				// Non-decision commands must not have cleared the wait park: the
				// key still takes its decision and only then starts.
				assert.False(t, h.outputContains("CONFIG test:collector:success:waity status running")(),
					"job started before its enable decision")
				h.dyncfg("3-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-enable"), charWait, charTick)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:waity status running"), charWait, charTick,
					"matching enable did not unpark the key")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := collectorapi.Registry{}
			reg.Register("success", charSuccessCreator())

			tc.run(t, startCharManager(t, reg))
		})
	}
}

// MUST-NOT-FLIP: each dyncfg domain keeps its current wire order. Shared
// handler collector commands emit terminal-before-CONFIG for the same command;
// secretstore dependent restarts emit dependent CONFIG STATUS records before
// the store command's terminal. These are atomic-record subsequence pins, not
// global transcript pins, so later cross-key interleavings can change safely.
func TestCharacterization_WireOrderByDomain(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"collector shared handler terminal precedes same-command CONFIG records": {
			run: func(t *testing.T) {
				reg := collectorapi.Registry{}
				reg.Register("success", charSuccessCreator())

				h := startCharManager(t, reg)

				cfg := prepareDyncfgCfg("success", "ord")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("success"), "add", "ord"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)

				require.Eventually(t, h.outputContains("CONFIG test:collector:success:ord status running"), charWait, charTick)

				wiretest.RequireSubsequence(t, h.out.String(), []wiretest.RecordWant{
					{
						Name:     "add terminal",
						Contains: []string{"FUNCTION_RESULT_BEGIN 1-add"},
					},
					{
						Name:     "add config create",
						Contains: []string{"CONFIG test:collector:success:ord create"},
					},
					{
						Name:     "enable terminal",
						Contains: []string{"FUNCTION_RESULT_BEGIN 2-enable"},
					},
					{
						Name:     "enable config status",
						Contains: []string{"CONFIG test:collector:success:ord status running"},
					},
				})
			},
		},
		"secretstore dependent CONFIG STATUS precedes store terminal": {
			run: func(t *testing.T) {
				mgr, out := newDyncfgSecretStoreTestManagerWithService(newTestSecretStoreService())
				mgr.modules["gated"] = collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 { return &secretAwareCollector{} },
				}

				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("gated", "mysql").
					Set("option_str", "${store:vault:vault_prod:value}").
					Set("option_int", 1)
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{Cfg: cfg, Status: dyncfg.StatusRunning})
				mgr.syncSecretStoreDepsForConfig(cfg)
				require.NoError(t, mgr.collectorCallbacks.Start(context.Background(), cfg))

				badFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-bad",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "bad"}),
					Args:        []string{mgr.dyncfgSecretStoreID(key), string(dyncfg.CommandUpdate)},
				})
				mgr.dyncfgSecretStoreSeqExec(badFn)

				output := out.String()
				wiretest.RequireSubsequence(t, output, []wiretest.RecordWant{
					{
						Name:     "dependent job failed status",
						Contains: []string{"CONFIG test:collector:gated:mysql status failed"},
					},
					{
						Name:     "store update terminal",
						Contains: []string{"FUNCTION_RESULT_BEGIN ss-update-bad"},
					},
				})

				var resp map[string]any
				mustDecodeFunctionPayload(t, output, "ss-update-bad", &resp)
				assert.Contains(t, resp["message"], "dependent collector restarts failed",
					"restart failures must be embedded in the terminal message body")
				assert.Contains(t, resp["message"], "gated:mysql")
			},
		},
		"secretstore multi-dependent CONFIG STATUS replay follows sorted job order": {
			run: func(t *testing.T) {
				mgr, out := newDyncfgSecretStoreTestManagerWithService(newTestSecretStoreService())
				mgr.modules["gated"] = collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 { return &secretAwareCollector{} },
				}

				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, dyncfg.StatusRunning)

				// Registered in reverse name order on purpose: the restart
				// sequence follows the dependency index's sorted job order,
				// not registration order.
				for _, name := range []string{"beta", "alpha"} {
					cfg := prepareDyncfgCfg("gated", name).
						Set("option_str", "${store:vault:vault_prod:value}").
						Set("option_int", 1)
					mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{Cfg: cfg, Status: dyncfg.StatusRunning})
					mgr.syncSecretStoreDepsForConfig(cfg)
					require.NoError(t, mgr.collectorCallbacks.Start(context.Background(), cfg))
				}

				badFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-bad",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "bad"}),
					Args:        []string{mgr.dyncfgSecretStoreID(key), string(dyncfg.CommandUpdate)},
				})
				mgr.dyncfgSecretStoreSeqExec(badFn)

				output := out.String()
				wiretest.RequireSubsequence(t, output, []wiretest.RecordWant{
					{
						Name:     "first dependent in sorted order fails",
						Contains: []string{"CONFIG test:collector:gated:alpha status failed"},
					},
					{
						Name:     "second dependent in sorted order fails",
						Contains: []string{"CONFIG test:collector:gated:beta status failed"},
					},
					{
						Name:     "store update terminal",
						Contains: []string{"FUNCTION_RESULT_BEGIN ss-update-bad"},
					},
				})

				var resp map[string]any
				mustDecodeFunctionPayload(t, output, "ss-update-bad", &resp)
				msg, _ := resp["message"].(string)
				assert.Regexp(t,
					`^Secretstore change applied, but dependent collector restarts failed: gated:alpha \(.+\); gated:beta \(.+\)\.$`,
					msg, "per-job failures must join in sorted job order with the pinned format")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// MUST-NOT-FLIP: dyncfg command handoff never drops. When the queue is full,
// enqueueDyncfgFunction blocks until space frees (back-pressure), and only
// shutdown produces a 503 instead of delivery.
func TestCharacterization_EnqueueDyncfgHandoff(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"full queue blocks until drained and shutdown returns 503": {
			run: func(t *testing.T) {
				var out bytes.Buffer
				mgr := New(Config{PluginName: testPluginName})
				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				mgr.ctx = ctx

				for i := 0; i < cap(mgr.dyncfgCh); i++ {
					mgr.dyncfgCh <- dyncfg.NewFunction(functions.Function{UID: fmt.Sprintf("fill-%d", i)})
				}

				delivered := make(chan struct{})
				go func() {
					mgr.enqueueDyncfgFunction(dyncfg.NewFunction(functions.Function{UID: "blocked"}))
					close(delivered)
				}()

				deliveredNow := func() bool {
					select {
					case <-delivered:
						return true
					default:
						return false
					}
				}
				require.Never(t, deliveredNow, charNeverWait, charTick,
					"enqueue must block (not drop, not fail) while the queue is full")

				<-mgr.dyncfgCh // free one slot
				require.Eventually(t, deliveredNow, charWait, charTick, "enqueue did not complete after space freed")

				// Shutdown is the only path that answers instead of delivering: 503.
				cancel()
				mgr.enqueueDyncfgFunction(dyncfg.NewFunction(functions.Function{UID: "after-shutdown"}))
				assert.Contains(t, out.String(), "FUNCTION_RESULT_BEGIN after-shutdown 503")

				// dyncfgConfig front door rejects with 503 after shutdown too.
				mgr.dyncfgConfig(dyncfg.NewFunction(functions.Function{
					UID:  "front-door",
					Args: []string{"test:collector:success:x", "enable"},
				}))
				assert.Contains(t, out.String(), "FUNCTION_RESULT_BEGIN front-door 503")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// MUST-NOT-FLIP: removing a config cancels its pending auto-detection retry;
// the retry must not resurrect the job after removal.
func TestCharacterization_RetryTasks(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"scheduled retry is canceled when discovery removes config": {
			run: func(t *testing.T) {
				reg := collectorapi.Registry{}
				reg.Register("flaky", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error { return fmt.Errorf("flaky init") },
						}
					},
				})

				h := startCharManager(t, reg)

				cfg := prepareUserCfg("flaky", "r1").Set("autodetection_retry", 1)
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:flaky:r1 create accepted"), charWait, charTick)

				h.dyncfg("1-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("job enable failed"), charWait, charTick)
				// Entry.Status is loop-owned (dyncfg cache contract) - observe the
				// failed transition through the wire output; check exposure by key only.
				require.Eventually(t, h.outputContains("CONFIG test:collector:flaky:r1 status failed"), charWait, charTick)
				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfg.ExposedKey())
					return ok
				}, charWait, charTick, "failed user config must stay exposed")

				// Discovery removes the config: the scheduled retry must be canceled.
				h.in <- prepareCfgGroups(cfg.Source())
				require.Eventually(t, func() bool { return h.mgr.collectorExposed.Count() == 0 }, charWait, charTick)

				// The retry interval is 1s; watch past it to prove the retry was canceled
				// and does not re-add the config.
				require.Never(t, func() bool { return h.mgr.collectorExposed.Count() != 0 }, 1500*time.Millisecond, 50*time.Millisecond,
					"a canceled auto-detection retry resurrected a removed config")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// MUST-NOT-FLIP: fileStatus add/remove hook points are part of the dyncfg
// persistence baseline. Start/Update do not add status directly; add happens
// only when a dyncfg-sourced config transitions to running, while Stop/Update
// remove the old persisted status.
func TestCharacterization_FileStatusHookPoints(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, cb *collectorCallbacks)
	}{
		"dyncfg start waits for running status transition before adding file status": {
			run: func(t *testing.T, mgr *Manager, cb *collectorCallbacks) {
				cfg := prepareDyncfgCfg("success", "job")

				require.NoError(t, cb.Start(context.Background(), cfg))
				t.Cleanup(func() { mgr.stopRunningJob(context.Background(), cfg.FullName()) })
				_, running := mgr.runningJobs.lookup(cfg.FullName())
				require.True(t, running)

				_, ok := mgr.fileStatus.lookup(cfg)
				assert.False(t, ok, "Start must not add fileStatus before the running status transition")

				cb.OnStatusChange(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				}, dyncfg.StatusAccepted, dyncfg.NewFunction(functions.Function{}))

				status, ok := mgr.fileStatus.lookup(cfg)
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning.String(), status)
			},
		},
		"non-dyncfg running status transition does not add file status": {
			run: func(t *testing.T, mgr *Manager, cb *collectorCallbacks) {
				cfg := prepareUserCfg("success", "job")

				cb.OnStatusChange(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				}, dyncfg.StatusAccepted, dyncfg.NewFunction(functions.Function{}))

				_, ok := mgr.fileStatus.lookup(cfg)
				assert.False(t, ok)
			},
		},
		"stop removes file status": {
			run: func(t *testing.T, mgr *Manager, cb *collectorCallbacks) {
				cfg := prepareDyncfgCfg("success", "job")
				job := &collectorProbeJob{
					fullName:   cfg.FullName(),
					moduleName: cfg.Module(),
					name:       cfg.Name(),
				}
				mgr.runningJobs.lock()
				mgr.runningJobs.add(job.FullName(), job)
				mgr.runningJobs.unlock()
				mgr.fileStatus.add(cfg, dyncfg.StatusRunning.String())

				cb.Stop(context.Background(), cfg)

				assert.True(t, job.stopped)
				_, ok := mgr.fileStatus.lookup(cfg)
				assert.False(t, ok)
			},
		},
		"update removes old file status and does not add new status before transition": {
			run: func(t *testing.T, mgr *Manager, cb *collectorCallbacks) {
				oldCfg := prepareDyncfgCfg("success", "job")
				newCfg := prepareDyncfgCfg("success", "job").Set("option_str", "changed")
				job := &collectorProbeJob{
					fullName:   oldCfg.FullName(),
					moduleName: oldCfg.Module(),
					name:       oldCfg.Name(),
				}
				mgr.runningJobs.lock()
				mgr.runningJobs.add(job.FullName(), job)
				mgr.runningJobs.unlock()
				mgr.fileStatus.add(oldCfg, dyncfg.StatusRunning.String())

				require.NoError(t, cb.Update(context.Background(), oldCfg, newCfg))
				t.Cleanup(func() { mgr.stopRunningJob(context.Background(), newCfg.FullName()) })

				assert.True(t, job.stopped)
				_, oldOK := mgr.fileStatus.lookup(oldCfg)
				assert.False(t, oldOK)
				_, newOK := mgr.fileStatus.lookup(newCfg)
				assert.False(t, newOK, "Update must wait for OnStatusChange before adding the new fileStatus entry")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			tc.run(t, mgr, &collectorCallbacks{mgr: mgr})
		})
	}
}
