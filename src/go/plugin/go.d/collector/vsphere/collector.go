// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/vmware/govmomi/performance"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("vsphere", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 20,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 20),
				},
			},
			DiscoveryInterval: confopt.Duration(time.Minute * 5),
			HostsInclude:      []string{"/*"},
			VMsInclude:        []string{"/*"},
		},
		collectionLock:  &sync.RWMutex{},
		charts:          &module.Charts{},
		discoveredHosts: make(map[string]int),
		discoveredVMs:   make(map[string]int),
		charted:         make(map[string]bool),
	}
}

type Config struct {
	Vnode             string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery       int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig    `yaml:",inline" json:""`
	DiscoveryInterval confopt.Duration   `yaml:"discovery_interval,omitempty" json:"discovery_interval"`
	HostsInclude      match.HostIncludes `yaml:"host_include,omitempty" json:"host_include"`
	VMsInclude        match.VMIncludes   `yaml:"vm_include,omitempty" json:"vm_include"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		discoverer
		scraper

		collectionLock  *sync.RWMutex
		resources       *rs.Resources
		discoveryTask   *task
		discoveredHosts map[string]int
		discoveredVMs   map[string]int
		charted         map[string]bool
	}
	discoverer interface {
		Discover() (*rs.Resources, error)
	}
	scraper interface {
		ScrapeHosts(rs.Hosts) []performance.EntityMetric
		ScrapeVMs(rs.VMs) []performance.EntityMetric
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("error on validating config: %v", err)
	}

	vsClient, err := c.initClient()
	if err != nil {
		return fmt.Errorf("error on creating vsphere client: %v", err)
	}

	if err := c.initDiscoverer(vsClient); err != nil {
		return fmt.Errorf("error on creating vsphere discoverer: %v", err)
	}

	c.initScraper(vsClient)

	if err := c.discoverOnce(); err != nil {
		return fmt.Errorf("error on discovering: %v", err)
	}

	c.goDiscovery()

	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.discoveryTask == nil {
		return
	}
	c.discoveryTask.stop()
}
