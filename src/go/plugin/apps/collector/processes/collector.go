// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/apps/collector/processes/processesfunc"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/grouping"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/native"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplate string

func init() {
	collectorapi.Register("processes", collectorapi.Creator{
		Defaults:        collectorapi.Defaults{UpdateEvery: 1},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		JobConfigSchema: configSchema,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: func() []funcapi.FunctionConfig { return processesfunc.Methods(1) },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			if c, ok := job.Collector().(*Collector); ok {
				return c.function
			}
			return nil
		},
	})
}

type Config struct {
	UpdateEvery int             `yaml:"update_every" json:"update_every"`
	ProcPath    string          `yaml:"proc_path" json:"proc_path"`
	CollectFDs  bool            `yaml:"collect_fds" json:"collect_fds"`
	CollectPSS  bool            `yaml:"collect_pss" json:"collect_pss"`
	Groups      []grouping.Rule `yaml:"groups" json:"groups"`
}

type scanner interface {
	Scan(context.Context) (model.Snapshot, error)
	Finalize(uint64, []model.Assignment) ([]model.GroupFD, error)
	Close()
}

type Collector struct {
	collectorapi.Base
	Config     `yaml:",inline" json:""`
	store      metrix.CollectorStore
	metrics    metricInstruments
	mu         sync.Mutex
	scanner    scanner
	grouping   *grouping.Engine
	snapshot   atomic.Pointer[model.Snapshot]
	function   funcapi.MethodHandler
	newScanner func(native.Options) (scanner, error)
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{
		Config: Config{
			UpdateEvery: 1,
			ProcPath:    filepath.Join(pluginconfig.HostPrefix(), "/proc"),
			CollectFDs:  true,
			CollectPSS:  true,
			Groups:      []grouping.Rule{},
		},
		store:      store,
		metrics:    newMetricInstruments(store),
		newScanner: func(opts native.Options) (scanner, error) { return native.New(opts) },
	}
	c.function = processesfunc.NewRouter(c)
	return c
}

func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplate }
func (c *Collector) CurrentSnapshot() *model.Snapshot   { return c.snapshot.Load() }

func (c *Collector) Init(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scanner != nil {
		return errors.New("collector already initialized")
	}
	if c.UpdateEvery < 1 {
		return collectorapi.PermanentError(errors.New("update_every must be at least 1 second"))
	}
	if !filepath.IsAbs(c.ProcPath) {
		return collectorapi.PermanentError(errors.New("proc_path must be an absolute procfs path"))
	}
	engine, err := grouping.New(c.Groups)
	if err != nil {
		return collectorapi.PermanentError(fmt.Errorf("groups: %w", err))
	}
	s, err := c.newScanner(native.Options{ProcPath: c.ProcPath, CollectFDs: c.CollectFDs, CollectPSS: c.CollectPSS})
	if err != nil {
		return fmt.Errorf("create native scanner: %w", err)
	}
	c.grouping, c.scanner = engine, s
	return nil
}

// Check probes availability without scanning processes or modifying baselines.
func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scanner == nil {
		return errors.New("collector not initialized")
	}
	st, err := os.Stat(c.ProcPath)
	if err != nil {
		return fmt.Errorf("procfs: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("proc_path is not a directory")
	}
	f, err := os.Open(filepath.Join(c.ProcPath, "stat"))
	if err != nil {
		return fmt.Errorf("procfs CPU statistics: %w", err)
	}
	return f.Close()
}

func (c *Collector) Cleanup(context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot.Store(nil)
	if c.scanner != nil {
		c.scanner.Close()
		c.scanner = nil
	}
	c.grouping = nil
}

var _ collectorapi.CollectorV2 = (*Collector)(nil)
