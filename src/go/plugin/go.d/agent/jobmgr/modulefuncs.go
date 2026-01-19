// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// moduleFuncRegistry tracks module-level functions and their running jobs
type moduleFuncRegistry struct {
	mu sync.RWMutex

	// moduleName → moduleFunc
	// e.g., "postgres" → {methods: [...], jobs: {"master-db": job1, "replica-db": job2}}
	modules map[string]*moduleFunc
}

type moduleFunc struct {
	creator module.Creator        // The module creator (has Methods())
	methods []module.MethodConfig // Static methods from creator
	jobs    map[string]*jobEntry  // jobName → job entry with generation
}

// jobEntry wraps a job with a generation number for race detection
type jobEntry struct {
	job        *module.Job
	generation uint64 // Incremented each time this job name is replaced
}

func newModuleFuncRegistry() *moduleFuncRegistry {
	return &moduleFuncRegistry{
		modules: make(map[string]*moduleFunc),
	}
}

// registerModule is called at startup for each module that implements FunctionProvider
func (r *moduleFuncRegistry) registerModule(name string, creator module.Creator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var methods []module.MethodConfig
	if creator.Methods != nil {
		methods = creator.Methods()
	}

	r.modules[name] = &moduleFunc{
		creator: creator,
		methods: methods,
		jobs:    make(map[string]*jobEntry),
	}
}

// addJob is called when jobs start.
// NOTE: Job names MUST be unique per module. If a job with the same name
// already exists, it is replaced (this handles config reload scenarios)
func (r *moduleFuncRegistry) addJob(moduleName, jobName string, job *module.Job) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return // Module not registered (doesn't implement FunctionProvider)
	}

	// Replace any existing job with the same name
	// Increment generation to invalidate any in-flight requests to old job
	oldGen := uint64(0)
	if existing, ok := mf.jobs[jobName]; ok {
		oldGen = existing.generation
	}
	mf.jobs[jobName] = &jobEntry{
		job:        job,
		generation: oldGen + 1,
	}
}

// removeJob is called when jobs stop
func (r *moduleFuncRegistry) removeJob(moduleName, jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if mf, ok := r.modules[moduleName]; ok {
		delete(mf.jobs, jobName)
	}
}

// getJobWithGeneration returns the job and its generation for race detection
func (r *moduleFuncRegistry) getJobWithGeneration(moduleName, jobName string) (*module.Job, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil, 0
	}
	entry, ok := mf.jobs[jobName]
	if !ok {
		return nil, 0
	}
	return entry.job, entry.generation
}

// verifyJobGeneration checks if the job still has the expected generation
// Returns false if the job was replaced OR stopped during request processing
func (r *moduleFuncRegistry) verifyJobGeneration(moduleName, jobName string, expectedGen uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return false
	}
	entry, ok := mf.jobs[jobName]
	if !ok {
		return false // Job was removed
	}

	// CRITICAL: Also check if job is still running
	// The job could be stopped (but not yet removed) during our request
	// This catches the race where generation matches but job.Stop() was called
	if !entry.job.IsRunning() {
		return false // Job was stopped
	}

	return entry.generation == expectedGen
}

// getMethods returns the method configurations for a module
func (r *moduleFuncRegistry) getMethods(moduleName string) []module.MethodConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return mf.methods
}

// getJobNames returns job names in STABLE alphabetical order
// This ensures consistent UI presentation across refreshes
func (r *moduleFuncRegistry) getJobNames(moduleName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil
	}

	// Extract job names and sort for stable ordering
	names := make([]string, 0, len(mf.jobs))
	for name := range mf.jobs {
		names = append(names, name)
	}
	sort.Strings(names) // Alphabetical order for consistent UI
	return names
}

// getJob returns the job by name for routing requests
func (r *moduleFuncRegistry) getJob(moduleName, jobName string) (*module.Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil, false
	}
	entry, ok := mf.jobs[jobName]
	if !ok {
		return nil, false
	}
	return entry.job, true
}

// getCreator returns the module creator for a registered module
func (r *moduleFuncRegistry) getCreator(moduleName string) (module.Creator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return module.Creator{}, false
	}
	return mf.creator, true
}

// isModuleRegistered checks if a module is registered (implements FunctionProvider)
func (r *moduleFuncRegistry) isModuleRegistered(moduleName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.modules[moduleName]
	return ok
}
