// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"context"
	"fmt"

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

	registry          *moduleFuncRegistry
	staticMethodsSeen map[string]struct{}
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
		Logger:            log,
		api:               opts.API,
		jsonWriter:        opts.JSONWriter,
		fnReg:             reg,
		registry:          newModuleFuncRegistry(),
		staticMethodsSeen: make(map[string]struct{}),
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
	for name, creator := range modules {
		if creator.Methods == nil && creator.JobMethods == nil {
			continue
		}
		c.registry.registerModule(name, creator)
	}
}

func (c *Controller) GetJobNames(moduleName string) []string {
	return c.registry.getJobNames(moduleName)
}

func (c *Controller) OnJobStart(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.registry.addJob(job.ModuleName(), job.Name(), job)
	c.registerModuleMethodsOnFirstJobStart(job.ModuleName())

	creator, ok := c.registry.getCreator(job.ModuleName())
	if !ok || creator.JobMethods == nil {
		return
	}

	methods := creator.JobMethods(job)
	if len(methods) > 0 {
		c.registerJobMethods(job, methods)
	}
}

func (c *Controller) OnJobStop(job collectorapi.RuntimeJob) {
	if job == nil {
		return
	}

	c.unregisterJobMethods(job)
	c.registry.removeJob(job.ModuleName(), job.Name())
}

func (c *Controller) Cleanup() {
	for name, creator := range c.registry.snapshotCreators() {
		if creator.Methods == nil {
			continue
		}
		for _, method := range creator.Methods() {
			if method.ID == "" {
				continue
			}
			for _, funcName := range methodFunctionNames(name, method) {
				c.fnReg.Unregister(funcName)
				if c.api != nil {
					c.api.FunctionRemove(funcName)
				}
			}
		}
	}
}

func (c *Controller) registerModuleMethodsOnFirstJobStart(moduleName string) {
	if _, ok := c.staticMethodsSeen[moduleName]; ok {
		return
	}

	creator, ok := c.registry.getCreator(moduleName)
	if !ok || creator.Methods == nil {
		return
	}

	for _, method := range creator.Methods() {
		if method.ID == "" {
			c.Warningf("skipping function registration for module '%s': empty method ID", moduleName)
			continue
		}

		if c.api != nil {
			help := method.Help
			if help == "" {
				help = fmt.Sprintf("%s %s data function", moduleName, method.ID)
			}

			const cloudAccess = "0x0013"
			access := "0x0000"
			if method.RequireCloud {
				access = cloudAccess
			}

			for _, funcName := range methodFunctionNames(moduleName, method) {
				c.fnReg.Register(funcName, c.makeMethodFuncHandler(moduleName, method.ID))
				c.api.FunctionGlobal(netdataapi.FunctionGlobalOpts{
					Name:     funcName,
					Timeout:  60,
					Help:     help,
					Tags:     "top",
					Access:   access,
					Priority: 100,
					Version:  3,
				})
			}
			continue
		}

		for _, funcName := range methodFunctionNames(moduleName, method) {
			c.fnReg.Register(funcName, c.makeMethodFuncHandler(moduleName, method.ID))
		}
	}

	c.staticMethodsSeen[moduleName] = struct{}{}
}

func methodFunctionNames(moduleName string, method funcapi.MethodConfig) []string {
	funcName := fmt.Sprintf("%s:%s", moduleName, method.ID)
	funcNames := []string{funcName}
	seen := map[string]struct{}{funcName: {}}

	for _, alias := range method.Aliases {
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		funcNames = append(funcNames, alias)
	}
	return funcNames
}

func (c *Controller) registerJobMethods(job collectorapi.RuntimeJob, methods []funcapi.MethodConfig) {
	planned := make(map[string]struct{}, len(methods))

	for _, method := range methods {
		if method.ID == "" {
			c.Warningf("skipping job method registration for %s[%s]: empty method ID", job.ModuleName(), job.Name())
			continue
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

	for _, method := range methods {
		if method.ID == "" {
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)
		c.fnReg.Register(funcName, c.makeJobMethodFuncHandler(job.ModuleName(), job.Name(), method.ID))

		if c.api != nil {
			help := method.Help
			if help == "" {
				help = fmt.Sprintf("%s %s data function", job.ModuleName(), method.ID)
			}

			const cloudAccess = "0x0013"
			access := "0x0000"
			if method.RequireCloud {
				access = cloudAccess
			}

			c.api.FunctionGlobal(netdataapi.FunctionGlobalOpts{
				Name:     funcName,
				Timeout:  60,
				Help:     help,
				Tags:     "top",
				Access:   access,
				Priority: 100,
				Version:  3,
			})
		}

		c.Debugf("registered job method: %s for job %s[%s]", funcName, job.ModuleName(), job.Name())
	}
}

func (c *Controller) unregisterJobMethods(job collectorapi.RuntimeJob) {
	methods := c.registry.getJobMethods(job.ModuleName(), job.Name())
	if len(methods) == 0 {
		return
	}

	for _, method := range methods {
		if method.ID == "" {
			continue
		}

		funcName := fmt.Sprintf("%s:%s", job.ModuleName(), method.ID)
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
