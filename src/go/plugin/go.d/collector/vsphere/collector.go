// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/vmware/govmomi/performance"
	mo25 "github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	clientpkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	scrapepkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("vsphere", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 20,
		},
		CreateV2:      func() collectorapi.CollectorV2 { return New() },
		Config:        func() any { return &Config{} },
		Methods:       vsphereMethods,
		MethodHandler: vsphereMethodHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)

	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 20),
				},
			},
			DiscoveryInterval:        confopt.Duration(time.Minute * 5),
			HostsInclude:             match.HostIncludes{"/*"},
			VMsInclude:               match.VMIncludes{"/*"},
			DatastoresInclude:        match.DatastoreIncludes{"/*"},
			ClustersInclude:          match.ClusterIncludes{"/*"},
			DatastoreClustersInclude: match.DatastoreClusterIncludes{"/*"},
			CollectVSAN:              false,
			VSANClustersInclude:      match.VSANClusterIncludes{"/*"},
			VSANHostsInclude:         match.VSANHostIncludes{"/*"},
			VSANVMsInclude:           match.VSANVMIncludes{"/*"},
		},
		store:          store,
		mx:             mx,
		collectionLock: &sync.RWMutex{},
	}
}

type Config struct {
	// Job identity and scheduling.
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`

	// Inventory discovery and resource selectors.
	DiscoveryInterval        confopt.Duration               `yaml:"discovery_interval,omitempty" json:"discovery_interval"`
	HostsInclude             match.HostIncludes             `yaml:"host_include,omitempty" json:"host_include"`
	VMsInclude               match.VMIncludes               `yaml:"vm_include,omitempty" json:"vm_include"`
	DatastoresInclude        match.DatastoreIncludes        `yaml:"datastore_include,omitempty" json:"datastore_include"`
	ClustersInclude          match.ClusterIncludes          `yaml:"cluster_include,omitempty" json:"cluster_include"`
	CollectDatastoreClusters bool                           `yaml:"collect_datastore_clusters,omitempty" json:"collect_datastore_clusters"`
	DatastoreClustersInclude match.DatastoreClusterIncludes `yaml:"datastore_cluster_include,omitempty" json:"datastore_cluster_include"`
	CollectVSAN              bool                           `yaml:"collect_vsan,omitempty" json:"collect_vsan"`
	VSANClustersInclude      match.VSANClusterIncludes      `yaml:"vsan_cluster_include,omitempty" json:"vsan_cluster_include"`
	VSANHostsInclude         match.VSANHostIncludes         `yaml:"vsan_host_include,omitempty" json:"vsan_host_include"`
	VSANVMsInclude           match.VSANVMIncludes           `yaml:"vsan_vm_include,omitempty" json:"vsan_vm_include"`

	// Opt-in label enrichment.
	TagCategories    []string `yaml:"tag_categories,omitempty" json:"tag_categories"`
	CustomAttributes []string `yaml:"custom_attributes,omitempty" json:"custom_attributes"`

	// Optional cached topology Function data.
	CollectNetworkTopology bool `yaml:"collect_network_topology,omitempty" json:"collect_network_topology"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		store metrix.CollectorStore
		mx    *collectorMetrics

		vsClient *clientpkg.Client
		discoverer
		scraper
		dsPropertyCollector
		clusterPropertyCollector
		rpPropertyCollector

		collectionLock            *sync.RWMutex
		resources                 *rs.Resources
		discoveryTask             *task
		datastoreClusterMatcher   match.DatastoreClusterMatcher
		vsanClusterMatcher        match.VSANClusterMatcher
		vsanHostMatcher           match.VSANHostMatcher
		vsanVMMatcher             match.VSANVMMatcher
		vsphereTagCategoryMatcher matcher.Matcher
		customAttributeMatcher    matcher.Matcher
		hostPowerPerfSamples      map[string]*hostPowerPerfSample
		vmPowerPerfSamples        map[string]*vmPowerPerfSample
		vsanMetrics               *scrapepkg.VSANMetrics
	}
	discoverer interface {
		Discover() (*rs.Resources, error)
	}
	scraper interface {
		ScrapeHosts(rs.Hosts) []performance.EntityMetric
		ScrapeVMs(rs.VMs) []performance.EntityMetric
		ScrapeDatastores(rs.Datastores) []performance.EntityMetric
		ScrapeClusters(rs.Clusters) []performance.EntityMetric
		ScrapeVSAN(rs.Clusters, rs.Hosts, rs.VMs) *scrapepkg.VSANMetrics
	}
	dsPropertyCollector interface {
		DatastoresByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo25.Datastore, error)
	}
	clusterPropertyCollector interface {
		ClustersByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo25.ClusterComputeResource, error)
	}
	rpPropertyCollector interface {
		ResourcePoolsByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo25.ResourcePool, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	c.ensureRuntimeState()
	c.stopDiscoveryTask(true)
	c.closeClient()
	c.resetRuntimeStateForInit()

	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("validate vSphere collector configuration: %w", err)
	}

	vsClient, err := c.initClient()
	if err != nil {
		return fmt.Errorf("create vSphere client: %w", err)
	}
	c.vsClient = vsClient

	if err := c.initDiscoverer(vsClient); err != nil {
		c.closeClient()
		return fmt.Errorf("create vSphere discoverer from configuration: %w", err)
	}

	c.initScraper(vsClient)

	if err := c.discoverOnce(); err != nil {
		c.closeClient()
		return fmt.Errorf("run initial vSphere discovery: %w", err)
	}

	c.goDiscovery()

	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Collect(context.Context) error {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	if err := c.collectLocked(); err != nil {
		return fmt.Errorf("collect vSphere metrics: %w", err)
	}

	return nil
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) Cleanup(context.Context) {
	c.stopDiscoveryTask(true)
	c.closeClient()
}

func (c *Collector) ensureRuntimeState() {
	if c.collectionLock == nil {
		c.collectionLock = &sync.RWMutex{}
	}
	if c.store == nil {
		c.store = metrix.NewCollectorStore()
	}
	if c.mx == nil {
		c.mx = newCollectorMetrics(c.store)
	}
}

func (c *Collector) resetRuntimeStateForInit() {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	c.discoverer = nil
	c.scraper = nil
	c.dsPropertyCollector = nil
	c.clusterPropertyCollector = nil
	c.rpPropertyCollector = nil
	c.resources = nil
	c.datastoreClusterMatcher = nil
	c.vsanClusterMatcher = nil
	c.vsanHostMatcher = nil
	c.vsanVMMatcher = nil
	c.vsphereTagCategoryMatcher = nil
	c.customAttributeMatcher = nil
	c.hostPowerPerfSamples = nil
	c.vmPowerPerfSamples = nil
	c.vsanMetrics = nil
}

func (c *Collector) closeClient() {
	if c.vsClient == nil {
		return
	}
	if err := c.vsClient.Close(); err != nil {
		c.Warningf("close vSphere client during collector cleanup: %v", err)
	}
	c.vsClient = nil
}

func (c *Collector) stopDiscoveryTask(wait bool) {
	if c.discoveryTask == nil {
		return
	}
	c.discoveryTask.stop()
	if wait {
		c.discoveryTask.wait()
	}
}
