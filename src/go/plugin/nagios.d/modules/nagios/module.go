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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/config"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/spec"
	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/timeperiod"
)

//go:embed config_schema.json
var configSchema string

func init() {
	module.Register("nagios", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

// Config represents a configuration shard loaded by go.d's file discovery.
// A single Config can define explicit jobs as well as directory provisioning rules.
type Config struct {
	Name               string                   `yaml:"name" json:"name"`
	Vnode              string                   `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int                      `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int                      `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	ExecutorWorkers    int                      `yaml:"executor_workers,omitempty" json:"executor_workers"`
	Defaults           config.Defaults          `yaml:"defaults,omitempty" json:"defaults"`
	UserMacros         map[string]string        `yaml:"user_macros,omitempty" json:"user_macros"`
	Jobs               []spec.JobConfig         `yaml:"jobs,omitempty" json:"jobs"`
	Directories        []config.DirectoryConfig `yaml:"directories,omitempty" json:"directories"`
	TimePeriods        []timeperiod.Config      `yaml:"time_periods,omitempty" json:"time_periods"`
	Logging            LoggingConfig            `yaml:"logging,omitempty" json:"logging"`
}

type LoggingConfig struct {
	Enabled bool              `yaml:"enabled,omitempty" json:"enabled"`
	OTLP    OTLPLoggingConfig `yaml:"otlp,omitempty" json:"otlp"`
}

type OTLPLoggingConfig struct {
	Endpoint string            `yaml:"endpoint,omitempty" json:"endpoint"`
	Timeout  confopt.Duration  `yaml:"timeout,omitempty" json:"timeout"`
	Insecure bool              `yaml:"insecure,omitempty" json:"insecure"`
	Headers  map[string]string `yaml:"headers,omitempty" json:"headers"`
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
	if !l.Enabled {
		l.Enabled = true
	}
}

func (l LoggingConfig) emitterConfig() runtime.OTLPEmitterConfig {
	return runtime.OTLPEmitterConfig{
		Endpoint: l.OTLP.Endpoint,
		Timeout:  time.Duration(l.OTLP.Timeout),
		Insecure: l.OTLP.Insecure,
		Headers:  l.OTLP.Headers,
	}
}

// Collector is a placeholder module that keeps the plugin wiring compiling while the real
// Nagios execution engine is being implemented.
type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts       *module.Charts
	jobSpecs     []spec.JobSpec
	scheduler    *runtime.Scheduler
	chartMu      sync.RWMutex
	schedulerMu  sync.RWMutex
	reloadMu     sync.Mutex
	perfCharts   map[string]struct{}
	reloadCh     chan struct{}
	reloadCancel context.CancelFunc
	reloadWG     sync.WaitGroup
	dirWatchers  []*directoryWatcher
	runCtx       context.Context
	periods      *timeperiod.Set
	vnodeInfo    map[string]runtime.VnodeInfo
	missingVnode map[string]struct{}
}

// New returns a collector with sensible defaults so the module registry can instantiate jobs.
func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery:     module.UpdateEvery,
			ExecutorWorkers: 50,
			UserMacros:      make(map[string]string),
			Defaults:        config.Defaults{CheckPeriod: timeperiod.DefaultPeriodName},
			TimePeriods:     []timeperiod.Config{timeperiod.DefaultPeriodConfig()},
			Logging: LoggingConfig{
				Enabled: true,
				OTLP: OTLPLoggingConfig{
					Endpoint: runtime.DefaultOTLPEndpoint,
					Timeout:  confopt.Duration(runtime.DefaultOTLPTimeout),
					Insecure: true,
					Headers:  make(map[string]string),
				},
			},
		},
		charts:     &module.Charts{},
		perfCharts: make(map[string]struct{}),
	}
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = module.UpdateEvery
	}
	if c.ExecutorWorkers <= 0 {
		c.ExecutorWorkers = 50
	}
	if c.charts == nil {
		c.charts = &module.Charts{}
	}
	if c.UserMacros == nil {
		c.UserMacros = make(map[string]string)
	}
	c.Logging.setDefaults()
	jobs, err := c.expandJobSpecs()
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return fmt.Errorf("no Nagios jobs defined in config '%s'", c.Name)
	}
	if err := c.compileTimePeriods(); err != nil {
		return err
	}
	c.refreshVnodeInfo()
	if ctx == nil {
		ctx = context.Background()
	}
	c.runCtx = ctx
	if err := c.rebuildRuntime(ctx, jobs); err != nil {
		return err
	}
	if err := c.startReloadInfrastructure(ctx); err != nil {
		return err
	}
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
	c.schedulerMu.RLock()
	sched := c.scheduler
	c.schedulerMu.RUnlock()
	if sched == nil {
		return nil
	}
	return sched.CollectMetrics()
}

func (c *Collector) Cleanup(context.Context) {
	if c.reloadCancel != nil {
		c.reloadCancel()
	}
	for _, w := range c.dirWatchers {
		w.Close()
	}
	c.reloadWG.Wait()
	c.stopScheduler()
}

func (c *Collector) expandJobSpecs() ([]spec.JobSpec, error) {
	defaults := c.Defaults
	var specs []spec.JobSpec

	for _, jobCfg := range c.Jobs {
		cfg := jobCfg
		defaults.Apply(&cfg)
		cfg.SetDefaults()
		if cfg.Vnode == "" {
			cfg.Vnode = c.Vnode
		}
		sp, err := cfg.ToSpec()
		if err != nil {
			return nil, fmt.Errorf("job '%s': %w", cfg.Name, err)
		}
		specs = append(specs, sp)
	}

	for _, dirCfg := range c.Directories {
		expanded, err := dirCfg.Expand(defaults)
		if err != nil {
			return nil, fmt.Errorf("directory '%s': %w", dirCfg.Path, err)
		}
		for _, cfg := range expanded {
			cfg.SetDefaults()
			if cfg.Vnode == "" {
				cfg.Vnode = c.Vnode
			}
			sp, err := cfg.ToSpec()
			if err != nil {
				return nil, fmt.Errorf("job '%s': %w", cfg.Name, err)
			}
			specs = append(specs, sp)
		}
	}

	return specs, nil
}

func (c *Collector) vnodeInfoFor(job spec.JobSpec) runtime.VnodeInfo {
	key := strings.ToLower(strings.TrimSpace(job.Vnode))
	if key == "" {
		return runtime.VnodeInfo{}
	}
	if info, ok := c.vnodeInfo[key]; ok {
		return cloneVnodeInfo(info)
	}
	c.warnMissingVnode(job.Vnode)
	return runtime.VnodeInfo{Hostname: job.Vnode}
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
	var defs []*module.Chart
	for idx, job := range c.jobSpecs {
		defs = append(defs, charts.BuildJobCharts(c.Name, job.Name, 100+idx*10)...)
	}
	defs = append(defs, charts.BuildSchedulerCharts(c.Name, 10)...)
	newCharts := module.Charts{}
	if err := newCharts.Add(defs...); err != nil {
		return err
	}
	c.chartMu.Lock()
	c.charts = &newCharts
	c.chartMu.Unlock()
	return nil
}

func (c *Collector) registerPerfdataChart(job spec.JobSpec, label string) {
	key := fmt.Sprintf("%s|%s", ids.Sanitize(job.Name), ids.Sanitize(label))
	c.chartMu.Lock()
	if _, ok := c.perfCharts[key]; ok {
		c.chartMu.Unlock()
		return
	}
	chart := charts.PerfdataChart(c.Name, job.Name, label, 200)
	if err := c.charts.Add(chart); err != nil {
		c.chartMu.Unlock()
		c.Errorf("failed to add perfdata chart for job %s label %s: %v", job.Name, label, err)
		return
	}
	c.perfCharts[key] = struct{}{}
	c.chartMu.Unlock()
}

func (c *Collector) rebuildRuntime(ctx context.Context, jobs []spec.JobSpec) error {
	c.stopScheduler()
	c.refreshVnodeInfo()
	c.jobSpecs = jobs
	if err := c.initCharts(); err != nil {
		return err
	}
	c.perfCharts = make(map[string]struct{})
	userMacros := c.copyUserMacros()
	emitter := c.buildEmitter()
	scheduler, err := runtime.NewScheduler(runtime.SchedulerConfig{
		Logger:      c.Logger,
		Jobs:        jobs,
		Workers:     c.ExecutorWorkers,
		Shard:       c.Name,
		Emitter:     emitter,
		UserMacros:  userMacros,
		Periods:     c.periods,
		VnodeLookup: c.vnodeInfoFor,
		RegisterPerfdata: func(job spec.JobSpec, label string) {
			c.registerPerfdataChart(job, label)
		},
	})
	if err != nil {
		_ = emitter.Close()
		return err
	}
	if err := scheduler.Start(ctx); err != nil {
		return err
	}
	c.schedulerMu.Lock()
	c.scheduler = scheduler
	c.schedulerMu.Unlock()
	return nil
}

func (c *Collector) startReloadInfrastructure(ctx context.Context) error {
	reloadCtx, cancel := context.WithCancel(ctx)
	c.reloadCancel = cancel
	c.reloadCh = make(chan struct{}, 1)
	c.dirWatchers = nil

	c.reloadWG.Add(1)
	go c.reloadLoop(reloadCtx)

	if len(c.Directories) == 0 {
		return nil
	}
	for _, dirCfg := range c.Directories {
		dc := dirCfg
		watcher, err := newDirectoryWatcher(reloadCtx, c.Logger, dc, c.requestReload)
		if err != nil {
			c.Warningf("nagios directory watch failed for %s: %v (falling back to periodic rescan)", dc.Path, err)
			c.spawnPeriodicRescan(reloadCtx, dc)
			continue
		}
		c.dirWatchers = append(c.dirWatchers, watcher)
	}
	return nil
}

func (c *Collector) reloadLoop(ctx context.Context) {
	defer c.reloadWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.reloadCh:
			c.reloadMu.Lock()
			jobs, err := c.expandJobSpecs()
			if err != nil {
				c.Errorf("nagios reload failed: %v", err)
				c.reloadMu.Unlock()
				continue
			}
			if len(jobs) == 0 {
				c.Warningf("nagios reload skipped: no jobs after expansion")
				c.reloadMu.Unlock()
				continue
			}
			if err := c.compileTimePeriods(); err != nil {
				c.Errorf("nagios reload failed to compile time periods: %v", err)
				c.reloadMu.Unlock()
				continue
			}
			if err := c.rebuildRuntime(ctx, jobs); err != nil {
				c.Errorf("nagios rebuild failed: %v", err)
			}
			c.reloadMu.Unlock()
		}
	}
}

func (c *Collector) requestReload() {
	select {
	case c.reloadCh <- struct{}{}:
	default:
	}
}

func (c *Collector) spawnPeriodicRescan(ctx context.Context, dirCfg config.DirectoryConfig) {
	c.reloadWG.Add(1)
	go func() {
		defer c.reloadWG.Done()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.requestReload()
			}
		}
	}()
}

func (c *Collector) stopScheduler() {
	c.schedulerMu.Lock()
	defer c.schedulerMu.Unlock()
	if c.scheduler != nil {
		c.scheduler.Stop()
		c.scheduler = nil
	}
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
	configs := c.TimePeriods
	configs = timeperiod.EnsureDefault(configs)
	set, err := timeperiod.Compile(configs)
	if err != nil {
		return err
	}
	c.periods = set
	c.TimePeriods = configs
	return nil
}

func (c *Collector) buildEmitter() runtime.ResultEmitter {
	if c.Logging.Enabled {
		cfg := c.Logging.emitterConfig()
		emitter, err := runtime.NewOTLPEmitter(cfg, c.Logger)
		if err == nil {
			return emitter
		}
		c.Errorf("failed to initialize OTLP emitter: %v", err)
	}
	return runtime.NewLogEmitter(c.Logger)
}
