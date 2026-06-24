// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestModuleFuncRegistry_RegisterModule(t *testing.T) {
	tests := map[string]struct {
		modules  []string
		expected []string
	}{
		"single module": {
			modules:  []string{"postgres"},
			expected: []string{"postgres"},
		},
		"multiple modules": {
			modules:  []string{"postgres", "mysql", "mssql"},
			expected: []string{"mysql", "mssql", "postgres"},
		},
		"duplicate registration overwrites": {
			modules:  []string{"postgres", "postgres"},
			expected: []string{"postgres"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := newModuleFuncRegistry()

			for _, module := range tc.modules {
				r.registerModule(module, collectorapi.Creator{
					Methods: func() []funcapi.MethodConfig {
						return []funcapi.MethodConfig{{ID: "test"}}
					},
				})
			}

			assert.Equal(t, len(tc.expected), len(r.modules))
			for _, module := range tc.expected {
				assert.True(t, r.isModuleRegistered(module))
			}
		})
	}
}

func TestModuleFuncRegistry_Operations(t *testing.T) {
	tests := map[string]struct{}{
		"add remove job":                                                    {},
		"job replacement increments generation":                             {},
		"job re-add after removal increments generation":                    {},
		"generation verification fails on wrong generation and missing job": {},
		"get methods":          {},
		"get job names sorted": {},
		"operations on unregistered module are no op": {},
		"get creator": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			r := newModuleFuncRegistry()

			switch name {
			case "add remove job":
				r.registerModule("postgres", collectorapi.Creator{})

				job1 := newTestRuntimeJob("postgres", "job1", true)
				job2 := newTestRuntimeJob("postgres", "job2", true)

				r.addJob("postgres", "job1", job1)
				r.addJob("postgres", "job2", job2)

				names := r.getJobNames("postgres")
				assert.ElementsMatch(t, []string{"job1", "job2"}, names)

				got1, ok := r.getJob("postgres", "job1")
				assert.True(t, ok)
				assert.Equal(t, job1, got1)

				r.removeJob("postgres", "job1")

				names = r.getJobNames("postgres")
				assert.ElementsMatch(t, []string{"job2"}, names)

				_, ok = r.getJob("postgres", "job1")
				assert.False(t, ok)

			case "job replacement increments generation":
				r.registerModule("postgres", collectorapi.Creator{})

				job1 := newTestRuntimeJob("postgres", "master", true)
				job2 := newTestRuntimeJob("postgres", "master", true)

				r.addJob("postgres", "master", job1)
				_, gen1 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(1), gen1)

				r.addJob("postgres", "master", job2)
				got, gen2 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(2), gen2)
				assert.Equal(t, job2, got)

			case "job re-add after removal increments generation":
				r.registerModule("postgres", collectorapi.Creator{})

				job1 := newTestRuntimeJob("postgres", "master", true)
				job2 := newTestRuntimeJob("postgres", "master", true)

				r.addJob("postgres", "master", job1)
				_, gen1 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(1), gen1)

				r.removeJob("postgres", "master")
				r.addJob("postgres", "master", job2)
				got, gen2 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(2), gen2)
				assert.Equal(t, job2, got)

			case "generation verification fails on wrong generation and missing job":
				r.registerModule("postgres", collectorapi.Creator{})

				job := newTestRuntimeJob("postgres", "master", true)
				r.addJob("postgres", "master", job)
				_, gen := r.getJobWithGeneration("postgres", "master")

				assert.False(t, r.verifyJobGeneration("postgres", "master", gen+1))
				r.removeJob("postgres", "master")
				assert.False(t, r.verifyJobGeneration("postgres", "master", gen))

			case "get methods":
				expectedMethods := []funcapi.MethodConfig{{ID: "top-queries", Name: "Top Queries"}}
				r.registerModule("postgres", collectorapi.Creator{
					Methods: func() []funcapi.MethodConfig { return expectedMethods },
				})

				assert.Equal(t, expectedMethods, r.getMethods("postgres"))
				assert.Nil(t, r.getMethods("nonexistent"))

			case "get job names sorted":
				r.registerModule("postgres", collectorapi.Creator{})

				r.addJob("postgres", "zebra-db", newTestRuntimeJob("postgres", "zebra-db", true))
				r.addJob("postgres", "alpha-db", newTestRuntimeJob("postgres", "alpha-db", true))
				r.addJob("postgres", "middle-db", newTestRuntimeJob("postgres", "middle-db", true))

				assert.Equal(t, []string{"alpha-db", "middle-db", "zebra-db"}, r.getJobNames("postgres"))

			case "operations on unregistered module are no op":
				r.addJob("nonexistent", "job1", newTestRuntimeJob("nonexistent", "job1", true))
				r.removeJob("nonexistent", "job1")

				assert.False(t, r.isModuleRegistered("nonexistent"))
				assert.Nil(t, r.getJobNames("nonexistent"))
				assert.Nil(t, r.getMethods("nonexistent"))

				_, ok := r.getJob("nonexistent", "job1")
				assert.False(t, ok)

			case "get creator":
				creator := collectorapi.Creator{JobConfigSchema: "test-schema"}
				r.registerModule("postgres", creator)

				got, ok := r.getCreator("postgres")
				require.True(t, ok)
				assert.Equal(t, "test-schema", got.JobConfigSchema)

				_, ok = r.getCreator("nonexistent")
				assert.False(t, ok)
			}
		})
	}
}

func TestModuleFuncRegistry_Concurrency(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "test"}}
		},
	})

	done := make(chan bool)

	go func() {
		for range 100 {
			job := newTestRuntimeJob("postgres", "job", true)
			r.addJob("postgres", "job", job)
			r.removeJob("postgres", "job")
		}
		done <- true
	}()

	go func() {
		for range 100 {
			_ = r.getJobNames("postgres")
			_ = r.getMethods("postgres")
			_, _ = r.getJob("postgres", "job")
		}
		done <- true
	}()

	<-done
	<-done
}

func TestModuleFuncRegistry_MethodRouteCollisionUsesDeterministicOwner(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModuleWithMethods("bbb", collectorapi.Creator{}, []funcapi.MethodConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})
	r.registerModuleWithMethods("aaa", collectorapi.Creator{}, []funcapi.MethodConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})

	moduleName, methodID, ok := r.resolveMethodRoute("shared:logs")
	require.True(t, ok)
	assert.Equal(t, "aaa", moduleName)
	assert.Equal(t, "logs", methodID)
}

func TestModuleFuncRegistry_VerifyJobGeneration_JobStopped(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	job := newTestRuntimeJob("postgres", "master", false)
	r.addJob("postgres", "master", job)
	_, gen := r.getJobWithGeneration("postgres", "master")

	assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
}

func TestDispatchHelpers(t *testing.T) {
	tests := map[string]struct{}{
		"extract param values": {},
		"build params":         {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			switch name {
			case "extract param values":
				cases := map[string]struct {
					payload  map[string]any
					key      string
					expected []string
				}{
					"string value": {
						payload:  map[string]any{"__job": "local"},
						key:      "__job",
						expected: []string{"local"},
					},
					"array value": {
						payload:  map[string]any{"__sort": []any{"calls", "total_time"}},
						key:      "__sort",
						expected: []string{"calls", "total_time"},
					},
					"string array": {
						payload:  map[string]any{"__job": []string{"local"}},
						key:      "__job",
						expected: []string{"local"},
					},
					"missing key": {
						payload:  map[string]any{"__job": "local"},
						key:      "__sort",
						expected: nil,
					},
					"prefers selections": {
						payload: map[string]any{
							"__job": "root",
							"selections": map[string]any{
								"__job": []any{"selected"},
							},
						},
						key:      "__job",
						expected: []string{"selected"},
					},
				}

				for caseName, tc := range cases {
					t.Run(caseName, func(t *testing.T) {
						assert.Equal(t, tc.expected, extractParamValues(tc.payload, tc.key))
					})
				}

			case "build params":
				cases := map[string]struct{}{
					"build accepted params":                        {},
					"build required params uses select type":       {},
					"build required params agent-wide omits __job": {},
				}

				for caseName := range cases {
					t.Run(caseName, func(t *testing.T) {
						switch caseName {
						case "build accepted params":
							sortDir := funcapi.FieldSortDescending
							methodParams := []funcapi.ParamConfig{
								{ID: "__sort", Selection: funcapi.ParamSelect, Options: []funcapi.ParamOption{{ID: "calls", Name: "Calls", Sort: &sortDir}}},
								{ID: "db"},
								{ID: "extra"},
							}

							assert.Equal(t, []string{"__job", "__sort", "db", "extra"}, buildAcceptedParams(methodParams, true))
							assert.Equal(t, []string{"__sort", "db", "extra"}, buildAcceptedParams(methodParams, false))

						case "build required params uses select type":
							controller := New(Options{})
							controller.RegisterModules(collectorapi.Registry{
								"postgres": collectorapi.Creator{
									Methods: func() []funcapi.MethodConfig {
										return []funcapi.MethodConfig{{ID: "top-queries", Name: "Top Queries"}}
									},
								},
							})
							controller.registry.addJob("postgres", "master-db", newTestRuntimeJob("postgres", "master-db", true))

							methodParams := []funcapi.ParamConfig{{
								ID:         "__sort",
								Name:       "Filter By",
								Selection:  funcapi.ParamSelect,
								UniqueView: true,
								Options: []funcapi.ParamOption{
									{ID: "total_time", Name: "By Total Time", Default: true},
								},
							}}
							params := controller.buildRequiredParams("postgres", methodParams, true)

							assert.Len(t, params, 2)
							for _, param := range params {
								paramType, ok := param["type"]
								assert.True(t, ok)
								assert.Equal(t, "select", paramType)
								assert.Contains(t, param, "id")
								assert.Contains(t, param, "name")
								assert.Contains(t, param, "options")
								assert.Contains(t, param, "unique_view")
								uniqueView, _ := param["unique_view"].(bool)
								assert.True(t, uniqueView)
							}

							assert.Equal(t, "__job", params[0]["id"])
							assert.Equal(t, "__sort", params[1]["id"])

						case "build required params agent-wide omits __job":
							controller := New(Options{})
							controller.RegisterModules(collectorapi.Registry{
								"snmp": collectorapi.Creator{
									Methods: func() []funcapi.MethodConfig {
										return []funcapi.MethodConfig{{ID: "topology:snmp", AgentWide: true}}
									},
								},
							})
							controller.registry.addJob("snmp", "router", newTestRuntimeJob("snmp", "router", true))

							methodParams := []funcapi.ParamConfig{{
								ID:        "topology_view",
								Name:      "Topology View",
								Selection: funcapi.ParamSelect,
								Options: []funcapi.ParamOption{
									{ID: "l2", Name: "L2", Default: true},
								},
							}}
							params := controller.buildRequiredParams("snmp", methodParams, false)

							assert.Len(t, params, 1)
							assert.Equal(t, "topology_view", params[0]["id"])
						}
					})
				}
			}
		})
	}
}

func TestParseArgsParams(t *testing.T) {
	args := []string{
		"__job:snmp-a",
		"view=detailed",
		"labels:src_ip,dst_ip",
		"info",
		"invalid",
		"empty:",
		"=novalue",
	}

	got := parseArgsParams(args)

	assert.Equal(t, []string{"snmp-a"}, got["__job"])
	assert.Equal(t, []string{"detailed"}, got["view"])
	assert.Equal(t, []string{"src_ip", "dst_ip"}, got["labels"])
	assert.NotContains(t, got, "invalid")
	assert.NotContains(t, got, "empty")
}

func TestControllerLifecycleHooks(t *testing.T) {
	tests := map[string]struct{}{
		"register modules does not register static methods yet":         {},
		"register modules registers available agent-wide methods":       {},
		"first job start registers static methods once":                 {},
		"availability-gated static method registers when available":     {},
		"availability-gated agent-wide method registers when available": {},
		"reconcile registers late available static method":              {},
		"reconcile ignores stopped job":                                 {},
		"reconcile does not duplicate published static method":          {},
		"public method name collision skips colliding module":           {},
		"rejected module does not poison planned public names":          {},
		"topology methods register direct alias":                        {},
		"job stop unregisters job methods":                              {},
		"cleanup unregisters static methods":                            {},
		"cleanup ignores unavailable static methods":                    {},
		"cleanup with api configured still unregisters static methods":  {},
		"api registration honors method tags":                           {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})

			switch name {
			case "register modules does not register static methods yet":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "a"}} },
					},
				})

				assert.Empty(t, reg.registeredNames())

			case "register modules registers available agent-wide methods":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{ID: "logs", AgentWide: true}}
						},
					},
				})

				assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())

			case "first job start registers static methods once":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "a"}} },
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))

				assert.Equal(t, []string{"mod:a"}, reg.registeredNames())

			case "availability-gated static method registers when available":
				available := false
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:        "logs",
								Available: func() bool { return available },
							}}
						},
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				assert.Empty(t, reg.registeredNames())

				available = true
				controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))
				controller.OnJobStart(newTestRuntimeJob("mod", "job3", true))

				assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())

			case "availability-gated agent-wide method registers when available":
				available := false
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:        "logs",
								AgentWide: true,
								Available: func() bool { return available },
							}}
						},
					},
				})

				assert.Empty(t, reg.registeredNames())

				available = true
				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))

				assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())

			case "reconcile registers late available static method":
				available := false
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:        "logs",
								Available: func() bool { return available },
							}}
						},
					},
				})

				job := newTestRuntimeJob("mod", "job1", true)
				controller.OnJobStart(job)
				assert.Empty(t, reg.registeredNames())

				available = true
				controller.ReconcileModuleMethodsForJob(job)

				assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())

			case "reconcile ignores stopped job":
				available := true
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:        "logs",
								Available: func() bool { return available },
							}}
						},
					},
				})

				controller.ReconcileModuleMethodsForJob(newTestRuntimeJob("mod", "job1", false))

				assert.Empty(t, reg.registeredNames())

			case "reconcile does not duplicate published static method":
				availableCalls := 0
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID: "logs",
								Available: func() bool {
									availableCalls++
									return true
								},
							}}
						},
					},
				})

				job := newTestRuntimeJob("mod", "job1", true)
				controller.OnJobStart(job)
				controller.ReconcileModuleMethodsForJob(job)
				controller.ReconcileModuleMethodsForJob(job)

				assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
				assert.Equal(t, 1, availableCalls)

			case "public method name collision skips colliding module":
				controller.RegisterModules(collectorapi.Registry{
					"aaa": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{ID: "logs", FunctionName: "shared:logs"}}
						},
					},
					"bbb": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{ID: "logs", FunctionName: "shared:logs"}}
						},
					},
				})

				controller.OnJobStart(newTestRuntimeJob("aaa", "job1", true))
				controller.OnJobStart(newTestRuntimeJob("bbb", "job1", true))

				assert.Equal(t, []string{"shared:logs"}, reg.registeredNames())
				assert.True(t, controller.registry.isModuleRegistered("aaa"))
				assert.False(t, controller.registry.isModuleRegistered("bbb"))

			case "rejected module does not poison planned public names":
				controller.RegisterModules(collectorapi.Registry{
					"aaa": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{
								{ID: "first", FunctionName: "later:logs"},
								{ID: "second", FunctionName: "later:logs"},
							}
						},
					},
					"ccc": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{ID: "logs", FunctionName: "later:logs"}}
						},
					},
				})

				controller.OnJobStart(newTestRuntimeJob("aaa", "job1", true))
				controller.OnJobStart(newTestRuntimeJob("ccc", "job1", true))

				assert.False(t, controller.registry.isModuleRegistered("aaa"))
				assert.True(t, controller.registry.isModuleRegistered("ccc"))
				assert.Equal(t, []string{"later:logs"}, reg.registeredNames())

			case "topology methods register direct alias":
				controller.RegisterModules(collectorapi.Registry{
					"snmp_topology": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:           "topology:snmp",
								FunctionName: "snmp:topology:snmp",
								Aliases:      []string{"topology:snmp"},
								AgentWide:    true,
							}}
						},
					},
				})

				assert.ElementsMatch(t, []string{"snmp:topology:snmp", "topology:snmp"}, reg.registeredNames())

				controller.Cleanup()

				assert.ElementsMatch(t, []string{"snmp:topology:snmp", "topology:snmp"}, reg.unregisteredNames())

			case "job stop unregisters job methods":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{ID: "job-method"}}
						},
					},
				})

				job := newTestRuntimeJob("mod", "job1", true)
				controller.OnJobStart(job)
				controller.OnJobStop(job)

				assert.Contains(t, reg.unregisteredNames(), "mod:job-method")

			case "cleanup unregisters static methods":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "a"}} },
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.Cleanup()

				assert.Contains(t, reg.unregisteredNames(), "mod:a")

			case "cleanup ignores unavailable static methods":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:        "logs",
								Available: func() bool { return false },
							}}
						},
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.Cleanup()

				assert.Empty(t, reg.registeredNames())
				assert.Empty(t, reg.unregisteredNames())

			case "cleanup with api configured still unregisters static methods":
				var buf bytes.Buffer
				controller = New(Options{
					FnReg: reg,
					API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
				})
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "a"}} },
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"mod:a\"")

				controller.Cleanup()

				assert.Contains(t, reg.unregisteredNames(), "mod:a")

			case "api registration honors method tags":
				var buf bytes.Buffer
				controller = New(Options{
					FnReg: reg,
					API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
				})
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig {
							return []funcapi.MethodConfig{{
								ID:           "logs",
								FunctionName: "snmp:traps",
								Tags:         "logs",
							}}
						},
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))

				assert.Contains(t, reg.registeredNames(), "snmp:traps")
				assert.NotContains(t, reg.registeredNames(), "mod:logs")
				assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"snmp:traps\"")
				assert.NotContains(t, buf.String(), "FUNCTION GLOBAL \"mod:logs\"")
				assert.Contains(t, buf.String(), "\"logs\" 0x0000")
			}
		})
	}
}

func TestControllerRegisterJobMethods(t *testing.T) {
	tests := map[string]struct{}{
		"fail fast on collision with static method":          {},
		"fail fast on collision with other job":              {},
		"fail fast on duplicate within batch":                {},
		"fail fast on job method public names":               {},
		"registry is populated before handlers are callable": {},
		"success commits all methods":                        {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})

			switch name {
			case "fail fast on collision with static method":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "dup"}} },
					},
				})

				controller.registerJobMethods(newTestRuntimeJob("mod", "job1", true), []funcapi.MethodConfig{{ID: "dup"}})

				assert.Empty(t, reg.registeredNames())
				assert.Empty(t, controller.registry.getJobMethods("mod", "job1"))

			case "fail fast on collision with other job":
				controller.registry.registerModule("mod", collectorapi.Creator{})
				controller.registry.registerJobMethods("mod", "jobA", []funcapi.MethodConfig{{ID: "dup"}})

				controller.registerJobMethods(newTestRuntimeJob("mod", "jobB", true), []funcapi.MethodConfig{{ID: "dup"}})

				assert.Empty(t, reg.registeredNames())
				assert.Empty(t, controller.registry.getJobMethods("mod", "jobB"))

			case "fail fast on duplicate within batch":
				controller.registry.registerModule("mod", collectorapi.Creator{})

				controller.registerJobMethods(newTestRuntimeJob("mod", "job1", true), []funcapi.MethodConfig{{ID: "dup"}, {ID: "dup"}})

				assert.Empty(t, reg.registeredNames())
				assert.Empty(t, controller.registry.getJobMethods("mod", "job1"))

			case "fail fast on job method public names":
				controller.registry.registerModule("mod", collectorapi.Creator{})

				controller.registerJobMethods(newTestRuntimeJob("mod", "job1", true), []funcapi.MethodConfig{
					{ID: "logs", FunctionName: "public:logs", Aliases: []string{"public:alias"}},
				})

				assert.Empty(t, reg.registeredNames())
				assert.Empty(t, controller.registry.getJobMethods("mod", "job1"))

			case "registry is populated before handlers are callable":
				var gotCode int
				var gotResp map[string]any

				reg.onRegister = func(_ string, fn func(functions.Function)) {
					fn(functions.Function{
						UID:  "during-register",
						Args: []string{"info"},
					})
				}
				controller = New(Options{
					FnReg: reg,
					JSONWriter: func(data []byte, code int) {
						gotCode = code
						require.NoError(t, json.Unmarshal(data, &gotResp))
					},
				})
				controller.registry.registerModule("mod", collectorapi.Creator{})

				controller.registerJobMethods(newTestRuntimeJob("mod", "job1", true), []funcapi.MethodConfig{{ID: "a", Help: "job method help"}})

				assert.Equal(t, 200, gotCode)
				assert.Equal(t, float64(200), gotResp["status"])
				assert.Equal(t, "job method help", gotResp["help"])
				assert.Len(t, controller.registry.getJobMethods("mod", "job1"), 1)

			case "success commits all methods":
				controller.registry.registerModule("mod", collectorapi.Creator{})

				controller.registerJobMethods(newTestRuntimeJob("mod", "job1", true), []funcapi.MethodConfig{{ID: "a"}, {ID: "b"}})

				assert.ElementsMatch(t, []string{"mod:a", "mod:b"}, reg.registeredNames())
				assert.Len(t, controller.registry.getJobMethods("mod", "job1"), 2)
			}
		})
	}
}

type rawControllerMethodCase struct {
	fn            functions.Function
	cancelContext bool
	raw           func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
	wantCode      int
	check         func(*testing.T, map[string]any)
}

func runRawControllerMethodCase(t *testing.T, tc rawControllerMethodCase, creator collectorapi.Creator, callName string) {
	t.Helper()

	var gotCode int
	var gotResp map[string]any
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	handler := &rawTestHandler{raw: tc.raw}
	creator.MethodHandler = func(collectorapi.RuntimeJob) funcapi.MethodHandler {
		return handler
	}
	controller.RegisterModules(collectorapi.Registry{"mod": creator})
	controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))

	call := reg.handlers[callName]
	require.NotNil(t, call)
	ctx := context.Background()
	if tc.cancelContext {
		ctx = canceledTestContext()
	}
	reg.call(callName, ctx, tc.fn)

	assert.Equal(t, tc.wantCode, gotCode)
	tc.check(t, gotResp)
}

func canceledTestContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestControllerRawModuleMethodRequest(t *testing.T) {
	tests := map[string]rawControllerMethodCase{
		"raw query response is passed through": {
			fn: functions.Function{
				UID:     "raw-query",
				Timeout: time.Second,
				Payload: []byte(`{"last":10}`),
			},
			raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.Equal(t, "logs", req.Method)
				assert.False(t, req.Info)
				assert.JSONEq(t, `{"last":10}`, string(req.Payload))
				return funcapi.RawResponse(map[string]any{
					"status":      200,
					"type":        "table",
					"has_history": true,
					"data":        []any{[]any{"ok"}},
				})
			},
			wantCode: 200,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, float64(200), resp["status"])
				assert.Equal(t, "table", resp["type"])
				assert.Equal(t, true, resp["has_history"])
				assert.NotContains(t, resp, "accepted_params")
			},
		},
		"raw info response is handled by collector": {
			fn: functions.Function{
				UID:     "raw-info",
				Timeout: time.Second,
				Args:    []string{"info"},
			},
			raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.True(t, req.Info)
				return funcapi.RawResponse(map[string]any{
					"status":          200,
					"type":            "table",
					"has_history":     true,
					"accepted_params": []any{"last"},
				})
			},
			wantCode: 200,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, true, resp["has_history"])
				assert.Contains(t, resp, "accepted_params")
				assert.NotContains(t, resp, "required_params")
			},
		},
		"canceled function context reaches raw handler": {
			fn: functions.Function{
				UID:     "raw-cancel",
				Timeout: time.Second,
			},
			cancelContext: true,
			raw: func(ctx context.Context, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.ErrorIs(t, ctx.Err(), context.Canceled)
				return funcapi.RawResponse(map[string]any{
					"status":       499,
					"errorMessage": "Request cancelled.",
				})
			},
			wantCode: 499,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, float64(499), resp["status"])
				assert.Equal(t, "Request cancelled.", resp["errorMessage"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runRawControllerMethodCase(t, tc, collectorapi.Creator{
				Methods: func() []funcapi.MethodConfig {
					return []funcapi.MethodConfig{{
						ID:         "logs",
						RawRequest: true,
						AgentWide:  true,
					}}
				},
			}, "mod:logs")
		})
	}
}

func TestControllerRawAgentWideModuleMethodDoesNotRequireRunningJob(t *testing.T) {
	var gotCode int
	var gotResp map[string]any
	var gotJob collectorapi.RuntimeJob
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})

	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{
					ID:         "logs",
					RawRequest: true,
					AgentWide:  true,
				}}
			},
			MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
				gotJob = job
				return &rawTestHandler{
					raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
						assert.Equal(t, "logs", req.Method)
						return funcapi.RawResponse(map[string]any{
							"status": 200,
							"type":   "table",
						})
					},
				}
			},
		},
	})

	reg.call("mod:logs", context.Background(), functions.Function{
		UID:     "raw-agent-wide",
		Timeout: time.Second,
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Nil(t, gotJob)
}

func TestControllerRawSingleInstanceAgentWideModuleMethodUsesRunningJob(t *testing.T) {
	var gotCode int
	var gotResp map[string]any
	var gotJob collectorapi.RuntimeJob
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			InstancePolicy: collectorapi.InstancePolicySingle,
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{
					ID:         "logs",
					RawRequest: true,
					AgentWide:  true,
				}}
			},
			MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
				gotJob = job
				return &rawTestHandler{
					raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
						assert.Equal(t, "logs", req.Method)
						return funcapi.RawResponse(map[string]any{
							"status": 200,
							"type":   "table",
						})
					},
				}
			},
		},
	})
	job := newTestRuntimeJob("mod", "mod", true)
	controller.OnJobStart(job)

	reg.call("mod:logs", context.Background(), functions.Function{
		UID:     "raw-single-agent-wide",
		Timeout: time.Second,
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Same(t, job, gotJob)
}

func TestControllerSingleInstanceAgentWideModuleMethodUsesRunningJob(t *testing.T) {
	var gotCode int
	var gotResp map[string]any
	var gotJob collectorapi.RuntimeJob
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			InstancePolicy: collectorapi.InstancePolicySingle,
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{
					ID:        "status",
					AgentWide: true,
				}}
			},
			MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
				gotJob = job
				return &rawTestHandler{
					params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
						return []funcapi.ParamConfig{{
							ID:        "scope",
							Name:      "Scope",
							Selection: funcapi.ParamSelect,
							Options: []funcapi.ParamOption{{
								ID:      "all",
								Name:    "All",
								Default: true,
							}},
						}}, nil
					},
					handle: func(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
						assert.Equal(t, "status", method)
						assert.Equal(t, "all", params.GetOne("scope"))
						return &funcapi.FunctionResponse{
							Status:       200,
							ResponseType: "table",
							Help:         "status",
						}
					},
				}
			},
		},
	})
	job := newTestRuntimeJob("mod", "mod", true)
	controller.OnJobStart(job)

	reg.call("mod:status", context.Background(), functions.Function{
		UID:     "single-agent-wide",
		Timeout: time.Second,
		Payload: []byte(`{"scope":"all"}`),
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Same(t, job, gotJob)
	assert.Equal(t, []any{"scope"}, gotResp["accepted_params"])
}

func TestControllerSingleInstanceAgentWideModuleMethodRequiresRunningJob(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Controller)
		message string
	}{
		"before start": {
			message: "module 'mod' is not running",
		},
		"after stop": {
			setup: func(controller *Controller) {
				job := newTestRuntimeJob("mod", "mod", true)
				controller.OnJobStart(job)
				controller.OnJobStop(job)
			},
			message: "module 'mod' is not running",
		},
		"registered but not running": {
			setup: func(controller *Controller) {
				controller.OnJobStart(newTestRuntimeJob("mod", "mod", false))
			},
			message: "job 'mod' is no longer running",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotCode int
			var gotResp map[string]any
			var gotHandler bool
			reg := newTestFunctionRegistry()
			controller := New(Options{
				FnReg: reg,
				JSONWriter: func(data []byte, code int) {
					gotCode = code
					require.NoError(t, json.Unmarshal(data, &gotResp))
				},
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					InstancePolicy: collectorapi.InstancePolicySingle,
					Methods: func() []funcapi.MethodConfig {
						return []funcapi.MethodConfig{{
							ID:        "status",
							AgentWide: true,
						}}
					},
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						gotHandler = true
						return &tableTestHandler{}
					},
				},
			})
			if tc.setup != nil {
				tc.setup(controller)
			}

			reg.call("mod:status", context.Background(), functions.Function{
				UID:     "single-agent-wide-missing-job",
				Timeout: time.Second,
			})

			assert.Equal(t, 503, gotCode)
			assert.Equal(t, float64(503), gotResp["status"])
			assert.Equal(t, tc.message, gotResp["errorMessage"])
			assert.False(t, gotHandler)
		})
	}
}

func TestControllerModuleMethodRequestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotCode int
	var gotResp map[string]any
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: "query"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &rawTestHandler{
					handle: func(ctx context.Context, _ string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
						assert.ErrorIs(t, ctx.Err(), context.Canceled)
						return funcapi.ErrorResponse(499, "Request cancelled.")
					},
				}
			},
		},
	})
	controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))

	reg.call("mod:query", ctx, functions.Function{
		UID:     "normal-cancel",
		Timeout: time.Second,
	})

	assert.Equal(t, 499, gotCode)
	assert.Equal(t, float64(499), gotResp["status"])
	assert.Equal(t, "Request cancelled.", gotResp["errorMessage"])
}

func TestControllerRawJobMethodRequest(t *testing.T) {
	tests := map[string]rawControllerMethodCase{
		"raw query response is passed through": {
			fn: functions.Function{
				UID:     "raw-query",
				Timeout: time.Second,
				Payload: []byte(`{"last":10}`),
			},
			raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.Equal(t, "job1:logs", req.Method)
				assert.False(t, req.Info)
				assert.JSONEq(t, `{"last":10}`, string(req.Payload))
				return funcapi.RawResponse(map[string]any{
					"status":      200,
					"type":        "table",
					"has_history": true,
					"data":        []any{[]any{"ok"}},
				})
			},
			wantCode: 200,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, float64(200), resp["status"])
				assert.Equal(t, "table", resp["type"])
				assert.Equal(t, true, resp["has_history"])
				assert.NotContains(t, resp, "accepted_params")
			},
		},
		"raw info response is handled by collector": {
			fn: functions.Function{
				UID:     "raw-info",
				Timeout: time.Second,
				Args:    []string{"info"},
			},
			raw: func(_ context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.True(t, req.Info)
				return funcapi.RawResponse(map[string]any{
					"status":          200,
					"type":            "table",
					"has_history":     true,
					"accepted_params": []any{"last"},
				})
			},
			wantCode: 200,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, true, resp["has_history"])
				assert.Contains(t, resp, "accepted_params")
				assert.NotContains(t, resp, "required_params")
			},
		},
		"canceled function context reaches raw handler": {
			fn: functions.Function{
				UID:     "raw-cancel",
				Timeout: time.Second,
			},
			cancelContext: true,
			raw: func(ctx context.Context, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				assert.ErrorIs(t, ctx.Err(), context.Canceled)
				return funcapi.RawResponse(map[string]any{
					"status":       499,
					"errorMessage": "Request cancelled.",
				})
			},
			wantCode: 499,
			check: func(t *testing.T, resp map[string]any) {
				assert.Equal(t, float64(499), resp["status"])
				assert.Equal(t, "Request cancelled.", resp["errorMessage"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			runRawControllerMethodCase(t, tc, collectorapi.Creator{
				JobMethods: func(job collectorapi.RuntimeJob) []funcapi.MethodConfig {
					return []funcapi.MethodConfig{{
						ID:         job.Name() + ":logs",
						RawRequest: true,
					}}
				},
			}, "mod:job1:logs")
		})
	}
}

func TestControllerRawJobMethodRequiresRawHandler(t *testing.T) {
	var gotCode int
	var gotResp map[string]any
	reg := newTestFunctionRegistry()
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			JobMethods: func(job collectorapi.RuntimeJob) []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: job.Name() + ":logs", RawRequest: true}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &tableTestHandler{}
			},
		},
	})
	controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))

	reg.handlers["mod:job1:logs"](functions.Function{
		UID:     "missing-raw-handler",
		Timeout: time.Second,
	})

	assert.Equal(t, 500, gotCode)
	assert.Equal(t, float64(500), gotResp["status"])
	assert.Contains(t, gotResp["errorMessage"], "requires raw request handling")
}

type testRuntimeJob struct {
	fullName   string
	moduleName string
	name       string
	running    bool
}

func newTestRuntimeJob(moduleName, name string, running bool) *testRuntimeJob {
	return &testRuntimeJob{
		fullName:   moduleName + "_" + name,
		moduleName: moduleName,
		name:       name,
		running:    running,
	}
}

func (j *testRuntimeJob) FullName() string   { return j.fullName }
func (j *testRuntimeJob) ModuleName() string { return j.moduleName }
func (j *testRuntimeJob) Name() string       { return j.name }
func (j *testRuntimeJob) IsRunning() bool    { return j.running }
func (j *testRuntimeJob) Collector() any     { return nil }

type rawTestHandler struct {
	params func(context.Context, string) ([]funcapi.ParamConfig, error)
	handle func(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse
	raw    func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
}

var _ funcapi.RawMethodHandler = (*rawTestHandler)(nil)

func (h *rawTestHandler) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if h.params != nil {
		return h.params(ctx, method)
	}
	return nil, nil
}

func (h *rawTestHandler) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if h.handle != nil {
		return h.handle(ctx, method, params)
	}
	return &funcapi.FunctionResponse{Status: 200}
}

func (h *rawTestHandler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if h.raw == nil {
		return &funcapi.FunctionResponse{Status: 200}
	}
	return h.raw(ctx, req)
}

func (h *rawTestHandler) Cleanup(context.Context) {}

type tableTestHandler struct{}

var _ funcapi.MethodHandler = (*tableTestHandler)(nil)

func (h *tableTestHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (h *tableTestHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (h *tableTestHandler) Cleanup(context.Context) {}

type testFunctionRegistry struct {
	handlers        map[string]func(functions.Function)
	contextHandlers map[string]functions.Handler
	registered      []string
	unregistered    []string
	onRegister      func(string, func(functions.Function))
}

func newTestFunctionRegistry() *testFunctionRegistry {
	return &testFunctionRegistry{
		handlers:        make(map[string]func(functions.Function)),
		contextHandlers: make(map[string]functions.Handler),
	}
}

func (r *testFunctionRegistry) Register(name string, fn func(functions.Function)) {
	r.handlers[name] = fn
	r.registered = append(r.registered, name)
	if r.onRegister != nil {
		r.onRegister(name, fn)
	}
}

func (r *testFunctionRegistry) RegisterWithContext(name string, fn functions.Handler) {
	legacy := func(f functions.Function) {
		fn(context.Background(), f)
	}
	r.handlers[name] = legacy
	r.contextHandlers[name] = fn
	r.registered = append(r.registered, name)
	if r.onRegister != nil {
		r.onRegister(name, legacy)
	}
}

func (r *testFunctionRegistry) Unregister(name string) {
	r.unregistered = append(r.unregistered, name)
	delete(r.handlers, name)
	delete(r.contextHandlers, name)
}

func (r *testFunctionRegistry) RegisterPrefix(string, string, func(functions.Function)) {}
func (r *testFunctionRegistry) UnregisterPrefix(string, string)                         {}

func (r *testFunctionRegistry) RegisterPrefixWithContext(string, string, functions.Handler) {}

func (r *testFunctionRegistry) call(name string, ctx context.Context, fn functions.Function) {
	if handler := r.contextHandlers[name]; handler != nil {
		handler(ctx, fn)
		return
	}
	r.handlers[name](fn)
}

func (r *testFunctionRegistry) registeredNames() []string {
	out := make([]string, len(r.registered))
	copy(out, r.registered)
	return out
}

func (r *testFunctionRegistry) unregisteredNames() []string {
	out := make([]string, len(r.unregistered))
	copy(out, r.unregistered)
	return out
}
