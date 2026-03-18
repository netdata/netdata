// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type moduleFuncRegistry struct {
	mu      sync.RWMutex
	modules map[string]*moduleFunc
}

type moduleFunc struct {
	creator        collectorapi.Creator
	methods        []funcapi.MethodConfig
	methodsByID    map[string]funcapi.MethodConfig
	jobs           map[string]*jobEntry
	nextGeneration uint64
	jobMethods     map[string][]funcapi.MethodConfig
}

type jobEntry struct {
	job        collectorapi.RuntimeJob
	generation uint64
}

func newModuleFuncRegistry() *moduleFuncRegistry {
	return &moduleFuncRegistry{
		modules: make(map[string]*moduleFunc),
	}
}

func (r *moduleFuncRegistry) registerModule(name string, creator collectorapi.Creator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var methods []funcapi.MethodConfig
	if creator.Methods != nil {
		methods = creator.Methods()
	}

	r.modules[name] = &moduleFunc{
		creator:     creator,
		methods:     methods,
		methodsByID: indexMethods(methods),
		jobs:        make(map[string]*jobEntry),
		jobMethods:  make(map[string][]funcapi.MethodConfig),
	}
}

func indexMethods(methods []funcapi.MethodConfig) map[string]funcapi.MethodConfig {
	if len(methods) == 0 {
		return nil
	}

	idx := make(map[string]funcapi.MethodConfig, len(methods))
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		idx[method.ID] = method
	}
	return idx
}

func (r *moduleFuncRegistry) addJob(moduleName, jobName string, job collectorapi.RuntimeJob) {
	r.mu.Lock()
	defer r.mu.Unlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return
	}

	module.nextGeneration++
	newGen := module.nextGeneration
	module.jobs[jobName] = &jobEntry{
		job:        job,
		generation: newGen,
	}
}

func (r *moduleFuncRegistry) removeJob(moduleName, jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if module, ok := r.modules[moduleName]; ok {
		delete(module.jobs, jobName)
	}
}

func (r *moduleFuncRegistry) getJobWithGeneration(moduleName, jobName string) (collectorapi.RuntimeJob, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil, 0
	}
	entry, ok := module.jobs[jobName]
	if !ok {
		return nil, 0
	}
	return entry.job, entry.generation
}

func (r *moduleFuncRegistry) verifyJobGeneration(moduleName, jobName string, expectedGen uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return false
	}
	entry, ok := module.jobs[jobName]
	if !ok {
		return false
	}
	if !entry.job.IsRunning() {
		return false
	}
	return entry.generation == expectedGen
}

func (r *moduleFuncRegistry) getMethod(moduleName, methodID string) (*funcapi.MethodConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok || module.methodsByID == nil {
		return nil, false
	}
	cfg, ok := module.methodsByID[methodID]
	if !ok {
		return nil, false
	}
	return &cfg, true
}

func (r *moduleFuncRegistry) getMethods(moduleName string) []funcapi.MethodConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return module.methods
}

func (r *moduleFuncRegistry) getJobNames(moduleName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil
	}

	names := make([]string, 0, len(module.jobs))
	for name := range module.jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *moduleFuncRegistry) getJob(moduleName, jobName string) (collectorapi.RuntimeJob, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil, false
	}
	entry, ok := module.jobs[jobName]
	if !ok {
		return nil, false
	}
	return entry.job, true
}

func (r *moduleFuncRegistry) getCreator(moduleName string) (collectorapi.Creator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return collectorapi.Creator{}, false
	}
	return module.creator, true
}

func (r *moduleFuncRegistry) isModuleRegistered(moduleName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.modules[moduleName]
	return ok
}

func (r *moduleFuncRegistry) registerJobMethods(moduleName, jobName string, methods []funcapi.MethodConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return
	}
	module.jobMethods[jobName] = methods
}

func (r *moduleFuncRegistry) unregisterJobMethods(moduleName, jobName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return
	}
	delete(module.jobMethods, jobName)
}

func (r *moduleFuncRegistry) getJobMethods(moduleName, jobName string) []funcapi.MethodConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return module.jobMethods[jobName]
}

func (r *moduleFuncRegistry) getJobMethod(moduleName, jobName, methodID string) (*funcapi.MethodConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil, false
	}
	methods, ok := module.jobMethods[jobName]
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

func (r *moduleFuncRegistry) findMethodCollision(moduleName, jobName, methodID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return "", false
	}

	if module.methodsByID != nil {
		if _, exists := module.methodsByID[methodID]; exists {
			return "static method", true
		}
	}

	for ownerJob, methods := range module.jobMethods {
		if ownerJob == jobName {
			continue
		}
		for _, method := range methods {
			if method.ID == methodID {
				return "job method on " + ownerJob, true
			}
		}
	}

	return "", false
}

func (r *moduleFuncRegistry) snapshotCreators() map[string]collectorapi.Creator {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]collectorapi.Creator, len(r.modules))
	for name, module := range r.modules {
		out[name] = module.creator
	}
	return out
}
