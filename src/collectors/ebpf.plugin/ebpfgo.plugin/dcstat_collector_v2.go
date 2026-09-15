// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "charts/dcstat.yaml"
var dcstatChartTemplateV2 string

//go:embed "config_schemas/dcstat_schema.json"
var dcstatConfigSchema string

func init() {
	collectorapi.Register("dcstat", collectorapi.Creator{
		JobConfigSchema: dcstatConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewDCStatCollector() },
		Config:   func() any { return &DCStatConfig{} },
	})
}

type DCStatConfig struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	AppsEnabled    bool   `yaml:"apps" json:"apps"`
	CgroupsEnabled bool   `yaml:"cgroups" json:"cgroups"`
}

type DCStatCollector struct {
	collectorapi.Base
	Config    DCStatConfig
	handle    *DCStatLegacyHandle
	state     *dcstatGlobalState
	publisher *PublisherService
}

func NewDCStatCollector() *DCStatCollector {
	return &DCStatCollector{
		Config: DCStatConfig{
			Enabled:     true,
			UpdateEvery: dcstatDefaultUpdateEvery,
		},
		publisher: GetPublisher(),
		state:     &dcstatGlobalState{},
	}
}

func (c *DCStatCollector) Configuration() any {
	return c.Config
}

func (c *DCStatCollector) Init(ctx context.Context) error {
	legacyCfg, err := resolveDCStatLegacyConfig()
	if err == nil && legacyCfg.Enabled {
		c.Config.Enabled = true
		c.Config.AppsEnabled = legacyCfg.AppsEnabled
		c.Config.CgroupsEnabled = legacyCfg.CgroupsEnabled
	}

	if !c.Config.Enabled {
		return errors.New("dcstat disabled in configuration")
	}

	handle, err := LoadDCStatLegacy(DCStatLegacyConfig{
		Enabled:        c.Config.Enabled,
		AppsEnabled:    c.Config.AppsEnabled,
		CgroupsEnabled: c.Config.CgroupsEnabled,
		UpdateEvery:    c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load dcstat: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("dcstat runtime initialization failed")
	}

	c.handle = handle
	return nil
}

func (c *DCStatCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("dcstat not initialized")
	}
	_, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	return err
}

func (c *DCStatCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("dcstat not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	counters := dcstatGlobalCounters{
		GetpageNonvectorAddr: snapshot.GetpageNonvectorAddr,
		GetpageVectorAddr:    snapshot.GetpageVectorAddr,
		Filemap_gfp_mask:     snapshot.Filemap_gfp_mask,
		PageCacheInsertIon:   snapshot.PageCacheInsertIon,
	}

	publish, ok := c.state.Update(counters)
	if !ok {
		return nil
	}

	meter := c.publisher.MetricStore().Write().SnapshotMeter("")
	meter.Counter("ratio").ObserveTotal(float64(publish.Ratio))
	meter.Counter("getpage_vect").ObserveTotal(float64(publish.GetpageVect))
	meter.Counter("getpage_nonvect").ObserveTotal(float64(publish.GetpageNonvect))
	meter.Counter("cache_insertion").ObserveTotal(float64(publish.CacheInsertion))

	if c.Config.AppsEnabled || c.Config.CgroupsEnabled {
		apps, err := c.handle.Runtime.SnapshotApps(c.handle.MapsPerCore)
		if err != nil {
			c.Debugf("snapshot apps error: %v", err)
		} else {
			_ = apps
		}
	}

	return nil
}

func (c *DCStatCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *DCStatCollector) ChartTemplateYAML() string {
	return dcstatChartTemplateV2
}

func (c *DCStatCollector) MetricStore() metrix.CollectorStore {
	return c.publisher.MetricStore()
}
