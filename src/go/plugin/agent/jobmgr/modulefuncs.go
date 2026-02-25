// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// moduleFuncRegistry tracks module-level functions and their running jobs
type moduleFuncRegistry struct {
	mu sync.RWMutex

	// moduleName → moduleFunc
	// e.g., "postgres" → {methods: [...], jobs: {"master-db": job1, "replica-db": job2}}
	modules map[string]*moduleFunc
}

type moduleFunc struct {
	creator        collectorapi.Creator   // The module creator (has Methods())
	methods        []funcapi.MethodConfig // Static methods from creator (ordered)
	methodsByID    map[string]funcapi.MethodConfig
	jobs           map[string]*jobEntry              // jobName → job entry with generation
	lastGeneration map[string]uint64                 // jobName → last known generation (persists across removals)
	jobMethods     map[string][]funcapi.MethodConfig // jobName → methods registered for that job
}

// jobEntry wraps a job with a generation number for race detection
type jobEntry struct {
	job        collectorapi.RuntimeJob
	generation uint64 // Incremented each time this job name is replaced
}

func newModuleFuncRegistry() *moduleFuncRegistry {
	return &moduleFuncRegistry{
		modules: make(map[string]*moduleFunc),
	}
}

// registerModule is called at startup for each module that implements FunctionProvider
func (r *moduleFuncRegistry) registerModule(name string, creator collectorapi.Creator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var methods []funcapi.MethodConfig
	if creator.Methods != nil {
		methods = creator.Methods()
	}

	r.modules[name] = &moduleFunc{
		creator:        creator,
		methods:        methods,
		methodsByID:    indexMethods(methods),
		jobs:           make(map[string]*jobEntry),
		lastGeneration: make(map[string]uint64),
		jobMethods:     make(map[string][]funcapi.MethodConfig),
	}
}

func indexMethods(methods []funcapi.MethodConfig) map[string]funcapi.MethodConfig {
	if len(methods) == 0 {
		return nil
	}
	idx := make(map[string]funcapi.MethodConfig, len(methods))
	for _, m := range methods {
		if m.ID == "" {
			continue
		}
		idx[m.ID] = m
	}
	return idx
}

// addJob is called when jobs start.
// NOTE: Job names MUST be unique per module. If a job with the same name
// already exists, it is replaced (this handles config reload scenarios)
func (r *moduleFuncRegistry) addJob(moduleName, jobName string, job collectorapi.RuntimeJob) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return // CollectorV1 not registered (doesn't implement FunctionProvider)
	}

	// Replace any existing job with the same name
	// Increment generation to invalidate any in-flight requests to old job
	// Use lastGeneration to ensure generation monotonically increases even across removals
	lastGen := mf.lastGeneration[jobName]
	newGen := lastGen + 1
	mf.jobs[jobName] = &jobEntry{
		job:        job,
		generation: newGen,
	}
	mf.lastGeneration[jobName] = newGen
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
func (r *moduleFuncRegistry) getJobWithGeneration(moduleName, jobName string) (collectorapi.RuntimeJob, uint64) {
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
func (r *moduleFuncRegistry) getMethods(moduleName string) []funcapi.MethodConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return mf.methods
}

// getMethod returns a method config by ID for a module.
func (r *moduleFuncRegistry) getMethod(moduleName, methodID string) (*funcapi.MethodConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok || mf.methodsByID == nil {
		return nil, false
	}
	cfg, ok := mf.methodsByID[methodID]
	if !ok {
		return nil, false
	}
	return &cfg, true
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
func (r *moduleFuncRegistry) getJob(moduleName, jobName string) (collectorapi.RuntimeJob, bool) {
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
func (r *moduleFuncRegistry) getCreator(moduleName string) (collectorapi.Creator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return collectorapi.Creator{}, false
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

// registerJobMethods stores methods registered for a specific job
func (r *moduleFuncRegistry) registerJobMethods(moduleName, jobName string, methods []funcapi.MethodConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return
	}
	mf.jobMethods[jobName] = methods
}

// unregisterJobMethods removes methods registered for a specific job
func (r *moduleFuncRegistry) unregisterJobMethods(moduleName, jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return
	}
	delete(mf.jobMethods, jobName)
}

// getJobMethods returns methods registered for a specific job
func (r *moduleFuncRegistry) getJobMethods(moduleName, jobName string) []funcapi.MethodConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return mf.jobMethods[jobName]
}

// getJobMethod returns a specific method registered for a job by method ID
func (r *moduleFuncRegistry) getJobMethod(moduleName, jobName, methodID string) (*funcapi.MethodConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mf, ok := r.modules[moduleName]
	if !ok {
		return nil, false
	}
	methods, ok := mf.jobMethods[jobName]
	if !ok {
		return nil, false
	}
	for i := range methods {
		if methods[i].ID == methodID {
			return &methods[i], true
		}
	}
	return nil, false
}
