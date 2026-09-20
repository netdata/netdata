// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/smbiosfunc"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

const defaultUpdateEvery = 60

func init() {
	collectorapi.Register("smbios_memory", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: func() []funcapi.FunctionConfig { return smbiosfunc.Methods(defaultUpdateEvery) },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			if c, ok := job.Collector().(*Collector); ok {
				return c.router
			}
			return nil
		},
	})
}

type Config struct {
	// Recognize the framework key only to reject moving host inventory to a vnode.
	Vnode       string `yaml:"vnode,omitempty"        json:"vnode,omitempty"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
}

type Collector struct {
	collectorapi.Base
	Config      `yaml:",inline" json:""`
	store       metrix.CollectorStore
	metrics     *collectorMetrics
	router      funcapi.MethodHandler
	snapshot    atomic.Pointer[inventory.Snapshot]
	read        func() (*inventory.Table, error)
	now         func() time.Time
	statePath   string
	owner       string
	state       *persistentState
	stateErr    error
	dirty       bool
	stateLoaded bool
}

type functionDeps struct {
	snapshot *atomic.Pointer[inventory.Snapshot]
}

func (d functionDeps) CurrentSnapshot() *inventory.Snapshot { return d.snapshot.Load() }

func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
		},
		store:   store,
		metrics: newCollectorMetrics(store),
		now:     time.Now,
	}
	c.router = smbiosfunc.NewRouter(functionDeps{
		snapshot: &c.snapshot,
	})
	return c
}

func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplateYAML }

func (c *Collector) Init(context.Context) error {
	if c.Vnode != "" {
		return errors.New("smbios_memory monitors the local host and does not support vnode")
	}
	if c.UpdateEvery <= 0 {
		return errors.New("update_every must be positive")
	}
	if c.read == nil {
		dir := filepath.Join(pluginconfig.HostPrefix(), "/sys/firmware/dmi/tables")
		c.read = func() (*inventory.Table, error) {
			entry, err := os.ReadFile(filepath.Join(dir, "smbios_entry_point"))
			if err != nil {
				return nil, fmt.Errorf("read SMBIOS entry point: %w", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "DMI"))
			if err != nil {
				return nil, fmt.Errorf("read DMI table: %w", err)
			}
			return parseTable(entry, data)
		}
	}
	if c.statePath == "" {
		dir := pluginconfig.VarLibDir()
		if dir == "" {
			dir = buildinfo.VarLibDir
		}
		if dir == "" {
			dir = buildinfo.DefaultVarLibDir
		}
		c.statePath = filepath.Join(dir, "smbios-memory.json")
	}
	if c.owner == "" {
		c.owner = pluginconfig.RegistryUniqueID()
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Preserve monitoring of a previously enrolled host during source failures,
	// including an invalid state file that needs operator attention.
	if _, err := os.Stat(c.statePath); !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_, err := c.read()
	return err
}

func (c *Collector) Cleanup(context.Context) { c.snapshot.Store(nil) }
