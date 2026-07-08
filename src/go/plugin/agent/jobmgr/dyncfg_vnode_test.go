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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/vnodectl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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

			job, err := mgr.createCollectorJob(context.Background(), tc.cfg)
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

func TestRunningJobPullsVnodeUpdateFromStore(t *testing.T) {
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

	var out, jobOut simOutput
	mgr := New(Config{PluginName: testPluginName, Out: &jobOut})
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.modules = reg
	mgr.vnodesCtl = vnodectl.New(vnodectl.Options{
		Logger:       mgr.Logger,
		API:          mgr.dyncfgResponder,
		Plugin:       testPluginName,
		AffectedJobs: mgr.affectedVnodeJobs,
	})

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

	const guid = "b0b0b0b0-0000-4000-8000-000000000055"
	h.dyncfg("vn-add", []string{h.mgr.dyncfgVnodePrefixValue(), "add", "v-pull"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-one"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-add 202"), charWait, charTick)

	cfg := prepareDyncfgCfg("gated", "mysql").Set("vnode", "v-pull")
	h.dyncfg("1-add", []string{h.mgr.dyncfgModID("gated"), "add", "mysql"},
		mustJSON(t, map[string]any{"vnode": "v-pull", "update_every": 1}))
	h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(cfg), "enable"}, nil)
	require.Eventually(t, func() bool { return strings.Contains(jobOut.String(), "host-one") }, charWait, charTick)

	h.dyncfg("vn-update", []string{h.mgr.dyncfgVnodePrefixValue() + ":v-pull", "update"},
		mustJSON(t, map[string]any{"guid": guid, "hostname": "host-two"}))
	require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN vn-update 202"), charWait, charTick)
	require.Eventually(t, func() bool { return strings.Contains(jobOut.String(), "host-two") }, charWait, charTick,
		"running job must refresh from the vnode store")
}
