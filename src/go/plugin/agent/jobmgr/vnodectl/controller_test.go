// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginName = "test"

func TestControllerSeqExec(t *testing.T) {
	tests := map[string]struct {
		initial map[string]*vnodes.VirtualNode
		run     func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams)
	}{
		"schema dispatch": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{UID: "vn-schema", Args: []string{ctl.Prefix(), string(dyncfg.CommandSchema)}})
				ctl.SeqExec(fn)

				assert.Contains(t, out.String(), "FUNCTION_RESULT_BEGIN vn-schema 200 application/json")
				assert.Empty(t, seams.affectedJobsCalls)
				assert.Empty(t, seams.applyCalls)
			},
		},
		"userconfig generation": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-userconfig",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid":   "11111111-1111-1111-1111-111111111111",
						"labels": map[string]string{"env": "prod"},
					}),
					Args: []string{ctl.Prefix(), string(dyncfg.CommandUserconfig), "db"},
				})
				ctl.SeqExec(fn)

				body := mustFunctionBody(t, out.String(), "vn-userconfig")
				assert.Contains(t, out.String(), "FUNCTION_RESULT_BEGIN vn-userconfig 200 application/yaml")
				assert.Contains(t, body, "name: db")
				assert.Contains(t, body, "hostname: db")
				assert.Empty(t, seams.affectedJobsCalls)
				assert.Empty(t, seams.applyCalls)
			},
		},
		"get returns stored config": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{UID: "vn-get", Args: []string{ctl.configID("db"), string(dyncfg.CommandGet)}})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-get", &payload)
				assert.Equal(t, "db", payload["name"])
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", payload["guid"])
				assert.Empty(t, seams.affectedJobsCalls)
				assert.Empty(t, seams.applyCalls)
			},
		},
		"add applies vnode update seam": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-add",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid":   "11111111-1111-1111-1111-111111111111",
						"labels": map[string]string{"env": "prod"},
					}),
					Args: []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-add", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Equal(t, []string{"db"}, seams.applyCalls)
				cfg, ok := ctl.Lookup("db")
				require.True(t, ok)
				assert.Equal(t, confgroup.TypeDyncfg, cfg.SourceType)
			},
		},
		"add no-op keeps apply seam unused": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-add-noop",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-add-noop", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Empty(t, seams.applyCalls)
			},
		},
		"add equal user vnode rewrites stored source metadata to dyncfg": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-add-promote-user",
					Source:      "user=alice",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-add-promote-user", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Equal(t, []string{"db"}, seams.applyCalls)

				cfg, ok := ctl.Lookup("db")
				require.True(t, ok)
				assert.Equal(t, confgroup.TypeDyncfg, cfg.SourceType)
				assert.Equal(t, "user=alice", cfg.Source)
			},
		},
		"duplicate hostname is rejected": {
			initial: map[string]*vnodes.VirtualNode{
				"other": testVnode("other", "shared", "22222222-2222-2222-2222-222222222222", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-add-dup-host",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid":     "33333333-3333-3333-3333-333333333333",
						"hostname": "shared",
					}),
					Args: []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-add-dup-host", &payload)
				assert.Equal(t, float64(400), payload["status"])
				assert.Contains(t, fmt.Sprint(payload["errorMessage"]), "duplicate virtual node hostname")
				assert.Empty(t, seams.applyCalls)
			},
		},
		"update applies vnode update seam": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-update",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid":   "11111111-1111-1111-1111-111111111111",
						"labels": map[string]string{"team": "db"},
					}),
					Args: []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-update", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Equal(t, []string{"db"}, seams.applyCalls)
			},
		},
		"update no-op keeps apply seam unused": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-update-noop",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-update-noop", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Empty(t, seams.applyCalls)
			},
		},
		"update equal user vnode rewrites stored source metadata to dyncfg": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-update-promote-user",
					Source:      "user=alice",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-update-promote-user", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Equal(t, []string{"db"}, seams.applyCalls)

				cfg, ok := ctl.Lookup("db")
				require.True(t, ok)
				assert.Equal(t, confgroup.TypeDyncfg, cfg.SourceType)
				assert.Equal(t, "user=alice", cfg.Source)
			},
		},
		"update rejects duplicate hostname": {
			initial: map[string]*vnodes.VirtualNode{
				"db":    testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
				"other": testVnode("other", "shared", "22222222-2222-2222-2222-222222222222", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-update-dup-host",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid":     "11111111-1111-1111-1111-111111111111",
						"hostname": "shared",
					}),
					Args: []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-update-dup-host", &payload)
				assert.Equal(t, float64(400), payload["status"])
				assert.Contains(t, fmt.Sprint(payload["errorMessage"]), "duplicate virtual node hostname")
				assert.Empty(t, seams.applyCalls)
			},
		},
		"update rejects duplicate guid": {
			initial: map[string]*vnodes.VirtualNode{
				"db":    testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
				"other": testVnode("other", "other", "22222222-2222-2222-2222-222222222222", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-update-dup-guid",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"guid": "22222222-2222-2222-2222-222222222222",
					}),
					Args: []string{ctl.configID("db"), string(dyncfg.CommandUpdate)},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-update-dup-guid", &payload)
				assert.Equal(t, float64(400), payload["status"])
				assert.Contains(t, fmt.Sprint(payload["errorMessage"]), "duplicate virtual node guid")
				assert.Empty(t, seams.applyCalls)
			},
		},
		"invalid guid is rejected": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-invalid-guid",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "bad-guid"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-invalid-guid", &payload)
				assert.Equal(t, float64(400), payload["status"])
				assert.Empty(t, seams.applyCalls)
			},
		},
		"empty vnode name is rejected": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-empty-name",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandAdd), ""},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-empty-name", &payload)
				assert.Equal(t, float64(400), payload["status"])
				assert.Contains(t, fmt.Sprint(payload["errorMessage"]), "Missing vnode name")
				assert.Empty(t, seams.applyCalls)
			},
		},
		"remove rejects non dyncfg source": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeUser),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{UID: "vn-remove-user", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-remove-user", &payload)
				assert.Equal(t, float64(405), payload["status"])
				assert.Empty(t, seams.affectedJobsCalls)
			},
		},
		"remove uses affected jobs seam": {
			initial: map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			},
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				seams.affectedJobs["db"] = []string{"mysql:prod"}

				fn := dyncfg.NewFunction(functions.Function{UID: "vn-remove-running", Args: []string{ctl.configID("db"), string(dyncfg.CommandRemove)}})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-remove-running", &payload)
				assert.Equal(t, float64(409), payload["status"])
				assert.Contains(t, fmt.Sprint(payload["errorMessage"]), "referenced by configs")
				assert.Equal(t, []string{"db"}, seams.affectedJobsCalls)
				_, ok := ctl.Lookup("db")
				assert.True(t, ok)
			},
		},
		"test preview uses affected jobs seam": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				seams.affectedJobs["db"] = []string{"mysql:prod"}

				fn := dyncfg.NewFunction(functions.Function{
					UID:         "vn-test",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"guid": "11111111-1111-1111-1111-111111111111"}),
					Args:        []string{ctl.Prefix(), string(dyncfg.CommandTest), "db"},
				})
				ctl.SeqExec(fn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "vn-test", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Equal(t, "Updated configuration will affect configs: mysql:prod.", payload["message"])
				assert.Equal(t, []string{"db"}, seams.affectedJobsCalls)
				assert.Empty(t, seams.applyCalls)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, seams := newControllerTestSubject(tc.initial)
			tc.run(t, ctl, out, seams)
		})
	}
}

func TestControllerPublicationAndLookup(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, ctl *Controller, out *bytes.Buffer)
	}{
		"create module and publish existing": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer) {
				ctl.CreateTemplates()
				ctl.PublishExisting(dyncfg.StatusRunning)

				assert.Contains(t, out.String(), "CONFIG test:vnode create accepted template /collectors/test/Vnodes")
				assert.Contains(t, out.String(), "CONFIG test:vnode:db create running job /collectors/test/Vnodes")

				cfg, ok := ctl.Lookup("db")
				require.True(t, ok)
				assert.Equal(t, "db", cfg.Name)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, _ := newControllerTestSubject(map[string]*vnodes.VirtualNode{
				"db": testVnode("db", "db", "11111111-1111-1111-1111-111111111111", confgroup.TypeDyncfg),
			})
			tc.run(t, ctl, out)
		})
	}
}

func TestControllerSetAPI_NilPreservesResponder(t *testing.T) {
	tests := map[string]struct {
		uid string
	}{
		"nil SetAPI keeps existing responder for schema responses": {
			uid: "vn-schema-nil-rebind",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, _ := newControllerTestSubject(nil)
			ctl.SetAPI(nil)

			fn := dyncfg.NewFunction(functions.Function{
				UID:  tc.uid,
				Args: []string{ctl.Prefix(), string(dyncfg.CommandSchema)},
			})
			ctl.SeqExec(fn)

			assert.Contains(t, out.String(), "FUNCTION_RESULT_BEGIN "+tc.uid+" 200 application/json")
		})
	}
}

type controllerSeams struct {
	affectedJobs      map[string][]string
	affectedJobsCalls []string
	applyCalls        []string
}

func newControllerTestSubject(initial map[string]*vnodes.VirtualNode) (*Controller, *bytes.Buffer, *controllerSeams) {
	var out bytes.Buffer
	seams := &controllerSeams{affectedJobs: make(map[string][]string)}
	ctl := New(Options{
		Logger:  logger.New(),
		API:     dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))),
		Plugin:  testPluginName,
		Initial: initial,
		AffectedJobs: func(vnode string) []string {
			seams.affectedJobsCalls = append(seams.affectedJobsCalls, vnode)
			return seams.affectedJobs[vnode]
		},
		ApplyVnodeUpdate: func(name string, _ *vnodes.VirtualNode) {
			seams.applyCalls = append(seams.applyCalls, name)
		},
	})
	return ctl, &out, seams
}

func testVnode(name, hostname, guid, sourceType string) *vnodes.VirtualNode {
	return &vnodes.VirtualNode{Name: name, Hostname: hostname, GUID: guid, Source: sourceType, SourceType: sourceType}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	bs, err := json.Marshal(v)
	require.NoError(t, err)
	return bs
}

func mustFunctionBody(t *testing.T, output, uid string) string {
	t.Helper()
	re := regexp.MustCompile("(?s)FUNCTION_RESULT_BEGIN " + regexp.QuoteMeta(uid) + " [^\\n]+\\n(.*?)\\nFUNCTION_RESULT_END")
	match := re.FindStringSubmatch(output)
	require.Len(t, match, 2, "function result for uid '%s' not found in output:\n%s", uid, output)
	return match[1]
}

func mustDecodeFunctionPayload(t *testing.T, output, uid string, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(mustFunctionBody(t, output, uid)), dst))
}
