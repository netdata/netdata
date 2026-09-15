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

//go:embed "charts/dns.yaml"
var dnsChartTemplateV2 string

//go:embed "config_schemas/dns_schema.json"
var dnsConfigSchema string

func init() {
	collectorapi.Register("dns", collectorapi.Creator{
		JobConfigSchema: dnsConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewDNSCollector() },
		Config:   func() any { return &DNSConfig{} },
	})
}

type DNSConfig struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
}

type DNSCollector struct {
	collectorapi.Base
	Config    DNSConfig
	handle    *DNSLegacyHandle
	publisher *PublisherService
}

func NewDNSCollector() *DNSCollector {
	return &DNSCollector{
		Config: DNSConfig{
			Enabled:     true,
			UpdateEvery: dnsDefaultUpdateEvery,
		},
		publisher: GetPublisher(),
	}
}

func (c *DNSCollector) Configuration() any {
	return c.Config
}

func (c *DNSCollector) Init(ctx context.Context) error {
	legacyCfg, err := resolveDNSLegacyConfig()
	if err == nil && legacyCfg.Enabled {
		c.Config.Enabled = true
	}

	if !c.Config.Enabled {
		return errors.New("dns disabled in configuration")
	}

	handle, err := LoadDNSLegacy(DNSLegacyConfig{
		Enabled:     c.Config.Enabled,
		UpdateEvery: c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load dns: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("dns runtime initialization failed")
	}

	c.handle = handle
	return nil
}

func (c *DNSCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("dns not initialized")
	}
	_, err := c.handle.Runtime.Snapshot()
	return err
}

func (c *DNSCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("dns not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot()
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	meter := c.publisher.MetricStore().Write().SnapshotMeter("")
	meter.Counter("requests").ObserveTotal(float64(snapshot.Requests))
	meter.Counter("responses").ObserveTotal(float64(snapshot.Responses))

	return nil
}

func (c *DNSCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *DNSCollector) ChartTemplateYAML() string {
	return dnsChartTemplateV2
}

func (c *DNSCollector) MetricStore() metrix.CollectorStore {
	return c.publisher.MetricStore()
}
