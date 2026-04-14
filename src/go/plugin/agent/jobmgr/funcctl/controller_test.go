// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"bytes"
	"encoding/json"
	"testing"

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
					"build accepted params":                  {},
					"build required params uses select type": {},
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

							assert.Equal(t, []string{"__job", "__sort", "db", "extra"}, buildAcceptedParams(methodParams))

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
							params := controller.buildRequiredParams("postgres", methodParams)

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
						}
					})
				}
			}
		})
	}
}

func TestControllerLifecycleHooks(t *testing.T) {
	tests := map[string]struct{}{
		"register modules does not register static methods yet":        {},
		"first job start registers static methods once":                {},
		"job stop unregisters job methods":                             {},
		"cleanup unregisters static methods":                           {},
		"cleanup with api configured still unregisters static methods": {},
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

			case "first job start registers static methods once":
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						Methods: func() []funcapi.MethodConfig { return []funcapi.MethodConfig{{ID: "a"}} },
					},
				})

				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))

				assert.Equal(t, []string{"mod:a"}, reg.registeredNames())

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
			}
		})
	}
}

func TestControllerRegisterJobMethods(t *testing.T) {
	tests := map[string]struct{}{
		"fail fast on collision with static method":          {},
		"fail fast on collision with other job":              {},
		"fail fast on duplicate within batch":                {},
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

type testFunctionRegistry struct {
	handlers     map[string]func(functions.Function)
	registered   []string
	unregistered []string
	onRegister   func(string, func(functions.Function))
}

func newTestFunctionRegistry() *testFunctionRegistry {
	return &testFunctionRegistry{
		handlers: make(map[string]func(functions.Function)),
	}
}

func (r *testFunctionRegistry) Register(name string, fn func(functions.Function)) {
	r.handlers[name] = fn
	r.registered = append(r.registered, name)
	if r.onRegister != nil {
		r.onRegister(name, fn)
	}
}

func (r *testFunctionRegistry) Unregister(name string) {
	r.unregistered = append(r.unregistered, name)
	delete(r.handlers, name)
}

func (r *testFunctionRegistry) RegisterPrefix(string, string, func(functions.Function)) {}
func (r *testFunctionRegistry) UnregisterPrefix(string, string)                         {}

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
