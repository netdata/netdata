// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
)

//go:embed config_schema.json
var configSchema string

func init() {
	module.Register("zabbix", module.Creator{
		JobConfigSchema: configSchema,
		Defaults:        module.Defaults{AutoDetectionRetry: 60},
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

// Config collects Zabbix jobs defined in a module configuration file.
type Config struct {
	zabbix.JobConfig `yaml:",inline" json:",inline"`
	Jobs             []zabbix.JobConfig `yaml:"jobs,omitempty" json:"jobs,omitempty"`
}

// Collector is the placeholder module.
type Collector struct {
	module.Base
	Config       `yaml:",inline" json:",inline"`
	jobs         []zabbix.JobConfig
	proc         *zpre.Preprocessor
	runner       *runner
	emitter      *pipelineEmitter
	charts       *module.Charts
	vnodeInfo    map[string]runtime.VnodeInfo
	missingVnode map[string]struct{}

	currentVnode *vnodes.VirtualNode
	vnodeMu      sync.RWMutex
}

func New() *Collector { return &Collector{charts: &module.Charts{}} }

func (c *Collector) Configuration() any { return &c.Config }

func (c *Collector) Init(ctx context.Context) error {
	jobs, err := c.materializeJobs()
	if err != nil {
		return err
	}
	if c.charts == nil {
		c.charts = &module.Charts{}
	}
	c.jobs = jobs
	c.refreshVnodeInfo()
	for i := range c.jobs {
		if name := strings.TrimSpace(c.jobs[i].Vnode); name != "" {
			if info, ok := c.lookupVnode(name); !ok {
				return fmt.Errorf("job '%s': vnode '%s' not found", c.jobs[i].Name, name)
			} else {
				c.setCurrentVnode(info)
			}
		}
	}
	c.proc = acquirePreprocessor()
	for i := range c.jobs {
		c.jobs[i].Scheduler = schedulerName(c.jobs[i].Scheduler)
	}
	emitter, err := newPipelineEmitter(c.Logger, c.proc, c.charts, c.jobs)
	if err != nil {
		return err
	}
	c.emitter = emitter
	run, err := newRunner(ctx, c.Logger, c.jobs, c.proc, c.emitter, c.vnodeLookup)
	if err != nil {
		return err
	}
	c.runner = run
	return nil
}

func (c *Collector) materializeJobs() ([]zabbix.JobConfig, error) {
	var src []zabbix.JobConfig
	if len(c.Jobs) > 0 {
		src = c.Jobs
	} else {
		src = []zabbix.JobConfig{c.JobConfig}
	}
	seen := make(map[string]struct{}, len(src))
	jobs := make([]zabbix.JobConfig, len(src))
	for i := range src {
		job := src[i]
		if err := job.Validate(); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(job.Name))
		if key == "" {
			return nil, fmt.Errorf("job name is required")
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate job name '%s'", job.Name)
		}
		seen[key] = struct{}{}
		jobs[i] = job
	}
	return jobs, nil
}

func (c *Collector) Check(context.Context) error { return nil }
func (c *Collector) Charts() *module.Charts {
	return c.charts
}
func (c *Collector) Collect(context.Context) map[string]int64 {
	metrics := make(map[string]int64)
	if c.runner != nil {
		for k, v := range c.runner.Collect() {
			metrics[k] += v
		}
	}
	if c.emitter != nil {
		for k, v := range c.emitter.Flush() {
			metrics[k] = v
		}
	}
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

func (c *Collector) Cleanup(context.Context) {
	if c.runner != nil {
		c.runner.Stop()
	}
	if c.emitter != nil {
		_ = c.emitter.Close()
	}
}

func schedulerName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "default"
	}
	return name
}

func (c *Collector) vnodeLookup(sp spec.JobSpec) runtime.VnodeInfo {
	if info, ok := c.lookupVnode(sp.Vnode); ok {
		c.setCurrentVnode(info)
		return cloneVnodeInfo(info)
	}
	if strings.TrimSpace(sp.Vnode) != "" {
		c.warnMissingVnode(sp.Vnode)
	}
	return runtime.VnodeInfo{Hostname: sp.Vnode}
}

func (c *Collector) lookupVnode(name string) (runtime.VnodeInfo, bool) {
	if c.vnodeInfo == nil {
		return runtime.VnodeInfo{}, false
	}
	info, ok := c.vnodeInfo[strings.ToLower(strings.TrimSpace(name))]
	return cloneVnodeInfo(info), ok
}

func (c *Collector) refreshVnodeInfo() {
	configDirs := pluginconfig.ConfigDir()
	if len(configDirs) == 0 {
		c.vnodeInfo = nil
		c.missingVnode = nil
		return
	}
	path, err := configDirs.Find("vnodes")
	if err != nil {
		if !multipath.IsNotFound(err) {
			c.Warningf("zabbix: failed to locate vnodes directory: %v", err)
		}
		c.vnodeInfo = nil
		c.missingVnode = nil
		return
	}
	registry := vnodes.Load(path)
	if len(registry) == 0 {
		c.vnodeInfo = nil
		c.missingVnode = nil
		return
	}
	info := make(map[string]runtime.VnodeInfo, len(registry)*3)
	for key, vnode := range registry {
		if vnode == nil {
			continue
		}
		converted := runtime.VnodeInfo{
			Hostname: firstNonEmpty(vnode.Hostname, vnode.Name, key),
			Alias:    firstNonEmpty(vnode.Alias, vnode.Name, key),
			Address:  firstNonEmpty(vnode.Address, vnode.Labels["address"], vnode.Labels["_net_default_iface_ip"]),
			Labels:   maps.Clone(vnode.Labels),
			Custom:   maps.Clone(vnode.Custom),
		}
		for _, alias := range []string{key, vnode.Hostname, vnode.Name, vnode.GUID} {
			if strings.TrimSpace(alias) == "" {
				continue
			}
			info[strings.ToLower(alias)] = cloneVnodeInfo(converted)
		}
	}
	c.vnodeInfo = info
	c.missingVnode = make(map[string]struct{})
}

func (c *Collector) setCurrentVnode(info runtime.VnodeInfo) {
	node := &vnodes.VirtualNode{
		Name:     info.Hostname,
		Hostname: info.Hostname,
		Alias:    info.Alias,
		Address:  info.Address,
		Labels:   maps.Clone(info.Labels),
		Custom:   maps.Clone(info.Custom),
	}
	c.vnodeMu.Lock()
	c.currentVnode = node
	c.vnodeMu.Unlock()
}

func (c *Collector) VirtualNode() *vnodes.VirtualNode {
	c.vnodeMu.RLock()
	defer c.vnodeMu.RUnlock()
	if c.currentVnode == nil {
		return nil
	}
	return c.currentVnode.Copy()
}

func (c *Collector) warnMissingVnode(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if c.missingVnode == nil {
		c.missingVnode = make(map[string]struct{})
	}
	key := strings.ToLower(name)
	if _, seen := c.missingVnode[key]; seen {
		return
	}
	c.missingVnode[key] = struct{}{}
	c.Warningf("zabbix: vnode '%s' not found; macros fall back to literal hostname", name)
}

func cloneVnodeInfo(src runtime.VnodeInfo) runtime.VnodeInfo {
	clone := src
	if len(src.Labels) > 0 {
		clone.Labels = maps.Clone(src.Labels)
	} else {
		clone.Labels = nil
	}
	if len(src.Custom) > 0 {
		clone.Custom = maps.Clone(src.Custom)
	} else {
		clone.Custom = nil
	}
	return clone
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
