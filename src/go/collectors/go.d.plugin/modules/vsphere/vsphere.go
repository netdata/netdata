// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	_ "embed"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/modules/vsphere/match"
	rs "github.com/netdata/go.d.plugin/modules/vsphere/resources"
	"github.com/netdata/go.d.plugin/pkg/web"

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
	})
}

func New() *VSphere {
	config := Config{
		HTTP: web.HTTP{
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second * 20},
			},
		},
		DiscoveryInterval: web.Duration{Duration: time.Minute * 5},
		HostsInclude:      []string{"/*"},
		VMsInclude:        []string{"/*"},
	}

	return &VSphere{
		collectionLock:  new(sync.RWMutex),
		Config:          config,
		charts:          &module.Charts{},
		discoveredHosts: make(map[string]int),
		discoveredVMs:   make(map[string]int),
		charted:         make(map[string]bool),
	}
}

type Config struct {
	web.HTTP          `yaml:",inline"`
	DiscoveryInterval web.Duration       `yaml:"discovery_interval"`
	HostsInclude      match.HostIncludes `yaml:"host_include"`
	VMsInclude        match.VMIncludes   `yaml:"vm_include"`
}

type (
	VSphere struct {
		module.Base
		UpdateEvery int `yaml:"update_every"`
		Config      `yaml:",inline"`

		discoverer
		scraper

		collectionLock  *sync.RWMutex
		resources       *rs.Resources
		discoveryTask   *task
		discoveredHosts map[string]int
		discoveredVMs   map[string]int
		charted         map[string]bool
		charts          *module.Charts
	}
	discoverer interface {
		Discover() (*rs.Resources, error)
	}
	scraper interface {
		ScrapeHosts(rs.Hosts) []performance.EntityMetric
		ScrapeVMs(rs.VMs) []performance.EntityMetric
	}
)

func (vs *VSphere) Init() bool {
	if err := vs.validateConfig(); err != nil {
		vs.Errorf("error on validating config: %v", err)
		return false
	}

	vsClient, err := vs.initClient()
	if err != nil {
		vs.Errorf("error on creating vsphere client: %v", err)
		return false
	}

	err = vs.initDiscoverer(vsClient)
	if err != nil {
		vs.Errorf("error on creating vsphere discoverer: %v", err)
		return false
	}

	vs.initScraper(vsClient)

	err = vs.discoverOnce()
	if err != nil {
		vs.Errorf("error on discovering: %v", err)
		return false
	}

	vs.goDiscovery()

	return true
}

func (vs *VSphere) Check() bool {
	return true
}

func (vs *VSphere) Charts() *module.Charts {
	return vs.charts
}

func (vs *VSphere) Collect() map[string]int64 {
	mx, err := vs.collect()
	if err != nil {
		vs.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (vs *VSphere) Cleanup() {
	if vs.discoveryTask == nil {
		return
	}
	vs.discoveryTask.stop()
}
