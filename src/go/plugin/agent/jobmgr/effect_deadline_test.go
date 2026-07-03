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

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/wiretest"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// startTunedCharManager is startCharManager with a pre-Run mutation hook
// (the deadline battery shortens the effect deadline per manager).
func startTunedCharManager(t *testing.T, modules collectorapi.Registry, tune func(mgr *Manager)) *charHarness {
	t.Helper()

	var out simOutput
	mgr := New(Config{PluginName: testPluginName})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = modules
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

const testEffectDeadline = 200 * time.Millisecond

func TestEffect_DeadlineAbandon(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"abandoned detection fails, wedges the key, and warm-starts on late success": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var initCalls atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								initCalls.Add(1)
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("wedge"), "add", "w"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", "w")), "enable"}, nil)

				// The deadline outcome commits while the module call still runs.
				require.Eventually(t, h.outputContains("timed out after"), charWait, charTick,
					"the enable must fail with the abandonment message at the deadline")
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status failed"), charWait, charTick)

				// The key is wedged: its own commands park...
				h.dyncfg("3-get", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", "w")), "get"}, nil)
				require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-get"), charNeverWait, charTick,
					"a wedged key must hold its parked commands until the late return")
				// ...while unrelated keys proceed.
				h.dyncfg("4-add-b", []string{h.mgr.dyncfgModID("success"), "add", "b"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 4-add-b"), charWait, charTick,
					"unrelated keys must not wait on a wedged key")

				// Late success: the warm job starts once, with no re-detection.
				close(release)
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status running"), charWait, charTick,
					"the late detection success must start the warm job")
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-get"), charWait, charTick,
					"the parked command must run after the key unwedges")
				assert.Equal(t, int32(1), initCalls.Load(), "the warm start must not re-run detection")
			},
		},
		"late retryable failure schedules a retry under the normal rules": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var initCalls atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("flaky", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							// A plain Init failure disables auto-detection; Check
							// failures are the retryable class.
							CheckFunc: func(context.Context) error {
								if initCalls.Add(1) == 1 {
									<-release
									return errors.New("late first failure")
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareUserCfg("flaky", "r").Set("autodetection_retry", 1)
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:flaky:r create accepted"), charWait, charTick)
				h.dyncfg("1-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)

				require.Eventually(t, h.outputContains("timed out after"), charWait, charTick)

				// No retry is scheduled at the abandon moment; the late failure
				// evaluates retry eligibility, the retry re-adds the config, and
				// the daemon (played here) echoes the enable for its CREATE.
				close(release)
				require.Eventually(t, func() bool { return initCalls.Load() >= 1 && h.outputContains("timed out after")() }, charWait, charTick)
				require.Eventually(t, func() bool {
					return strings.Count(h.out.String(), "CONFIG test:collector:flaky:r create accepted") >= 2
				}, 10*time.Second, charTick, "the retry must re-add the config")
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:flaky:r status running"), charWait, charTick,
					"the retried detection must succeed")
				assert.Equal(t, int32(2), initCalls.Load())
			},
		},
		"late non-retryable failure schedules no retry": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var initCalls atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("dead", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								initCalls.Add(1)
								<-release
								return errors.New("late failure")
							},
						}
					},
				})

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				// No autodetection_retry: today's rules schedule nothing.
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("dead"), "add", "d"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("dead", "d")), "enable"}, nil)
				require.Eventually(t, h.outputContains("timed out after"), charWait, charTick)

				close(release)
				require.Never(t, func() bool { return initCalls.Load() > 1 }, 1500*time.Millisecond, 50*time.Millisecond,
					"a late non-retryable failure must not schedule a retry")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestEffect_StopDeadline(t *testing.T) {
	newBlockingCollectJob := func(entered chan<- struct{}, release <-chan struct{}) collectorapi.Creator {
		return collectorapi.Creator{
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
		}
	}

	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"disable of a wedged stop commits at the deadline and fences late output": {
			run: func(t *testing.T) {
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				defer close(release)

				reg := collectorapi.Registry{}
				reg.Register("stuck", newBlockingCollectJob(entered, release))

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareDyncfgCfg("stuck", "s")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("stuck"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)

				// Wait for a collection to be in flight so the stop wedges.
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}
				gate, ok := h.mgr.emissionGates.lookup(cfg.FullName())
				require.True(t, ok)

				h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)

				// Disable still "always succeeds": terminal 200 and the CONFIG
				// status publish at the deadline, while the stop stays wedged.
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable 200"), charWait, charTick,
					"disable must commit successfully at the deadline")
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status disabled"), charWait, charTick)

				// The gate is closed: once the blocked collection unblocks, its
				// flush after the publish is suppressed, never written.
				release <- struct{}{}
				require.Eventually(t, func() bool { return gate.SuppressedWrites() >= 1 }, charWait, charTick,
					"the late flush must be swallowed by the closed gate")
			},
		},
		"restart of a wedged stop fails 422 and never starts a second instance": {
			run: func(t *testing.T) {
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				defer close(release)

				reg := collectorapi.Registry{}
				reg.Register("stuck", newBlockingCollectJob(entered, release))

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareDyncfgCfg("stuck", "s")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("stuck"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				h.dyncfg("3-restart", []string{h.mgr.dyncfgJobID(cfg), "restart"}, nil)

				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-restart 422"), charWait, charTick,
					"a restart whose stop wedges must fail 422")
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status failed"), charWait, charTick)

				// The start phase never began: only one instance was ever
				// created (one running job existed; none is running now and no
				// new detection started while the key is wedged).
				require.Never(t, func() bool {
					h.mgr.runningJobs.lock()
					_, running := h.mgr.runningJobs.lookup(cfg.FullName())
					h.mgr.runningJobs.unlock()
					return running
				}, charNeverWait, charTick, "no second instance may start after a stop-deadline failure")
			},
		},
		"update of a wedged stop fails at the deadline and never creates the replacement": {
			run: func(t *testing.T) {
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				var detections atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("stuck", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								detections.Add(1)
								return nil
							},
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareDyncfgCfg("stuck", "s")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("stuck"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				h.dyncfg("3-update", []string{h.mgr.dyncfgJobID(cfg), "update"},
					mustJSON(t, map[string]any{"option_str": "changed"}))

				// The update commits its failure at the deadline via the
				// existing update-failure mapping (plain error -> 200 with the
				// message + status failed), while the old stop stays wedged.
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-update 200"), charWait, charTick,
					"an update whose stop wedges must commit failed at the deadline")
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status failed"), charWait, charTick)
				assert.True(t, h.outputContains("timed out after")(),
					"the terminal must carry the abandonment message")

				// Unwedge: the late stop return must release the key only -
				// the replacement is never created, never detected, never
				// started (no warm continuation exists for an abandoned stop).
				release <- struct{}{}
				close(release)
				require.Never(t, func() bool {
					h.mgr.runningJobs.lock()
					_, running := h.mgr.runningJobs.lookup(cfg.FullName())
					h.mgr.runningJobs.unlock()
					return running
				}, charNeverWait, charTick, "the replacement must never start after an abandoned stop")
				assert.Equal(t, int32(1), detections.Load(),
					"only the original instance may ever run detection")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// Bridge pin: secretstore dependent restarts run loop-synchronously and
// SKIP-AND-REPORT dependents whose collector key has work in flight; idle
// dependents restart inline exactly as before.
func TestSecretstoreBridge_SkipsBusyDependents(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"busy-key dependent is skipped and reported in the terminal message": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var inits atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("gated", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								if inits.Add(1) > 1 {
									<-release // the update's detection blocks: key busy
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

				// Store, then a running collector job that references it.
				h.dyncfg("ss-add", []string{mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
					mustJSON(t, map[string]any{"value": "good"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)

				h.dyncfg("1-add", []string{mgr.dyncfgModID("gated"), "add", "mysql"},
					mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
				cfg := prepareDyncfgCfg("gated", "mysql")
				h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)
				require.Eventually(t, func() bool {
					return len(mgr.restartableAffectedJobs("vault:vault_prod")) == 1
				}, charWait, charTick, "the job must be registered as a restartable store dependent")

				// Make the dependent's key busy without leaving Running state
				// (which would drop it from the restartable set): a restart
				// whose detection blocks.
				h.dyncfg("3-restart", []string{mgr.dyncfgJobID(cfg), "restart"}, nil)
				require.Eventually(t, func() bool { return inits.Load() >= 2 }, charWait, charTick,
					"the restart's detection did not start")

				// The store update must NOT wait on the busy dependent.
				h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
					mustJSON(t, map[string]any{"value": "rotated"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick,
					"the store update must complete while its dependent is busy")

				var resp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
				assert.Contains(t, resp["message"], "dependent collector restarts failed")
				assert.Contains(t, resp["message"], "skipped: operation in progress")

				close(release)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-restart 200"), charWait, charTick,
					"the blocked restart must complete after release")
			},
		},
		"a dependent with an enable in flight is reported, never silently missed": {
			run: func(t *testing.T) {
				gate := make(chan struct{})
				var inits atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("gated", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								inits.Add(1)
								<-gate // the enable's detection holds the key busy
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

				h.dyncfg("ss-add", []string{mgr.dyncfgSecretStorePrefixValue() + "vault", "add", "vault_prod"},
					mustJSON(t, map[string]any{"value": "good"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-add 200"), charWait, charTick)
				h.dyncfg("1-add", []string{mgr.dyncfgModID("gated"), "add", "mysql"},
					mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-add"), charWait, charTick)
				require.Eventually(t, func() bool {
					return len(mgr.affectedJobs("vault:vault_prod")) == 1
				}, charWait, charTick, "the job must be registered as a store dependent")

				// The enable's effect starts (it resolved the OLD secret) but
				// its commit is pending: the exposed status still reads
				// accepted while the key is busy.
				cfg := prepareDyncfgCfg("gated", "mysql")
				h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, func() bool { return inits.Load() >= 1 }, charWait, charTick,
					"the enable's detection did not start")

				// The store update must not silently miss the in-flight
				// dependent: it is selected and reported as skipped.
				h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
					mustJSON(t, map[string]any{"value": "rotated"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)

				var resp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
				assert.Contains(t, resp["message"], "skipped: operation in progress",
					"an in-flight dependent must be reported, not silently missed")

				close(gate)
				require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick,
					"the enable must complete after release")
			},
		},
		"a wait-parked accepted dependent stays ignored": {
			run: func(t *testing.T) {
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
				mgr := New(Config{PluginName: testPluginName, SecretStoreService: newTestSecretStoreService()})
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

				// A discovery config referencing the store, wait-parked for a
				// user decision: no job, no resolved secret - a store change
				// has nothing to restart and must not report it.
				cfg := prepareUserCfg("gated", "waiting").Set("option_str", "${store:vault:vault_prod:value}")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:gated:waiting create accepted"), charWait, charTick)
				require.Eventually(t, func() bool {
					return len(mgr.affectedJobs("vault:vault_prod")) == 1
				}, charWait, charTick, "the config must be registered as a store dependent")

				h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
					mustJSON(t, map[string]any{"value": "rotated"}))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update 200"), charWait, charTick)

				var resp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "ss-update", &resp)
				msg, _ := resp["message"].(string)
				assert.NotContains(t, msg, "restarts failed",
					"a wait-parked accepted dependent must not surface as a restart failure")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// Pool and FIFO invariants under wedged keys.
func TestEffect_WedgeInvariants(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"abandoned effects free their pool slots: work proceeds with every slot leaked": {
			run: func(t *testing.T) {
				release := make(chan struct{})

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								<-release
								return nil
							},
						}
					},
				})
				reg.Register("success", charSuccessCreator())

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
				defer close(release)

				// Wedge as many keys as the pool has workers.
				for i := range effectPoolSize {
					name := string(rune('a' + i))
					h.dyncfg("add-"+name, []string{h.mgr.dyncfgModID("wedge"), "add", name}, []byte("{}"))
					h.dyncfg("enable-"+name, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", name)), "enable"}, nil)
				}
				for i := range effectPoolSize {
					name := string(rune('a' + i))
					require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:"+name+" status failed"), charWait, charTick,
						"every wedged key must commit its deadline failure")
				}

				// All four module calls are leaked; the pool slots are free.
				h.dyncfg("5-add", []string{h.mgr.dyncfgModID("success"), "add", "fresh"}, []byte("{}"))
				h.dyncfg("6-enable", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("success", "fresh")), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:fresh status running"), charWait, charTick,
					"a fresh key must get a pool slot while all abandoned calls are still leaked")
			},
		},
		"a disable parked behind the wedge prevents the warm start": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var initCalls atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								initCalls.Add(1)
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareDyncfgCfg("wedge", "w")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("wedge"), "add", "w"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status failed"), charWait, charTick)

				// Parked behind the wedge.
				h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)
				require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable"), charNeverWait, charTick)

				close(release)

				// The user's queued stop intent wins over the continuation: the
				// warm job is dropped (never started) and the parked disable
				// executes against the failed entry.
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status disabled"), charWait, charTick)
				wiretest.RequireSubsequence(t, h.out.String(), []wiretest.RecordWant{
					{Name: "parked disable terminal", Contains: []string{"FUNCTION_RESULT_BEGIN 3-disable 200"}},
					{Name: "disabled", Contains: []string{"CONFIG test:collector:wedge:w status disabled"}},
				})
				assert.False(t, h.outputContains("CONFIG test:collector:wedge:w status running")(),
					"a queued disable must prevent the warm start entirely")
				assert.Equal(t, int32(1), initCalls.Load(), "no re-detection may run")
				_, hasGate := h.mgr.emissionGates.lookup(cfg.FullName())
				assert.False(t, hasGate, "the dropped warm job's gate must be deregistered")
			},
		},
		"function routing to a wedged-stop job ends at the stop's start, not its return": {
			run: func(t *testing.T) {
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				defer close(release)

				reg := collectorapi.Registry{}
				reg.Register("stuck", collectorapi.Creator{
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
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "probe", Name: "Probe"}}
					},
				})

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })

				cfg := prepareDyncfgCfg("stuck", "s")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("stuck"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				require.Eventually(t, func() bool {
					return len(h.mgr.GetJobNames("stuck")) == 1
				}, charWait, charTick, "the running job must be routable")
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status disabled"), charWait, charTick)

				// The stop is still wedged (collect blocked), yet function
				// routing already ended: the registry removal ran at the
				// stop's start, before the blocking wait.
				assert.Empty(t, h.mgr.GetJobNames("stuck"),
					"function routing must end when the stop begins, not when it returns")
			},
		},
		"auto-enable replacement over a wedged stop starts after the wedge clears": {
			run: func(t *testing.T) {
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				var inits atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("stuck", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								inits.Add(1)
								return nil
							},
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) {
					mgr.effectDeadline = testEffectDeadline
					mgr.runModePolicy.AutoEnableDiscovered = true
				})
				defer close(release)

				// The stock config auto-enables and its collection wedges.
				stockCfg := prepareStockCfg("stuck", "s")
				h.in <- prepareCfgGroups(stockCfg.Source(), stockCfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				// A higher-priority config replaces it: the old stop wedges at
				// the deadline (abandoned, fenced); the auto-enable must park
				// behind the wedge instead of failing against it.
				userCfg := prepareUserCfg("stuck", "s")
				h.in <- prepareCfgGroups(userCfg.Source(), userCfg)
				require.Eventually(t, func() bool {
					return strings.Count(h.out.String(), "CONFIG test:collector:stuck:s create accepted") >= 2
				}, charWait, charTick, "the replacement must commit after the abandoned stop")

				// The leaked stop returns; the parked enable replays and the
				// replacement starts - it must never have been failed.
				release <- struct{}{}
				require.Eventually(t, func() bool {
					return inits.Load() == 2 &&
						strings.Count(h.out.String(), "CONFIG test:collector:stuck:s status running") >= 2
				}, charWait, charTick, "the replacement must start after the wedge clears")
				assert.False(t, h.outputContains("CONFIG test:collector:stuck:s status failed")(),
					"a replacement must not fail solely because the old stop wedged")
			},
		},
		"a test command never parks behind a wedged key": {
			run: func(t *testing.T) {
				release := make(chan struct{})
				var initCalls atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								if initCalls.Add(1) == 1 {
									<-release // only the enable's detection wedges
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

				h := startTunedCharManager(t, reg, func(mgr *Manager) { mgr.effectDeadline = testEffectDeadline })
				defer close(release)

				cfg := prepareDyncfgCfg("wedge", "w")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("wedge"), "add", "w"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status failed"), charWait, charTick,
					"the detection must wedge the key")

				// Test is KEYLESS by contract: it answers while the key is
				// wedged instead of parking behind the abandoned detection.
				h.dyncfg("3-test", []string{h.mgr.dyncfgJobID(cfg), "test"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-test"), charWait, charTick,
					"a test command must never park behind a busy key")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// Shutdown battery: every admitted command reaches a terminal outcome, no
// goroutine blocks forever, and late children after shutdown are dropped.
func TestEffect_Shutdown(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"saturated shutdown answers undispatched commands and returns promptly": {
			run: func(t *testing.T) {
				// No goroutine created by this test may survive it: workers
				// exit on shutdown, and leaked children drain via lateDrop
				// once released. (IgnoreCurrent is evaluated here, at defer
				// time, snapshotting pre-test goroutines.)
				defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

				release := make(chan struct{})
				var detStarted, cleanups atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								detStarted.Add(1)
								<-release
								return nil
							},
							CleanupFunc: func(context.Context) {
								cleanups.Add(1)
							},
						}
					},
				})

				var out simOutput
				mgr := New(Config{PluginName: testPluginName})
				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
				mgr.modules = reg
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

				// Saturate the pool and its queue with blocking detections on
				// distinct keys, plus extras that stay undispatched. Whether a
				// given validate wins a pool slot before the wedged detections
				// occupy them all is scheduling-dependent - the invariant under
				// test does not depend on it: every consumed command must reach
				// a terminal across shutdown, ran or not.
				total := effectPoolSize*2 + 2
				for i := range total {
					name := string(rune('a' + i))
					h.dyncfg("add-"+name, []string{h.mgr.dyncfgModID("wedge"), "add", name}, []byte("{}"))
					h.dyncfg("en-"+name, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", name)), "enable"}, nil)
				}
				require.Eventually(t, func() bool { return len(h.mgr.dyncfgCh) == 0 }, charWait, charTick,
					"every command must be consumed by the loop (staged, queued, or parked)")
				// All pool workers end up inside blocked detections (while any
				// worker is free, validates keep completing and detections
				// keep starting), making the leaked accounting deterministic.
				require.Eventually(t, func() bool { return detStarted.Load() == int32(effectPoolSize) }, charWait, charTick,
					"every pool worker must be inside a blocked detection")

				cancel()
				select {
				case <-done:
				case <-time.After(charWait):
					t.Fatal("shutdown did not complete with a saturated executor")
				}

				// The in-flight detections were abandoned at shutdown and are
				// accounted as leaked until their module calls return.
				assert.Equal(t, int64(effectPoolSize), h.mgr.leakedNow.Load(),
					"every abandoned in-flight call must be accounted as leaked")

				// Blocked module calls (ignoring cancellation) are leaked;
				// their children must not block anything after shutdown.
				close(release)
				require.Eventually(t, func() bool { return h.mgr.leakedNow.Load() == 0 }, charWait, charTick,
					"returned module calls must leave the leaked accounting")

				// The late SUCCESSFUL detections arrive after lateDrop closed:
				// their warm jobs are disposed in place - modules cleaned and
				// gates deregistered, nothing started.
				require.Eventually(t, func() bool { return cleanups.Load() == int32(effectPoolSize) }, charWait, charTick,
					"every dropped warm job must clean its module exactly once")
				for i := range total {
					name := string(rune('a' + i))
					_, hasGate := h.mgr.emissionGates.lookup("wedge_" + name)
					assert.False(t, hasGate, "no gate may survive for job %q", name)
				}

				// Terminals for all: in-flight phases fail via shutdown-abandon,
				// queued-but-unpicked and pending phases fail never-ran, parked
				// commands answer 503 - silence for any command is a bug.
				results := 0
				for i := range total {
					name := string(rune('a' + i))
					if h.outputContains("FUNCTION_RESULT_BEGIN add-" + name)() {
						results++
					}
					if h.outputContains("FUNCTION_RESULT_BEGIN en-" + name)() {
						results++
					}
				}
				assert.Equal(t, 2*total, results, "every admitted command must reach a terminal outcome across shutdown")
			},
		},
		"queued disable at shutdown answers 503 and publishes nothing": {
			run: func(t *testing.T) {
				defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

				entered := make(chan struct{}, 1)
				release := make(chan struct{})

				reg := collectorapi.Registry{}
				reg.Register("stuck", collectorapi.Creator{
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
				var detStarted atomic.Int32
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								detStarted.Add(1)
								<-release
								return nil
							},
						}
					},
				})

				var out simOutput
				mgr := New(Config{PluginName: testPluginName})
				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
				mgr.modules = reg
				mgr.drainWait = 300 * time.Millisecond
				mgr.effectDeadline = time.Hour // nothing may abandon before shutdown

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

				cfg := prepareDyncfgCfg("stuck", "s")
				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("stuck"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				// Block every pool worker with an unrelated detection. Exactly
				// pool-size pairs keep this deterministic: detection i can
				// only dispatch after validate i completed, so every validate
				// finds a free slot, and then the detections pin all workers.
				// Only AFTER that stage the disable: no slot can free before
				// cancel, and after cancel no worker may start new work - so
				// its stop phase deterministically never runs.
				saturate := effectPoolSize
				for i := range saturate {
					name := string(rune('a' + i))
					h.dyncfg("w-add-"+name, []string{h.mgr.dyncfgModID("wedge"), "add", name}, []byte("{}"))
					h.dyncfg("w-en-"+name, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", name)), "enable"}, nil)
				}
				for i := range saturate {
					name := string(rune('a' + i))
					require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN w-add-"+name), charWait, charTick)
				}
				// All validates answered and every pool worker sits in a
				// blocked detection: no slot can free from here on. (The
				// remaining detections wait in the pool queue or the
				// executor's pending list; executor key state is loop-owned
				// and must not be read from the test goroutine.)
				require.Eventually(t, func() bool { return detStarted.Load() == int32(effectPoolSize) }, charWait, charTick,
					"every pool worker must be inside a blocked detection")

				h.dyncfg("3-disable", []string{h.mgr.dyncfgJobID(cfg), "disable"}, nil)
				require.Eventually(t, func() bool { return len(h.mgr.dyncfgCh) == 0 }, charWait, charTick,
					"the disable must be consumed by the loop (staged as pending)")

				cancel()
				// Unblock the leaked module calls so cleanup can stop the
				// still-running job; the disable's 503 was already decided by
				// the drain, before anything unblocked.
				close(release)
				select {
				case <-done:
				case <-time.After(charWait):
					t.Fatal("shutdown did not complete")
				}

				// The stop NEVER RAN: the disable must answer retryably and
				// publish nothing - a disabled status would be a lie.
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-disable 503"), charWait, charTick,
					"a never-ran disable must answer 503")
				assert.False(t, h.outputContains("CONFIG test:collector:stuck:s status disabled")(),
					"no disabled status may be published for a stop that never ran")
			},
		},
		"late detection success during the drain is dropped, never started": {
			run: func(t *testing.T) {
				defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

				release := make(chan struct{})

				reg := collectorapi.Registry{}
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								<-release
								return nil // a SUCCESSFUL late detection
							},
							ChartsFunc: func() *collectorapi.Charts {
								return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
							},
							CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
						}
					},
				})

				var out simOutput
				mgr := New(Config{PluginName: testPluginName})
				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
				mgr.modules = reg
				mgr.effectDeadline = testEffectDeadline
				mgr.drainWait = 2 * time.Second // wide window: the late success arrives DURING the drain

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

				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("wedge"), "add", "w"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", "w")), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:wedge:w status failed"), charWait, charTick,
					"the detection must wedge and commit its deadline failure")

				cancel()
				// The leaked detection succeeds while shutdownDrain is inside
				// its window: the warm job must be dropped, not started.
				release <- struct{}{}
				close(release)
				select {
				case <-done:
				case <-time.After(charWait):
					t.Fatal("shutdown did not complete")
				}

				assert.False(t, h.outputContains("CONFIG test:collector:wedge:w status running")(),
					"shutdown must never start fresh collector work from a late detection success")
			},
		},
		"discovery removal whose stop never ran publishes no delete": {
			run: func(t *testing.T) {
				defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				var detStarted atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("stuck", collectorapi.Creator{
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
				reg.Register("wedge", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								detStarted.Add(1)
								<-release
								return nil
							},
						}
					},
				})

				var out simOutput
				mgr := New(Config{PluginName: testPluginName})
				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
				mgr.modules = reg
				mgr.drainWait = 300 * time.Millisecond
				mgr.effectDeadline = time.Hour // nothing may abandon before shutdown

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

				// A discovery-origin config, enabled by the played daemon,
				// with a collection in flight (so its stop would block).
				cfg := prepareUserCfg("stuck", "s")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s create accepted"), charWait, charTick)
				h.dyncfg("1-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:stuck:s status running"), charWait, charTick)
				select {
				case <-entered:
				case <-time.After(charWait):
					t.Fatal("no collection started")
				}

				// Pin every pool worker (pool-size pairs: deterministic, see
				// the queued-disable case).
				for i := range effectPoolSize {
					name := string(rune('a' + i))
					h.dyncfg("w-add-"+name, []string{h.mgr.dyncfgModID("wedge"), "add", name}, []byte("{}"))
					h.dyncfg("w-en-"+name, []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("wedge", name)), "enable"}, nil)
				}
				for i := range effectPoolSize {
					name := string(rune('a' + i))
					require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN w-add-"+name), charWait, charTick)
				}
				require.Eventually(t, func() bool { return detStarted.Load() == int32(effectPoolSize) }, charWait, charTick,
					"every pool worker must be inside a blocked detection")

				// Remove the config via discovery: its stop phase cannot get
				// a slot before cancel and may not start after it.
				h.in <- prepareCfgGroups(cfg.Source())
				require.Eventually(t, func() bool { return len(h.mgr.rmCh) == 0 }, charWait, charTick,
					"the removal must be consumed by the loop")

				cancel()
				close(release)
				select {
				case <-done:
				case <-time.After(charWait):
					t.Fatal("shutdown did not complete")
				}

				// The stop never ran: publishing the removal would break
				// no-output-after-publish for a job that was never stopped.
				assert.False(t, h.outputContains("CONFIG test:collector:stuck:s delete")(),
					"no delete may be published for a discovery removal whose stop never ran")
			},
		},
		"commands accepted before shutdown always reach a terminal": {
			run: func(t *testing.T) {
				defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

				release := make(chan struct{})
				var inits atomic.Int32

				reg := collectorapi.Registry{}
				reg.Register("gated", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(context.Context) error {
								if inits.Add(1) > 1 {
									<-release // the bridge restart's detection holds the LOOP
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
				h.dyncfg("1-add", []string{mgr.dyncfgModID("gated"), "add", "mysql"},
					mustJSON(t, map[string]any{"option_str": "${store:vault:vault_prod:value}"}))
				cfg := prepareDyncfgCfg("gated", "mysql")
				h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:gated:mysql status running"), charWait, charTick)
				require.Eventually(t, func() bool {
					return len(mgr.restartableAffectedJobs("vault:vault_prod")) == 1
				}, charWait, charTick)

				// The store update's inline dependent restart blocks the RUN
				// LOOP mid-command; commands buffered behind it sit in
				// dyncfgCh when shutdown starts.
				h.dyncfg("ss-update", []string{mgr.dyncfgSecretStorePrefixValue() + "vault:vault_prod", "update"},
					mustJSON(t, map[string]any{"value": "rotated"}))
				require.Eventually(t, func() bool { return inits.Load() >= 2 }, charWait, charTick,
					"the inline restart's detection did not start")

				buffered := 5
				for i := range buffered {
					h.dyncfg(fmt.Sprintf("g-%d", i), []string{mgr.dyncfgJobID(cfg), "get"}, nil)
				}

				cancel()
				close(release)
				select {
				case <-done:
				case <-time.After(charWait):
					t.Fatal("shutdown did not complete")
				}

				// Silence is a bug: the loop-held command finishes, and every
				// buffered command is either executed or drained with 503.
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN ss-update"), charWait, charTick,
					"the loop-held store update must reach a terminal")
				for i := range buffered {
					assert.True(t, h.outputContains(fmt.Sprintf("FUNCTION_RESULT_BEGIN g-%d", i))(),
						"a command accepted into dyncfgCh must reach a terminal across shutdown")
				}

				// Post-shutdown commands are rejected up front.
				h.dyncfg("late", []string{mgr.dyncfgJobID(cfg), "get"}, nil)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN late 503"), charWait, charTick,
					"a post-shutdown command must answer 503 immediately")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestCallbacks_NoFreshWorkAfterShutdown pins the success-tail shutdown
// guard: a detection that finishes normally against a shutdown-cancelled
// context must not start the job - the enable answers retryably (never-ran)
// and an update fails disruptively, and neither leaves a gate registered.
func TestCallbacks_NoFreshWorkAfterShutdown(t *testing.T) {
	newMgr := func(cleanups *atomic.Int32, initFn func(context.Context) error) *Manager {
		reg := collectorapi.Registry{}
		reg.Register("quick", collectorapi.Creator{
			JobConfigSchema: collectorapi.MockConfigSchema,
			Create: func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					InitFunc: initFn,
					CleanupFunc: func(context.Context) {
						if cleanups != nil {
							cleanups.Add(1)
						}
					},
					ChartsFunc: func() *collectorapi.Charts {
						return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
					},
					CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
				}
			},
		})
		var out simOutput
		mgr := New(Config{PluginName: testPluginName})
		mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
		mgr.modules = reg
		mgr.fileStatus = newFileStatus() // Run initializes this; the pin drives callbacks directly
		return mgr
	}

	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	t.Run("enable answers never-ran and starts nothing", func(t *testing.T) {
		var cleanups atomic.Int32
		mgr := newMgr(&cleanups, nil)
		cfg := prepareDyncfgCfg("quick", "q")

		err := mgr.collectorCallbacks.Start(cancelled(), cfg)

		require.Error(t, err)
		assert.True(t, errors.Is(err, dyncfg.ErrPhaseNeverRan))
		mgr.runningJobs.lock()
		_, running := mgr.runningJobs.lookup(cfg.FullName())
		mgr.runningJobs.unlock()
		assert.False(t, running, "shutdown must never start fresh collector work")
		_, hasGate := mgr.emissionGates.lookup(cfg.FullName())
		assert.False(t, hasGate, "the unstarted job's gate must be deregistered")
		assert.Equal(t, int32(1), cleanups.Load(),
			"a detected-but-never-started module must be cleaned up exactly once")
	})

	t.Run("interrupted detection answers never-ran, not a failure", func(t *testing.T) {
		var cleanups atomic.Int32
		// A ctx-honoring module returns the cancellation as its error.
		mgr := newMgr(&cleanups, func(ctx context.Context) error { return ctx.Err() })
		cfg := prepareDyncfgCfg("quick", "q")

		err := mgr.collectorCallbacks.Start(cancelled(), cfg)

		require.Error(t, err)
		assert.True(t, errors.Is(err, dyncfg.ErrPhaseNeverRan),
			"a shutdown-interrupted detection is not a detection failure")
		assert.Equal(t, int32(1), cleanups.Load(),
			"the module must be cleaned up exactly once (defer + drop paths share the guard)")
	})

	t.Run("update of a non-running config answers never-ran", func(t *testing.T) {
		// The old config was never started: the stop is a no-op, nothing is
		// torn down, so a shutdown interruption must roll back retryably
		// instead of committing a disruptive failure.
		mgr := newMgr(nil, nil)
		oldCfg := prepareDyncfgCfg("quick", "q")
		newCfg := prepareDyncfgCfg("quick", "q").Set("option_str", "changed")

		err := mgr.collectorCallbacks.Update(cancelled(), oldCfg, newCfg)

		require.Error(t, err)
		assert.True(t, errors.Is(err, dyncfg.ErrPhaseNeverRan),
			"a no-op stop tears nothing down - never-ran is the truthful outcome")
		mgr.runningJobs.lock()
		_, running := mgr.runningJobs.lookup(newCfg.FullName())
		mgr.runningJobs.unlock()
		assert.False(t, running)
	})

	t.Run("update fails disruptively and starts nothing", func(t *testing.T) {
		mgr := newMgr(nil, nil)
		oldCfg := prepareDyncfgCfg("quick", "q")
		require.NoError(t, mgr.collectorCallbacks.Start(context.Background(), oldCfg))

		newCfg := prepareDyncfgCfg("quick", "q").Set("option_str", "changed")
		err := mgr.collectorCallbacks.Update(cancelled(), oldCfg, newCfg)

		require.Error(t, err)
		assert.False(t, errors.Is(err, dyncfg.ErrPhaseNeverRan),
			"half the update happened (the old instance stopped) - it must fail, not roll back")
		assert.Contains(t, err.Error(), "shutting down")
		mgr.runningJobs.lock()
		_, running := mgr.runningJobs.lookup(newCfg.FullName())
		mgr.runningJobs.unlock()
		assert.False(t, running, "shutdown must never start the replacement")
		_, hasGate := mgr.emissionGates.lookup(newCfg.FullName())
		assert.False(t, hasGate, "the unstarted replacement's gate must be deregistered")
	})
}

// TestWaitUnparkReplayOrder pins same-kind FIFO order across the wait-unpark
// replay: discovery events must never reorder relative to each other, even
// when a wait-parked key went busy (an update's validation), collected
// discovery and decision events in its fifo, and the decision's replay then
// re-queues wait-parked events while the key is busy again.
func TestWaitUnparkReplayOrder(t *testing.T) {
	gate := make(chan struct{})
	var creates atomic.Int32

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			if creates.Add(1) == 1 {
				<-gate // the update's validation holds the key busy
			}
			return &collectorapi.MockCollectorV1{
				ChartsFunc: func() *collectorapi.Charts {
					return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
				},
				CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
			}
		},
	})

	var out simOutput
	mgr := New(Config{PluginName: testPluginName})
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

	// Discovery config, wait-parked for a decision.
	cfg := prepareUserCfg("gated", "g")
	h.in <- prepareCfgGroups(cfg.Source(), cfg)
	require.Eventually(t, h.outputContains("CONFIG test:collector:gated:g create accepted"), charWait, charTick)

	// A non-decision update executes during the wait; its validation blocks,
	// so the key is busy AND waiting.
	h.dyncfg("1-update", []string{mgr.dyncfgJobID(cfg), "update"}, mustJSON(t, map[string]any{"option_str": "x"}))
	require.Eventually(t, func() bool { return creates.Load() >= 1 }, charWait, charTick,
		"the update's validation did not start")

	// Arrival order into the busy fifo: discovery REMOVE, then the enable
	// decision, then discovery re-ADD. Empty same-channel batches fence the
	// discovery sends (runProcessConfGroups is sequential), and a drained
	// dyncfgCh fences the enable.
	h.in <- prepareCfgGroups(cfg.Source()) // A: discovery remove
	h.in <- prepareCfgGroups("fence-1")
	h.dyncfg("2-enable", []string{mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, func() bool { return len(mgr.dyncfgCh) == 0 }, charWait, charTick)
	h.in <- prepareCfgGroups(cfg.Source(), cfg) // B: discovery re-add
	h.in <- prepareCfgGroups("fence-2")

	// Validation completes: the update answers 403 (Accepted), settle moves
	// the remove to the wait queue, the enable decision goes busy, and the
	// replay must put the remove BACK IN FRONT of the re-add.
	close(gate)

	// Order preserved means: remove runs first (delete), re-add runs last
	// (the config exists again, accepted). Reordered, the re-add is a no-op
	// against the running job and the remove then deletes the config for
	// good - the second create-accepted never appears.
	require.Eventually(t, func() bool {
		return strings.Count(h.out.String(), "CONFIG test:collector:gated:g create accepted") >= 2
	}, charWait, charTick, "discovery events must not reorder across the wait-unpark replay")
}
