// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	_ "embed"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/modules/vsphere/match"
	rs "github.com/netdata/netdata/go/go.d.plugin/modules/vsphere/resources"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

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
	return &VSphere{
		Config: Config{
			HTTP: web.HTTP{
				Client: web.Client{
					Timeout: web.Duration(time.Second * 20),
				},
			},
			DiscoveryInterval: web.Duration(time.Minute * 5),
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
	web.HTTP          `yaml:",inline" json:""`
	UpdateEvery       int                `yaml:"update_every" json:"update_every"`
	DiscoveryInterval web.Duration       `yaml:"discovery_interval" json:"discovery_interval"`
	HostsInclude      match.HostIncludes `yaml:"host_include" json:"host_include"`
	VMsInclude        match.VMIncludes   `yaml:"vm_include" json:"vm_include"`
}

type (
	VSphere struct {
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

func (vs *VSphere) Configuration() any {
	return vs.Config
}

func (vs *VSphere) Init() error {
	if err := vs.validateConfig(); err != nil {
		vs.Errorf("error on validating config: %v", err)
		return err
	}

	vsClient, err := vs.initClient()
	if err != nil {
		vs.Errorf("error on creating vsphere client: %v", err)
		return err
	}

	if err := vs.initDiscoverer(vsClient); err != nil {
		vs.Errorf("error on creating vsphere discoverer: %v", err)
		return err
	}

	vs.initScraper(vsClient)

	if err := vs.discoverOnce(); err != nil {
		vs.Errorf("error on discovering: %v", err)
		return err
	}

	vs.goDiscovery()

	return nil
}

func (vs *VSphere) Check() error {
	return nil
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
