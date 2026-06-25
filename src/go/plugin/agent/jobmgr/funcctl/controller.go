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
	c.registerAvailableAgentFunctions(job.ModuleName(), c.registry.getMethods(job.ModuleName()))

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
	c.reconcileSharedModuleFunctions(moduleName)
	c.reconcileInstanceFunctions(moduleName)
}

func (c *Controller) OnJobStop(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.unregisterInstanceFunctions(job)
	c.registry.removeJob(job.ModuleName(), job.Name())
	c.reconcileSharedModuleFunctions(job.ModuleName())
}

func (c *Controller) Cleanup() {
	c.publishedMu.Lock()
	names := c.publishedFns.removeKind(publishedFunctionModuleMethod)
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
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var publishes []functionPublish
	for i, method := range methods {
		kind := c.registry.getModuleFunctionKind(moduleName, method.ID)
		if kind != moduleFunctionAgent {
			continue
		}
		if method.ID != "" && c.allFunctionNamesPublishedLocked(moduleName, method) {
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

		if c.api != nil {
			for _, funcName := range funcapi.FunctionNames(moduleName, method) {
				if c.publishedFns.has(funcName) {
					continue
				}
				record, _ := c.publishedFns.add(funcName, moduleMethodOwner(moduleName, method.ID))
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
			record, _ := c.publishedFns.add(funcName, moduleMethodOwner(moduleName, method.ID))
			publishes = append(publishes, functionPublish{
				name:    funcName,
				handler: c.makePublishedMethodFuncHandler(funcName, record.generation, moduleName, method.ID),
			})
		}
	}
	return publishes
}

func (c *Controller) reconcileSharedModuleFunctions(moduleName string) {
	methods := c.registry.getMethods(moduleName)
	if len(methods) == 0 {
		return
	}

	publishes, withdraws := c.collectSharedModuleFunctionChanges(moduleName, methods)
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

func (c *Controller) collectSharedModuleFunctionChanges(moduleName string, methods []funcapi.FunctionConfig) ([]functionPublish, []string) {
	var publishes []functionPublish
	var withdraws []string

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
			withdraws = append(withdraws, c.collectSharedModuleFunctionWithdraws(moduleName, method.ID)...)
			continue
		}

		if c.allFunctionNamesPublished(moduleName, method) {
			continue
		}
		publishes = append(publishes, c.collectSharedModuleFunctionPublishes(moduleName, method)...)
	}

	sort.Strings(withdraws)
	return publishes, withdraws
}

func (c *Controller) collectSharedModuleFunctionWithdraws(moduleName, methodID string) []string {
	c.publishedMu.Lock()
	names := c.publishedFns.removeOwner(moduleMethodOwner(moduleName, methodID))
	c.publishedMu.Unlock()

	return names
}

func (c *Controller) collectSharedModuleFunctionPublishes(moduleName string, method funcapi.FunctionConfig) []functionPublish {
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var publishes []functionPublish
	for _, funcName := range funcapi.FunctionNames(moduleName, method) {
		if c.publishedFns.has(funcName) {
			continue
		}
		record, _ := c.publishedFns.add(funcName, moduleMethodOwner(moduleName, method.ID))
		publishes = append(publishes, functionPublish{
			name:    funcName,
			handler: c.makePublishedMethodFuncHandler(funcName, record.generation, moduleName, method.ID),
			opts:    c.functionGlobalOpts(moduleName, method, funcName),
		})
	}
	return publishes
}

func (c *Controller) allFunctionNamesPublished(moduleName string, method funcapi.FunctionConfig) bool {
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()
	return c.allFunctionNamesPublishedLocked(moduleName, method)
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

func jobUsesFunctionAvailability(job collectorapi.RuntimeJob) bool {
	if job == nil {
		return false
	}
	availability, ok := job.Collector().(collectorapi.FunctionAvailability)
	return ok && availability != nil
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

	// Record methods before publishing handlers so startup-time calls do not race a false 404.
	c.registry.registerInstanceFunctions(job.ModuleName(), job.Name(), methods)

	if jobUsesFunctionAvailability(job) {
		return
	}

	publishes := c.collectInstanceFunctionPublishes(job, methods)

	for _, publish := range publishes {
		c.registerFunction(publish.name, publish.handler)
		if c.api != nil {
			c.api.FunctionGlobal(publish.opts)
		}

		c.Debugf("registered instance function: %s for job %s[%s]", publish.name, job.ModuleName(), job.Name())
	}
}

func (c *Controller) collectInstanceFunctionPublishes(job collectorapi.RuntimeJob, methods []funcapi.FunctionConfig) []functionPublish {
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var publishes []functionPublish
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)
		if c.publishedFns.has(funcName) {
			continue
		}

		record, _ := c.publishedFns.add(funcName, instanceFunctionOwner(job.ModuleName(), job.Name(), method.ID))

		help := method.Help
		if help == "" {
			help = fmt.Sprintf("%s %s data function", job.ModuleName(), method.ID)
		}

		const cloudAccess = "0x0013"
		access := "0x0000"
		if method.RequireCloud {
			access = cloudAccess
		}

		publishes = append(publishes, functionPublish{
			name:    funcName,
			handler: c.makePublishedInstanceFunctionHandler(funcName, record.generation, job.ModuleName(), job.Name(), method.ID),
			opts: netdataapi.FunctionGlobalOpts{
				Name:     funcName,
				Timeout:  60,
				Help:     help,
				Tags:     methodTags(method),
				Access:   access,
				Priority: 100,
				Version:  3,
			},
		})
	}
	return publishes
}

func (c *Controller) reconcileInstanceFunctions(moduleName string) {
	publishes, withdraws := c.collectInstanceFunctionChanges(moduleName)
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

func (c *Controller) collectInstanceFunctionChanges(moduleName string) ([]functionPublish, []string) {
	var publishes []functionPublish
	var withdraws []string

	for _, snapshot := range c.registry.getInstanceFunctionSnapshots(moduleName) {
		var availableMethods []funcapi.FunctionConfig
		for _, method := range snapshot.methods {
			if method.ID == "" {
				continue
			}
			if jobBackedFunctionAvailable(snapshot.job, method.ID) {
				availableMethods = append(availableMethods, method)
				continue
			}
			withdraws = append(withdraws, c.collectInstanceFunctionWithdraws(snapshot.job, method.ID)...)
		}
		publishes = append(publishes, c.collectInstanceFunctionPublishes(snapshot.job, availableMethods)...)
	}

	sort.Strings(withdraws)
	return publishes, withdraws
}

func (c *Controller) collectInstanceFunctionWithdraws(job collectorapi.RuntimeJob, methodID string) []string {
	if job == nil {
		return nil
	}
	c.publishedMu.Lock()
	names := c.publishedFns.removeOwner(instanceFunctionOwner(job.ModuleName(), job.Name(), methodID))
	c.publishedMu.Unlock()
	return names
}

func (c *Controller) unregisterInstanceFunctions(job collectorapi.RuntimeJob) {
	methods := c.registry.getInstanceFunctions(job.ModuleName(), job.Name())
	if len(methods) == 0 {
		return
	}

	var names []string
	c.publishedMu.Lock()
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		names = append(names, c.publishedFns.removeOwner(instanceFunctionOwner(job.ModuleName(), job.Name(), method.ID))...)
	}
	c.publishedMu.Unlock()

	sort.Strings(names)
	for _, funcName := range names {
		c.fnReg.Unregister(funcName)
		if c.api != nil {
			c.api.FunctionRemove(funcName)
		}
		c.Debugf("unregistered instance function: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}

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
