// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type Options struct {
	Logger     *logger.Logger
	FnReg      functions.Registry
	API        *dyncfg.Responder
	JSONWriter func([]byte, int)
}

type Controller struct {
	*logger.Logger

	api        *dyncfg.Responder
	jsonWriter func([]byte, int)
	fnReg      functions.Registry
	ctx        context.Context

	registry     *moduleFuncRegistry
	publishedMu  sync.Mutex
	publishedFns *publishedFunctionStore
}

type functionPublish struct {
	name    string
	handler functions.Handler
	opts    netdataapi.FunctionGlobalOpts
}

type desiredFunctionPublication struct {
	name       string
	owner      publishedFunctionOwner
	moduleName string
	jobName    string
	method     funcapi.FunctionConfig
}

func New(opts Options) *Controller {
	log := opts.Logger
	if log == nil {
		log = logger.New()
	}
	reg := opts.FnReg
	if reg == nil {
		reg = noopRegistry{}
	}

	return &Controller{
		Logger:       log,
		api:          opts.API,
		jsonWriter:   opts.JSONWriter,
		fnReg:        reg,
		registry:     newModuleFuncRegistry(),
		publishedFns: newPublishedFunctionStore(),
	}
}

func (c *Controller) Init(ctx context.Context) {
	c.ctx = ctx
}

func (c *Controller) SetAPI(api *dyncfg.Responder) {
	if api == nil {
		// Nil means "keep the current responder" rather than clearing output wiring.
		return
	}
	c.api = api
}

func (c *Controller) RegisterModules(modules collectorapi.Registry) {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)

	plannedPublicNames := make(map[string]string)
	for _, name := range names {
		creator := modules[name]
		if creator.SharedFunctions == nil && creator.AgentFunctions == nil && creator.InstanceFunctions == nil {
			continue
		}
		functions := moduleFunctionDeclarations(creator)
		if functionName, owner, collides := c.moduleMethodPublicNameCollision(name, functions.configs, plannedPublicNames); collides {
			c.Errorf("skipping function registration for module '%s': public function name '%s' collides with %s", name, functionName, owner)
			continue
		}
		c.registry.registerModuleWithDeclarations(name, creator, functions)
		c.registerAvailableAgentFunctions(name, functions.configs)
	}
}

type moduleFunctionDecls struct {
	configs []funcapi.FunctionConfig
	kinds   map[string]moduleFunctionKind
}

func moduleFunctionDeclarations(creator collectorapi.Creator) moduleFunctionDecls {
	shared := moduleSharedFunctions(creator)
	agent := moduleAgentFunctions(creator)
	configs := make([]funcapi.FunctionConfig, 0, len(shared)+len(agent))
	configs = append(configs, shared...)
	configs = append(configs, agent...)
	return moduleFunctionDecls{
		configs: configs,
		kinds:   indexModuleFunctionKinds(shared, agent),
	}
}

func moduleSharedFunctions(creator collectorapi.Creator) []funcapi.FunctionConfig {
	if creator.SharedFunctions == nil {
		return nil
	}
	return creator.SharedFunctions()
}

func moduleAgentFunctions(creator collectorapi.Creator) []funcapi.FunctionConfig {
	if creator.AgentFunctions == nil {
		return nil
	}
	return creator.AgentFunctions()
}

func (c *Controller) moduleMethodPublicNameCollision(moduleName string, methods []funcapi.FunctionConfig, planned map[string]string) (string, string, bool) {
	modulePlanned := make(map[string]string)
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		owner := fmt.Sprintf("%s:%s", moduleName, method.ID)
		for _, functionName := range funcapi.FunctionNames(moduleName, method) {
			if routeModule, routeMethod, ok := c.registry.resolveMethodRoute(functionName); ok {
				return functionName, fmt.Sprintf("%s:%s", routeModule, routeMethod), true
			}
			if existing := planned[functionName]; existing != "" && existing != owner {
				return functionName, existing, true
			}
			if existing := modulePlanned[functionName]; existing != "" && existing != owner {
				return functionName, existing, true
			}
			modulePlanned[functionName] = owner
		}
	}
	maps.Copy(planned, modulePlanned)
	return "", "", false
}

func (c *Controller) GetJobNames(moduleName string) []string {
	return c.registry.getJobNames(moduleName)
}

func (c *Controller) OnJobStart(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.registry.addJob(job.ModuleName(), job.Name(), job)

	creator, ok := c.registry.getCreator(job.ModuleName())
	if !ok || creator.InstanceFunctions == nil {
		return
	}

	methods := creator.InstanceFunctions(job)
	if len(methods) > 0 {
		c.registerInstanceFunctions(job, methods)
	}
}

// ReconcileModuleMethods rechecks static and instance Function availability for modules
// with at least one registered running job.
func (c *Controller) ReconcileModuleMethods(moduleName string) {
	if moduleName == "" {
		return
	}
	methods := c.registry.getMethods(moduleName)
	if c.registry.hasRunningJob(moduleName) {
		c.registerAvailableAgentFunctions(moduleName, methods)
	}
	c.reconcileModuleFunctions(moduleName, methods)
}

func (c *Controller) OnJobStop(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.unregisterInstanceFunctions(job)
	c.registry.removeJob(job.ModuleName(), job.Name())
}

func (c *Controller) Cleanup() {
	c.publishedMu.Lock()
	names := c.publishedFns.removeKinds(publishedFunctionShared, publishedFunctionAgent, publishedFunctionInstance)
	c.publishedMu.Unlock()

	sort.Strings(names)
	for _, funcName := range names {
		c.fnReg.Unregister(funcName)
	}
	if c.api != nil {
		for _, funcName := range names {
			c.api.FunctionRemove(funcName)
		}
	}
}

func (c *Controller) registerAvailableAgentFunctions(moduleName string, methods []funcapi.FunctionConfig) {
	publishes := c.collectAvailableAgentFunctionPublishes(moduleName, methods)
	for _, publish := range publishes {
		c.registerFunction(publish.name, publish.handler)
	}
	if c.api == nil {
		return
	}
	for _, publish := range publishes {
		c.api.FunctionGlobal(publish.opts)
	}
}

func (c *Controller) collectAvailableAgentFunctionPublishes(moduleName string, methods []funcapi.FunctionConfig) []functionPublish {
	var available []funcapi.FunctionConfig
	for i, method := range methods {
		kind := c.registry.getModuleFunctionKind(moduleName, method.ID)
		if kind != moduleFunctionAgent {
			continue
		}
		if !methodAvailable(method) {
			continue
		}
		if method.ID == "" {
			c.Once(fmt.Sprintf("funcctl:static-method:%s:%d:empty-id", moduleName, i)).
				Warningf("skipping function registration for module '%s': empty method ID", moduleName)
			continue
		}
		available = append(available, method)
	}

	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var publishes []functionPublish
	for _, method := range available {
		if c.allFunctionNamesPublishedLocked(moduleName, method) {
			continue
		}
		if c.api != nil {
			for _, funcName := range funcapi.FunctionNames(moduleName, method) {
				if c.publishedFns.has(funcName) {
					continue
				}
				record, _ := c.publishedFns.add(funcName, agentFunctionOwner(moduleName, method.ID))
				publishes = append(publishes, functionPublish{
					name:    funcName,
					handler: c.makePublishedMethodFuncHandler(funcName, record.generation, moduleName, method.ID),
					opts:    c.functionGlobalOpts(moduleName, method, funcName),
				})
			}
			continue
		}

		for _, funcName := range funcapi.FunctionNames(moduleName, method) {
			if c.publishedFns.has(funcName) {
				continue
			}
			record, _ := c.publishedFns.add(funcName, agentFunctionOwner(moduleName, method.ID))
			publishes = append(publishes, functionPublish{
				name:    funcName,
				handler: c.makePublishedMethodFuncHandler(funcName, record.generation, moduleName, method.ID),
			})
		}
	}
	return publishes
}

func (c *Controller) reconcileModuleFunctions(moduleName string, methods []funcapi.FunctionConfig) {
	desired := c.desiredModuleFunctions(moduleName, methods)
	publishes, withdraws := c.collectModuleFunctionChanges(moduleName, desired)
	for _, name := range withdraws {
		c.fnReg.Unregister(name)
		if c.api != nil {
			c.api.FunctionRemove(name)
		}
	}
	for _, publish := range publishes {
		c.registerFunction(publish.name, publish.handler)
	}
	if c.api == nil {
		return
	}
	for _, publish := range publishes {
		c.api.FunctionGlobal(publish.opts)
	}
}

func (c *Controller) desiredModuleFunctions(moduleName string, methods []funcapi.FunctionConfig) map[string]desiredFunctionPublication {
	desired := make(map[string]desiredFunctionPublication)

	for i, method := range methods {
		kind := c.registry.getModuleFunctionKind(moduleName, method.ID)
		if kind != moduleFunctionShared {
			continue
		}
		if method.ID == "" {
			c.Once(fmt.Sprintf("funcctl:static-method:%s:%d:empty-id", moduleName, i)).
				Warningf("skipping function registration for module '%s': empty method ID", moduleName)
			continue
		}

		availableJobs := c.availableSharedJobNames(moduleName, method.ID)
		if len(availableJobs) == 0 {
			continue
		}

		for _, funcName := range funcapi.FunctionNames(moduleName, method) {
			desired[funcName] = desiredFunctionPublication{
				name:       funcName,
				owner:      sharedFunctionOwner(moduleName, method.ID),
				moduleName: moduleName,
				method:     method,
			}
		}
	}

	for _, snapshot := range c.registry.getInstanceFunctionSnapshots(moduleName) {
		for _, method := range snapshot.methods {
			if method.ID == "" {
				continue
			}
			if !jobBackedFunctionAvailable(snapshot.job, method.ID) {
				continue
			}
			funcName := fmt.Sprintf("%s:%s", snapshot.job.ModuleName(), method.ID)
			desired[funcName] = desiredFunctionPublication{
				name:       funcName,
				owner:      instanceFunctionOwner(snapshot.job.ModuleName(), snapshot.job.Name(), method.ID),
				moduleName: snapshot.job.ModuleName(),
				jobName:    snapshot.job.Name(),
				method:     method,
			}
		}
	}

	return desired
}

func (c *Controller) collectModuleFunctionChanges(moduleName string, desired map[string]desiredFunctionPublication) ([]functionPublish, []string) {
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var withdraws []string
	for name, record := range c.publishedFns.moduleRecords(moduleName) {
		if record.owner.kind != publishedFunctionShared && record.owner.kind != publishedFunctionInstance {
			continue
		}
		want, ok := desired[name]
		if ok && want.owner == record.owner {
			continue
		}
		c.publishedFns.removeName(name)
		withdraws = append(withdraws, name)
	}

	var publishes []functionPublish
	for _, want := range desired {
		if c.publishedFns.has(want.name) {
			continue
		}
		record, _ := c.publishedFns.add(want.name, want.owner)
		publishes = append(publishes, c.makeFunctionPublish(want, record.generation))
	}

	sort.Slice(publishes, func(i, j int) bool {
		return publishes[i].name < publishes[j].name
	})
	sort.Strings(withdraws)
	return publishes, withdraws
}

func (c *Controller) makeFunctionPublish(want desiredFunctionPublication, generation uint64) functionPublish {
	if want.owner.kind == publishedFunctionInstance {
		return functionPublish{
			name:    want.name,
			handler: c.makePublishedInstanceFunctionHandler(want.name, generation, want.moduleName, want.jobName, want.method.ID),
			opts:    c.functionGlobalOpts(want.moduleName, want.method, want.name),
		}
	}

	return functionPublish{
		name:    want.name,
		handler: c.makePublishedMethodFuncHandler(want.name, generation, want.moduleName, want.method.ID),
		opts:    c.functionGlobalOpts(want.moduleName, want.method, want.name),
	}
}

func (c *Controller) availableSharedJobNames(moduleName, methodID string) []string {
	snapshots := c.registry.getJobSnapshots(moduleName)
	if len(snapshots) == 0 {
		return nil
	}

	names := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if jobBackedFunctionAvailable(snapshot.job, methodID) {
			names = append(names, snapshot.name)
		}
	}
	return names
}

func jobBackedFunctionAvailable(job collectorapi.RuntimeJob, methodID string) bool {
	if job == nil || !job.IsRunning() {
		return false
	}
	availability, ok := job.Collector().(collectorapi.FunctionAvailability)
	if !ok || availability == nil {
		return true
	}
	return availability.FunctionAvailable(methodID)
}

func (c *Controller) allFunctionNamesPublishedLocked(moduleName string, method funcapi.FunctionConfig) bool {
	names := funcapi.FunctionNames(moduleName, method)
	return c.publishedFns.all(names)
}

func (c *Controller) functionGlobalOpts(moduleName string, method funcapi.FunctionConfig, funcName string) netdataapi.FunctionGlobalOpts {
	help := method.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", moduleName, method.ID)
	}

	const cloudAccess = "0x0013"
	access := "0x0000"
	if method.RequireCloud {
		access = cloudAccess
	}

	return netdataapi.FunctionGlobalOpts{
		Name:     funcName,
		Timeout:  60,
		Help:     help,
		Tags:     methodTags(method),
		Access:   access,
		Priority: 100,
		Version:  3,
	}
}

func methodTags(method funcapi.FunctionConfig) string {
	if method.Tags != "" {
		return method.Tags
	}
	return "top"
}

func methodAvailable(method funcapi.FunctionConfig) bool {
	return method.Available == nil || method.Available()
}

func (c *Controller) registerFunction(name string, handler functions.Handler) {
	if ctxReg, ok := c.fnReg.(functions.ContextRegistry); ok {
		ctxReg.RegisterWithContext(name, handler)
		return
	}

	c.fnReg.Register(name, func(fn functions.Function) {
		handler(c.baseContext(), fn)
	})
}

func (c *Controller) registerInstanceFunctions(job collectorapi.RuntimeJob, methods []funcapi.FunctionConfig) {
	planned := make(map[string]struct{}, len(methods))

	for _, method := range methods {
		if method.ID == "" {
			c.Warningf("skipping instance function registration for %s[%s]: empty method ID", job.ModuleName(), job.Name())
			continue
		}
		if method.FunctionName != "" || len(method.Aliases) != 0 {
			c.Errorf("instance function registration aborted for %s[%s]: method '%s' uses public FunctionName or Aliases, which are supported only for static Functions", job.ModuleName(), job.Name(), method.ID)
			return
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)
		if _, exists := planned[method.ID]; exists {
			c.Errorf("instance function registration aborted for %s[%s]: duplicate method ID in batch ('%s')", job.ModuleName(), job.Name(), funcName)
			return
		}
		planned[method.ID] = struct{}{}

		if collision, exists := c.registry.findMethodCollision(job.ModuleName(), job.Name(), method.ID); exists {
			c.Errorf("instance function registration aborted for %s[%s]: collision on '%s' (%s)", job.ModuleName(), job.Name(), funcName, collision)
			return
		}
	}

	c.registry.registerInstanceFunctions(job.ModuleName(), job.Name(), methods)
}

func (c *Controller) unregisterInstanceFunctions(job collectorapi.RuntimeJob) {
	c.registry.unregisterInstanceFunctions(job.ModuleName(), job.Name())
}

func (c *Controller) baseContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

type noopRegistry struct{}

func (noopRegistry) Register(string, func(functions.Function)) {}
func (noopRegistry) Unregister(string)                         {}
func (noopRegistry) RegisterPrefix(string, string, func(functions.Function)) {
}
func (noopRegistry) UnregisterPrefix(string, string) {}
