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
		if creator.Methods == nil && creator.JobMethods == nil {
			continue
		}
		methods := moduleMethods(creator)
		if functionName, owner, collides := c.moduleMethodPublicNameCollision(name, methods, plannedPublicNames); collides {
			c.Errorf("skipping function registration for module '%s': public function name '%s' collides with %s", name, functionName, owner)
			continue
		}
		c.registry.registerModuleWithMethods(name, creator, methods)
		c.registerAvailableModuleMethods(name, methods, true)
	}
}

func moduleMethods(creator collectorapi.Creator) []funcapi.MethodConfig {
	if creator.Methods == nil {
		return nil
	}
	return creator.Methods()
}

func (c *Controller) moduleMethodPublicNameCollision(moduleName string, methods []funcapi.MethodConfig, planned map[string]string) (string, string, bool) {
	modulePlanned := make(map[string]string)
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		owner := fmt.Sprintf("%s:%s", moduleName, method.ID)
		for _, functionName := range funcapi.MethodFunctionNames(moduleName, method) {
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
	c.registerModuleMethodsOnJobStart(job.ModuleName())

	creator, ok := c.registry.getCreator(job.ModuleName())
	if !ok || creator.JobMethods == nil {
		return
	}

	methods := creator.JobMethods(job)
	if len(methods) > 0 {
		c.registerJobMethods(job, methods)
	}
}

// ReconcileModuleMethods rechecks module/static method availability for modules
// with at least one registered running job.
func (c *Controller) ReconcileModuleMethods(moduleName string) {
	if moduleName == "" || !c.registry.hasRunningJob(moduleName) {
		return
	}
	methods := c.registry.getMethods(moduleName)
	if len(methods) == 0 {
		return
	}
	c.registerAvailableModuleMethods(moduleName, methods, false)
}

func (c *Controller) OnJobStop(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.unregisterJobMethods(job)
	c.registry.removeJob(job.ModuleName(), job.Name())
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

func (c *Controller) registerModuleMethodsOnJobStart(moduleName string) {
	methods := c.registry.getMethods(moduleName)
	if len(methods) == 0 {
		return
	}

	c.registerAvailableModuleMethods(moduleName, methods, false)
}

func (c *Controller) registerAvailableModuleMethods(moduleName string, methods []funcapi.MethodConfig, agentScopeOnly bool) {
	publishes := c.collectAvailableModuleMethodPublishes(moduleName, methods, agentScopeOnly)
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

func (c *Controller) collectAvailableModuleMethodPublishes(moduleName string, methods []funcapi.MethodConfig, agentScopeOnly bool) []functionPublish {
	c.publishedMu.Lock()
	defer c.publishedMu.Unlock()

	var publishes []functionPublish
	for i, method := range methods {
		if agentScopeOnly && method.Scope != funcapi.MethodScopeAgent {
			continue
		}
		if method.ID != "" && c.allMethodFunctionNamesPublishedLocked(moduleName, method) {
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

		help := method.Help
		if help == "" {
			help = fmt.Sprintf("%s %s data function", moduleName, method.ID)
		}

		const cloudAccess = "0x0013"
		access := "0x0000"
		if method.RequireCloud {
			access = cloudAccess
		}

		if c.api != nil {
			for _, funcName := range funcapi.MethodFunctionNames(moduleName, method) {
				if c.publishedFns.has(funcName) {
					continue
				}
				record, _ := c.publishedFns.add(funcName, moduleMethodOwner(moduleName, method.ID))
				publishes = append(publishes, functionPublish{
					name:    funcName,
					handler: c.makePublishedMethodFuncHandler(funcName, record.generation, moduleName, method.ID),
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
			continue
		}

		for _, funcName := range funcapi.MethodFunctionNames(moduleName, method) {
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

func (c *Controller) allMethodFunctionNamesPublishedLocked(moduleName string, method funcapi.MethodConfig) bool {
	names := funcapi.MethodFunctionNames(moduleName, method)
	return c.publishedFns.all(names)
}

func methodTags(method funcapi.MethodConfig) string {
	if method.Tags != "" {
		return method.Tags
	}
	return "top"
}

func methodAvailable(method funcapi.MethodConfig) bool {
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

func (c *Controller) registerJobMethods(job collectorapi.RuntimeJob, methods []funcapi.MethodConfig) {
	planned := make(map[string]struct{}, len(methods))

	for _, method := range methods {
		if method.ID == "" {
			c.Warningf("skipping job method registration for %s[%s]: empty method ID", job.ModuleName(), job.Name())
			continue
		}
		if method.FunctionName != "" || len(method.Aliases) != 0 {
			c.Errorf("job method registration aborted for %s[%s]: method '%s' uses public FunctionName or Aliases, which are supported only for module methods", job.ModuleName(), job.Name(), method.ID)
			return
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)
		if _, exists := planned[method.ID]; exists {
			c.Errorf("job method registration aborted for %s[%s]: duplicate method ID in batch ('%s')", job.ModuleName(), job.Name(), funcName)
			return
		}
		planned[method.ID] = struct{}{}

		if collision, exists := c.registry.findMethodCollision(job.ModuleName(), job.Name(), method.ID); exists {
			c.Errorf("job method registration aborted for %s[%s]: collision on '%s' (%s)", job.ModuleName(), job.Name(), funcName, collision)
			return
		}
	}

	// Record methods before publishing handlers so startup-time calls do not race a false 404.
	c.registry.registerJobMethods(job.ModuleName(), job.Name(), methods)

	publishes := c.collectJobMethodPublishes(job, methods)

	for _, publish := range publishes {
		c.registerFunction(publish.name, publish.handler)
		if c.api != nil {
			c.api.FunctionGlobal(publish.opts)
		}

		c.Debugf("registered job method: %s for job %s[%s]", publish.name, job.ModuleName(), job.Name())
	}
}

func (c *Controller) collectJobMethodPublishes(job collectorapi.RuntimeJob, methods []funcapi.MethodConfig) []functionPublish {
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

		record, _ := c.publishedFns.add(funcName, jobMethodOwner(job.ModuleName(), job.Name(), method.ID))

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
			handler: c.makePublishedJobMethodFuncHandler(funcName, record.generation, job.ModuleName(), job.Name(), method.ID),
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

func (c *Controller) unregisterJobMethods(job collectorapi.RuntimeJob) {
	methods := c.registry.getJobMethods(job.ModuleName(), job.Name())
	if len(methods) == 0 {
		return
	}

	var names []string
	c.publishedMu.Lock()
	for _, method := range methods {
		if method.ID == "" {
			continue
		}
		names = append(names, c.publishedFns.removeOwner(jobMethodOwner(job.ModuleName(), job.Name(), method.ID))...)
	}
	c.publishedMu.Unlock()

	sort.Strings(names)
	for _, funcName := range names {
		c.fnReg.Unregister(funcName)
		if c.api != nil {
			c.api.FunctionRemove(funcName)
		}
		c.Debugf("unregistered job method: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}

	c.registry.unregisterJobMethods(job.ModuleName(), job.Name())
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
