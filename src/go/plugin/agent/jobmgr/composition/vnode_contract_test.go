// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestVNodeCommandsValidateUnmodifiedNames(t *testing.T) {
	for _, command := range []string{"add", "test", "userconfig"} {
		for _, name := range []string{"db:one", "db one", "db\u00a0one", "db\\one", "db'one", "db=one"} {
			t.Run(command+"/"+name, func(t *testing.T) {
				binding, configured := newTestVNodeBinding(t, confgroup.TypeUser, nil)
				before := configured.Entries()
				input := functionadapter.HandlerInput{
					Args: []string{"go.d:vnode", command, name}, HasPayload: true,
					Payload:     []byte(`{"hostname":"replacement","guid":"22222222-2222-2222-2222-222222222222"}`),
					ContentType: "application/json", CallerSource: "user=test",
				}
				var result lifecycle.SealedResult
				if command == "add" {
					prepared, err := binding.prepare(t.Context(), input, nil, lifecycle.ResourceTransactionScope{ID: "vnode:" + name}, lifecycle.LongLivedPermit{})
					require.NoError(t, err)
					applied, err := prepared.Apply(t.Context())
					require.NoError(t, err)
					result = applied.Result()
				} else {
					var err error
					result, err = binding.handle(t.Context(), input)
					require.NoError(t, err)
				}
				var wire bytes.Buffer
				frames, err := lifecycle.NewFrameOwner(&wire)
				require.NoError(t, err)
				frame, err := lifecycle.PrepareFrame("result", result, 1)
				require.NoError(t, err)
				require.NoError(t, frames.Commit(frame))
				require.Contains(t, wire.String(), "FUNCTION_RESULT_BEGIN result 400 ")
				require.Equal(t, before, configured.Entries())
			})
		}
	}
}

func TestProcessVNodeNameRoundTrip(t *testing.T) {
	for name, test := range map[string]struct {
		name      string
		incumbent string
		reject    bool
	}{
		"colon":              {name: "db:one", incumbent: "db_one", reject: true},
		"inner NBSP":         {name: "db\u00a0one", incumbent: "db", reject: true},
		"trailing NBSP":      {name: "db\u00a0", incumbent: "db", reject: true},
		"Unicode space":      {name: "db\u2003", incumbent: "db", reject: true},
		"literal hex":        {name: `d\x62`, incumbent: "db", reject: true},
		"literal Unicode":    {name: `d\u0062`, incumbent: "db", reject: true},
		"literal newline":    {name: `db\n`, incumbent: "db", reject: true},
		"trailing backslash": {name: `db\`, incumbent: "db", reject: true},
		"dot":                {name: "db.one", incumbent: "db_one"},
		"underscore":         {name: "db_one", incumbent: "db_one"},
		"hyphen":             {name: "db-one", incumbent: "db_one"},
		"plain":              {name: "vnode", incumbent: "db_one"},
		"plus":               {name: "db+one", incumbent: "db_one"},
		"safe Unicode":       {name: "db-α", incumbent: "db_one"},
	} {
		t.Run(name, func(t *testing.T) {
			reader, writer := io.Pipe()
			output := newProcessSynchronizedBuffer()
			jobs := testRunJobServices(t)
			secretConfig := testRunSecrets(t)
			jobs.InitialVnodes = map[string]*vnodes.Config{
				test.incumbent: {VirtualNode: vnodes.VirtualNode{Name: test.incumbent, Hostname: "original", GUID: testVNodeGUID, Source: "file=test", SourceType: confgroup.TypeUser}},
			}
			process, err := newProcessCore(processCoreConfig{
				Secrets: secretConfig,
				Input:   reader, Output: output, ShutdownTimeout: time.Second, Modules: collectorapi.Registry{},
				Jobs: jobs, Discovery: testRunDiscoveryServices(t), Diagnostics: testProcessDiagnostics(),
			})
			require.NoError(t, err)
			controls := newTestProcessControls(1)
			done := make(chan error, 1)
			go func() { done <- process.run(context.Background(), controls) }()
			t.Cleanup(func() {
				// Close stdin only after Run returns: EOF without QUIT is a terminal
				// input failure and would race the requested termination.
				defer func() { require.NoError(t, writer.Close()) }()
				controls.sendTerminate(testProcessControl())
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(3 * time.Second):
					t.Fatal("process did not terminate")
				}
			})
			output.waitContains(t, "CONFIG go.d:vnode:"+test.incumbent+" create")
			_, err = fmt.Fprintf(writer, "FUNCTION_PAYLOAD add 30 \"config go.d:vnode add %s\" 0xFFFF \"user=test\" application/json\n"+
				"{\"hostname\":\"replacement\",\"guid\":\"22222222-2222-2222-2222-222222222222\"}\nFUNCTION_PAYLOAD_END\n", test.name)
			require.NoError(t, err)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN add ")
			if test.reject {
				require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN add 400 ")
				_, err = io.WriteString(writer, "FUNCTION get 30 \"config go.d:vnode:"+test.incumbent+" get\" 0xFFFF \"user=test\"\n")
				require.NoError(t, err)
				output.waitContains(t, "FUNCTION_RESULT_BEGIN get 200 ")
				require.Contains(t, output.String(), `"hostname":"original"`)
				require.NotContains(t, output.String(), "create running job /collectors/go.d/Vnodes dyncfg")
			} else {
				output.waitContains(t, "CONFIG go.d:vnode:"+test.name+" create running job /collectors/go.d/Vnodes dyncfg")
				_, err = fmt.Fprintf(writer, "FUNCTION get 30 \"config go.d:vnode:%s get\" 0xFFFF \"user=test\"\n", test.name)
				require.NoError(t, err)
				output.waitContains(t, "FUNCTION_RESULT_BEGIN get 200 ")
				require.Contains(t, output.String(), `"name":"`+test.name+`","hostname":"replacement"`)
			}
		})
	}
}

func TestVNodeUpdatePreflightChecksLifetime(t *testing.T) {
	for _, mutation := range []string{"remove", "replace", "replace identical", "update"} {
		t.Run(mutation, func(t *testing.T) {
			var inits, checks atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			modules := collectorapi.Registry{"module": {
				CreateV2: func() collectorapi.CollectorV2 {
					c := &handoffCollector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						inits:                         &inits, checks: &checks,
					}
					c.check = func() error {
						if checks.Load() == 2 {
							close(entered)
							<-release
						}
						return nil
					}
					return c
				},
				Config: func() any { return &collectorapi.MockConfiguration{} },
			}}
			cfg := confgroup.Config{"module": "module", "name": "receiver", "update_every": 1}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			output := newProcessSynchronizedBuffer()
			frames, err := lifecycle.NewFrameOwner(output)
			require.NoError(t, err)
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:    testRunSecrets(t),
				Generation: 1, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
				Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				generation.Stop()
				require.NoError(t, generation.Wait(context.Background()))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			require.Eventually(t, func() bool {
				r, ok := generation.vnodes.graph.Lookup(cfg.FullName())
				return ok && r.Status == dyncfg.StatusRunning.String()
			}, time.Second, time.Millisecond)
			before, _ := generation.vnodes.graph.Lookup(cfg.FullName())
			submit := func(uid string, args []string, payload string) {
				t.Helper()
				require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
					UID: uid, Source: lifecycle.SourceFunction, Route: "config", Args: args, CallerSource: "user=test",
					Deadline: time.Now().Add(10 * time.Second), HasPayload: payload != "", Payload: []byte(payload), ContentType: "application/json",
				}))
			}
			const original = `{"guid":"33333333-3333-3333-3333-333333333333"}`
			submit("add", []string{"go.d:vnode", "add", "x"}, original)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN add 202 ")
			submit("edit", []string{"go.d:collector:module:receiver", "update"}, `{"update_every":1,"vnode":"x"}`)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("update Check did not start")
			}
			if mutation == "update" {
				submit("vnode-edit", []string{"go.d:vnode:x", "update"}, `{"hostname":"changed","guid":"44444444-4444-4444-4444-444444444444"}`)
				output.waitContains(t, "FUNCTION_RESULT_BEGIN vnode-edit 202 ")
			} else {
				submit("remove", []string{"go.d:vnode:x", "remove"}, "")
				output.waitContains(t, "FUNCTION_RESULT_BEGIN remove 200 ")
				_, exists := generation.vnodeConfig.Lookup("x")
				require.False(t, exists)
				if mutation != "remove" {
					payload := original
					if mutation == "replace" {
						payload = `{"guid":"44444444-4444-4444-4444-444444444444"}`
					}
					submit("readd", []string{"go.d:vnode", "add", "x"}, payload)
					output.waitContains(t, "FUNCTION_RESULT_BEGIN readd 202 ")
				}
			}
			releaseOnce.Do(func() { close(release) })
			output.waitContains(t, "FUNCTION_RESULT_BEGIN edit ")
			if mutation == "update" {
				require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN edit 202 ")
				require.Eventually(t, func() bool {
					r, _ := generation.vnodes.graph.Lookup(cfg.FullName())
					return r.Status == dyncfg.StatusRunning.String() && r.Payload() != before.Payload()
				}, time.Second, time.Millisecond)
			} else {
				require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN edit 503 ")
				after, _ := generation.vnodes.graph.Lookup(cfg.FullName())
				require.Equal(t, before, after, "rejected preflight must preserve the incumbent")
			}
			require.EqualValues(t, 2, checks.Load(), "ordinary vnode updates must not cause another preflight")
		})
	}
}

func TestDiscoveredJobRebuildsAfterVNodeRemovalDuringPreflight(t *testing.T) {
	for _, readdBeforeAdoption := range []bool{false, true} {
		t.Run(fmt.Sprintf("readd-before-adoption=%t", readdBeforeAdoption), func(t *testing.T) {
			var inits, checks atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			modules := collectorapi.Registry{"module": {
				CreateV2: func() collectorapi.CollectorV2 {
					return &handoffCollector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						inits:                         &inits, checks: &checks,
						check: func() error {
							if checks.Load() == 1 {
								close(entered)
								<-release
							}
							return nil
						},
					}
				},
				Config: func() any { return &collectorapi.MockConfiguration{} },
			}}
			cfg := confgroup.Config{"module": "module", "name": "receiver", "vnode": "x", "update_every": 1, "autodetection_retry": 0}.
				SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			jobs := testRunJobServices(t)
			secretConfig := testRunSecrets(t)
			jobs.InitialVnodes = map[string]*vnodes.Config{
				"x": {VirtualNode: vnodes.VirtualNode{Name: "x", Hostname: "x", GUID: testVNodeGUID, Source: "user=test", SourceType: confgroup.TypeDyncfg}},
			}
			output := newProcessSynchronizedBuffer()
			frames, err := lifecycle.NewFrameOwner(output)
			require.NoError(t, err)
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:    secretConfig,
				Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames,
				Modules: modules, Jobs: jobs, Discovery: testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				generation.Stop()
				require.NoError(t, generation.Wait(context.Background()))
			})
			require.NoError(t, generation.start(context.Background()))
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("discovered Check did not start")
			}
			require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
				UID: "remove", Source: lifecycle.SourceFunction, Route: "config", Args: []string{"go.d:vnode:x", "remove"},
			}))
			output.waitContains(t, "FUNCTION_RESULT_BEGIN remove 200 ")
			readd := func() {
				t.Helper()
				require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
					UID: "readd", Source: lifecycle.SourceFunction, Route: "config", Args: []string{"go.d:vnode", "add", "x"},
					HasPayload: true, Payload: []byte(`{"hostname":"x","guid":"` + testVNodeGUID + `"}`), ContentType: "application/json", CallerSource: "user=test",
				}))
				output.waitContains(t, "FUNCTION_RESULT_BEGIN readd 202 ")
			}
			if readdBeforeAdoption {
				readd()
			}
			releaseOnce.Do(func() { close(release) })
			if !readdBeforeAdoption {
				require.Eventually(t, func() bool {
					r, _ := generation.vnodes.graph.Lookup(cfg.FullName())
					return r.Status == dyncfg.StatusAccepted.String()
				}, time.Second, time.Millisecond)
				require.EqualValues(t, 1, checks.Load())
				readd()
			}
			require.Eventually(t, func() bool {
				r, _ := generation.vnodes.graph.Lookup(cfg.FullName())
				return r.Status == dyncfg.StatusRunning.String()
			}, time.Second, time.Millisecond)
			require.EqualValues(t, 2, checks.Load(), "discovery must discard the removed vnode's candidate and probe the recreated record")
		})
	}
}

func TestVNodeArrivalStartsOnlyEnabledJobsWithoutOperationalRetry(t *testing.T) {
	for _, mode := range []string{"static", "snmp"} {
		for _, intent := range []string{"enabled", "passive", "disabled"} {
			t.Run(mode+"/"+intent, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					var inits, checks atomic.Int32
					modules := collectorapi.Registry{"module": {
						CreateV2: func() collectorapi.CollectorV2 {
							return &handoffCollector{
								readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
								inits:                         &inits, checks: &checks,
							}
						},
						Config: func() any { return &collectorapi.MockConfiguration{} },
					}}
					cfg := confgroup.Config{"module": "module", "name": "receiver", "vnode": "x", "update_every": 1, "autodetection_retry": 0}.
						SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
					jobs := testRunJobServices(t)
					secretConfig := testRunSecrets(t)
					acquirer := controlledAcquirer{attempts: make(chan acquisitionAttempt, 4)}
					jobs.SNMPVnodeAcquirer = acquirer
					output := newProcessSynchronizedBuffer()
					frames, err := lifecycle.NewFrameOwner(output)
					require.NoError(t, err)
					generation, err := newTestRunGeneration(t, runGenerationConfig{
						Secrets:    secretConfig,
						Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames,
						Modules: modules, Jobs: jobs, Discovery: testRunDiscoveryServicesAccepted(t, cfg),
					})
					require.NoError(t, err)
					t.Cleanup(func() {
						generation.Stop()
						require.NoError(t, generation.Wait(context.Background()))
						require.NoError(t, generation.run.FinishShutdown())
					})
					require.NoError(t, generation.start(context.Background()))
					synctest.Wait()
					status := func() string {
						r, _ := generation.vnodes.graph.Lookup(cfg.FullName())
						return r.Status
					}
					require.Equal(t, dyncfg.StatusAccepted.String(), status())
					submit := func(uid string, args []string, payload string, code int) {
						t.Helper()
						require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
							UID: uid, Source: lifecycle.SourceFunction, Route: "config", Args: args,
							HasPayload: payload != "", Payload: []byte(payload), ContentType: "application/json", CallerSource: "user=test",
						}))
						synctest.Wait()
						require.Contains(t, output.String(), fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, code))
					}
					if intent != "passive" {
						submit("enable", []string{"go.d:collector:module:receiver", "enable"}, "", 202)
					}
					if intent == "disabled" {
						submit("disable", []string{"go.d:collector:module:receiver", "disable"}, "", 200)
					}
					require.Zero(t, checks.Load())
					payload := `{"guid":"` + testVNodeGUID + `"}`
					if mode == "snmp" {
						payload = `{"mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"fixture"}}}`
					}
					submit("vnode-add", []string{"go.d:vnode", "add", "x"}, payload, 202)
					if mode == "snmp" {
						require.Zero(t, checks.Load(), "pending identity cannot start a collector")
						attempt := nextAcquisition(t, acquirer)
						submit("pending-remove", []string{"go.d:vnode:x", "remove"}, "", 409)
						select {
						case <-attempt.canceled:
							t.Fatal("rejected removal canceled identity acquisition")
						default:
						}
						attempt.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "acquired"}}
						synctest.Wait()
					}
					if intent == "enabled" {
						require.Equal(t, dyncfg.StatusRunning.String(), status())
						require.EqualValues(t, 1, checks.Load())
					} else {
						want := dyncfg.StatusAccepted
						if intent == "disabled" {
							want = dyncfg.StatusDisabled
						}
						require.Equal(t, want.String(), status())
						require.Zero(t, checks.Load())
					}
					submit("referenced-remove", []string{"go.d:vnode:x", "remove"}, "", 409)
				})
			})
		}
	}
}

func TestVNodeUserConfigPreservesNamesAndFormattingFallback(t *testing.T) {
	for _, name := range []string{"absent", "", "db.one", "db_one", "db-one", "vnode", "db+one", "db-α"} {
		t.Run(name, func(t *testing.T) {
			binding, configured := newTestVNodeBinding(t, confgroup.TypeUser, nil)
			before := configured.Entries()
			args := []string{"go.d:vnode", "userconfig"}
			if name != "absent" {
				args = append(args, name)
			}
			result, err := binding.handle(t.Context(), functionadapter.HandlerInput{
				Args: args, HasPayload: true, ContentType: "application/json",
				Payload: []byte(`{"labels":{"site":"west"}}`),
			})
			require.NoError(t, err)
			var wire bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&wire)
			require.NoError(t, err)
			frame, err := lifecycle.PrepareFrame("format", result, 1)
			require.NoError(t, err)
			require.NoError(t, frames.Commit(frame))
			want := name
			if name == "absent" || name == "" {
				want = "test"
			}
			require.Contains(t, wire.String(), "FUNCTION_RESULT_BEGIN format 200 application/yaml ")
			require.Contains(t, wire.String(), "name: "+want+"\n")
			require.Contains(t, wire.String(), "site: west\n")
			require.Equal(t, before, configured.Entries(), "formatting must not adopt even an incomplete configuration")
		})
	}
}
