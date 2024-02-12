// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/agent/functions"
	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

const dynCfg = "dyncfg"

func NewDiscovery(cfg Config) (*Discovery, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	mgr := &Discovery{
		Logger: logger.New().With(
			slog.String("component", "discovery dyncfg"),
		),
		Plugin:               cfg.Plugin,
		API:                  cfg.API,
		Modules:              cfg.Modules,
		ModuleConfigDefaults: nil,
		mux:                  &sync.Mutex{},
		configs:              make(map[string]confgroup.Config),
	}

	mgr.registerFunctions(cfg.Functions)

	return mgr, nil
}

type Discovery struct {
	*logger.Logger

	Plugin               string
	API                  NetdataDyncfgAPI
	Modules              module.Registry
	ModuleConfigDefaults confgroup.Registry

	in chan<- []*confgroup.Group

	mux     *sync.Mutex
	configs map[string]confgroup.Config
}

func (d *Discovery) String() string {
	return d.Name()
}

func (d *Discovery) Name() string {
	return "dyncfg discovery"
}

func (d *Discovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer func() { d.Info("instance is stopped") }()

	d.in = in

	if reload, ok := ctx.Value("reload").(bool); ok && reload {
		_ = d.API.DynCfgReset()
	}

	_ = d.API.DynCfgEnable(d.Plugin)

	for k := range d.Modules {
		_ = d.API.DyncCfgRegisterModule(k)
	}

	<-ctx.Done()
}

func (d *Discovery) registerFunctions(r FunctionRegistry) {
	r.Register("get_plugin_config", d.getPluginConfig)
	r.Register("get_plugin_config_schema", d.getModuleConfigSchema)
	r.Register("set_plugin_config", d.setPluginConfig)

	r.Register("get_module_config", d.getModuleConfig)
	r.Register("get_module_config_schema", d.getModuleConfigSchema)
	r.Register("set_module_config", d.setModuleConfig)

	r.Register("get_job_config", d.getJobConfig)
	r.Register("get_job_config_schema", d.getJobConfigSchema)
	r.Register("set_job_config", d.setJobConfig)
	r.Register("delete_job", d.deleteJobName)
}

func (d *Discovery) getPluginConfig(fn functions.Function)       { d.notImplemented(fn) }
func (d *Discovery) getPluginConfigSchema(fn functions.Function) { d.notImplemented(fn) }
func (d *Discovery) setPluginConfig(fn functions.Function)       { d.notImplemented(fn) }

func (d *Discovery) getModuleConfig(fn functions.Function)       { d.notImplemented(fn) }
func (d *Discovery) getModuleConfigSchema(fn functions.Function) { d.notImplemented(fn) }
func (d *Discovery) setModuleConfig(fn functions.Function)       { d.notImplemented(fn) }

func (d *Discovery) getJobConfig(fn functions.Function) {
	if err := d.verifyFn(fn, 2); err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	moduleName, jobName := fn.Args[0], fn.Args[1]

	bs, err := d.getConfigBytes(moduleName + "_" + jobName)
	if err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	d.apiSuccessYAML(fn, string(bs))
}

func (d *Discovery) getJobConfigSchema(fn functions.Function) {
	if err := d.verifyFn(fn, 1); err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	name := fn.Args[0]

	v, ok := d.Modules[name]
	if !ok {
		msg := jsonErrorf("module %s is not registered", name)
		d.apiReject(fn, msg)
		return
	}

	d.apiSuccessJSON(fn, v.JobConfigSchema)
}

func (d *Discovery) setJobConfig(fn functions.Function) {
	if err := d.verifyFn(fn, 2); err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	var cfg confgroup.Config
	if err := yaml.NewDecoder(bytes.NewBuffer(fn.Payload)).Decode(&cfg); err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	modName, jobName := fn.Args[0], fn.Args[1]
	def, _ := d.ModuleConfigDefaults.Lookup(modName)
	src := source(modName, jobName)

	cfg.SetProvider(dynCfg)
	cfg.SetSource(src)
	cfg.SetModule(modName)
	cfg.SetName(jobName)
	cfg.Apply(def)

	d.in <- []*confgroup.Group{
		{
			Configs: []confgroup.Config{cfg},
			Source:  src,
		},
	}

	d.apiSuccessJSON(fn, "")
}

func (d *Discovery) deleteJobName(fn functions.Function) {
	if err := d.verifyFn(fn, 2); err != nil {
		d.apiReject(fn, err.Error())
		return
	}

	modName, jobName := fn.Args[0], fn.Args[1]

	cfg, ok := d.getConfig(modName + "_" + jobName)
	if !ok {
		d.apiReject(fn, jsonErrorf("module '%s' job '%s': not registered", modName, jobName))
		return
	}
	if cfg.Provider() != dynCfg {
		d.apiReject(fn, jsonErrorf("module '%s' job '%s': can't remove non Dyncfg job", modName, jobName))
		return
	}

	d.in <- []*confgroup.Group{
		{
			Configs: []confgroup.Config{},
			Source:  source(modName, jobName),
		},
	}

	d.apiSuccessJSON(fn, "")
}

func (d *Discovery) apiSuccessJSON(fn functions.Function, payload string) {
	_ = d.API.FunctionResultSuccess(fn.UID, "application/json", payload)
}

func (d *Discovery) apiSuccessYAML(fn functions.Function, payload string) {
	_ = d.API.FunctionResultSuccess(fn.UID, "application/x-yaml", payload)
}

func (d *Discovery) apiReject(fn functions.Function, msg string) {
	_ = d.API.FunctionResultReject(fn.UID, "application/json", msg)
}

func (d *Discovery) notImplemented(fn functions.Function) {
	d.Infof("not implemented: '%s'", fn.String())
	msg := jsonErrorf("function '%s' is not implemented", fn.Name)
	d.apiReject(fn, msg)
}

func (d *Discovery) verifyFn(fn functions.Function, wantArgs int) error {
	if got := len(fn.Args); got != wantArgs {
		msg := jsonErrorf("wrong number of arguments: want %d, got %d (args: '%v')", wantArgs, got, fn.Args)
		return fmt.Errorf(msg)
	}

	if isSetFunction(fn) && len(fn.Payload) == 0 {
		msg := jsonErrorf("no payload")
		return fmt.Errorf(msg)
	}

	return nil
}

func jsonErrorf(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	msg = strings.ReplaceAll(msg, "\n", " ")

	return fmt.Sprintf(`{ "error": "%s" }`+"\n", msg)
}

func source(modName, jobName string) string {
	return fmt.Sprintf("%s/%s/%s", dynCfg, modName, jobName)
}

func cfgJobName(cfg confgroup.Config) string {
	if strings.HasPrefix(cfg.Source(), "dyncfg") {
		return cfg.Name()
	}
	return cfg.NameWithHash()
}

func isSetFunction(fn functions.Function) bool {
	return strings.HasPrefix(fn.Name, "set_")
}
