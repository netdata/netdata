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

//go:embed "charts/socket.yaml"
var socketChartTemplateV2 string

//go:embed "config_schemas/socket_schema.json"
var socketConfigSchema string

func init() {
	collectorapi.Register("socket", collectorapi.Creator{
		JobConfigSchema: socketConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewSocketCollector() },
		Config:   func() any { return &SocketConfig{} },
	})
}

type SocketConfig struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
}

type SocketCollector struct {
	collectorapi.Base
	Config SocketConfig
	handle *SocketLegacyHandle
	store  metrix.CollectorStore
}

func NewSocketCollector() *SocketCollector {
	return &SocketCollector{
		Config: SocketConfig{
			Enabled:     true,
			UpdateEvery: socketDefaultUpdateEvery,
		},
		store: metrix.NewCollectorStore(),
	}
}

func (c *SocketCollector) Configuration() any {
	return c.Config
}

func (c *SocketCollector) Init(ctx context.Context) error {
	legacyCfg, err := resolveSocketLegacyConfig()
	if err == nil && legacyCfg.Enabled {
		c.Config.Enabled = true
	}

	if !c.Config.Enabled {
		return errors.New("socket disabled in configuration")
	}

	handle, err := LoadSocketLegacy(SocketLegacyConfig{
		Enabled:     c.Config.Enabled,
		UpdateEvery: c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load socket: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("socket runtime initialization failed")
	}

	c.handle = handle
	return nil
}

func (c *SocketCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("socket not initialized")
	}
	_, err := c.handle.Runtime.Snapshot()
	return err
}

func (c *SocketCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("socket not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot()
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	meter := c.store.Write().SnapshotMeter("")
	meter.Counter("ipv4_send").ObserveTotal(float64(snapshot.Ipv4Send))
	meter.Counter("ipv4_recv").ObserveTotal(float64(snapshot.Ipv4Recv))
	meter.Counter("ipv6_send").ObserveTotal(float64(snapshot.Ipv6Send))
	meter.Counter("ipv6_recv").ObserveTotal(float64(snapshot.Ipv6Recv))

	return nil
}

func (c *SocketCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *SocketCollector) ChartTemplateYAML() string {
	return socketChartTemplateV2
}

func (c *SocketCollector) MetricStore() metrix.CollectorStore {
	return c.store
}
