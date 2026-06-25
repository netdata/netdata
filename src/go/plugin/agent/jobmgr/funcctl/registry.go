// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type moduleFuncRegistry struct {
	mu           sync.RWMutex
	modules      map[string]*moduleFunc
	methodRoutes map[string]methodRoute
}

type moduleFunctionKind uint8

const (
	moduleFunctionShared moduleFunctionKind = iota + 1
	moduleFunctionAgent
)

type methodRoute struct {
	moduleName string
	methodID   string
}

type moduleFunc struct {
	creator        collectorapi.Creator
	methods        []funcapi.FunctionConfig
	methodsByID    map[string]funcapi.FunctionConfig
	methodKinds    map[string]moduleFunctionKind
	jobs           map[string]*jobEntry
	nextGeneration uint64
	jobMethods     map[string][]funcapi.FunctionConfig
}

type jobEntry struct {
	job        collectorapi.RuntimeJob
	generation uint64
}

type jobSnapshot struct {
	name string
	job  collectorapi.RuntimeJob
}

func newModuleFuncRegistry() *moduleFuncRegistry {
	return &moduleFuncRegistry{
		modules:      make(map[string]*moduleFunc),
		methodRoutes: make(map[string]methodRoute),
	}
}

func (r *moduleFuncRegistry) registerModule(name string, creator collectorapi.Creator) {
	r.registerModuleWithDeclarations(name, creator, moduleFunctionDeclarations(creator))
}

func (r *moduleFuncRegistry) registerModuleWithMethods(name string, creator collectorapi.Creator, methods []funcapi.FunctionConfig) {
	r.registerModuleWithDeclarations(name, creator, moduleFunctionDecls{
		configs: methods,
		kinds:   indexFunctionKinds(methods, moduleFunctionShared),
	})
}

func (r *moduleFuncRegistry) registerModuleWithDeclarations(name string, creator collectorapi.Creator, functions moduleFunctionDecls) {
	r.mu.Lock()
	defer r.mu.Unlock()

	affectedRoutes := make(map[string]struct{})
	if existing, ok := r.modules[name]; ok {
		collectModuleFunctionNames(name, existing.methods, affectedRoutes)
	}
	collectModuleFunctionNames(name, functions.configs, affectedRoutes)

	r.modules[name] = &moduleFunc{
		creator:     creator,
		methods:     functions.configs,
		methodsByID: indexMethods(functions.configs),
		methodKinds: functions.kinds,
		jobs:        make(map[string]*jobEntry),
		jobMethods:  make(map[string][]funcapi.FunctionConfig),
	}
	r.refreshModuleMethodRoutesLocked(affectedRoutes)
}

func indexMethods(methods []funcapi.FunctionConfig) map[string]funcapi.FunctionConfig {
	if len(methods) == 0 {
		return nil
	}

	idx := make(map[string]funcapi.FunctionConfig, len(methods))
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		idx[method.ID] = method
	}
	return idx
}

func indexModuleFunctionKinds(shared, agent []funcapi.FunctionConfig) map[string]moduleFunctionKind {
	idx := indexFunctionKinds(shared, moduleFunctionShared)
	if len(agent) != 0 && idx == nil {
		idx = make(map[string]moduleFunctionKind, len(agent))
	}
	for _, method := range agent {
		idx[method.ID] = moduleFunctionAgent
	}
	if len(idx) == 0 {
		return nil
	}
	return idx
}

func indexFunctionKinds(methods []funcapi.FunctionConfig, kind moduleFunctionKind) map[string]moduleFunctionKind {
	if len(methods) == 0 {
		return nil
	}
	idx := make(map[string]moduleFunctionKind)
	for _, method := range methods {
		idx[method.ID] = kind
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

func (r *moduleFuncRegistry) getMethod(moduleName, methodID string) (*funcapi.FunctionConfig, moduleFunctionKind, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok || module.methodsByID == nil {
		return nil, 0, false
	}
	cfg, ok := module.methodsByID[methodID]
	if !ok {
		return nil, 0, false
	}
	return &cfg, module.methodKinds[methodID], true
}

func (r *moduleFuncRegistry) getModuleFunctionKind(moduleName, methodID string) moduleFunctionKind {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return 0
	}
	return module.methodKinds[methodID]
}

func (r *moduleFuncRegistry) resolveMethodRoute(functionName string) (string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	route, ok := r.methodRoutes[functionName]
	if !ok {
		return "", "", false
	}
	return route.moduleName, route.methodID, true
}

func (r *moduleFuncRegistry) getMethods(moduleName string) []funcapi.FunctionConfig {
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

func (r *moduleFuncRegistry) getJobSnapshots(moduleName string) []jobSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil
	}

	snapshots := make([]jobSnapshot, 0, len(module.jobs))
	for name, entry := range module.jobs {
		snapshots = append(snapshots, jobSnapshot{
			name: name,
			job:  entry.job,
		})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].name < snapshots[j].name
	})
	return snapshots
}

func (r *moduleFuncRegistry) hasRunningJob(moduleName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return false
	}
	for _, entry := range module.jobs {
		if entry.job.IsRunning() {
			return true
		}
	}
	return false
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

func (r *moduleFuncRegistry) registerJobMethods(moduleName, jobName string, methods []funcapi.FunctionConfig) {
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

func (r *moduleFuncRegistry) getJobMethods(moduleName, jobName string) []funcapi.FunctionConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[moduleName]
	if !ok {
		return nil
	}
	return module.jobMethods[jobName]
}

func (r *moduleFuncRegistry) getJobMethod(moduleName, jobName, methodID string) (*funcapi.FunctionConfig, bool) {
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

func collectModuleFunctionNames(moduleName string, methods []funcapi.FunctionConfig, out map[string]struct{}) {
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		for _, functionName := range funcapi.FunctionNames(moduleName, method) {
			out[functionName] = struct{}{}
		}
	}
}

func (r *moduleFuncRegistry) refreshModuleMethodRoutesLocked(functionNames map[string]struct{}) {
	if len(functionNames) == 0 {
		return
	}
	for functionName := range functionNames {
		delete(r.methodRoutes, functionName)
	}

	moduleNames := make([]string, 0, len(r.modules))
	for moduleName := range r.modules {
		moduleNames = append(moduleNames, moduleName)
	}
	sort.Strings(moduleNames)

	for _, moduleName := range moduleNames {
		module := r.modules[moduleName]
		for _, method := range module.methods {
			if method.ID == "" {
				continue
			}
			route := methodRoute{moduleName: moduleName, methodID: method.ID}
			for _, functionName := range funcapi.FunctionNames(moduleName, method) {
				if _, affected := functionNames[functionName]; !affected {
					continue
				}
				if _, exists := r.methodRoutes[functionName]; exists {
					continue
				}
				r.methodRoutes[functionName] = route
			}
		}
	}
}
