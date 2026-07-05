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
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
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

func TestExecutor_CommandPlanClaims(t *testing.T) {
	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	seedStore := func(t *testing.T, mgr *Manager) {
		t.Helper()
		mgr.dyncfgSecretStoreSeqExec(newExecutorTestFn("seed-store",
			[]string{"test:secretstore:vault", "add", "vault_prod"},
			mustJSON(t, map[string]any{"value": "secret"})))
	}
	claimSet := func(plan eventPlan) []claim {
		return normalizeClaims(plan.computeClaims())
	}

	tests := map[string]struct {
		setup func(t *testing.T, mgr *Manager)
		event func(t *testing.T, mgr *Manager) event
		check func(t *testing.T, plan eventPlan, claims []claim)
	}{
		"collector update claims old and new refs": {
			setup: func(t *testing.T, mgr *Manager) {
				oldCfg := prepareDyncfgCfg("success", "job").
					Set("password", "${store:vault:vault_old:secret/data/mysql#password}").
					Set("vnode", "oldvn")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{Cfg: oldCfg, Status: dyncfg.StatusRunning})
			},
			event: func(t *testing.T, mgr *Manager) event {
				newCfg := prepareDyncfgCfg("success", "job").
					Set("password", "${store:vault:vault_new:secret/data/mysql#password}").
					Set("vnode", "newvn")
				return mgr.newDyncfgEvent(newExecutorTestFn("collector-update",
					[]string{"test:collector:success:job", "update"},
					mustJSON(t, newCfg)))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.True(t, plan.needsClaims())
				assert.Equal(t, normalizeClaims([]claim{
					{key: collectorStateKey("success_job"), mode: claimWrite},
					{key: secretStoreStateKey("vault:vault_old"), mode: claimRead},
					{key: secretStoreStateKey("vault:vault_new"), mode: claimRead},
					{key: vnodeStateKey("oldvn"), mode: claimRead},
					{key: vnodeStateKey("newvn"), mode: claimRead},
				}), claims)
			},
		},
		"collector status no-op is hold-aware claimless": {
			setup: func(t *testing.T, mgr *Manager) {
				cfg := prepareDyncfgCfg("success", "job")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{Cfg: cfg, Status: dyncfg.StatusRunning})
			},
			event: func(t *testing.T, mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("collector-enable",
					[]string{"test:collector:success:job", "enable"}, nil))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.False(t, plan.needsClaims())
				assert.False(t, plan.bypassesForeignWriteHold())
				assert.Empty(t, claims)
			},
		},
		"secretstore test claims store read": {
			setup: seedStore,
			event: func(t *testing.T, mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("store-test",
					[]string{"test:secretstore:vault:vault_prod", "test"}, nil))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.True(t, plan.needsClaims())
				assert.Equal(t, []claim{{key: secretStoreStateKey(storeKey), mode: claimRead}}, claims)
			},
		},
		"secretstore update claims store write and dependent writes": {
			setup: func(t *testing.T, mgr *Manager) {
				seedStore(t, mgr)
				cfg := prepareDyncfgCfg("success", "dep").
					Set("password", "${store:vault:vault_prod:secret/data/mysql#password}")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{Cfg: cfg, Status: dyncfg.StatusRunning})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), cfg.ExposedKey(), []string{storeKey})
			},
			event: func(t *testing.T, mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("store-update",
					[]string{"test:secretstore:vault:vault_prod", "update"},
					mustJSON(t, map[string]any{"value": "rotated"})))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.True(t, plan.needsClaims())
				assert.Equal(t, normalizeClaims([]claim{
					{key: secretStoreStateKey(storeKey), mode: claimWrite},
					{key: collectorStateKey("success_dep"), mode: claimWrite},
				}), claims)
			},
		},
		"vnode mutation claims vnode write": {
			event: func(t *testing.T, mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("vnode-add",
					[]string{"test:vnode", "add", "db"},
					mustJSON(t, map[string]any{"guid": "b0b0b0b0-0000-4000-8000-0000000000aa", "hostname": "db"})))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.True(t, plan.needsClaims())
				assert.Equal(t, []claim{{key: vnodeStateKey("db"), mode: claimWrite}}, claims)
			},
		},
		"underivable command is claimless": {
			event: func(t *testing.T, mgr *Manager) event {
				return mgr.newDyncfgEvent(newExecutorTestFn("bad",
					[]string{"test:collector:success", "add"}, nil))
			},
			check: func(t *testing.T, plan eventPlan, claims []claim) {
				assert.False(t, plan.needsClaims())
				assert.True(t, plan.bypassesForeignWriteHold())
				assert.Empty(t, claims)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newExecutorTestManager()
			if tc.setup != nil {
				tc.setup(t, mgr)
			}

			ev := tc.event(t, mgr)
			plan := mgr.executor.planEvent(ev)
			tc.check(t, plan, claimSet(plan))
		})
	}
}

func TestExecutor_SnapshotWedgedDepsCapturesReadStoreClaims(t *testing.T) {
	mgr, _ := newExecutorTestManager()
	storeA := secretstore.StoreKey(secretstore.KindVault, "vault_a")
	storeB := secretstore.StoreKey(secretstore.KindVault, "vault_b")
	seedSecretStore(t, mgr, secretstore.KindVault, "vault_a", map[string]any{"value": "a"}, dyncfg.StatusAccepted)
	seedSecretStore(t, mgr, secretstore.KindVault, "vault_b", map[string]any{"value": "b"}, dyncfg.StatusAccepted)

	got := mgr.executor.snapshotWedgedDeps(&claimGrant{held: []claim{
		{key: secretStoreStateKey(storeA), mode: claimRead},
		{key: vnodeStateKey("vnode"), mode: claimRead},
		{key: secretStoreStateKey("vault:write"), mode: claimWrite},
		{key: collectorStateKey("success_job"), mode: claimRead},
		{key: secretStoreStateKey(storeB), mode: claimRead},
	}})

	assert.Equal(t, []wedgedDep{
		{stateKey: secretStoreStateKey(storeA), identity: mgr.secretStoreDepIdentity(storeA)},
		{stateKey: secretStoreStateKey(storeB), identity: mgr.secretStoreDepIdentity(storeB)},
	}, got)
}

func TestExecutor_StaleWarmDep(t *testing.T) {
	storeKey := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	stateKey := secretStoreStateKey(storeKey)
	referencingCfg := prepareDyncfgCfg("success", "job").
		Set("password", "${store:vault:vault_prod:secret/data/mysql#password}")
	unreferencedCfg := prepareDyncfgCfg("success", "job")

	tests := map[string]struct {
		deps      func(*Manager) []wedgedDep
		cfg       confgroup.Config
		writeHold bool
		wantEmpty bool
		wantText  string
	}{
		"no deps is fresh": {
			cfg:       referencingCfg,
			wantEmpty: true,
		},
		"unreferenced dep is ignored": {
			deps: func(mgr *Manager) []wedgedDep {
				return []wedgedDep{{stateKey: stateKey, identity: "stale"}}
			},
			cfg:       unreferencedCfg,
			wantEmpty: true,
		},
		"matching identity is fresh": {
			deps: func(mgr *Manager) []wedgedDep {
				return []wedgedDep{{stateKey: stateKey, identity: mgr.secretStoreDepIdentity(storeKey)}}
			},
			cfg:       referencingCfg,
			wantEmpty: true,
		},
		"changed identity is stale": {
			deps: func(mgr *Manager) []wedgedDep {
				return []wedgedDep{{stateKey: stateKey, identity: "old-identity"}}
			},
			cfg:      referencingCfg,
			wantText: "changed while the key was wedged",
		},
		"write-held dependency is stale": {
			deps: func(mgr *Manager) []wedgedDep {
				return []wedgedDep{{stateKey: stateKey, identity: mgr.secretStoreDepIdentity(storeKey)}}
			},
			cfg:       referencingCfg,
			writeHold: true,
			wantText:  "is in flight",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newExecutorTestManager()
			seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", map[string]any{"value": "secret"}, dyncfg.StatusAccepted)
			e := newExecutor(mgr)
			if tc.writeHold {
				holder := &claimProbe{label: "store-mutation"}
				e.claims.acquire(holder.request(staticClaims(claim{stateKey, claimWrite})))
				require.True(t, holder.granted())
			}
			var deps []wedgedDep
			if tc.deps != nil {
				deps = tc.deps(mgr)
			}

			got := e.staleWarmDep(deps, tc.cfg)
			if tc.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tc.wantText)
			}
		})
	}
}

func TestKeyFIFOHasStopIntent(t *testing.T) {
	fn := func(cmd dyncfg.Command) dyncfg.Function {
		return newExecutorTestFn("uid-"+string(cmd), []string{"test:collector:success:job", string(cmd)}, nil)
	}

	tests := map[string]struct {
		fifo []event
		want bool
	}{
		"empty fifo": {},
		"discovery add is not stop intent": {
			fifo: []event{{kind: eventDiscoveryAdd}},
		},
		"discovery remove is stop intent": {
			fifo: []event{{kind: eventDiscoveryRemove}},
			want: true,
		},
		"dyncfg disable is stop intent": {
			fifo: []event{{kind: eventDyncfgCommand, fn: fn(dyncfg.CommandDisable)}},
			want: true,
		},
		"dyncfg remove is stop intent": {
			fifo: []event{{kind: eventDyncfgCommand, fn: fn(dyncfg.CommandRemove)}},
			want: true,
		},
		"dyncfg restart is not stop intent": {
			fifo: []event{{kind: eventDyncfgCommand, fn: fn(dyncfg.CommandRestart)}},
		},
		"later stop intent is detected": {
			fifo: []event{
				{kind: eventDyncfgCommand, fn: fn(dyncfg.CommandGet)},
				{kind: eventDyncfgCommand, fn: fn(dyncfg.CommandDisable)},
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, keyFIFOHasStopIntent(&keyState{fifo: tc.fifo}))
		})
	}
}

func TestExecutor_ShutdownAbandonKeepsWedgeUntilLateReturn(t *testing.T) {
	mgr, _ := newExecutorTestManager()
	e := newExecutor(mgr)
	const key = "shutdown-abandon"

	holder := &claimProbe{label: "running-command"}
	e.claims.acquire(holder.request(staticClaims(claim{key: key, mode: claimWrite})))
	require.True(t, holder.granted())

	var committed error
	ks := &keyState{
		busy:  true,
		grant: holder.grant,
		pendingCommit: func(err error) {
			committed = err
		},
	}
	e.keys[key] = ks
	e.inflight = 1

	e.onEffectDoneShutdown(effectResult{
		key:     key,
		outcome: effectOutcomeAbandoned,
		err:     errors.New("deadline"),
	})

	require.True(t, e.draining)
	require.ErrorIs(t, committed, dyncfg.ErrPhaseNeverRan)
	require.NotNil(t, ks.wedge, "shutdown rewrites the committed error, not the abandon lifecycle outcome")
	assert.True(t, ks.busy, "the lane stays wedged until the leaked child returns")
	assert.True(t, e.claims.heldForWrite(key), "write claims stay held while wedged")
	assert.Equal(t, 0, e.inflight)

	e.onEffectDone(effectResult{key: key, outcome: effectOutcomeLateReturn})

	assert.False(t, e.claims.heldForWrite(key), "late return releases the retained write claim")
	if ks := e.keys[key]; ks != nil {
		assert.False(t, ks.busy)
		assert.Nil(t, ks.wedge)
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
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	newMgr := func() *Manager {
		mgr := New(Config{PluginName: testPluginName})
		mgr.effectDeadline = 50 * time.Millisecond
		return mgr
	}

	t.Run("unfenced abandon is a plain failure", func(t *testing.T) {
		mgr := newMgr()
		block := make(chan struct{})

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(context.Context) error {
			<-block
			return nil
		}})
		defer drainAbandonedEffect(mgr, res, block)

		require.Equal(t, effectOutcomeAbandoned, res.outcome)
		assert.False(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned),
			"an abandon with no fence installed must not claim the fenced outcome")
	})

	t.Run("fenced abandon carries ErrPhaseAbandoned", func(t *testing.T) {
		mgr := newMgr()
		block := make(chan struct{})
		var fenceRan atomic.Bool

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(ctx context.Context) error {
			effectControlFrom(ctx).setQuarantine(func() { fenceRan.Store(true) })
			<-block
			return nil
		}})
		defer drainAbandonedEffect(mgr, res, block)

		require.Equal(t, effectOutcomeAbandoned, res.outcome)
		assert.True(t, fenceRan.Load(), "the worker must run the fence before reporting the abandon")
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned))
	})

	t.Run("unfenced shutdown abandon is never-ran", func(t *testing.T) {
		mgr := newMgr()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		mgr.ctx = ctx // shutdown already began
		block := make(chan struct{})

		res := mgr.superviseEffect(effectTask{key: "k", effect: func(context.Context) error {
			<-block
			return nil
		}})
		defer drainAbandonedEffect(mgr, res, block)

		require.Equal(t, effectOutcomeAbandoned, res.outcome)
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseNeverRan),
			"a shutdown interruption must answer retryably no matter which side wins the claim")
	})

	t.Run("fenced shutdown abandon is never-ran too", func(t *testing.T) {
		// ONE RULE at shutdown: every non-terminal command answers 503 and
		// publishes nothing - a fence changes nothing about the outcome (it
		// still runs mechanically to suppress late output). Only a
		// DEADLINE-caused fenced abandon may publish success.
		mgr := newMgr()
		mgr.effectDeadline = time.Hour // only the cancellation may abandon
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mgr.ctx = ctx
		block := make(chan struct{})
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
		defer drainAbandonedEffect(mgr, res, block)

		require.Equal(t, effectOutcomeAbandoned, res.outcome)
		assert.True(t, errors.Is(res.err, dyncfg.ErrPhaseNeverRan),
			"a shutdown interruption answers retryably even when the stop's fence ran")
		assert.False(t, errors.Is(res.err, dyncfg.ErrPhaseAbandoned),
			"no stop-shaped success may be published at shutdown")
	})
}

// The fence disarm after a completed stop is gated on WINNING the effect's
// completion claim: a worker that abandoned during the stop - or wins the
// CAS between Stop returning and the claim - must find the fence STILL
// ARMED, so its abandon classifies as the fenced stop success
// (ErrPhaseAbandoned) instead of an unfenced plain timeout that would take
// the broken-stop 500 + cache-restore branch for a cleanly completed stop.
func TestWaitStoppedJob_ClaimGatesTheFenceDisarm(t *testing.T) {
	newCase := func() (*Manager, *effectControl, context.Context, runtimeJob) {
		mgr := New(Config{PluginName: testPluginName})
		ctl := &effectControl{}
		ctx := context.WithValue(context.Background(), effectControlKey{}, ctl)
		return mgr, ctl, ctx, &tickProbeJob{}
	}

	t.Run("winning the claim disarms the fence", func(t *testing.T) {
		mgr, ctl, ctx, job := newCase()
		mgr.waitStoppedJob(ctx, "j", job, true)
		assert.False(t, ctl.claimCompletion(), "the wait must have claimed the completion itself")
		assert.Nil(t, ctl.takeQuarantine(), "a won claim disarms the fence")
	})

	t.Run("losing the claim leaves the fence armed for the worker", func(t *testing.T) {
		mgr, ctl, ctx, job := newCase()
		require.True(t, ctl.claimAbandon(), "the worker wins the race in this scenario")
		mgr.waitStoppedJob(ctx, "j", job, true)
		assert.NotNil(t, ctl.takeQuarantine(),
			"a lost claim must leave the fence for the abandoning worker - its abandon then publishes the fenced stop success")
	})

	t.Run("an inner-phase stop never claims and always disarms", func(t *testing.T) {
		mgr, ctl, ctx, job := newCase()
		mgr.waitStoppedJob(ctx, "j", job, false)
		assert.True(t, ctl.claimCompletion(), "the outermost closure still owns the completion")
		assert.Nil(t, ctl.takeQuarantine(), "the inner-phase disarm keeps today's semantics")
	})
}

func TestDiscoveryStagedStopsAreFinalPhase(t *testing.T) {
	runWithAbandonedStop := func(t *testing.T, action func(*Manager, dyncfg.StepRunner)) {
		t.Helper()
		mgr, _ := newExecutorTestManager()
		cfg := prepareDiscoveredCfg("success", "disc")
		mgr.collectorHandler.AddDiscoveredConfig(cfg, dyncfg.StatusRunning)
		mgr.runningJobs.lock()
		mgr.runningJobs.add(cfg.FullName(), &tickProbeJob{
			fullName:   cfg.FullName(),
			moduleName: cfg.Module(),
			name:       cfg.Name(),
		})
		mgr.runningJobs.unlock()

		var ran bool
		run := func(effect func(context.Context) error, commit func(error)) {
			ran = true
			ctl := &effectControl{}
			require.True(t, ctl.claimAbandon(), "the worker wins the deadline race")
			ctx := context.WithValue(context.Background(), effectControlKey{}, ctl)
			require.NoError(t, effect(ctx))
			assert.NotNil(t, ctl.takeQuarantine(),
				"discovery one-phase stops must leave the fence armed when the worker won the race")
			commit(dyncfg.ErrPhaseAbandoned)
		}

		action(mgr, run)
		require.True(t, ran)
	}

	t.Run("replace", func(t *testing.T) {
		runWithAbandonedStop(t, func(mgr *Manager, run dyncfg.StepRunner) {
			replacement := prepareUserCfg("success", "disc")
			mgr.stagedAddConfig(replacement, run, func() {}, func(dyncfg.Function) {})
		})
	})

	t.Run("remove", func(t *testing.T) {
		runWithAbandonedStop(t, func(mgr *Manager, run dyncfg.StepRunner) {
			cfg := prepareDiscoveredCfg("success", "disc")
			mgr.stagedRemoveConfig(cfg, run)
		})
	})
}

// A collector test completing DURING the shutdown drain answers 503, never
// its natural 200/422: the keyless test path bypasses the executor's
// receive-time one-rule choke points (its worker responds directly), so the
// boundary lives at the worker's send seam - and the drain WAITS for test
// workers, making this window deterministic, not a race.
func TestCollectorTest_AnswersShutdown503(t *testing.T) {
	gate := make(chan struct{})

	reg := collectorapi.Registry{}
	reg.Register("gated", collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					<-gate // the test's Init outlives the shutdown start
					return nil
				},
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
	h := &charHarness{mgr: mgr, out: &out, in: in}

	h.dyncfg("t1", []string{mgr.dyncfgModID("gated"), "test", "probe"}, []byte("{}"))
	require.Never(t, h.outputContains("FUNCTION_RESULT_BEGIN t1"), charNeverWait, charTick,
		"the test worker must be in flight")

	// Shutdown begins while the test's Init is blocked; the drain waits for
	// test workers, so the worker completes DURING shutdown - with a module
	// SUCCESS that must still answer 503.
	cancel()
	close(gate)
	select {
	case <-done:
	case <-time.After(2 * cmdTestWorkerDrainWait):
		t.Fatal("shutdown did not complete")
	}

	require.True(t, h.outputContains("FUNCTION_RESULT_BEGIN t1 503")(),
		"a test completing during the drain must answer 503")
	assert.NotContains(t, out.String(), "FUNCTION_RESULT_BEGIN t1 200",
		"the module's success must not reach the wire after shutdown began")
}

// Once shutdown begins EVERY non-terminal command answers 503 - malformed
// ones included: the shutdown check precedes even argument validation, so a
// too-few-args request cannot answer 400 after the one-rule boundary.
func TestDyncfgConfig_MalformedAnswers503AtShutdown(t *testing.T) {
	var out simOutput
	mgr := New(Config{PluginName: testPluginName})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = collectorapi.Registry{}

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

	cancel()
	select {
	case <-done:
	case <-time.After(charWait):
		t.Fatal("shutdown did not complete")
	}

	h := &charHarness{mgr: mgr, out: &out, in: in}
	h.dyncfg("bad-args", []string{"test:collector:success"}, nil) // one arg only: malformed
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN bad-args 503"), charWait, charTick,
		"a malformed command after shutdown must answer 503, not 400")
	assert.NotContains(t, out.String(), "FUNCTION_RESULT_BEGIN bad-args 400")
}

// drainAbandonedEffect releases a directly-supervised abandoned effect the
// way the production worker and loop would: unblock the leaked module call,
// acknowledge the delivery gate (the worker closes it after its send
// reaches the loop), and wait the leaked child out - its late completion
// lands in the buffered effectDoneCh. Without this, the child parks at the
// delivery gate forever and the goroutine leaks into later tests.
func drainAbandonedEffect(mgr *Manager, res effectResult, release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
	if res.delivered != nil {
		close(res.delivered)
	}
	mgr.leakedChildren.Wait()
}

// TestSuperviseEffect_LateCompletionWaitsForAbandonDelivery pins the late
// protocol's delivery order: a leaked call that returns INSTANTLY at
// cancellation (a ctx-honoring backend) must not push its late completion
// into effectDoneCh before the abandoned result reaches the loop - an
// overtaken late result would be dropped as not-wedged and the key would
// wedge with nothing left to release it.
func TestSuperviseEffect_LateCompletionWaitsForAbandonDelivery(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	mgr.effectDeadline = 30 * time.Millisecond

	res := mgr.superviseEffect(effectTask{key: "k", effect: func(ctx context.Context) error {
		<-ctx.Done() // returns the instant the deadline fires
		return ctx.Err()
	}})
	require.Equal(t, effectOutcomeAbandoned, res.outcome)
	require.NotNil(t, res.delivered, "abandoned results carry the delivery gate")

	// The leaked child has already returned, but its late result must wait
	// for the abandon's delivery.
	require.Never(t, func() bool { return len(mgr.effectDoneCh) > 0 }, charNeverWait, charTick,
		"the late completion must not overtake the undelivered abandon")

	close(res.delivered) // what the worker does after its send reaches the loop
	require.Eventually(t, func() bool { return len(mgr.effectDoneCh) == 1 }, charWait, charTick,
		"the late completion must flow once the abandon was delivered")
	late := <-mgr.effectDoneCh
	assert.Equal(t, effectOutcomeLateReturn, late.outcome)
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
