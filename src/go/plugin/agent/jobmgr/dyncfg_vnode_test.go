// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDyncfgVnodeExec_Dispatch(t *testing.T) {
	tests := map[string]struct {
		fn            func(t *testing.T, mgr *Manager) dyncfg.Function
		wantQueuedCmd dyncfg.Command
		assertDirect  func(t *testing.T, out string)
	}{
		"schema stays direct": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "vn-schema", Args: []string{mgr.dyncfgVnodePrefixValue(), string(dyncfg.CommandSchema)}})
			},
			assertDirect: func(t *testing.T, out string) {
				assert.Contains(t, out, "FUNCTION_RESULT_BEGIN vn-schema 200 application/json")
			},
		},
		"userconfig stays direct": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "vn-userconfig",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{mgr.dyncfgVnodePrefixValue(), string(dyncfg.CommandUserconfig), "db"},
				})
			},
			assertDirect: func(t *testing.T, out string) {
				assert.Contains(t, out, "FUNCTION_RESULT_BEGIN vn-userconfig 200 application/yaml")
				assert.Contains(t, out, "name: db")
			},
		},
		"add is queued": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "vn-add",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{mgr.dyncfgVnodePrefixValue(), string(dyncfg.CommandAdd), "db"},
				})
			},
			wantQueuedCmd: dyncfg.CommandAdd,
		},
		"update is queued": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "vn-update",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{mgr.dyncfgVnodePrefixValue() + ":db", string(dyncfg.CommandUpdate)},
				})
			},
			wantQueuedCmd: dyncfg.CommandUpdate,
		},
		"remove is queued": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{UID: "vn-remove", Args: []string{mgr.dyncfgVnodePrefixValue() + ":db", string(dyncfg.CommandRemove)}})
			},
			wantQueuedCmd: dyncfg.CommandRemove,
		},
		"test is queued": {
			fn: func(t *testing.T, mgr *Manager) dyncfg.Function {
				return dyncfg.NewFunction(functions.Function{
					UID:         "vn-test",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{mgr.dyncfgVnodePrefixValue(), string(dyncfg.CommandTest), "db"},
				})
			},
			wantQueuedCmd: dyncfg.CommandTest,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := New(Config{PluginName: testPluginName})
			mgr.ctx = context.Background()
			mgr.dyncfgCh = make(chan dyncfg.Function, 1)
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			fn := tc.fn(t, mgr)
			mgr.dyncfgVnodeExec(fn)

			if tc.wantQueuedCmd != "" {
				select {
				case queued := <-mgr.dyncfgCh:
					assert.Equal(t, tc.wantQueuedCmd, queued.Command())
					assert.Equal(t, fn.UID(), queued.UID())
					assert.Equal(t, "", strings.TrimSpace(buf.String()))
				case <-time.After(time.Second):
					t.Fatal("vnode command was not queued")
				}
				return
			}

			select {
			case queued := <-mgr.dyncfgCh:
				t.Fatalf("unexpected queued vnode command: %s", queued.Command())
			default:
			}
			tc.assertDirect(t, buf.String())
		})
	}
}

func TestRun_PublishesExistingVnodesThroughController(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, out *bytes.Buffer, fnReg *recordingFunctionRegistry)
	}{
		"startup publishes vnode module and existing job": {
			run: func(t *testing.T, out *bytes.Buffer, fnReg *recordingFunctionRegistry) {
				assert.Contains(t, out.String(), "CONFIG test:vnode create accepted template /collectors/test/Vnodes")
				assert.Contains(t, out.String(), "CONFIG test:vnode:db create running job /collectors/test/Vnodes")
				prefixes := fnReg.registeredPrefixes()
				assert.Contains(t, prefixes, registeredPrefix{name: "config", prefix: "test:vnode"})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			fnReg := &recordingFunctionRegistry{}
			mgr := New(Config{
				PluginName: testPluginName,
				Out:        &buf,
				FnReg:      fnReg,
				Vnodes: map[string]*vnodes.VirtualNode{
					"db": {
						Name:       "db",
						Hostname:   "db",
						GUID:       "11111111-1111-1111-1111-111111111111",
						SourceType: confgroup.TypeDyncfg,
						Source:     confgroup.TypeDyncfg,
					},
				},
			})

			ctx, cancel := context.WithCancel(context.Background())
			in := make(chan []*confgroup.Group)
			done := make(chan struct{})
			go func() {
				mgr.Run(ctx, in)
				close(done)
			}()

			waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
			defer waitCancel()
			require.True(t, mgr.WaitStarted(waitCtx))

			cancel()
			close(in)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("manager did not stop after cancel")
			}

			tc.run(t, &buf, fnReg)
		})
	}
}

func TestCreateCollectorJob_UsesVnodeControllerLookup(t *testing.T) {
	tests := map[string]struct {
		cfg     confgroup.Config
		vnodes  map[string]*vnodes.VirtualNode
		wantErr string
		run     func(t *testing.T, job runtimeJob)
	}{
		"existing vnode is copied into created job": {
			cfg: prepareDyncfgCfg("success", "mysql").Set("vnode", "db"),
			vnodes: map[string]*vnodes.VirtualNode{
				"db": {
					Name:       "db",
					Hostname:   "db",
					GUID:       "11111111-1111-1111-1111-111111111111",
					SourceType: confgroup.TypeDyncfg,
					Source:     confgroup.TypeDyncfg,
				},
			},
			run: func(t *testing.T, job runtimeJob) {
				assert.Equal(t, "db", job.Vnode().Name)
			},
		},
		"missing vnode returns an error": {
			cfg:     prepareDyncfgCfg("success", "mysql").Set("vnode", "db"),
			wantErr: "vnode 'db' is not found",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName, Vnodes: tc.vnodes})
			mgr.modules = prepareMockRegistry()

			job, err := mgr.createCollectorJob(tc.cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			tc.run(t, job)
		})
	}
}

func TestDyncfgCmdTest_ValidatesVnodeThroughControllerLookup(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, out *bytes.Buffer)
	}{
		"missing vnode returns 400 before worker execution": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				cfg := prepareDyncfgCfg("success", "job").Set("vnode", "missing")
				payload, err := json.Marshal(cfg)
				require.NoError(t, err)

				fn := dyncfg.NewFunction(functions.Function{
					UID:         "collector-vnode-missing",
					ContentType: "application/json",
					Payload:     payload,
					Args:        []string{mgr.dyncfgModID("success"), string(dyncfg.CommandTest), "job"},
				})

				mgr.dyncfgCmdTest(fn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "collector-vnode-missing", &resp)
				assert.Equal(t, float64(400), resp["status"])
				assert.Contains(t, resp["errorMessage"], "missing")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := New(Config{PluginName: testPluginName})
			mgr.modules = prepareMockRegistry()
			mgr.ctx = context.Background()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))
			tc.run(t, mgr, &buf)
		})
	}
}

func TestApplyVnodeUpdate_UpdatesMatchingRunningJobs(t *testing.T) {
	mgr := New(Config{PluginName: testPluginName})

	dbJob := &vnodeUpdateProbeJob{
		fullName: "success_db",
		module:   "success",
		name:     "db",
		vnode:    vnodes.VirtualNode{Name: "db"},
	}
	otherJob := &vnodeUpdateProbeJob{
		fullName: "success_other",
		module:   "success",
		name:     "other",
		vnode:    vnodes.VirtualNode{Name: "other"},
	}

	mgr.runningJobs.lock()
	mgr.runningJobs.add(dbJob.FullName(), dbJob)
	mgr.runningJobs.add(otherJob.FullName(), otherJob)
	mgr.runningJobs.unlock()

	next := &vnodes.VirtualNode{
		Name:       "db",
		Hostname:   "db-new",
		GUID:       "11111111-1111-1111-1111-111111111111",
		SourceType: confgroup.TypeDyncfg,
		Source:     confgroup.TypeDyncfg,
	}

	mgr.applyVnodeUpdate("db", next)

	require.NotNil(t, dbJob.updated)
	assert.Equal(t, "db-new", dbJob.updated.Hostname)
	assert.Nil(t, otherJob.updated)
}

type vnodeUpdateProbeJob struct {
	fullName string
	module   string
	name     string
	vnode    vnodes.VirtualNode
	updated  *vnodes.VirtualNode
}

func (j *vnodeUpdateProbeJob) FullName() string   { return j.fullName }
func (j *vnodeUpdateProbeJob) ModuleName() string { return j.module }
func (j *vnodeUpdateProbeJob) Name() string       { return j.name }
func (j *vnodeUpdateProbeJob) Collector() any     { return nil }
func (j *vnodeUpdateProbeJob) Start()             {}
func (j *vnodeUpdateProbeJob) Stop()              {}
func (j *vnodeUpdateProbeJob) Tick(int)           {}
func (j *vnodeUpdateProbeJob) AutoDetection() error {
	return nil
}
func (j *vnodeUpdateProbeJob) AutoDetectionEvery() int { return 0 }
func (j *vnodeUpdateProbeJob) RetryAutoDetection() bool {
	return false
}
func (j *vnodeUpdateProbeJob) Cleanup()                  {}
func (j *vnodeUpdateProbeJob) IsRunning() bool           { return true }
func (j *vnodeUpdateProbeJob) Panicked() bool            { return false }
func (j *vnodeUpdateProbeJob) Vnode() vnodes.VirtualNode { return j.vnode }
func (j *vnodeUpdateProbeJob) UpdateVnode(vnode *vnodes.VirtualNode) {
	if vnode == nil {
		j.updated = nil
		return
	}
	copy := *vnode
	j.updated = &copy
	j.vnode = copy
}
