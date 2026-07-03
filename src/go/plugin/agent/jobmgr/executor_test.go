// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/wiretest"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newExecutorTestManager() (*Manager, *bytes.Buffer) {
	mgr := newCollectorTestManagerWithService(newTestSecretStoreService())
	mgr.modules.Register("single", collectorapi.Creator{
		InstancePolicy: collectorapi.InstancePolicySingle,
		Create:         func() collectorapi.CollectorV1 { return &collectorapi.MockCollectorV1{} },
	})

	var buf bytes.Buffer
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))
	return mgr, &buf
}

func newExecutorTestFn(uid string, args []string, payload []byte) dyncfg.Function {
	fn := functions.Function{UID: uid, Args: args}
	if payload != nil {
		fn.Payload = payload
		fn.ContentType = "application/json"
	}
	return dyncfg.NewFunction(fn)
}

func TestExecutor_KeyDerivation(t *testing.T) {
	tests := map[string]struct {
		event      func(mgr *Manager) event
		wantDomain eventDomain
		wantKey    string
	}{
		// Collector discovery events key by the config's exposed key.
		"collector discovery add keys by exposed key": {
			event: func(mgr *Manager) event {
				return mgr.newDiscoveryAddEvent(prepareUserCfg("success", "a"))
			},
			wantDomain: domainCollector,
			wantKey:    "success_a",
		},
		"collector discovery remove keys by exposed key": {
			event: func(mgr *Manager) event {
				return mgr.newDiscoveryRemoveEvent(prepareUserCfg("success", "a"))
			},
			wantDomain: domainCollector,
			wantKey:    "success_a",
		},

		// Collector commands mirror the handler callbacks' ExtractKey.
		"collector add keys by module plus job name from args": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:success", "add", "a"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "success_a",
		},
		"collector add sanitizes the job name like JobName": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:success", "add", "a b"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "success_a_b",
		},
		"collector per-job command keys by module and job from ID": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:success:a", "enable"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "success_a",
		},
		"collector per-job command with job equal to module collapses the key": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:success:success", "get"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "success",
		},
		"collector unregistered module still derives a per-job key": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:ghost:j", "enable"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "ghost_j",
		},
		"single-instance collector keys by module": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:single", "enable"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "single",
		},
		"single-instance add is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:single", "add", "x"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "test:collector:",
		},
		"single-instance per-job ID is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:single:x", "disable"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "test:collector:",
		},
		"collector add without job name is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:success", "add"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "test:collector:",
		},
		"collector empty module is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:collector:", "enable"}, nil))
			},
			wantDomain: domainCollector,
			wantKey:    "test:collector:",
		},

		// Secretstore commands key by store key (kind:name).
		"secretstore command keys by store key": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:secretstore:vault:prod", "update"}, nil))
			},
			wantDomain: domainSecretStore,
			wantKey:    "vault:prod",
		},
		"secretstore template add keys by kind and name from args": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:secretstore:vault", "add", "prod"}, nil))
			},
			wantDomain: domainSecretStore,
			wantKey:    "vault:prod",
		},
		"secretstore invalid kind is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:secretstore:bogus:x", "update"}, nil))
			},
			wantDomain: domainSecretStore,
			wantKey:    "test:secretstore:",
		},
		"secretstore template add without name is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:secretstore:vault", "add"}, nil))
			},
			wantDomain: domainSecretStore,
			wantKey:    "test:secretstore:",
		},

		// Vnode commands key by vnode name.
		"vnode job command keys by name from ID": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:vnode:myhost", "update"}, nil))
			},
			wantDomain: domainVnode,
			wantKey:    "myhost",
		},
		"vnode add keys by name from args": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:vnode", "add", "myhost"}, nil))
			},
			wantDomain: domainVnode,
			wantKey:    "myhost",
		},
		"vnode template schema is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:vnode", "schema"}, nil))
			},
			wantDomain: domainVnode,
			wantKey:    "test:vnode",
		},
		"vnode empty name is underivable": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"test:vnode:", "get"}, nil))
			},
			wantDomain: domainVnode,
			wantKey:    "test:vnode",
		},

		// Unknown prefixes have no domain and no domain key.
		"unknown prefix has no domain key": {
			event: func(mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("uid", []string{"bogus:thing:x", "enable"}, nil))
			},
			wantDomain: domainUnknown,
			wantKey:    "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newExecutorTestManager()

			ev := tc.event(mgr)

			assert.Equal(t, tc.wantDomain, ev.domain)
			assert.Equal(t, tc.wantKey, ev.key)
		})
	}
}

// Per-key FIFO pin: events for ONE key execute and emit in arrival order
// through the running manager (cross-key order is unconstrained by design).
func TestExecutor_DispatchOrder(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, h *charHarness)
	}{
		"same-key dyncfg commands emit terminals in dispatch order": {
			run: func(t *testing.T, h *charHarness) {
				var wants []wiretest.RecordWant
				for i := 1; i <= 3; i++ {
					uid := fmt.Sprintf("order-%d", i)
					h.dyncfg(uid, []string{"test:collector:success:missing", "get"}, nil)
					wants = append(wants, wiretest.RecordWant{
						Name:     uid + " terminal",
						Contains: []string{"FUNCTION_RESULT_BEGIN " + uid},
					})
				}

				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN order-3"), charWait, charTick)
				wiretest.RequireSubsequence(t, h.out.String(), wants)
			},
		},
		"same-key discovery and dyncfg events keep arrival order": {
			run: func(t *testing.T, h *charHarness) {
				cfg := prepareUserCfg("success", "o1")

				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:o1 create"), charWait, charTick)
				h.dyncfg("mid-update", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN mid-update"), charWait, charTick)
				// The decision unparks the key; a wait-parked key's own discovery
				// events (including its removal) stay parked until it.
				h.dyncfg("then-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:o1 status running"), charWait, charTick)
				h.in <- prepareCfgGroups(cfg.Source())
				require.Eventually(t, func() bool {
					_, ok := h.mgr.collectorExposed.LookupByKey(cfg.ExposedKey())
					return !ok
				}, charWait, charTick)

				wiretest.RequireSubsequence(t, h.out.String(), []wiretest.RecordWant{
					{Name: "discovery add config create", Contains: []string{"CONFIG test:collector:success:o1 create"}},
					{Name: "premature update terminal", Contains: []string{"FUNCTION_RESULT_BEGIN mid-update"}},
					{Name: "enable terminal", Contains: []string{"FUNCTION_RESULT_BEGIN then-enable"}},
					{Name: "running status", Contains: []string{"CONFIG test:collector:success:o1 status running"}},
					{Name: "discovery remove config delete", Contains: []string{"CONFIG test:collector:success:o1 delete"}},
				})
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
func TestExecutor_UnderivableEventExecution(t *testing.T) {
	tests := map[string]struct {
		fn       dyncfg.Function
		wantCode int
		wantMsg  string
	}{
		"single-instance add answers 400": {
			fn:       newExecutorTestFn("si-add", []string{"test:collector:single", "add", "x"}, []byte("{}")),
			wantCode: 400,
			wantMsg:  "invalid config ID format",
		},
		"single-instance per-job enable answers 400": {
			fn:       newExecutorTestFn("si-enable", []string{"test:collector:single:x", "enable"}, nil),
			wantCode: 400,
			wantMsg:  "invalid config ID format",
		},
		"collector add without job name answers 400": {
			fn:       newExecutorTestFn("no-name-add", []string{"test:collector:success", "add"}, []byte("{}")),
			wantCode: 400,
		},
		"secretstore malformed store key answers 400": {
			fn:       newExecutorTestFn("ss-bad-update", []string{"test:secretstore:", "update"}, nil),
			wantCode: 400,
			wantMsg:  "invalid config ID format",
		},
		"vnode empty name answers 404": {
			fn:       newExecutorTestFn("vn-bad-get", []string{"test:vnode:", "get"}, nil),
			wantCode: 404,
			wantMsg:  "is not registered",
		},
		"vnode template schema still succeeds": {
			fn:       newExecutorTestFn("vn-schema", []string{"test:vnode", "schema"}, nil),
			wantCode: 200,
		},
		"unknown prefix answers 503": {
			fn:       newExecutorTestFn("unknown-fn", []string{"bogus:thing:x", "enable"}, nil),
			wantCode: 503,
			wantMsg:  "unknown function",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, buf := newExecutorTestManager()

			mgr.executor.dispatch(mgr.newDyncfgEvent(tc.fn))

			output := buf.String()
			require.Contains(t, output,
				fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d", tc.fn.UID(), tc.wantCode),
				"expected a terminal response with the pinned code")
			if tc.wantMsg != "" {
				assert.True(t, strings.Contains(output, tc.wantMsg),
					"expected output to contain %q, got:\n%s", tc.wantMsg, output)
			}
		})
	}
}

// captureDeriverRegistry records lane derivers so their key derivation can be
// pinned without a live functions manager.
type captureDeriverRegistry struct {
	noop
	derivers map[string]functions.LaneKeyDeriver
}

func (c *captureDeriverRegistry) RegisterPrefixLaneDeriver(_, prefix string, d functions.LaneKeyDeriver) {
	c.derivers[prefix] = d
}

// TestDyncfgLaneDerivers_TestIsKeyless pins the functions-lane half of the
// keyless-test contract: the collector deriver gives every test request its
// own lane, so a test never serializes behind the config's mutating lane
// (held until that command's terminal finalize) or behind other tests.
func TestDyncfgLaneDerivers_TestIsKeyless(t *testing.T) {
	reg := &captureDeriverRegistry{derivers: map[string]functions.LaneKeyDeriver{}}
	mgr := newCollectorTestManagerWithService(newTestSecretStoreService())
	mgr.fnReg = reg
	mgr.registerDyncfgLaneDerivers()

	derive := reg.derivers[mgr.dyncfgCollectorPrefixValue()]
	require.NotNil(t, derive, "the collector prefix must have a lane deriver")

	jobID := mgr.dyncfgJobID(prepareDyncfgCfg("success", "web"))
	enableKey, _ := derive(functions.Function{UID: "u1", Args: []string{jobID, "enable"}})
	testKey1, _ := derive(functions.Function{UID: "u2", Args: []string{jobID, "test"}})
	testKey2, _ := derive(functions.Function{UID: "u3", Args: []string{jobID, "test"}})
	templateTestKey, _ := derive(functions.Function{UID: "u4", Args: []string{mgr.dyncfgModID("success"), "test"}})

	require.NotEmpty(t, enableKey)
	require.NotEmpty(t, testKey1)
	assert.NotEqual(t, enableKey, testKey1,
		"a test must not share the lane of the config's mutating commands")
	assert.NotEqual(t, testKey1, testKey2,
		"each test request must get its own lane")
	require.NotEmpty(t, templateTestKey,
		"a template-ID test must not fall back to the registration-wide lane")
	assert.NotEqual(t, testKey1, templateTestKey,
		"template-ID tests get their own lanes too")
}

// TestSuperviseEffect_AbandonFenceClassification pins that only a FENCED
// abandon carries ErrPhaseAbandoned: stop-shaped commits publish success on
// that sentinel, so an abandon that wins before the stop registered its
// quarantine must classify as a plain failure instead.
func TestSuperviseEffect_AbandonFenceClassification(t *testing.T) {
	newMgr := func() *Manager {
		mgr := New(Config{PluginName: testPluginName})
		mgr.effectDeadline = 50 * time.Millisecond
		return mgr
	}

	t.Run("unfenced abandon is a plain failure", func(t *testing.T) {
		mgr := newMgr()
		block := make(chan struct{})
		defer close(block)

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(context.Context) error {
			<-block
			return nil
		}})

		require.True(t, res.abandoned)
		assert.False(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned),
			"an abandon with no fence installed must not claim the fenced outcome")
	})

	t.Run("fenced abandon carries ErrPhaseAbandoned", func(t *testing.T) {
		mgr := newMgr()
		block := make(chan struct{})
		defer close(block)
		var fenceRan atomic.Bool

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(ctx context.Context) error {
			effectControlFrom(ctx).setQuarantine(func() { fenceRan.Store(true) })
			<-block
			return nil
		}})

		require.True(t, res.abandoned)
		assert.True(t, fenceRan.Load(), "the worker must run the fence before reporting the abandon")
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned))
	})

	t.Run("unfenced shutdown abandon is never-ran", func(t *testing.T) {
		mgr := newMgr()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		mgr.ctx = ctx // shutdown already began
		block := make(chan struct{})
		defer close(block)

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(context.Context) error {
			<-block
			return nil
		}})

		require.True(t, res.abandoned)
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseNeverRan),
			"a shutdown interruption must answer retryably no matter which side wins the claim")
	})

	t.Run("disruptive shutdown abandon is a plain failure, never never-ran", func(t *testing.T) {
		mgr := newMgr()
		mgr.effectDeadline = time.Hour // only the cancellation may abandon
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mgr.ctx = ctx
		block := make(chan struct{})
		defer close(block)
		marked := make(chan struct{})
		go func() {
			// Shutdown lands strictly AFTER the point of no return.
			<-marked
			cancel()
		}()

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(ctx context.Context) error {
			effectControlFrom(ctx).markDisruptive()
			close(marked)
			<-block
			return nil
		}})

		require.True(t, res.abandoned)
		assert.False(t, errors.Is(res.err, dyncfg.ErrPhaseNeverRan),
			"a never-ran outcome would roll back caches for a state that no longer exists")
		assert.False(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned),
			"no fence ran - success must not be published")
	})

	t.Run("fenced shutdown abandon carries ErrPhaseAbandoned", func(t *testing.T) {
		mgr := newMgr()
		mgr.effectDeadline = time.Hour // only the cancellation may abandon
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mgr.ctx = ctx
		block := make(chan struct{})
		defer close(block)
		fenceSet := make(chan struct{})
		go func() {
			// Shutdown lands strictly AFTER the stop registered its fence.
			<-fenceSet
			cancel()
		}()

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(ctx context.Context) error {
			effectControlFrom(ctx).setQuarantine(func() {})
			close(fenceSet)
			<-block
			return nil
		}})

		require.True(t, res.abandoned)
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned),
			"a fenced stop may publish success even when shutdown is the cause")
	})
}

// TestResumeWarmJob_IneligibleDropDisposesGate pins that a warm job dropped
// on the ineligible path (its exposed entry is gone or replaced - reachable
// when a stock enable failure removed the entry) deregisters its own
// emission gate along with the module cleanup.
func TestResumeWarmJob_IneligibleDropDisposesGate(t *testing.T) {
	mgr, _ := newExecutorTestManager()
	cfg := prepareDyncfgCfg("success", "gone")

	job, err := mgr.createCollectorJob(context.Background(), cfg)
	require.NoError(t, err)
	gate, hasGate := mgr.emissionGates.lookup(cfg.FullName())
	require.True(t, hasGate, "creation must register the gate")

	// The config is NOT exposed: the continuation is ineligible.
	mgr.resumeWarmJob(&warmResume{cfg: cfg, job: job, gate: gate})

	_, hasGate = mgr.emissionGates.lookup(cfg.FullName())
	assert.False(t, hasGate, "the dropped warm job's gate must be deregistered")
	mgr.runningJobs.lock()
	_, running := mgr.runningJobs.lookup(cfg.FullName())
	mgr.runningJobs.unlock()
	assert.False(t, running, "an ineligible warm job must never start")
}
