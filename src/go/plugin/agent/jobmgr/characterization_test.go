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

type wireRecordWant struct {
	name     string
	contains []string
}

func atomicWireRecords(output string) []string {
	var records []string
	lines := strings.Split(output, "\n")
	for i := 0; i < len(lines); {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "FUNCTION_RESULT_BEGIN ") {
			records = append(records, line)
			i++
			continue
		}

		var b strings.Builder
		b.WriteString(line)
		i++
		for i < len(lines) {
			b.WriteByte('\n')
			b.WriteString(lines[i])
			if lines[i] == "FUNCTION_RESULT_END" {
				i++
				break
			}
			i++
		}
		records = append(records, b.String())
	}
	return records
}

func requireWireRecordSubsequence(t *testing.T, output string, wants []wireRecordWant) {
	t.Helper()

	records := atomicWireRecords(output)
	next := 0
	for _, want := range wants {
		found := -1
		for i := next; i < len(records); i++ {
			matched := true
			for _, substr := range want.contains {
				if !strings.Contains(records[i], substr) {
					matched = false
					break
				}
			}
			if matched {
				found = i
				break
			}
		}
		require.NotEqualf(t, -1, found, "wire record %q not found after index %d in records:\n%s",
			want.name, next, strings.Join(records, "\n---\n"))
		next = found + 1
	}
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

// FLIPS-BY-DESIGN: today one slow enable (synchronous AutoDetection on the
// single run loop) blocks every other dyncfg command, including commands for
// unrelated configs. The per-key concurrency redesign inverts this: an
// unrelated config's commands must complete while another config detects.
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

				// Queued behind the blocked enable: even the unrelated add's terminal
				// response cannot be emitted while the loop is inside AutoDetection.
				h.dyncfg("3-add-b", []string{h.mgr.dyncfgModID("success"), "add", "b"}, []byte("{}"))

				require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-add-b"), charNeverWait, charTick,
					"unrelated command completed while another config's enable was blocked in AutoDetection; "+
						"global serialization no longer holds - update this FLIPS-BY-DESIGN pin deliberately")

				release <- struct{}{}

				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-enable-a"), charWait, charTick)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-add-b"), charWait, charTick)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// Mixed classes, marked per case:
//   - FLIPS-BY-DESIGN: the gate is global today - while one discovered config
//     awaits its enable/disable decision, discovery processing (addCh AND
//     rmCh) freezes for ALL other configs; the redesign scopes waiting to the
//     affected key only, so unrelated adds/removals proceed.
//   - MUST-NOT-FLIP: dyncfg commands execute immediately while the gate is
//     armed, with state-machine outcomes - load-bearing in the redesign too
//     (a parked command would deadlock its functions lane against the
//     gate-clearing enable).
func TestCharacterization_WaitGateBehavior(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, h *charHarness)
	}{
		"global wait gate freezes unrelated discovery": {
			run: func(t *testing.T, h *charHarness) {
				cfgA := prepareUserCfg("success", "a")
				cfgB := prepareUserCfg("success", "b")

				h.in <- prepareCfgGroups(cfgA.Source(), cfgA)
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick,
					"wait gate did not arm for the first discovered config")

				go func() { h.in <- prepareCfgGroups(cfgB.Source(), cfgB) }()

				require.Never(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return ok
				}, charNeverWait, charTick,
					"second discovered config was exposed while the first awaited a decision; "+
						"the global wait gate no longer freezes discovery - update this FLIPS-BY-DESIGN pin deliberately")

				h.dyncfg("1-enable-a", []string{h.mgr.dyncfgJobID(cfgA), "enable"}, nil)

				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return ok
				}, charWait, charTick, "second config was not processed after the first received its decision")
			},
		},
		"global wait gate freezes unrelated removal": {
			run: func(t *testing.T, h *charHarness) {
				cfgA := prepareUserCfg("success", "a")
				cfgB := prepareUserCfg("success", "b")

				// Establish B as a running job first, then arm the gate with A.
				h.in <- prepareCfgGroups(cfgB.Source(), cfgB)
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick)
				h.dyncfg("1-enable-b", []string{h.mgr.dyncfgJobID(cfgB), "enable"}, nil)
				// Entry.Status is loop-owned (dyncfg cache contract) - observe the
				// transition through the wire output instead of reading the field.
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:b status running"), charWait, charTick)

				h.in <- prepareCfgGroups(cfgA.Source(), cfgA)
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick,
					"wait gate did not arm for the second discovered config")

				// B vanishes from discovery while A awaits its decision: the
				// removal must NOT be consumed (rmCh is frozen by the gate).
				h.in <- prepareCfgGroups(cfgB.Source())

				require.Never(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return !ok
				}, charNeverWait, charTick,
					"unrelated config's removal was processed while another config awaited a decision; "+
						"the global wait gate no longer freezes rmCh - update this FLIPS-BY-DESIGN pin deliberately")

				h.dyncfg("2-enable-a", []string{h.mgr.dyncfgJobID(cfgA), "enable"}, nil)

				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfgB.ExposedKey())
					return !ok
				}, charWait, charTick, "pending removal was not processed after the decision cleared the gate")
			},
		},
		"dyncfg commands execute while wait gate is armed": {
			run: func(t *testing.T, h *charHarness) {
				cfg := prepareUserCfg("success", "waity")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick)

				h.dyncfg("1-update", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 1-update"), charWait, charTick)
				var updateResp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "1-update", &updateResp)
				assert.Equal(t, float64(403), updateResp["status"], "premature update must be rejected with 403")
				assert.True(t, h.mgr.collectorHandler.WaitingForDecision(), "non-decision command must not clear the wait gate")

				h.dyncfg("2-restart", []string{h.mgr.dyncfgJobID(cfg), "restart"}, nil)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 2-restart"), charWait, charTick)
				var restartResp map[string]any
				mustDecodeFunctionPayload(t, h.out.String(), "2-restart", &restartResp)
				assert.Equal(t, float64(405), restartResp["status"], "premature restart must be rejected with 405")
				assert.True(t, h.mgr.collectorHandler.WaitingForDecision())

				h.dyncfg("3-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, func() bool { return !h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick,
					"matching enable did not clear the wait gate")
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN 3-enable"), charWait, charTick)
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

				requireWireRecordSubsequence(t, h.out.String(), []wireRecordWant{
					{
						name:     "add terminal",
						contains: []string{"FUNCTION_RESULT_BEGIN 1-add"},
					},
					{
						name:     "add config create",
						contains: []string{"CONFIG test:collector:success:ord create"},
					},
					{
						name:     "enable terminal",
						contains: []string{"FUNCTION_RESULT_BEGIN 2-enable"},
					},
					{
						name:     "enable config status",
						contains: []string{"CONFIG test:collector:success:ord status running"},
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
				require.NoError(t, mgr.collectorCallbacks.Start(cfg))

				badFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-bad",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "bad"}),
					Args:        []string{mgr.dyncfgSecretStoreID(key), string(dyncfg.CommandUpdate)},
				})
				mgr.dyncfgSecretStoreSeqExec(badFn)

				output := out.String()
				requireWireRecordSubsequence(t, output, []wireRecordWant{
					{
						name:     "dependent job failed status",
						contains: []string{"CONFIG test:collector:gated:mysql status failed"},
					},
					{
						name:     "store update terminal",
						contains: []string{"FUNCTION_RESULT_BEGIN ss-update-bad"},
					},
				})

				var resp map[string]any
				mustDecodeFunctionPayload(t, output, "ss-update-bad", &resp)
				assert.Contains(t, resp["message"], "dependent collector restarts failed",
					"restart failures must be embedded in the terminal message body")
				assert.Contains(t, resp["message"], "gated:mysql")
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
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick)

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

				require.NoError(t, cb.Start(cfg))
				t.Cleanup(func() { mgr.stopRunningJob(cfg.FullName()) })
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

				cb.Stop(cfg)

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

				require.NoError(t, cb.Update(oldCfg, newCfg))
				t.Cleanup(func() { mgr.stopRunningJob(newCfg.FullName()) })

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
