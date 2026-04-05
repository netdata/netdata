// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestDyncfgConfigUserconfig_InvalidPayload_Returns400Only(t *testing.T) {
	tests := map[string]struct {
		contentType string
		payload     []byte
	}{
		"invalid json payload should stop after 400": {
			contentType: "application/json",
			payload:     []byte("{"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			mgr := New(Config{PluginName: testPluginName})
			mgr.modules = prepareMockRegistry()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			fn := dyncfg.NewFunction(functions.Function{
				UID:         "bad-userconfig",
				ContentType: tc.contentType,
				Payload:     tc.payload,
				Args: []string{
					mgr.dyncfgModID("success"),
					string(dyncfg.CommandUserconfig),
					"test",
				},
			})

			mgr.dyncfgCmdUserconfig(fn)

			out := buf.String()
			assert.Equal(t, 1, strings.Count(out, "FUNCTION_RESULT_BEGIN bad-userconfig"))
			assert.Contains(t, out, "\"status\":400")
			assert.NotContains(t, out, "application/yaml")
		})
	}
}

func TestDyncfgCollectorExec_TestCommandQueued(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})
	mgr.ctx = context.Background()
	mgr.dyncfgCh = make(chan dyncfg.Function, 1)

	fn := dyncfg.NewFunction(functions.Function{
		UID:  "queued-test",
		Args: []string{mgr.dyncfgModID("success"), "test", "job"},
	})

	mgr.dyncfgCollectorExec(fn)

	select {
	case queued := <-mgr.dyncfgCh:
		assert.Equal(t, dyncfg.CommandTest, queued.Command())
		assert.Equal(t, fn.UID(), queued.UID())
	case <-time.After(time.Second):
		t.Fatal("test command was not queued")
	}
}

func TestDyncfgCmdTest_WhenWorkerPoolFull_Returns503(t *testing.T) {
	var buf bytes.Buffer

	mgr := New(Config{PluginName: testPluginName})
	mgr.modules = prepareMockRegistry()
	mgr.ctx = context.Background()
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

	for i := 0; i < cap(mgr.cmdTestSem); i++ {
		mgr.cmdTestSem <- struct{}{}
	}

	cfg := prepareDyncfgCfg("success", "job")
	payload, err := json.Marshal(cfg)
	require.NoError(t, err)

	fn := dyncfg.NewFunction(functions.Function{
		UID:         "pool-full",
		ContentType: "application/json",
		Payload:     payload,
		Args: []string{
			mgr.dyncfgModID("success"),
			string(dyncfg.CommandTest),
			"job",
		},
	})

	mgr.dyncfgCmdTest(fn)

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "FUNCTION_RESULT_BEGIN pool-full"))
	assert.Contains(t, out, "\"status\":503")
}

func TestDyncfgCmdTestTimeout_RequestTimeoutOverridesDefault(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})

	withRequestTimeout := dyncfg.NewFunction(functions.Function{Timeout: 7 * time.Second})
	assert.Equal(t, 7*time.Second, mgr.dyncfgCmdTestTimeout(withRequestTimeout))

	withoutTimeout := dyncfg.NewFunction(functions.Function{})
	assert.Equal(t, cmdTestDefaultTimeout, mgr.dyncfgCmdTestTimeout(withoutTimeout))
}

func TestDyncfgCollectorSeqExec_SyncsSecretStoreDepsForMutatingCommands(t *testing.T) {
	tests := map[string]struct {
		command        dyncfg.Command
		oldCfg         confgroup.Config
		newCfg         confgroup.Config
		args           []string
		wantOldExposed int
		wantNewExposed int
		wantOldRunning int
		wantNewRunning int
	}{
		"add syncs active deps from newly exposed config": {
			command:        dyncfg.CommandAdd,
			newCfg:         prepareDyncfgCfg("success", "job").Set("password", "${store:vault:vault_prod:secret/data/mysql#password}"),
			args:           []string{"success", string(dyncfg.CommandAdd), "job"},
			wantOldExposed: 0,
			wantNewExposed: 1,
			wantOldRunning: 0,
			wantNewRunning: 0,
		},
		"update replaces active deps with updated config": {
			command:        dyncfg.CommandUpdate,
			oldCfg:         prepareDyncfgCfg("success", "job").Set("password", "${store:vault:vault_old:secret/data/mysql#password}"),
			newCfg:         prepareDyncfgCfg("success", "job").Set("password", "${store:vault:vault_new:secret/data/mysql#password}"),
			args:           []string{"success:job", string(dyncfg.CommandUpdate)},
			wantOldExposed: 0,
			wantNewExposed: 1,
			wantOldRunning: 0,
			wantNewRunning: 0,
		},
		"remove clears active deps when config disappears": {
			command:        dyncfg.CommandRemove,
			oldCfg:         prepareDyncfgCfg("success", "job").Set("password", "${store:vault:vault_old:secret/data/mysql#password}"),
			args:           []string{"success:job", string(dyncfg.CommandRemove)},
			wantOldExposed: 0,
			wantNewExposed: 0,
			wantOldRunning: 0,
			wantNewRunning: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorSeqTestCallbacks{mgr: mgr, parsed: map[dyncfg.Command]confgroup.Config{}}
			if tc.newCfg != nil {
				cb.parsed[tc.command] = tc.newCfg
			}
			mgr.collectorHandler = newCollectorTestHandler(mgr, cb)

			if tc.oldCfg != nil {
				seedCollectorEntry(mgr, tc.oldCfg, dyncfg.StatusDisabled)
				mgr.syncSecretStoreDepsForConfig(tc.oldCfg)
			}

			var payload []byte
			if tc.command == dyncfg.CommandAdd || tc.command == dyncfg.CommandUpdate {
				payload = mustMarshalCollectorConfigPayload(t, tc.newCfg)
			}

			fn := dyncfg.NewFunction(functions.Function{
				UID:         name,
				ContentType: "application/json",
				Payload:     payload,
				Args:        collectorTestArgs(mgr, tc.args...),
			})

			mgr.dyncfgCollectorSeqExec(fn)

			oldExposed, oldRunning := mgr.secretStoreDeps.Impacted("vault:vault_old")
			newExposed, newRunning := mgr.secretStoreDeps.Impacted("vault:vault_prod")
			if tc.command == dyncfg.CommandUpdate {
				newExposed, newRunning = mgr.secretStoreDeps.Impacted("vault:vault_new")
			}

			assert.Len(t, oldExposed, tc.wantOldExposed)
			assert.Len(t, oldRunning, tc.wantOldRunning)
			assert.Len(t, newExposed, tc.wantNewExposed)
			assert.Len(t, newRunning, tc.wantNewRunning)
		})
	}
}

func TestDyncfgCollectorSeqExec_DoesNotSyncSecretStoreDepsForNonMutatingCommands(t *testing.T) {
	tests := map[string]struct {
		command dyncfg.Command
		args    []string
		payload confgroup.Config
		status  dyncfg.Status
	}{
		"restart leaves deps unchanged": {
			command: dyncfg.CommandRestart,
			args:    []string{"success:job", string(dyncfg.CommandRestart)},
			status:  dyncfg.StatusDisabled,
		},
		"test leaves deps unchanged": {
			command: dyncfg.CommandTest,
			args:    []string{"success", string(dyncfg.CommandTest), "job"},
			payload: prepareDyncfgCfg("success", "job"),
			status:  dyncfg.StatusDisabled,
		},
		"schema leaves deps unchanged": {
			command: dyncfg.CommandSchema,
			args:    []string{"success", string(dyncfg.CommandSchema)},
			status:  dyncfg.StatusDisabled,
		},
		"get leaves deps unchanged": {
			command: dyncfg.CommandGet,
			args:    []string{"success:job", string(dyncfg.CommandGet)},
			status:  dyncfg.StatusDisabled,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cfg := prepareDyncfgCfg("success", "job").Set("password", "${store:vault:vault_prod:secret/data/mysql#password}")
			seedCollectorEntry(mgr, cfg, tc.status)
			mgr.syncSecretStoreDepsForConfig(cfg)

			beforeExposed, beforeRunning := mgr.secretStoreDeps.Impacted("vault:vault_prod")
			require.Len(t, beforeExposed, 1)

			var payload []byte
			if tc.payload != nil {
				payload = mustMarshalCollectorConfigPayload(t, tc.payload)
			}

			fn := dyncfg.NewFunction(functions.Function{
				UID:         name,
				ContentType: "application/json",
				Payload:     payload,
				Args:        collectorTestArgs(mgr, tc.args...),
			})

			mgr.dyncfgCollectorSeqExec(fn)
			if tc.command == dyncfg.CommandTest {
				mgr.cmdTestWG.Wait()
			}

			afterExposed, afterRunning := mgr.secretStoreDeps.Impacted("vault:vault_prod")
			assert.Equal(t, beforeExposed, afterExposed)
			assert.Equal(t, beforeRunning, afterRunning)
		})
	}
}

func TestCollectorCallbacks_ParseAndValidate(t *testing.T) {
	tests := map[string]struct {
		args          []string
		cfg           confgroup.Config
		payload       []byte
		wantErr       string
		wantModule    string
		wantName      string
		wantProvider  string
		wantSourceTyp string
	}{
		"invalid id is rejected": {
			args:    []string{"", string(dyncfg.CommandAdd), "validated"},
			cfg:     prepareDyncfgCfg("success", "validated"),
			wantErr: "could not extract module name from ID",
		},
		"invalid payload is rejected": {
			args:    []string{"success", string(dyncfg.CommandAdd), "validated"},
			payload: []byte("{"),
			wantErr: "invalid configuration format",
		},
		"valid payload is accepted and metadata is set": {
			args:          []string{"success", string(dyncfg.CommandAdd), "validated"},
			cfg:           prepareDyncfgCfg("success", "payload-name").Set("option_str", "one").Set("option_int", 2),
			wantModule:    "success",
			wantName:      "validated",
			wantProvider:  "dyncfg",
			wantSourceTyp: confgroup.TypeDyncfg,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorCallbacks{mgr: mgr}
			payload := tc.payload
			if payload == nil && tc.cfg != nil {
				payload = mustMarshalCollectorConfigPayload(t, tc.cfg)
			}
			fn := dyncfg.NewFunction(functions.Function{
				UID:         name,
				ContentType: "application/json",
				Payload:     payload,
				Args:        collectorTestArgs(mgr, tc.args...),
			})

			cfg, err := cb.ParseAndValidate(fn, "validated")
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantModule, cfg.Module())
			assert.Equal(t, tc.wantName, cfg.Name())
			assert.Equal(t, tc.wantProvider, cfg.Provider())
			assert.Equal(t, tc.wantSourceTyp, cfg.SourceType())
		})
	}
}

func TestCollectorCallbacks_ParseAndValidate_SuppressesAuditSideEffects(t *testing.T) {
	tempDir := t.TempDir()
	analyzer := &auditAnalyzerSpy{}
	mgr := newCollectorTestManager()
	mgr.auditAnalyzer = analyzer
	mgr.auditDataDir = tempDir
	cb := &collectorCallbacks{mgr: mgr}

	cfg := prepareDyncfgCfg("success", "payload-name").Set("option_str", "one").Set("option_int", 2)
	fn := dyncfg.NewFunction(functions.Function{
		UID:         "validation-audit-side-effects",
		ContentType: "application/json",
		Payload:     mustMarshalCollectorConfigPayload(t, cfg),
		Args:        collectorTestArgs(mgr, "success", string(dyncfg.CommandAdd), "validated"),
	})

	_, err := cb.ParseAndValidate(fn, "validated")
	require.NoError(t, err)

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Empty(t, analyzer.registered)
}

func TestCollectorCallbacks_ApplyConfigLoggingHonorsValidationMode(t *testing.T) {
	tests := map[string]struct {
		run            func(t *testing.T, mgr *Manager, logBuf *bytes.Buffer)
		wantLogMessage bool
	}{
		"validation suppresses expected applyConfig error logs": {
			run: func(t *testing.T, mgr *Manager, _ *bytes.Buffer) {
				cb := &collectorCallbacks{mgr: mgr}
				cfg := prepareDyncfgCfg("success", "payload-name").Set("option_str", "one").Set("option_int", "bad")
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "validation-no-log",
					ContentType: "application/json",
					Payload:     mustMarshalCollectorConfigPayload(t, cfg),
					Args:        collectorTestArgs(mgr, "success", string(dyncfg.CommandAdd), "validated"),
				})

				_, err := cb.ParseAndValidate(fn, "validated")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to apply configuration")
			},
		},
		"runtime creation still logs applyConfig errors": {
			run: func(t *testing.T, mgr *Manager, _ *bytes.Buffer) {
				cfg := prepareDyncfgCfg("success", "runtime-job").Set("option_str", "one").Set("option_int", "bad")

				_, err := mgr.createCollectorJob(cfg)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot unmarshal")
			},
			wantLogMessage: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logBuf bytes.Buffer
			mgr := newCollectorTestManager()
			mgr.Logger = logger.NewWithWriter(&logBuf)

			tc.run(t, mgr, &logBuf)

			const msg = "failed to apply config for"
			if tc.wantLogMessage {
				assert.Contains(t, logBuf.String(), msg)
			} else {
				assert.NotContains(t, logBuf.String(), msg)
			}
		})
	}
}

func TestCollectorCallbacks_Start(t *testing.T) {
	tests := map[string]struct {
		cfg              confgroup.Config
		wantErr          string
		wantCode         int
		wantRunning      bool
		wantRetryPending bool
	}{
		"success starts the job": {
			cfg:         prepareDyncfgCfg("success", "job"),
			wantRunning: true,
		},
		"autodetection failure schedules retry": {
			cfg:              prepareDyncfgCfg("retrycheck", "job").Set("autodetection_retry", 1),
			wantErr:          "job enable failed",
			wantRetryPending: true,
		},
		"invalid config returns coded validation error": {
			cfg:      prepareDyncfgCfg("missing", "job"),
			wantErr:  "invalid configuration",
			wantCode: 400,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorCallbacks{mgr: mgr}

			err := cb.Start(tc.cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				if tc.wantCode != 0 {
					var coded interface{ Code() int }
					require.ErrorAs(t, err, &coded)
					assert.Equal(t, tc.wantCode, coded.Code())
				}
			} else {
				require.NoError(t, err)
			}

			_, running := mgr.runningJobs.lookup(tc.cfg.FullName())
			assert.Equal(t, tc.wantRunning, running)

			_, retryPending := mgr.retryingTasks.lookup(tc.cfg)
			assert.Equal(t, tc.wantRetryPending, retryPending)

			if tc.wantRunning {
				mgr.stopRunningJob(tc.cfg.FullName())
			}
			if tc.wantRetryPending {
				mgr.retryingTasks.remove(tc.cfg)
			}
		})
	}
}

func TestCollectorCallbacks_Update(t *testing.T) {
	tests := map[string]struct {
		oldCfg           confgroup.Config
		newCfg           confgroup.Config
		wantErr          string
		wantRunning      bool
		wantRetryPending bool
	}{
		"success restarts with the new config and clears old state": {
			oldCfg:      prepareDyncfgCfg("success", "job"),
			newCfg:      prepareDyncfgCfg("success", "job").Set("option_str", "changed"),
			wantRunning: true,
		},
		"autodetection failure clears old state and schedules retry": {
			oldCfg:           prepareDyncfgCfg("retrycheck", "job"),
			newCfg:           prepareDyncfgCfg("retrycheck", "job").Set("autodetection_retry", 1),
			wantErr:          "job update failed",
			wantRetryPending: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorCallbacks{mgr: mgr}
			oldJob := &collectorProbeJob{
				fullName:   tc.oldCfg.FullName(),
				moduleName: tc.oldCfg.Module(),
				name:       tc.oldCfg.Name(),
			}

			mgr.runningJobs.lock()
			mgr.runningJobs.add(oldJob.FullName(), oldJob)
			mgr.runningJobs.unlock()
			mgr.fileStatus.add(tc.oldCfg, dyncfg.StatusRunning.String())

			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			mgr.retryingTasks.add(tc.oldCfg, &retryTask{cancel: cancel})

			err := cb.Update(tc.oldCfg, tc.newCfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.True(t, oldJob.stopped)

			_, oldRetryPending := mgr.retryingTasks.lookup(tc.oldCfg)
			assert.False(t, oldRetryPending)

			_, oldFileStatus := mgr.fileStatus.lookup(tc.oldCfg)
			assert.False(t, oldFileStatus)

			_, running := mgr.runningJobs.lookup(tc.newCfg.FullName())
			assert.Equal(t, tc.wantRunning, running)

			_, retryPending := mgr.retryingTasks.lookup(tc.newCfg)
			assert.Equal(t, tc.wantRetryPending, retryPending)

			if tc.wantRunning {
				mgr.stopRunningJob(tc.newCfg.FullName())
			}
			if tc.wantRetryPending {
				mgr.retryingTasks.remove(tc.newCfg)
			}
		})
	}
}

func TestCollectorCallbacks_Stop(t *testing.T) {
	tests := map[string]struct{}{
		"stop removes retry task, running job, and file status": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorCallbacks{mgr: mgr}
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

			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			mgr.retryingTasks.add(cfg, &retryTask{cancel: cancel})

			cb.Stop(cfg)

			assert.True(t, job.stopped)
			_, running := mgr.runningJobs.lookup(cfg.FullName())
			assert.False(t, running)
			_, retryPending := mgr.retryingTasks.lookup(cfg)
			assert.False(t, retryPending)
			_, fileStatus := mgr.fileStatus.lookup(cfg)
			assert.False(t, fileStatus)
		})
	}
}

func TestCollectorCallbacks_OnStatusChange(t *testing.T) {
	tests := map[string]struct {
		cfg      confgroup.Config
		status   dyncfg.Status
		wantSeen bool
	}{
		"running dyncfg config is persisted to file status": {
			cfg:      prepareDyncfgCfg("success", "job"),
			status:   dyncfg.StatusRunning,
			wantSeen: true,
		},
		"failed dyncfg config is ignored": {
			cfg:    prepareDyncfgCfg("success", "job"),
			status: dyncfg.StatusFailed,
		},
		"running non-dyncfg config is ignored": {
			cfg:    prepareUserCfg("success", "job"),
			status: dyncfg.StatusRunning,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := newCollectorTestManager()
			cb := &collectorCallbacks{mgr: mgr}
			entry := &dyncfg.Entry[confgroup.Config]{
				Cfg:    tc.cfg,
				Status: tc.status,
			}

			cb.OnStatusChange(entry, dyncfg.StatusAccepted, dyncfg.NewFunction(functions.Function{}))

			_, ok := mgr.fileStatus.lookup(tc.cfg)
			assert.Equal(t, tc.wantSeen, ok)
		})
	}
}

func TestRunDyncfgCmdTest_CleanupIsDeferred(t *testing.T) {
	tests := map[string]struct {
		initErr    string
		checkErr   string
		wantStatus float64
	}{
		"cleanup runs after init failure": {
			initErr:    "init failed",
			wantStatus: 422,
		},
		"cleanup runs after check failure": {
			checkErr:   "check failed",
			wantStatus: 422,
		},
		"cleanup runs after success": {
			wantStatus: 200,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := newCollectorTestManager()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			module := &collectorapi.MockCollectorV1{}
			if tc.initErr != "" {
				module.InitFunc = func(context.Context) error { return errors.New(tc.initErr) }
			}
			if tc.checkErr != "" {
				module.CheckFunc = func(context.Context) error { return errors.New(tc.checkErr) }
			}

			task := dyncfgCmdTestTask{
				fn: dyncfg.NewFunction(functions.Function{
					UID: name,
				}),
				moduleName: "success",
				creator: collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 {
						return module
					},
				},
				cfg:     prepareDyncfgCfg("success", "job"),
				timeout: time.Second,
			}

			mgr.cmdTestSem <- struct{}{}
			mgr.runDyncfgCmdTest(task)

			assert.True(t, module.CleanupDone)

			var resp map[string]any
			mustDecodeFunctionPayload(t, buf.String(), name, &resp)
			assert.Equal(t, tc.wantStatus, resp["status"])
		})
	}
}

func TestRunDyncfgCmdTest_ApplyConfigUsesRequestTimeout(t *testing.T) {
	tests := map[string]struct {
		timeout time.Duration
	}{
		"secret resolution sees request deadline": {
			timeout: 20 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			blockingSvc := &blockingSecretStoreService{}
			mgr := newCollectorTestManagerWithService(blockingSvc)
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			task := dyncfgCmdTestTask{
				fn: dyncfg.NewFunction(functions.Function{
					UID: name,
				}),
				moduleName: "success",
				creator: collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{}
					},
				},
				cfg: prepareDyncfgCfg("success", "job").
					Set("password", "${store:vault:vault_prod:secret/data/mysql#password}"),
				timeout: tc.timeout,
			}

			mgr.cmdTestSem <- struct{}{}
			start := time.Now()
			mgr.runDyncfgCmdTest(task)
			elapsed := time.Since(start)

			var resp map[string]any
			mustDecodeFunctionPayload(t, buf.String(), name, &resp)
			assert.Equal(t, float64(400), resp["status"])
			assert.Contains(t, resp["errorMessage"], context.DeadlineExceeded.Error())
			assert.True(t, blockingSvc.sawDeadline)
			assert.Less(t, elapsed, tc.timeout+250*time.Millisecond)
		})
	}
}

func TestDyncfgCmdTest_ShutdownBeforeWorker_Returns503(t *testing.T) {
	tests := map[string]struct{}{
		"shutdown manager returns 503 before scheduling worker": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := newCollectorTestManager()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			mgr.ctx = ctx

			fn := dyncfg.NewFunction(functions.Function{
				UID:         name,
				ContentType: "application/json",
				Payload:     mustMarshalCollectorConfigPayload(t, prepareDyncfgCfg("success", "job")),
				Args:        []string{mgr.dyncfgModID("success"), string(dyncfg.CommandTest), "job"},
			})

			mgr.dyncfgCmdTest(fn)

			var resp map[string]any
			mustDecodeFunctionPayload(t, buf.String(), name, &resp)
			assert.Equal(t, float64(503), resp["status"])
		})
	}
}

func TestDyncfgCmdTest_MissingPayload_Returns400(t *testing.T) {
	tests := map[string]struct{}{
		"missing payload is rejected before config parsing": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := newCollectorTestManager()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			fn := dyncfg.NewFunction(functions.Function{
				UID:  name,
				Args: []string{mgr.dyncfgModID("success"), string(dyncfg.CommandTest), "job"},
			})

			mgr.dyncfgCmdTest(fn)

			var resp map[string]any
			mustDecodeFunctionPayload(t, buf.String(), name, &resp)
			assert.Equal(t, float64(400), resp["status"])
			assert.Contains(t, resp["errorMessage"], "Missing configuration payload.")
		})
	}
}

func newCollectorTestManager() *Manager {
	return newCollectorTestManagerWithService(nil)
}

func newCollectorTestManagerWithService(secretStoreSvc secretstore.Service) *Manager {
	mgr := New(Config{
		PluginName:         testPluginName,
		SecretStoreService: secretStoreSvc,
	})
	mgr.ctx = context.Background()
	mgr.modules = prepareMockRegistry()
	mgr.modules.Register("retrycheck", collectorapi.Creator{
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				CheckFunc: func(context.Context) error { return errors.New("mock failed check") },
			}
		},
	})
	mgr.fileStatus = newFileStatus()
	return mgr
}

func newCollectorTestHandler(mgr *Manager, cb dyncfg.Callbacks[confgroup.Config]) *dyncfg.Handler[confgroup.Config] {
	return dyncfg.NewHandler(dyncfg.HandlerOpts[confgroup.Config]{
		Logger:    mgr.Logger,
		API:       mgr.dyncfgResponder,
		Seen:      mgr.collectorSeen,
		Exposed:   mgr.collectorExposed,
		Callbacks: cb,
		WaitKey: func(cfg confgroup.Config) string {
			return cfg.FullName()
		},
		WaitTimeout:             waitDecisionTimeout,
		Path:                    "/collectors/test/Jobs",
		EnableFailCode:          200,
		RemoveStockOnEnableFail: true,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandRestart,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})
}

func seedCollectorEntry(mgr *Manager, cfg confgroup.Config, status dyncfg.Status) {
	mgr.collectorSeen.Add(cfg)
	mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
		Cfg:    cfg,
		Status: status,
	})
}

func collectorTestArgs(mgr *Manager, args ...string) []string {
	out := make([]string, len(args))
	copy(out, args)
	if len(out) == 0 {
		return out
	}

	switch {
	case strings.Contains(out[0], ":"):
		out[0] = mgr.dyncfgCollectorPrefixValue() + out[0]
	case out[0] != "":
		out[0] = mgr.dyncfgModID(out[0])
	}

	return out
}

func mustMarshalCollectorConfigPayload(t *testing.T, cfg confgroup.Config) []byte {
	t.Helper()

	payload, err := json.Marshal(cfg)
	require.NoError(t, err)
	return payload
}

type collectorSeqTestCallbacks struct {
	mgr    *Manager
	parsed map[dyncfg.Command]confgroup.Config
}

func (cb *collectorSeqTestCallbacks) ExtractKey(fn dyncfg.Function) (key, name string, ok bool) {
	return cb.mgr.collectorCallbacks.ExtractKey(fn)
}

func (cb *collectorSeqTestCallbacks) ParseAndValidate(fn dyncfg.Function, _ string) (confgroup.Config, error) {
	cfg, ok := cb.parsed[fn.Command()]
	if !ok {
		return nil, errors.New("unexpected parse request")
	}
	return cfg, nil
}

func (cb *collectorSeqTestCallbacks) Start(confgroup.Config) error { return nil }

func (cb *collectorSeqTestCallbacks) Update(_, _ confgroup.Config) error { return nil }

func (cb *collectorSeqTestCallbacks) Stop(confgroup.Config) {}

func (cb *collectorSeqTestCallbacks) OnStatusChange(*dyncfg.Entry[confgroup.Config], dyncfg.Status, dyncfg.Function) {
}

func (cb *collectorSeqTestCallbacks) ConfigID(cfg confgroup.Config) string {
	return cb.mgr.dyncfgJobID(cfg)
}

type collectorProbeJob struct {
	fullName   string
	moduleName string
	name       string
	stopped    bool
}

func (j *collectorProbeJob) FullName() string   { return j.fullName }
func (j *collectorProbeJob) ModuleName() string { return j.moduleName }
func (j *collectorProbeJob) Name() string       { return j.name }
func (j *collectorProbeJob) Collector() any     { return nil }
func (j *collectorProbeJob) Start()             {}
func (j *collectorProbeJob) Stop()              { j.stopped = true }
func (j *collectorProbeJob) Tick(int)           {}
func (j *collectorProbeJob) AutoDetection() error {
	return nil
}
func (j *collectorProbeJob) AutoDetectionEvery() int { return 0 }
func (j *collectorProbeJob) RetryAutoDetection() bool {
	return false
}

type blockingSecretStoreService struct {
	sawDeadline bool
}

func (s *blockingSecretStoreService) Capture() *secretstore.Snapshot { return nil }

func (s *blockingSecretStoreService) Resolve(ctx context.Context, _ *secretstore.Snapshot, _, _ string) (string, error) {
	_, s.sawDeadline = ctx.Deadline()
	if !s.sawDeadline {
		return "", errors.New("missing request deadline")
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func (*blockingSecretStoreService) Kinds() []secretstore.StoreKind { return nil }

func (*blockingSecretStoreService) DisplayName(secretstore.StoreKind) (string, bool) {
	return "", false
}

func (*blockingSecretStoreService) Schema(secretstore.StoreKind) (string, bool) { return "", false }

func (*blockingSecretStoreService) New(secretstore.StoreKind) (secretstore.Store, bool) {
	return nil, false
}

func (*blockingSecretStoreService) GetStatus(string) (secretstore.StoreStatus, bool) {
	return secretstore.StoreStatus{}, false
}

func (*blockingSecretStoreService) Validate(context.Context, secretstore.Config) error { return nil }

func (*blockingSecretStoreService) ValidateStored(context.Context, string) error { return nil }

func (*blockingSecretStoreService) Add(context.Context, secretstore.Config) error { return nil }

func (*blockingSecretStoreService) Update(context.Context, string, secretstore.Config) error {
	return nil
}

func (*blockingSecretStoreService) Remove(string) error      { return nil }
func (j *collectorProbeJob) Cleanup()                        {}
func (j *collectorProbeJob) IsRunning() bool                 { return true }
func (j *collectorProbeJob) Panicked() bool                  { return false }
func (j *collectorProbeJob) Vnode() vnodes.VirtualNode       { return vnodes.VirtualNode{} }
func (j *collectorProbeJob) UpdateVnode(*vnodes.VirtualNode) {}

type auditAnalyzerSpy struct {
	registered []string
}

func (a *auditAnalyzerSpy) RegisterJob(jobName, moduleName, dir string) {
	a.registered = append(a.registered, moduleName+":"+jobName+":"+dir)
}

func (*auditAnalyzerSpy) RecordJobStructure(string, string, *collectorapi.Charts) {}
func (*auditAnalyzerSpy) UpdateJobStructure(string, string, *collectorapi.Charts) {}
func (*auditAnalyzerSpy) RecordCollection(string, string, map[string]int64)       {}
