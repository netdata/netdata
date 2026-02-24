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

func TestModuleFuncRegistry_AddRemoveJob(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	// Create test jobs
	job1 := newTestModuleFuncsJob("job1")
	job2 := newTestModuleFuncsJob("job2")

	// Add jobs
	r.addJob("postgres", "job1", job1)
	r.addJob("postgres", "job2", job2)

	// Verify jobs are retrievable
	names := r.getJobNames("postgres")
	assert.ElementsMatch(t, []string{"job1", "job2"}, names)

	got1, ok := r.getJob("postgres", "job1")
	assert.True(t, ok)
	assert.Equal(t, job1, got1)

	// Remove job
	r.removeJob("postgres", "job1")

	names = r.getJobNames("postgres")
	assert.ElementsMatch(t, []string{"job2"}, names)

	_, ok = r.getJob("postgres", "job1")
	assert.False(t, ok)
}

func TestModuleFuncRegistry_JobReplacement(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	job1 := newTestModuleFuncsJob("master")
	job2 := newTestModuleFuncsJob("master") // Same name, different instance

	// Add first job
	r.addJob("postgres", "master", job1)
	_, gen1 := r.getJobWithGeneration("postgres", "master")
	assert.Equal(t, uint64(1), gen1)

	// Replace with second job
	r.addJob("postgres", "master", job2)
	got, gen2 := r.getJobWithGeneration("postgres", "master")
	assert.Equal(t, uint64(2), gen2) // Generation incremented
	assert.Equal(t, job2, got)       // New job returned
}

func TestModuleFuncRegistry_GenerationVerification(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	job := newTestModuleFuncsJob("master")

	r.addJob("postgres", "master", job)
	_, gen := r.getJobWithGeneration("postgres", "master")

	// Note: verifyJobGeneration checks BOTH generation AND IsRunning()
	// Since our test job isn't running, verification should fail
	// This is actually correct behavior - it catches stopped jobs

	// Verify with wrong generation - should fail
	assert.False(t, r.verifyJobGeneration("postgres", "master", gen+1))

	// Remove job and verify - should fail
	r.removeJob("postgres", "master")
	assert.False(t, r.verifyJobGeneration("postgres", "master", gen))
}

func TestModuleFuncRegistry_GetMethods(t *testing.T) {
	r := newModuleFuncRegistry()

	expectedMethods := []funcapi.MethodConfig{
		{ID: "top-queries", Name: "Top Queries"},
	}

	r.registerModule("postgres", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return expectedMethods
		},
	})

	methods := r.getMethods("postgres")
	assert.Equal(t, expectedMethods, methods)

	// Non-existent module
	assert.Nil(t, r.getMethods("nonexistent"))
}

func TestModuleFuncRegistry_GetJobNames_Sorted(t *testing.T) {
	r := newModuleFuncRegistry()
	r.registerModule("postgres", collectorapi.Creator{})

	// Add jobs in random order
	r.addJob("postgres", "zebra-db", newTestModuleFuncsJob("zebra"))
	r.addJob("postgres", "alpha-db", newTestModuleFuncsJob("alpha"))
	r.addJob("postgres", "middle-db", newTestModuleFuncsJob("middle"))

	names := r.getJobNames("postgres")

	// Should be sorted alphabetically
	assert.Equal(t, []string{"alpha-db", "middle-db", "zebra-db"}, names)
}

func TestModuleFuncRegistry_UnregisteredModule(t *testing.T) {
	r := newModuleFuncRegistry()

	// Operations on unregistered module should be no-ops
	r.addJob("nonexistent", "job1", newTestModuleFuncsJob("job1"))
	r.removeJob("nonexistent", "job1")

	assert.False(t, r.isModuleRegistered("nonexistent"))
	assert.Nil(t, r.getJobNames("nonexistent"))
	assert.Nil(t, r.getMethods("nonexistent"))

	_, ok := r.getJob("nonexistent", "job1")
	assert.False(t, ok)
}

func TestModuleFuncRegistry_GetCreator(t *testing.T) {
	r := newModuleFuncRegistry()

	creator := collectorapi.Creator{
		JobConfigSchema: "test-schema",
	}
	r.registerModule("postgres", creator)

	got, ok := r.getCreator("postgres")
	require.True(t, ok)
	assert.Equal(t, "test-schema", got.JobConfigSchema)

	// Non-existent module
	_, ok = r.getCreator("nonexistent")
	assert.False(t, ok)
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
