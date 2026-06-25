// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
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
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "test"}}
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
	tests := map[string]struct {
		run func(*testing.T, *moduleFuncRegistry)
	}{
		"add remove job": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
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
			},
		},
		"job replacement increments generation": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
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
			},
		},
		"job re-add after removal increments generation": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
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
			},
		},
		"generation verification fails on wrong generation and missing job": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.registerModule("postgres", collectorapi.Creator{})

				job := newTestRuntimeJob("postgres", "master", true)
				r.addJob("postgres", "master", job)
				_, gen := r.getJobWithGeneration("postgres", "master")

				assert.False(t, r.verifyJobGeneration("postgres", "master", gen+1))
				r.removeJob("postgres", "master")
				assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
			},
		},
		"get methods": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				expectedMethods := []funcapi.FunctionConfig{{ID: "top-queries", Name: "Top Queries"}}
				r.registerModule("postgres", collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return expectedMethods },
				})

				assert.Equal(t, expectedMethods, r.getMethods("postgres"))
				assert.Nil(t, r.getMethods("nonexistent"))
			},
		},
		"get job names sorted": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.registerModule("postgres", collectorapi.Creator{})

				r.addJob("postgres", "zebra-db", newTestRuntimeJob("postgres", "zebra-db", true))
				r.addJob("postgres", "alpha-db", newTestRuntimeJob("postgres", "alpha-db", true))
				r.addJob("postgres", "middle-db", newTestRuntimeJob("postgres", "middle-db", true))

				assert.Equal(t, []string{"alpha-db", "middle-db", "zebra-db"}, r.getJobNames("postgres"))
			},
		},
		"operations on unregistered module are no op": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.addJob("nonexistent", "job1", newTestRuntimeJob("nonexistent", "job1", true))
				r.removeJob("nonexistent", "job1")

				assert.False(t, r.isModuleRegistered("nonexistent"))
				assert.Nil(t, r.getJobNames("nonexistent"))
				assert.Nil(t, r.getMethods("nonexistent"))

				_, ok := r.getJob("nonexistent", "job1")
				assert.False(t, ok)
			},
		},
		"get creator": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				creator := collectorapi.Creator{JobConfigSchema: "test-schema"}
				r.registerModule("postgres", creator)

				got, ok := r.getCreator("postgres")
				require.True(t, ok)
				assert.Equal(t, "test-schema", got.JobConfigSchema)

				_, ok = r.getCreator("nonexistent")
				assert.False(t, ok)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := newModuleFuncRegistry()
			tc.run(t, r)
		})
	}
}

func TestModuleFuncRegistry_Concurrency(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "test"}}
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
	r.registerModuleWithMethods("bbb", collectorapi.Creator{}, []funcapi.FunctionConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})
	r.registerModuleWithMethods("aaa", collectorapi.Creator{}, []funcapi.FunctionConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})

	moduleName, methodID, ok := r.resolveMethodRoute("shared:logs")
	require.True(t, ok)
	assert.Equal(t, "aaa", moduleName)
	assert.Equal(t, "logs", methodID)
}

func TestModuleFuncRegistry_MethodRouteRefreshesAffectedNames(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModuleWithMethods("bbb", collectorapi.Creator{}, []funcapi.FunctionConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})
	r.registerModuleWithMethods("aaa", collectorapi.Creator{}, []funcapi.FunctionConfig{{
		ID:           "logs",
		FunctionName: "shared:logs",
	}})

	r.registerModuleWithMethods("aaa", collectorapi.Creator{}, nil)

	moduleName, methodID, ok := r.resolveMethodRoute("shared:logs")
	require.True(t, ok)
	assert.Equal(t, "bbb", moduleName)
	assert.Equal(t, "logs", methodID)

	r.registerModuleWithMethods("bbb", collectorapi.Creator{}, nil)

	_, _, ok = r.resolveMethodRoute("shared:logs")
	assert.False(t, ok)
}

func TestModuleFuncRegistry_VerifyJobGeneration_JobStopped(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	job := newTestRuntimeJob("postgres", "master", false)
	r.addJob("postgres", "master", job)
	_, gen := r.getJobWithGeneration("postgres", "master")

	assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
}

func TestExtractParamValues(t *testing.T) {
	tests := map[string]struct {
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractParamValues(tc.payload, tc.key))
		})
	}
}

func TestBuildAcceptedParams(t *testing.T) {
	sortDir := funcapi.FieldSortDescending
	methodParams := []funcapi.ParamConfig{
		{ID: "__sort", Selection: funcapi.ParamSelect, Options: []funcapi.ParamOption{{ID: "calls", Name: "Calls", Sort: &sortDir}}},
		{ID: "db"},
		{ID: "extra"},
	}

	assert.Equal(t, []string{"__job", "__sort", "db", "extra"}, buildAcceptedParams(methodParams, true))
	assert.Equal(t, []string{"__sort", "db", "extra"}, buildAcceptedParams(methodParams, false))
}

func TestBuildRequiredParamsUsesSelectType(t *testing.T) {
	controller := New(Options{})
	controller.RegisterModules(collectorapi.Registry{
		"postgres": collectorapi.Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "top-queries", Name: "Top Queries"}}
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
	params := controller.buildRequiredParams("postgres", "top-queries", methodParams, true)

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

func TestBuildRequiredParamsAgentScopeOmitsJob(t *testing.T) {
	controller := New(Options{})
	controller.RegisterModules(collectorapi.Registry{
		"snmp": collectorapi.Creator{
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "topology:snmp"}}
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
	params := controller.buildRequiredParams("snmp", "topology", methodParams, false)

	assert.Len(t, params, 1)
	assert.Equal(t, "topology_view", params[0]["id"])
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
	newController := func() (*Controller, *testFunctionRegistry) {
		reg := newTestFunctionRegistry()
		return New(Options{FnReg: reg}), reg
	}

	tests := map[string]func(*testing.T){
		"register modules does not register static methods yet": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "a"}} },
				},
			})

			assert.Empty(t, reg.registeredNames())
		},
		"register modules registers available agent-scope methods": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs"}}
					},
				},
			})

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
		},
		"reconcile after job start registers static methods once": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "a"}} },
				},
			})

			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))
			controller.ReconcileModuleMethods("mod")
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, []string{"mod:a"}, reg.registeredNames())
		},
		"function-availability-gated shared method registers when available": func(t *testing.T) {
			controller, reg := newController()
			available := &testFunctionAvailability{available: map[string]bool{"logs": false}}
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs"}}
					},
				},
			})

			controller.OnJobStart(&testRuntimeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", running: true, collector: available})
			assert.Empty(t, reg.registeredNames())

			available.available["logs"] = true
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
		},
		"availability-gated agent-scope method registers when available": func(t *testing.T) {
			controller, reg := newController()
			available := false
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID:        "logs",
							Available: func() bool { return available },
						}}
					},
				},
			})

			assert.Empty(t, reg.registeredNames())

			available = true
			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			controller.OnJobStart(newTestRuntimeJob("mod", "job2", true))
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
		},
		"shared method ignores FunctionConfig availability": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID:        "logs",
							Available: func() bool { return false },
						}}
					},
				},
			})

			job := newTestRuntimeJob("mod", "job1", true)
			controller.OnJobStart(job)
			controller.ReconcileModuleMethods(job.ModuleName())

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
		},
		"reconcile ignores module with no running job": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs"}}
					},
				},
			})

			controller.ReconcileModuleMethods("mod")

			assert.Empty(t, reg.registeredNames())
		},
		"reconcile does not duplicate published static method": func(t *testing.T) {
			controller, reg := newController()
			available := &testFunctionAvailability{available: map[string]bool{"logs": true}}
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs"}}
					},
				},
			})

			job := &testRuntimeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", running: true, collector: available}
			controller.OnJobStart(job)
			controller.ReconcileModuleMethods(job.ModuleName())
			controller.ReconcileModuleMethods(job.ModuleName())

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
		},
		"reconcile withdraws shared method when last job becomes unavailable": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			var buf bytes.Buffer
			available := &testFunctionAvailability{available: map[string]bool{"logs": true}}
			controller := New(Options{
				FnReg: reg,
				API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs"}}
					},
				},
			})

			job := &testRuntimeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", running: true, collector: available}
			controller.OnJobStart(job)
			controller.ReconcileModuleMethods(job.ModuleName())
			assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"mod:logs\"")

			available.available["logs"] = false
			controller.ReconcileModuleMethods(job.ModuleName())

			assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
			assert.Equal(t, []string{"mod:logs"}, reg.unregisteredNames())
			assert.Contains(t, buf.String(), "FUNCTION_DEL GLOBAL \"mod:logs\"")
		},
		"reconcile logs empty static method ID once": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			var logBuf bytes.Buffer
			controller := New(Options{
				FnReg:  reg,
				Logger: logger.NewWithWriter(&logBuf),
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{Name: "invalid"}}
					},
				},
			})

			job1 := newTestRuntimeJob("mod", "job1", true)
			job2 := newTestRuntimeJob("mod", "job2", true)
			controller.OnJobStart(job1)
			controller.ReconcileModuleMethods(job1.ModuleName())
			controller.OnJobStart(job2)
			controller.ReconcileModuleMethods(job2.ModuleName())

			assert.Equal(t, 1, strings.Count(logBuf.String(), "empty method ID"))
		},
		"public method name collision skips colliding module": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"aaa": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs", FunctionName: "shared:logs"}}
					},
				},
				"bbb": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs", FunctionName: "shared:logs"}}
					},
				},
			})

			controller.OnJobStart(newTestRuntimeJob("aaa", "job1", true))
			controller.OnJobStart(newTestRuntimeJob("bbb", "job1", true))
			controller.ReconcileModuleMethods("aaa")
			controller.ReconcileModuleMethods("bbb")

			assert.Equal(t, []string{"shared:logs"}, reg.registeredNames())
			assert.True(t, controller.registry.isModuleRegistered("aaa"))
			assert.False(t, controller.registry.isModuleRegistered("bbb"))
		},
		"rejected module does not poison planned public names": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"aaa": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{
							{ID: "first", FunctionName: "later:logs"},
							{ID: "second", FunctionName: "later:logs"},
						}
					},
				},
				"ccc": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "logs", FunctionName: "later:logs"}}
					},
				},
			})

			controller.OnJobStart(newTestRuntimeJob("aaa", "job1", true))
			controller.OnJobStart(newTestRuntimeJob("ccc", "job1", true))
			controller.ReconcileModuleMethods("aaa")
			controller.ReconcileModuleMethods("ccc")

			assert.False(t, controller.registry.isModuleRegistered("aaa"))
			assert.True(t, controller.registry.isModuleRegistered("ccc"))
			assert.Equal(t, []string{"later:logs"}, reg.registeredNames())
		},
		"topology methods register direct alias": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"snmp_topology": collectorapi.Creator{
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID:           "topology:snmp",
							FunctionName: "snmp:topology:snmp",
							Aliases:      []string{"topology:snmp"},
						}}
					},
				},
			})

			assert.ElementsMatch(t, []string{"snmp:topology:snmp", "topology:snmp"}, reg.registeredNames())

			controller.Cleanup()

			assert.ElementsMatch(t, []string{"snmp:topology:snmp", "topology:snmp"}, reg.unregisteredNames())
		},
		"job stop unregisters instance functions": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "job-method"}}
					},
				},
			})

			job := newTestRuntimeJob("mod", "job1", true)
			controller.OnJobStart(job)
			controller.ReconcileModuleMethods("mod")
			controller.OnJobStop(job)
			assert.Empty(t, reg.unregisteredNames())
			controller.ReconcileModuleMethods("mod")

			assert.Contains(t, reg.unregisteredNames(), "mod:job-method")
		},
		"cleanup unregisters static methods": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "a"}} },
				},
			})

			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			controller.ReconcileModuleMethods("mod")
			controller.Cleanup()

			assert.Contains(t, reg.unregisteredNames(), "mod:a")
		},
		"cleanup ignores unavailable agent methods": func(t *testing.T) {
			controller, reg := newController()
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
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
		},
		"cleanup with api configured still unregisters static methods": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			var buf bytes.Buffer
			controller := New(Options{
				FnReg: reg,
				API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "a"}} },
				},
			})

			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			controller.ReconcileModuleMethods("mod")
			assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"mod:a\"")

			controller.Cleanup()

			assert.Contains(t, reg.unregisteredNames(), "mod:a")
		},
		"api registration honors method tags": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			var buf bytes.Buffer
			controller := New(Options{
				FnReg: reg,
				API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID:           "logs",
							FunctionName: "snmp:traps",
							Tags:         "logs",
						}}
					},
				},
			})

			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			controller.ReconcileModuleMethods("mod")

			assert.Contains(t, reg.registeredNames(), "snmp:traps")
			assert.NotContains(t, reg.registeredNames(), "mod:logs")
			assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"snmp:traps\"")
			assert.NotContains(t, buf.String(), "FUNCTION GLOBAL \"mod:logs\"")
			assert.Contains(t, buf.String(), "\"logs\" 0x0000")
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}

func TestControllerSharedFunctionWithdrawsWhenLastDefaultJobStops(t *testing.T) {
	reg := newTestFunctionRegistry()
	var buf bytes.Buffer
	controller := New(Options{
		FnReg: reg,
		API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "logs"}}
			},
		},
	})

	job1 := newTestRuntimeJob("mod", "job1", true)
	job2 := newTestRuntimeJob("mod", "job2", true)
	controller.OnJobStart(job1)
	controller.OnJobStart(job2)
	controller.ReconcileModuleMethods("mod")
	controller.OnJobStop(job1)

	assert.Equal(t, []string{"mod:logs"}, reg.registeredNames())
	assert.Empty(t, reg.unregisteredNames())
	assert.Contains(t, buf.String(), "FUNCTION GLOBAL \"mod:logs\"")
	assert.NotContains(t, buf.String(), "FUNCTION_DEL GLOBAL \"mod:logs\"")

	controller.OnJobStop(job2)
	assert.Empty(t, reg.unregisteredNames())
	controller.ReconcileModuleMethods("mod")

	assert.Equal(t, []string{"mod:logs"}, reg.unregisteredNames())
	assert.Contains(t, buf.String(), "FUNCTION_DEL GLOBAL \"mod:logs\"")
}

func TestControllerSharedFunctionReconcileDoesNotWithdrawAgentFunctions(t *testing.T) {
	reg := newTestFunctionRegistry()
	var buf bytes.Buffer
	controller := New(Options{
		FnReg: reg,
		API:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "agent"}}
			},
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "shared"}}
			},
		},
	})

	job := newTestRuntimeJob("mod", "job1", true)
	controller.OnJobStart(job)
	controller.ReconcileModuleMethods("mod")
	controller.OnJobStop(job)
	controller.ReconcileModuleMethods("mod")

	assert.Contains(t, reg.registeredNames(), "mod:agent")
	assert.Contains(t, reg.registeredNames(), "mod:shared")
	assert.Contains(t, reg.unregisteredNames(), "mod:shared")
	assert.NotContains(t, reg.unregisteredNames(), "mod:agent")
	assert.NotContains(t, buf.String(), "FUNCTION_DEL GLOBAL \"mod:agent\"")
	assert.Contains(t, buf.String(), "FUNCTION_DEL GLOBAL \"mod:shared\"")
}

func TestControllerSharedFunctionAvailableJobsOnly(t *testing.T) {
	var gotCode int
	var gotResp map[string]any
	var dispatches int
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
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "logs"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				dispatches++
				return &tableTestHandler{}
			},
		},
	})
	available := &testFunctionAvailability{available: map[string]bool{"logs": true}}
	unavailable := &testFunctionAvailability{available: map[string]bool{"logs": false}}
	controller.OnJobStart(&testRuntimeJob{fullName: "mod_job1", moduleName: "mod", name: "job1", running: true, collector: available})
	controller.OnJobStart(&testRuntimeJob{fullName: "mod_job2", moduleName: "mod", name: "job2", running: true, collector: unavailable})
	controller.ReconcileModuleMethods("mod")

	reg.call("mod:logs", context.Background(), functions.Function{
		UID:     "shared-info",
		Args:    []string{"info"},
		Timeout: time.Second,
	})

	required, ok := gotResp["required_params"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, required)
	jobParam, ok := required[0].(map[string]any)
	require.True(t, ok)
	options, ok := jobParam["options"].([]any)
	require.True(t, ok)
	require.Len(t, options, 1)
	option, ok := options[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "job1", option["id"])

	reg.call("mod:logs", context.Background(), functions.Function{
		UID:     "shared-unavailable-job",
		Timeout: time.Second,
		Payload: []byte(`{"__job":"job2"}`),
	})

	assert.Equal(t, 404, gotCode)
	assert.Equal(t, float64(404), gotResp["status"])
	assert.Contains(t, gotResp["errorMessage"], "unknown job 'job2'")
	assert.Zero(t, dispatches)
}

func TestControllerSharedFunctionRepublishesWithNewGeneration(t *testing.T) {
	reg := newTestFunctionRegistry()
	var firstHandler functions.Handler
	reg.onRegister = func(name string, _ func(functions.Function)) {
		if name != "mod:logs" || firstHandler != nil {
			return
		}
		reg.mu.Lock()
		firstHandler = reg.contextHandlers[name]
		reg.mu.Unlock()
	}
	var gotCode int
	var gotResp map[string]any
	controller := New(Options{
		FnReg: reg,
		JSONWriter: func(data []byte, code int) {
			gotCode = code
			require.NoError(t, json.Unmarshal(data, &gotResp))
		},
	})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "logs"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &tableTestHandler{}
			},
		},
	})

	job1 := newTestRuntimeJob("mod", "job1", true)
	controller.OnJobStart(job1)
	controller.ReconcileModuleMethods("mod")
	require.NotNil(t, firstHandler)

	controller.OnJobStop(job1)
	controller.ReconcileModuleMethods("mod")
	firstHandler(context.Background(), functions.Function{
		UID:     "stale-generation",
		Timeout: time.Second,
	})
	assert.Equal(t, 404, gotCode)
	assert.Equal(t, float64(404), gotResp["status"])
	assert.Equal(t, "unknown function 'mod:logs'", gotResp["errorMessage"])

	job2 := newTestRuntimeJob("mod", "job2", true)
	controller.OnJobStart(job2)
	controller.ReconcileModuleMethods("mod")

	assert.Equal(t, []string{"mod:logs", "mod:logs"}, reg.registeredNames())
	assert.Equal(t, []string{"mod:logs"}, reg.unregisteredNames())
}

func TestControllerSharedSingleFunctionUsesAvailableCanonicalJob(t *testing.T) {
	reg := newTestFunctionRegistry()
	controller := New(Options{FnReg: reg})
	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			InstancePolicy: collectorapi.InstancePolicySingle,
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "status"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &tableTestHandler{}
			},
		},
	})

	controller.OnJobStart(&testRuntimeJob{
		fullName:   "mod_mod",
		moduleName: "mod",
		name:       "mod",
		running:    true,
		collector:  &testFunctionAvailability{available: map[string]bool{"status": false}},
	})
	controller.ReconcileModuleMethods("mod")

	assert.Empty(t, reg.registeredNames())

	controller.OnJobStart(&testRuntimeJob{
		fullName:   "mod_mod",
		moduleName: "mod",
		name:       "mod",
		running:    true,
		collector:  &testFunctionAvailability{available: map[string]bool{"status": true}},
	})
	controller.ReconcileModuleMethods("mod")

	assert.Equal(t, []string{"mod:status"}, reg.registeredNames())
}

func TestControllerReconcileModuleMethodsConcurrentLifecycle(t *testing.T) {
	reg := newTestFunctionRegistry()
	controller := New(Options{FnReg: reg})

	var enabled atomic.Bool
	var availableCalls atomic.Int32
	var firstAvailable sync.Once
	firstAvailableCalled := make(chan struct{})
	releaseAvailable := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseAvailable) })

	aliases := make([]string, 0, 32)
	for i := range 32 {
		aliases = append(aliases, fmt.Sprintf("mod:late:alias:%02d", i))
	}

	controller.RegisterModules(collectorapi.Registry{
		"mod": collectorapi.Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID:      "late",
					Aliases: aliases,
				}}
			},
		},
	})

	const seedJobs = 8
	jobs := make([]collectorapi.RuntimeJob, 0, seedJobs)
	for i := range seedJobs {
		job := newTestRuntimeJob("mod", fmt.Sprintf("seed-%02d", i), true)
		job.collector = &testFunctionAvailability{fn: func(string) bool {
			if !enabled.Load() {
				return false
			}
			if availableCalls.Add(1) == 1 {
				firstAvailable.Do(func() { close(firstAvailableCalled) })
			}
			<-releaseAvailable
			return true
		}}
		controller.OnJobStart(job)
		jobs = append(jobs, job)
	}
	require.Empty(t, reg.registeredNames())

	enabled.Store(true)

	const workers = 8
	const iterations = 40
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for n := range iterations {
				controller.ReconcileModuleMethods(jobs[(worker+n)%len(jobs)].ModuleName())
			}
		}(i)
	}
	for i := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for n := range iterations {
				job := newTestRuntimeJob("mod", fmt.Sprintf("runtime-%02d-%02d", worker, n), true)
				job.collector = &testFunctionAvailability{available: map[string]bool{"late": true}}
				controller.OnJobStart(job)
				controller.OnJobStop(job)
			}
		}(i)
	}

	close(start)
	select {
	case <-firstAvailableCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("available predicate was not reached")
	}
	time.Sleep(20 * time.Millisecond)
	releaseOnce.Do(func() { close(releaseAvailable) })
	wg.Wait()

	expected := append([]string{"mod:late"}, aliases...)
	assert.ElementsMatch(t, expected, reg.registeredNames())
}

func TestControllerStaticPublicationDoesNotHoldLockDuringFunctionGlobal(t *testing.T) {
	reg := newTestFunctionRegistry()
	writer := newBlockingFunctionWriter()
	t.Cleanup(writer.Release)
	controller := New(Options{
		FnReg: reg,
		API:   dyncfg.NewResponder(netdataapi.New(writer)),
	})

	registerDone := make(chan struct{})
	go func() {
		controller.RegisterModules(collectorapi.Registry{
			"mod": collectorapi.Creator{
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{
						ID: "late",
					}}
				},
			},
		})
		close(registerDone)
	}()

	select {
	case <-writer.started:
	case <-time.After(2 * time.Second):
		t.Fatal("FunctionGlobal write did not start")
	}

	startDone := make(chan struct{})
	go func() {
		controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
		close(startDone)
	}()

	select {
	case <-startDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("OnJobStart blocked while FunctionGlobal was writing")
	}

	writer.Release()
	select {
	case <-registerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RegisterModules did not finish")
	}
}

func TestControllerRegisterInstanceFunctions(t *testing.T) {
	tests := map[string]func(*testing.T){
		"fail fast on collision with static method": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "dup"}} },
				},
			})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "job1", true), []funcapi.FunctionConfig{{ID: "dup"}})

			assert.Empty(t, reg.registeredNames())
			assert.Empty(t, controller.registry.getInstanceFunctions("mod", "job1"))
		},
		"fail fast on collision with other job": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.registry.registerModule("mod", collectorapi.Creator{})
			controller.registry.registerInstanceFunctions("mod", "jobA", []funcapi.FunctionConfig{{ID: "dup"}})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "jobB", true), []funcapi.FunctionConfig{{ID: "dup"}})

			assert.Empty(t, reg.registeredNames())
			assert.Empty(t, controller.registry.getInstanceFunctions("mod", "jobB"))
		},
		"fail fast on duplicate within batch": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.registry.registerModule("mod", collectorapi.Creator{})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "job1", true), []funcapi.FunctionConfig{{ID: "dup"}, {ID: "dup"}})

			assert.Empty(t, reg.registeredNames())
			assert.Empty(t, controller.registry.getInstanceFunctions("mod", "job1"))
		},
		"fail fast on instance function public names": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.registry.registerModule("mod", collectorapi.Creator{})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "job1", true), []funcapi.FunctionConfig{
				{ID: "logs", FunctionName: "public:logs", Aliases: []string{"public:alias"}},
			})

			assert.Empty(t, reg.registeredNames())
			assert.Empty(t, controller.registry.getInstanceFunctions("mod", "job1"))
		},
		"success stores declarations without publishing handlers": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.registry.registerModule("mod", collectorapi.Creator{})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "job1", true), []funcapi.FunctionConfig{{ID: "a", Help: "instance function help"}})

			assert.Empty(t, reg.registeredNames())
			assert.Len(t, controller.registry.getInstanceFunctions("mod", "job1"), 1)
		},
		"success commits all methods": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.registry.registerModule("mod", collectorapi.Creator{})

			controller.registerInstanceFunctions(newTestRuntimeJob("mod", "job1", true), []funcapi.FunctionConfig{{ID: "a"}, {ID: "b"}})

			assert.Empty(t, reg.registeredNames())
			assert.Len(t, controller.registry.getInstanceFunctions("mod", "job1"), 2)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}

func TestControllerInstanceFunctionsUseJobBackedAvailability(t *testing.T) {
	tests := map[string]func(*testing.T){
		"publish withdraw and republish stored declarations": func(t *testing.T) {
			available := map[string]bool{"a": false}
			job := newTestRuntimeJob("mod", "job1", true)
			job.collector = &testFunctionAvailability{available: available}

			var gotCode int
			var gotResp map[string]any
			reg := newTestFunctionRegistry()
			declareCalls := 0
			controller := New(Options{
				FnReg: reg,
				JSONWriter: func(data []byte, code int) {
					gotCode = code
					require.NoError(t, json.Unmarshal(data, &gotResp))
				},
			})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
						declareCalls++
						return []funcapi.FunctionConfig{{ID: "a"}}
					},
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						return &tableTestHandler{}
					},
				},
			})

			controller.OnJobStart(job)

			assert.Equal(t, 1, declareCalls)
			assert.Empty(t, reg.registeredNames())
			assert.Len(t, controller.registry.getInstanceFunctions("mod", "job1"), 1)

			available["a"] = true
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, 1, declareCalls)
			assert.Equal(t, []string{"mod:a"}, reg.registeredNames())
			stale := reg.handlers["mod:a"]
			require.NotNil(t, stale)

			available["a"] = false
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, []string{"mod:a"}, reg.unregisteredNames())
			stale(functions.Function{UID: "withdrawn", Timeout: time.Second})
			assert.Equal(t, 404, gotCode)
			assert.Equal(t, float64(404), gotResp["status"])

			available["a"] = true
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, 1, declareCalls)
			assert.Equal(t, []string{"mod:a", "mod:a"}, reg.registeredNames())
			stale(functions.Function{UID: "stale-after-republish", Timeout: time.Second})
			assert.Equal(t, 404, gotCode)
			assert.Equal(t, float64(404), gotResp["status"])

			reg.handlers["mod:a"](functions.Function{UID: "republished", Timeout: time.Second})
			assert.Equal(t, 200, gotCode)
			assert.Equal(t, float64(200), gotResp["status"])
		},
		"missing FunctionAvailability publishes on reconcile": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			controller := New(Options{FnReg: reg})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "a"}}
					},
				},
			})

			controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
			assert.Empty(t, reg.registeredNames())
			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, []string{"mod:a"}, reg.registeredNames())
		},
		"FunctionAvailability is not called synchronously on job start": func(t *testing.T) {
			reg := newTestFunctionRegistry()
			var availabilityCalls atomic.Int32
			job := newTestRuntimeJob("mod", "job1", true)
			job.collector = &testFunctionAvailability{fn: func(string) bool {
				availabilityCalls.Add(1)
				return true
			}}
			controller := New(Options{FnReg: reg})
			controller.RegisterModules(collectorapi.Registry{
				"mod": collectorapi.Creator{
					InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "a"}}
					},
				},
			})

			controller.OnJobStart(job)

			assert.Equal(t, int32(0), availabilityCalls.Load())
			assert.Empty(t, reg.registeredNames())

			controller.ReconcileModuleMethods("mod")

			assert.Equal(t, int32(1), availabilityCalls.Load())
			assert.Equal(t, []string{"mod:a"}, reg.registeredNames())
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}

func TestControllerPublishedFunctionWrapperConcurrentMutation(t *testing.T) {
	tests := map[string]struct {
		setup  func(*Controller)
		mutate func(*Controller)
	}{
		"module cleanup": {
			setup: func(controller *Controller) {
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						SharedFunctions: func() []funcapi.FunctionConfig {
							return []funcapi.FunctionConfig{{ID: "a"}}
						},
						MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
							return &tableTestHandler{}
						},
					},
				})
				controller.OnJobStart(newTestRuntimeJob("mod", "job0", true))
				controller.ReconcileModuleMethods("mod")
			},
			mutate: func(controller *Controller) {
				for i := range 100 {
					controller.Cleanup()
					controller.OnJobStart(newTestRuntimeJob("mod", fmt.Sprintf("job%d", i+1), true))
					controller.ReconcileModuleMethods("mod")
				}
			},
		},
		"job stop": {
			setup: func(controller *Controller) {
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
							return []funcapi.FunctionConfig{{ID: "a"}}
						},
						MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
							return &tableTestHandler{}
						},
					},
				})
				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.ReconcileModuleMethods("mod")
			},
			mutate: func(controller *Controller) {
				job := newTestRuntimeJob("mod", "job1", true)
				for range 100 {
					controller.OnJobStop(job)
					controller.OnJobStart(job)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := newTestFunctionRegistry()
			firstHandler := make(chan func(functions.Function), 1)
			var captureOnce sync.Once
			reg.onRegister = func(name string, fn func(functions.Function)) {
				if name == "mod:a" {
					captureOnce.Do(func() { firstHandler <- fn })
				}
			}
			controller := New(Options{FnReg: reg})
			tc.setup(controller)

			var handler func(functions.Function)
			select {
			case handler = <-firstHandler:
			case <-time.After(2 * time.Second):
				t.Fatal("published handler was not registered")
			}

			start := make(chan struct{})
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					<-start
					for range 100 {
						handler(functions.Function{UID: "wrapped-dispatch", Timeout: time.Second})
					}
				})
			}

			wg.Go(func() {
				<-start
				tc.mutate(controller)
			})

			close(start)
			wg.Wait()
		})
	}
}

func TestControllerPublishedFunctionGenerationInvalidatesStaleHandlers(t *testing.T) {
	tests := map[string]struct {
		setup func(*Controller)
	}{
		"module method after cleanup": {
			setup: func(controller *Controller) {
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						SharedFunctions: func() []funcapi.FunctionConfig {
							return []funcapi.FunctionConfig{{ID: "a"}}
						},
						MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
							return &tableTestHandler{}
						},
					},
				})
				controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
				controller.ReconcileModuleMethods("mod")
				controller.Cleanup()
			},
		},
		"instance function after job stop": {
			setup: func(controller *Controller) {
				controller.RegisterModules(collectorapi.Registry{
					"mod": collectorapi.Creator{
						InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
							return []funcapi.FunctionConfig{{ID: "a"}}
						},
						MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
							return &tableTestHandler{}
						},
					},
				})
				job := newTestRuntimeJob("mod", "job1", true)
				controller.OnJobStart(job)
				controller.ReconcileModuleMethods("mod")
				controller.OnJobStop(job)
				controller.ReconcileModuleMethods("mod")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotCode int
			var gotResp map[string]any
			var stale func(functions.Function)
			reg := newTestFunctionRegistry()
			reg.onRegister = func(name string, fn func(functions.Function)) {
				if name == "mod:a" {
					stale = fn
				}
			}
			controller := New(Options{
				FnReg: reg,
				JSONWriter: func(data []byte, code int) {
					gotCode = code
					require.NoError(t, json.Unmarshal(data, &gotResp))
				},
			})

			tc.setup(controller)
			require.NotNil(t, stale)

			stale(functions.Function{UID: "stale-handler", Timeout: time.Second})

			assert.Equal(t, 404, gotCode)
			assert.Equal(t, float64(404), gotResp["status"])
			assert.Equal(t, "unknown function 'mod:a'", gotResp["errorMessage"])
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
	controller.ReconcileModuleMethods("mod")

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
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{
						ID:         "logs",
						RawRequest: true,
					}}
				},
			}, "mod:logs")
		})
	}
}

func TestControllerRawAgentScopeModuleMethodDoesNotRequireRunningJob(t *testing.T) {
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
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID:         "logs",
					RawRequest: true,
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
		UID:     "raw-agent-scope",
		Timeout: time.Second,
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Nil(t, gotJob)
}

func TestControllerRawSingleInstanceAgentScopeModuleMethodUsesRunningJob(t *testing.T) {
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
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID:         "logs",
					RawRequest: true,
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
	controller.ReconcileModuleMethods("mod")

	reg.call("mod:logs", context.Background(), functions.Function{
		UID:     "raw-single-agent-scope",
		Timeout: time.Second,
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Same(t, job, gotJob)
}

func TestControllerSingleInstanceAgentScopeModuleMethodUsesRunningJob(t *testing.T) {
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
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID: "status",
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
	controller.ReconcileModuleMethods("mod")

	reg.call("mod:status", context.Background(), functions.Function{
		UID:     "single-agent-scope",
		Timeout: time.Second,
		Payload: []byte(`{"scope":"all"}`),
	})

	assert.Equal(t, 200, gotCode)
	assert.Equal(t, float64(200), gotResp["status"])
	assert.Same(t, job, gotJob)
	assert.Equal(t, []any{"scope"}, gotResp["accepted_params"])
}

func TestControllerSingleInstanceAgentScopeModuleMethodRequiresPublishedAvailableJob(t *testing.T) {
	tests := map[string]struct {
		setup   func(*Controller)
		message string
	}{
		"before start": {
			message: "unknown function 'mod:status'",
		},
		"after stop": {
			setup: func(controller *Controller) {
				job := newTestRuntimeJob("mod", "mod", true)
				controller.OnJobStart(job)
				controller.ReconcileModuleMethods("mod")
				controller.OnJobStop(job)
				controller.ReconcileModuleMethods("mod")
			},
			message: "unknown function 'mod:status'",
		},
		"registered but not running": {
			setup: func(controller *Controller) {
				controller.OnJobStart(newTestRuntimeJob("mod", "mod", false))
			},
			message: "unknown function 'mod:status'",
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
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID: "status",
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

			controller.ExecuteFunction("mod:status", functions.Function{
				UID:     "single-agent-scope-missing-job",
				Timeout: time.Second,
			})

			assert.Equal(t, 404, gotCode)
			assert.Equal(t, float64(404), gotResp["status"])
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
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "query"}}
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
	controller.ReconcileModuleMethods("mod")

	reg.call("mod:query", ctx, functions.Function{
		UID:     "normal-cancel",
		Timeout: time.Second,
	})

	assert.Equal(t, 499, gotCode)
	assert.Equal(t, float64(499), gotResp["status"])
	assert.Equal(t, "Request cancelled.", gotResp["errorMessage"])
}

func TestControllerRawInstanceFunctionRequest(t *testing.T) {
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
				InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{
						ID:         job.Name() + ":logs",
						RawRequest: true,
					}}
				},
			}, "mod:job1:logs")
		})
	}
}

func TestControllerRawInstanceFunctionRequiresRawHandler(t *testing.T) {
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
			InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: job.Name() + ":logs", RawRequest: true}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &tableTestHandler{}
			},
		},
	})
	controller.OnJobStart(newTestRuntimeJob("mod", "job1", true))
	controller.ReconcileModuleMethods("mod")

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
	collector  any
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
func (j *testRuntimeJob) Collector() any     { return j.collector }

type testFunctionAvailability struct {
	available map[string]bool
	fn        func(string) bool
}

func (a *testFunctionAvailability) FunctionAvailable(functionID string) bool {
	if a.fn != nil {
		return a.fn(functionID)
	}
	return a.available[functionID]
}

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
	mu              sync.Mutex
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
	r.mu.Lock()
	r.handlers[name] = fn
	r.registered = append(r.registered, name)
	onRegister := r.onRegister
	r.mu.Unlock()

	if onRegister != nil {
		onRegister(name, fn)
	}
}

func (r *testFunctionRegistry) RegisterWithContext(name string, fn functions.Handler) {
	legacy := func(f functions.Function) {
		fn(context.Background(), f)
	}
	r.mu.Lock()
	r.handlers[name] = legacy
	r.contextHandlers[name] = fn
	r.registered = append(r.registered, name)
	onRegister := r.onRegister
	r.mu.Unlock()

	if onRegister != nil {
		onRegister(name, legacy)
	}
}

func (r *testFunctionRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.unregistered = append(r.unregistered, name)
	delete(r.handlers, name)
	delete(r.contextHandlers, name)
}

func (r *testFunctionRegistry) RegisterPrefix(string, string, func(functions.Function)) {}
func (r *testFunctionRegistry) UnregisterPrefix(string, string)                         {}

func (r *testFunctionRegistry) RegisterPrefixWithContext(string, string, functions.Handler) {}

func (r *testFunctionRegistry) call(name string, ctx context.Context, fn functions.Function) {
	r.mu.Lock()
	handler := r.contextHandlers[name]
	legacy := r.handlers[name]
	r.mu.Unlock()

	if handler != nil {
		handler(ctx, fn)
		return
	}
	legacy(fn)
}

func (r *testFunctionRegistry) registeredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.registered))
	copy(out, r.registered)
	return out
}

func (r *testFunctionRegistry) unregisteredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.unregistered))
	copy(out, r.unregistered)
	return out
}

type blockingFunctionWriter struct {
	started     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
}

func newBlockingFunctionWriter() *blockingFunctionWriter {
	return &blockingFunctionWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingFunctionWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(p), nil
}

func (w *blockingFunctionWriter) Release() {
	w.releaseOnce.Do(func() { close(w.release) })
}
