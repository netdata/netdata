// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			expected: []string{"mysql", "mssql", "postgres"}, // sorted
		},
		"duplicate registration overwrites": {
			modules:  []string{"postgres", "postgres"},
			expected: []string{"postgres"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := newModuleFuncRegistry()

			for _, m := range tc.modules {
				r.registerModule(m, collectorapi.Creator{
					Methods: func() []funcapi.MethodConfig {
						return []funcapi.MethodConfig{{ID: "test"}}
					},
				})
			}

			assert.Equal(t, len(tc.expected), len(r.modules))
			for _, m := range tc.expected {
				assert.True(t, r.isModuleRegistered(m))
			}
		})
	}
}

func TestModuleFuncRegistry_Operations(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, r *moduleFuncRegistry)
	}{
		"add/remove job": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.registerModule("postgres", collectorapi.Creator{})

				job1 := newTestModuleFuncsJob("job1")
				job2 := newTestModuleFuncsJob("job2")

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

				job1 := newTestModuleFuncsJob("master")
				job2 := newTestModuleFuncsJob("master")

				r.addJob("postgres", "master", job1)
				_, gen1 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(1), gen1)

				r.addJob("postgres", "master", job2)
				got, gen2 := r.getJobWithGeneration("postgres", "master")
				assert.Equal(t, uint64(2), gen2)
				assert.Equal(t, job2, got)
			},
		},
		"generation verification fails on wrong generation and missing job": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.registerModule("postgres", collectorapi.Creator{})

				job := newTestModuleFuncsJob("master")
				r.addJob("postgres", "master", job)
				_, gen := r.getJobWithGeneration("postgres", "master")

				assert.False(t, r.verifyJobGeneration("postgres", "master", gen+1))
				r.removeJob("postgres", "master")
				assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
			},
		},
		"get methods": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				expectedMethods := []funcapi.MethodConfig{
					{ID: "top-queries", Name: "Top Queries"},
				}

				r.registerModule("postgres", collectorapi.Creator{
					Methods: func() []funcapi.MethodConfig {
						return expectedMethods
					},
				})

				assert.Equal(t, expectedMethods, r.getMethods("postgres"))
				assert.Nil(t, r.getMethods("nonexistent"))
			},
		},
		"get job names sorted": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.registerModule("postgres", collectorapi.Creator{})

				r.addJob("postgres", "zebra-db", newTestModuleFuncsJob("zebra"))
				r.addJob("postgres", "alpha-db", newTestModuleFuncsJob("alpha"))
				r.addJob("postgres", "middle-db", newTestModuleFuncsJob("middle"))

				assert.Equal(t, []string{"alpha-db", "middle-db", "zebra-db"}, r.getJobNames("postgres"))
			},
		},
		"operations on unregistered module are no-op": {
			run: func(t *testing.T, r *moduleFuncRegistry) {
				r.addJob("nonexistent", "job1", newTestModuleFuncsJob("job1"))
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
				creator := collectorapi.Creator{
					JobConfigSchema: "test-schema",
				}
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
			tc.run(t, newModuleFuncRegistry())
		})
	}
}

// newTestModuleFuncsJob creates a minimal job for testing modulefuncs
func newTestModuleFuncsJob(name string) *jobruntime.Job {
	return jobruntime.NewJob(jobruntime.JobConfig{
		PluginName:      "test",
		Name:            name,
		ModuleName:      "test",
		FullName:        "test_" + name,
		Module:          &collectorapi.MockCollectorV1{},
		Out:             io.Discard,
		UpdateEvery:     1,
		AutoDetectEvery: 0,
		Priority:        1000,
	})
}

// TestModuleFuncRegistry_Concurrency tests thread safety
func TestModuleFuncRegistry_Concurrency(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "test"}}
		},
	})

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			job := newTestModuleFuncsJob("job")
			r.addJob("postgres", "job", job)
			r.removeJob("postgres", "job")
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = r.getJobNames("postgres")
			_ = r.getMethods("postgres")
			_, _ = r.getJob("postgres", "job")
		}
		done <- true
	}()

	<-done
	<-done
}

// TestModuleFuncRegistry_VerifyJobGeneration_JobStopped tests race detection when job stops
func TestModuleFuncRegistry_VerifyJobGeneration_JobStopped(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	job := jobruntime.NewJob(jobruntime.JobConfig{
		PluginName: "test",
		Name:       "master",
		ModuleName: "postgres",
		FullName:   "postgres_master",
		Module: &collectorapi.MockCollectorV1{
			InitFunc:    func(context.Context) error { return nil },
			CheckFunc:   func(context.Context) error { return nil },
			ChartsFunc:  func() *collectorapi.Charts { return &collectorapi.Charts{} },
			CollectFunc: func(context.Context) map[string]int64 { return nil },
		},
		Out:             io.Discard,
		UpdateEvery:     1,
		AutoDetectEvery: 0,
		Priority:        1000,
	})

	r.addJob("postgres", "master", job)
	_, gen := r.getJobWithGeneration("postgres", "master")

	// Job exists but is not running - verification should fail
	// (job.IsRunning() returns false because job hasn't started)
	assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
}
