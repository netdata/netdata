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

//go:embed "charts/fd.yaml"
var fdChartTemplateV2 string

//go:embed "config_schemas/fd_schema.json"
var fdConfigSchema string

func init() {
	collectorapi.Register("fd", collectorapi.Creator{
		JobConfigSchema: fdConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewFDCollector() },
		Config:   func() any { return &FDConfig{} },
	})
}

type FDConfig struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	AppsEnabled    bool   `yaml:"apps" json:"apps"`
	CgroupsEnabled bool   `yaml:"cgroups" json:"cgroups"`
}

type FDCollector struct {
	collectorapi.Base
	Config FDConfig
	handle *FDLegacyHandle
	state  *fdGlobalState
	store  metrix.CollectorStore
}

func NewFDCollector() *FDCollector {
	return &FDCollector{
		Config: FDConfig{
			Enabled:     true,
			UpdateEvery: fdDefaultUpdateEvery,
		},
		store: metrix.NewCollectorStore(),
		state: &fdGlobalState{},
	}
}

func (c *FDCollector) Configuration() any {
	return c.Config
}

func (c *FDCollector) Init(ctx context.Context) error {
	legacyCfg, _, err := resolveFDConfigForSelection(moduleSelectionNone, resolveFDLegacyConfig)
	if err == nil && legacyCfg.Enabled {
		c.Config.Enabled = true
		c.Config.AppsEnabled = legacyCfg.AppsEnabled
		c.Config.CgroupsEnabled = legacyCfg.CgroupsEnabled
	}

	if !c.Config.Enabled {
		return errors.New("fd disabled in configuration")
	}

	handle, err := LoadFDLegacy(FDLegacyConfig{
		Enabled:        c.Config.Enabled,
		AppsEnabled:    c.Config.AppsEnabled,
		CgroupsEnabled: c.Config.CgroupsEnabled,
		UpdateEvery:    c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load fd: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("fd runtime initialization failed")
	}

	c.handle = handle
	return nil
}

func (c *FDCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("fd not initialized")
	}
	_, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	return err
}

func (c *FDCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("fd not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot(c.handle.MapsPerCore)
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	meter := c.store.Write().SnapshotMeter("")
	meter.Counter("open").ObserveTotal(float64(snapshot.Open))
	meter.Counter("close").ObserveTotal(float64(snapshot.Close))
	meter.Counter("open_error").ObserveTotal(float64(snapshot.OpenErr))

	return nil
}

func (c *FDCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *FDCollector) ChartTemplateYAML() string {
	return fdChartTemplateV2
}

func (c *FDCollector) MetricStore() metrix.CollectorStore {
	return c.store
}
