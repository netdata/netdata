// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

//go:embed "charts/cachestat.yaml"
var cachestatChartTemplateV2 string

//go:embed "config_schemas/cachestat_schema.json"
var cachestatConfigSchema string

func init() {
	collectorapi.Register("cachestat", collectorapi.Creator{
		JobConfigSchema: cachestatConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewCachestatCollector() },
		Config:   func() any { return &CachestatConfig{} },
	})
}

type CachestatConfig struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	// DynCfg fields (from config_schema.json)
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	AppsEnabled    bool   `yaml:"apps" json:"apps"`
	CgroupsEnabled bool   `yaml:"cgroups" json:"cgroups"`
	MapPerCore     bool   `yaml:"per_core_stats" json:"per_core_stats"`
	KernelDebugfs  string `yaml:"kernel_debugfs" json:"kernel_debugfs"`
}

type CachestatCollector struct {
	collectorapi.Base
	Config CachestatConfig

	handle      *CachestatLegacyHandle
	state       *cachestatGlobalState
	store       metrix.CollectorStore
	sharedStore *ebpfSharedMemoryStore
}

func NewCachestatCollector() *CachestatCollector {
	return &CachestatCollector{
		Config: CachestatConfig{
			Enabled:        true,
			AppsEnabled:    false,
			CgroupsEnabled: false,
			UpdateEvery:    cachestatDefaultUpdateEvery,
		},
		store: metrix.NewCollectorStore(),
		state: &cachestatGlobalState{},
	}
}

func (c *CachestatCollector) Configuration() any {
	return c.Config
}

func (c *CachestatCollector) Init(ctx context.Context) error {
	// Agent provides config from YAML. Fall back to legacy config if not set.
	if !c.Config.Enabled {
		// Try legacy config as fallback
		legacyCfg, err := resolveCachestatLegacyConfig()
		if err == nil && legacyCfg.Enabled {
			c.Config.Enabled = true
			c.Config.AppsEnabled = legacyCfg.AppsEnabled
			c.Config.CgroupsEnabled = legacyCfg.CgroupsEnabled
		} else {
			return errors.New("cachestat disabled in configuration")
		}
	}

	c.Debugf("initializing cachestat: apps=%v cgroups=%v", c.Config.AppsEnabled, c.Config.CgroupsEnabled)

	// Load eBPF program
	handle, err := LoadCachestatLegacy(CachestatLegacyConfig{
		Enabled:        c.Config.Enabled,
		AppsEnabled:    c.Config.AppsEnabled,
		CgroupsEnabled: c.Config.CgroupsEnabled,
		UpdateEvery:    c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load cachestat: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("cachestat runtime initialization failed")
	}

	c.handle = handle
	return nil
}

func (c *CachestatCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("cachestat not initialized")
	}

	_, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	return err
}

func (c *CachestatCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("cachestat not initialized")
	}

	// Global snapshot
	snapshot, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	counters := cachestatGlobalCounters{
		MarkPageAccessed:   snapshot.MarkPageAccessed,
		MarkBufferDirty:    snapshot.MarkBufferDirty,
		AddToPageCacheLru:  snapshot.AddToPageCacheLru,
		AccountPageDirtied: snapshot.AccountPageDirtied,
	}

	publish, ok := c.state.Update(counters)
	if !ok {
		return nil
	}

	// Write metrics to store
	meter := c.store.Write().SnapshotMeter("")
	meter.Gauge("ratio").Observe(float64(publish.Ratio))
	meter.Counter("dirty").ObserveTotal(float64(publish.Dirty))
	meter.Counter("hit").ObserveTotal(float64(publish.Hit))
	meter.Counter("miss").ObserveTotal(float64(publish.Miss))

	// Per-PID snapshot
	if c.sharedStore != nil && (c.Config.AppsEnabled || c.Config.CgroupsEnabled) {
		apps, err := c.handle.Runtime.SnapshotApps(c.handle.MapsPerCore)
		if err != nil {
			c.Debugf("snapshot apps error: %v", err)
		} else {
			staleCandidates := c.sharedStore.UpdateApps(apps)
			if len(staleCandidates) > 0 {
				deadPIDs := staleCandidates[:0]
				for _, pid := range staleCandidates {
					if !libbpfloader.PidIsAlive(pid) {
						deadPIDs = append(deadPIDs, pid)
					}
				}
				if len(deadPIDs) > 0 {
					if err := c.handle.Runtime.DeletePids(deadPIDs); err != nil {
						c.Debugf("failed to delete stale PIDs: %v", err)
					} else {
						c.sharedStore.RemoveCachestatPIDs(deadPIDs)
					}
				}
			}
		}
	}

	return nil
}

func (c *CachestatCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *CachestatCollector) ChartTemplateYAML() string {
	return cachestatChartTemplateV2
}

func (c *CachestatCollector) MetricStore() metrix.CollectorStore {
	return c.store
}
