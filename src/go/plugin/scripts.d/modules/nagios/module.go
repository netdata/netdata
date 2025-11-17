// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/units"
)

//go:embed config_schema.json
var configSchema string

func init() {
	module.Register("nagios", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			AutoDetectionRetry: 60,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

// Config represents a Nagios module configuration loaded by go.d's file discovery.
// Each file may define multiple explicit jobs plus shared defaults/macros.
type Config struct {
	spec.JobConfig `yaml:",inline" json:",inline"`
	UserMacros     map[string]string `yaml:"user_macros,omitempty" json:"user_macros"`
	Logging        LoggingConfig     `yaml:"logging,omitempty" json:"logging"`
}

type LoggingConfig struct {
	Enabled bool              `yaml:"enabled,omitempty" json:"enabled"`
	OTLP    OTLPLoggingConfig `yaml:"otlp,omitempty" json:"otlp"`
}

type OTLPLoggingConfig struct {
	Endpoint         string            `yaml:"endpoint,omitempty" json:"endpoint"`
	Timeout          confopt.Duration  `yaml:"timeout,omitempty" json:"timeout"`
	TLS              *bool             `yaml:"tls,omitempty" json:"tls"`
	Headers          map[string]string `yaml:"headers,omitempty" json:"headers"`
	TLSServerName    string            `yaml:"tls_server_name,omitempty" json:"tls_server_name,omitempty"`
	tlscfg.TLSConfig `yaml:",inline" json:",inline"`
}

func (l *LoggingConfig) setDefaults() {
	if l == nil {
		return
	}
	if l.OTLP.Endpoint == "" {
		l.OTLP.Endpoint = runtime.DefaultOTLPEndpoint
	}
	if l.OTLP.Timeout == 0 {
		l.OTLP.Timeout = confopt.Duration(runtime.DefaultOTLPTimeout)
	}
	if l.OTLP.Headers == nil {
		l.OTLP.Headers = make(map[string]string)
	}
	if l.OTLP.TLS == nil {
		v := true
		l.OTLP.TLS = &v
	}
	if !l.Enabled {
		l.Enabled = true
	}
}

func (l LoggingConfig) emitterConfig() runtime.OTLPEmitterConfig {
	return runtime.OTLPEmitterConfig{
		Endpoint:   l.OTLP.Endpoint,
		Timeout:    time.Duration(l.OTLP.Timeout),
		UseTLS:     l.OTLP.tlsEnabled(),
		Headers:    l.OTLP.Headers,
		TLSConfig:  l.OTLP.TLSConfig,
		ServerName: l.OTLP.TLSServerName,
	}
}

func (c OTLPLoggingConfig) tlsEnabled() bool {
	if c.TLS == nil {
		return true
	}
	return *c.TLS
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

// Collector is a placeholder module that keeps the plugin wiring compiling while the real
// Nagios execution engine is being implemented.
type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts       *module.Charts
	chartMu      sync.RWMutex
	perfCharts   map[string]perfChartMeta
	periods      *timeperiod.Set
	jobSpec      spec.JobSpec
	identity     charts.JobIdentity
	jobHandle    *schedulers.JobHandle
	vnodeInfo    map[string]runtime.VnodeInfo
	missingVnode map[string]struct{}

	currentVnode *vnodes.VirtualNode
	vnodeMu      sync.RWMutex
}

type perfChartMeta struct {
	Scale units.Scale
}

// New returns a collector with sensible defaults so the module registry can instantiate jobs.
func New() *Collector {
	return &Collector{
		Config: Config{
			UserMacros: make(map[string]string),
			Logging: LoggingConfig{
				Enabled: true,
				OTLP: OTLPLoggingConfig{
					Endpoint: runtime.DefaultOTLPEndpoint,
					Timeout:  confopt.Duration(runtime.DefaultOTLPTimeout),
					TLS:      boolPtr(false),
					Headers:  make(map[string]string),
				},
			},
		},
		charts:     &module.Charts{},
		perfCharts: make(map[string]perfChartMeta),
	}
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	if c.charts == nil {
		c.charts = &module.Charts{}
	}
	if c.perfCharts == nil {
		c.perfCharts = make(map[string]perfChartMeta)
	}
	c.Logging.setDefaults()
	if err := c.compileTimePeriods(); err != nil {
		return err
	}
	sp, err := c.buildJobSpec()
	if err != nil {
		return err
	}
	c.jobSpec = sp
	c.identity = charts.NewJobIdentity(sp.Scheduler, sp)
	c.refreshVnodeInfo()
	if err := c.validateVnode(sp.Vnode); err != nil {
		return err
	}
	if err := c.initCharts(); err != nil {
		return err
	}
	emitter := c.buildEmitter(c.Logging.Enabled, c.Logging.emitterConfig())
	reg := runtime.JobRegistration{
		Spec:             sp,
		Emitter:          emitter,
		RegisterPerfdata: c.registerPerfdataChart,
		Periods:          c.periods,
		UserMacros:       c.copyUserMacros(),
		Vnode:            c.vnodeInfoFor(sp),
	}
	handle, err := schedulers.AttachJob(sp.Scheduler, reg, c.Logger)
	if err != nil {
		return err
	}
	c.jobHandle = handle
	return nil
}

func (c *Collector) Check(context.Context) error {
	// Placeholder check so autodetection succeeds during early development.
	return nil
}

func (c *Collector) Charts() *module.Charts {
	c.chartMu.RLock()
	defer c.chartMu.RUnlock()
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	all := schedulers.CollectMetrics(c.jobSpec.Scheduler)
	if len(all) == 0 {
		return nil
	}
	prefix := c.identity.MetricPrefix()
	metrics := make(map[string]int64)
	for k, v := range all {
		if strings.HasPrefix(k, prefix) {
			metrics[k] = v
		}
	}
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

func (c *Collector) Cleanup(context.Context) {
	if c.jobHandle != nil {
		schedulers.DetachJob(c.jobHandle)
		c.jobHandle = nil
	}
}

func (c *Collector) vnodeInfoFor(job spec.JobSpec) runtime.VnodeInfo {
	info, ok := c.lookupVnode(job.Vnode)
	if ok {
		return cloneVnodeInfo(info)
	}
	if strings.TrimSpace(job.Vnode) != "" {
		c.warnMissingVnode(job.Vnode)
	}
	return runtime.VnodeInfo{Hostname: job.Vnode}
}

func (c *Collector) lookupVnode(name string) (runtime.VnodeInfo, bool) {
	if c.vnodeInfo == nil {
		return runtime.VnodeInfo{}, false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return runtime.VnodeInfo{}, false
	}
	info, ok := c.vnodeInfo[key]
	return cloneVnodeInfo(info), ok
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
	c.Warningf("nagios: vnode '%s' not found; macros fallback to literal hostname", name)
}

func (c *Collector) initCharts() error {
	meta := charts.NewJobIdentity(c.jobSpec.Scheduler, c.jobSpec)
	jobCharts := charts.BuildJobCharts(meta, 100)
	newCharts := module.Charts{}
	if err := newCharts.Add(jobCharts...); err != nil {
		return err
	}
	c.chartMu.Lock()
	c.charts = &newCharts
	c.chartMu.Unlock()
	return nil
}

func (c *Collector) registerPerfdataChart(job spec.JobSpec, datum output.PerfDatum) {
	label := strings.TrimSpace(datum.Label)
	if label == "" {
		return
	}
	meta := c.identity
	labelID := ids.Sanitize(label)
	scale := units.NewScale(datum.Unit)
	key := fmt.Sprintf("%s|%s", meta.JobKey, labelID)
	c.Infof("nagios: registering perfdata chart scheduler=%s job=%s label=%s unit=%s", meta.Scheduler, job.Name, label, datum.Unit)
	c.chartMu.Lock()
	defer c.chartMu.Unlock()
	if existing, ok := c.perfCharts[key]; ok {
		if sameScale(existing.Scale, scale) {
			return
		}
		chartID := meta.PerfdataChartID(labelID)
		if chart := c.charts.Get(chartID); chart != nil {
			chart.MarkRemove()
		}
	}
	chart := charts.PerfdataChart(meta, label, scale, 200)
	if err := c.charts.Add(chart); err != nil {
		c.Errorf("failed to add perfdata chart for job %s label %s: %v", job.Name, label, err)
		return
	}
	c.perfCharts[key] = perfChartMeta{Scale: scale}
}

func sameScale(a, b units.Scale) bool {
	return a.Divisor == b.Divisor && a.CanonicalUnit == b.CanonicalUnit
}

func (c *Collector) copyUserMacros() map[string]string {
	userMacros := make(map[string]string, len(c.UserMacros))
	for k, v := range c.UserMacros {
		userMacros[k] = v
	}
	return userMacros
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
			c.Warningf("nagios: failed to locate vnodes directory: %v", err)
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
			if alias == "" {
				continue
			}
			info[strings.ToLower(alias)] = cloneVnodeInfo(converted)
		}
	}
	c.vnodeInfo = info
	c.missingVnode = make(map[string]struct{})
}

func (c *Collector) VirtualNode() *vnodes.VirtualNode {
	c.vnodeMu.RLock()
	defer c.vnodeMu.RUnlock()
	if c.currentVnode == nil {
		return nil
	}
	return c.currentVnode.Copy()
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

func (c *Collector) compileTimePeriods() error {
	configs := []timeperiod.Config{timeperiod.DefaultPeriodConfig()}
	set, err := timeperiod.Compile(configs)
	if err != nil {
		return err
	}
	c.periods = set
	return nil
}

func (c *Collector) buildJobSpec() (spec.JobSpec, error) {
	cfg := c.JobConfig
	sp, err := cfg.ToSpec()
	if err != nil {
		return spec.JobSpec{}, err
	}
	return sp, nil
}

func (c *Collector) validateVnode(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	info, ok := c.lookupVnode(name)
	if !ok {
		return fmt.Errorf("job '%s': vnode '%s' not found", c.jobSpec.Name, name)
	}
	c.setCurrentVnode(info)
	return nil
}

func (c *Collector) setCurrentVnode(info runtime.VnodeInfo) {
	converted := &vnodes.VirtualNode{
		Name:     info.Hostname,
		Hostname: info.Hostname,
		Alias:    info.Alias,
		Address:  info.Address,
		Labels:   maps.Clone(info.Labels),
		Custom:   maps.Clone(info.Custom),
	}
	c.vnodeMu.Lock()
	c.currentVnode = converted
	c.vnodeMu.Unlock()
}

func (c *Collector) buildEmitter(enabled bool, cfg runtime.OTLPEmitterConfig) runtime.ResultEmitter {
	if enabled {
		emitter, err := runtime.NewOTLPEmitter(cfg, c.Logger)
		if err == nil {
			return emitter
		}
		c.Errorf("failed to initialize OTLP emitter: %v", err)
	}
	return runtime.NewLogEmitter(c.Logger)
}
