// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
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

func TestExecutor_DispatchOrder(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, buf *bytes.Buffer)
	}{
		"dyncfg commands emit terminals in dispatch order": {
			run: func(t *testing.T, mgr *Manager, buf *bytes.Buffer) {
				var wants []wireRecordWant
				for i := 1; i <= 3; i++ {
					uid := fmt.Sprintf("order-%d", i)
					fn := newExecutorTestFn(uid, []string{fmt.Sprintf("test:collector:success:missing%d", i), "get"}, nil)
					mgr.executor.dispatch(mgr.newDyncfgEvent(fn))
					wants = append(wants, wireRecordWant{
						name:     uid + " terminal",
						contains: []string{"FUNCTION_RESULT_BEGIN " + uid},
					})
				}

				requireWireRecordSubsequence(t, buf.String(), wants)
			},
		},
		"discovery and dyncfg events execute in dispatch order": {
			run: func(t *testing.T, mgr *Manager, buf *bytes.Buffer) {
				cfg := prepareUserCfg("success", "o1")

				mgr.executor.dispatch(mgr.newDiscoveryAddEvent(cfg))
				mgr.executor.dispatch(mgr.newDyncfgEvent(newExecutorTestFn("mid-get", []string{mgr.dyncfgJobID(cfg), "get"}, nil)))
				mgr.executor.dispatch(mgr.newDiscoveryRemoveEvent(cfg))

				requireWireRecordSubsequence(t, buf.String(), []wireRecordWant{
					{
						name:     "discovery add config create",
						contains: []string{"CONFIG test:collector:success:o1 create"},
					},
					{
						name:     "get terminal",
						contains: []string{"FUNCTION_RESULT_BEGIN mid-get"},
					},
					{
						name:     "discovery remove config delete",
						contains: []string{"CONFIG test:collector:success:o1 delete"},
					},
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, buf := newExecutorTestManager()
			tc.run(t, mgr, buf)
		})
	}
}

// Underivable events take the fallback key and must still execute, reaching
// each domain's existing rejection (or template-scope success) paths.
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
