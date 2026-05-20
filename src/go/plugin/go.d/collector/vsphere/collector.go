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
	charts := inventoryChartsTmpl.Copy()

	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 20),
				},
			},
			DiscoveryInterval:          confopt.Duration(time.Minute * 5),
			HostsInclude:               []string{"/*"},
			VMsInclude:                 []string{"/*"},
			DatastoresInclude:          []string{"/*"},
			ClustersInclude:            []string{"/*"},
			DatastoreClustersInclude:   []string{"/*"},
			VMDisksInclude:             []string{"*"},
			VMNICsInclude:              []string{"*"},
			HostNICsInclude:            []string{"*"},
			HostDisksInclude:           []string{"*"},
			HostStorageAdaptersInclude: []string{"*"},
			HostStoragePathsInclude:    []string{"*"},
			HostCPUInstancesInclude:    []string{"*"},
			CollectVSAN:                false,
			VSANClustersInclude:        []string{"/*"},
			VSANHostsInclude:           []string{"/*"},
			VSANVMsInclude:             []string{"/*"},
		},
		store:                   store,
		mx:                      mx,
		collectionLock:          &sync.RWMutex{},
		charts:                  charts,
		discoveredHosts:         make(map[string]int),
		discoveredVMs:           make(map[string]int),
		discoveredDatastores:    make(map[string]int),
		discoveredClusters:      make(map[string]int),
		discoveredResourcePools: make(map[string]int),
		charted:                 make(map[string]bool),
		datastorePerfReceived:   make(map[string]bool),
		datastorePerfCharted:    make(map[string]bool),
		clusterPerfReceived:     make(map[string]bool),
		clusterPerfCharted:      make(map[string]bool),
	}
}

type Config struct {
	// Job identity and scheduling.
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	DiscoveryInterval  confopt.Duration `yaml:"discovery_interval,omitempty" json:"discovery_interval"`

	// Inventory filters.
	HostsInclude      match.HostIncludes      `yaml:"host_include,omitempty" json:"host_include"`
	VMsInclude        match.VMIncludes        `yaml:"vm_include,omitempty" json:"vm_include"`
	DatastoresInclude match.DatastoreIncludes `yaml:"datastore_include,omitempty" json:"datastore_include"`
	ClustersInclude   match.ClusterIncludes   `yaml:"cluster_include,omitempty" json:"cluster_include"`

	// Opt-in label enrichment.
	TagCategories    []string `yaml:"tag_categories,omitempty" json:"tag_categories"`
	CustomAttributes []string `yaml:"custom_attributes,omitempty" json:"custom_attributes"`

	// Optional datastore cluster metrics.
	CollectDatastoreClusters bool     `yaml:"collect_datastore_clusters,omitempty" json:"collect_datastore_clusters"`
	DatastoreClustersInclude []string `yaml:"datastore_cluster_include,omitempty" json:"datastore_cluster_include"`

	// Optional VM child-instance metrics.
	CollectVMDisks           bool     `yaml:"collect_vm_disks,omitempty" json:"collect_vm_disks"`
	CollectVMDiskPerformance bool     `yaml:"collect_vm_disk_performance,omitempty" json:"collect_vm_disk_performance"`
	VMDisksInclude           []string `yaml:"vm_disk_include,omitempty" json:"vm_disk_include"`
	CollectVMNICPerformance  bool     `yaml:"collect_vm_nic_performance,omitempty" json:"collect_vm_nic_performance"`
	VMNICsInclude            []string `yaml:"vm_nic_include,omitempty" json:"vm_nic_include"`

	// Optional host child-instance metrics.
	CollectHostNICPerformance            bool     `yaml:"collect_host_nic_performance,omitempty" json:"collect_host_nic_performance"`
	HostNICsInclude                      []string `yaml:"host_nic_include,omitempty" json:"host_nic_include"`
	CollectHostDiskPerformance           bool     `yaml:"collect_host_disk_performance,omitempty" json:"collect_host_disk_performance"`
	HostDisksInclude                     []string `yaml:"host_disk_include,omitempty" json:"host_disk_include"`
	CollectHostStorageAdapterPerformance bool     `yaml:"collect_host_storage_adapter_performance,omitempty" json:"collect_host_storage_adapter_performance"`
	HostStorageAdaptersInclude           []string `yaml:"host_storage_adapter_include,omitempty" json:"host_storage_adapter_include"`
	CollectHostStoragePathPerformance    bool     `yaml:"collect_host_storage_path_performance,omitempty" json:"collect_host_storage_path_performance"`
	HostStoragePathsInclude              []string `yaml:"host_storage_path_include,omitempty" json:"host_storage_path_include"`
	CollectHostCPUInstancePerformance    bool     `yaml:"collect_host_cpu_instance_performance,omitempty" json:"collect_host_cpu_instance_performance"`
	HostCPUInstancesInclude              []string `yaml:"host_cpu_instance_include,omitempty" json:"host_cpu_instance_include"`

	// Optional aggregate power metrics.
	CollectPowerMetrics bool `yaml:"collect_power_metrics,omitempty" json:"collect_power_metrics"`

	// Optional vSAN metrics.
	CollectVSAN         bool     `yaml:"collect_vsan,omitempty" json:"collect_vsan"`
	VSANClustersInclude []string `yaml:"vsan_cluster_include,omitempty" json:"vsan_cluster_include"`
	VSANHostsInclude    []string `yaml:"vsan_host_include,omitempty" json:"vsan_host_include"`
	VSANVMsInclude      []string `yaml:"vsan_vm_include,omitempty" json:"vsan_vm_include"`

	// Optional cached topology Function data.
	CollectNetworkTopology bool `yaml:"collect_network_topology,omitempty" json:"collect_network_topology"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts *collectorapi.Charts
		store  metrix.CollectorStore
		mx     *collectorMetrics

		vsClient *clientpkg.Client
		discoverer
		scraper
		dsPropertyCollector
		clusterPropertyCollector
		rpPropertyCollector

		collectionLock          *sync.RWMutex
		resources               *rs.Resources
		discoveryTask           *task
		discoveredHosts         map[string]int
		discoveredVMs           map[string]int
		discoveredDatastores    map[string]int
		discoveredClusters      map[string]int
		discoveredResourcePools map[string]int
		charted                 map[string]bool

		// two-phase chart creation: property charts always, perf charts only when data arrives
		datastorePerfReceived                  map[string]bool
		datastorePerfCharted                   map[string]bool
		clusterPerfReceived                    map[string]bool
		clusterPerfCharted                     map[string]bool
		datastoreClusterMatcher                matcher.Matcher
		vmDiskMatcher                          matcher.Matcher
		vmNICMatcher                           matcher.Matcher
		hostNICMatcher                         matcher.Matcher
		hostDiskMatcher                        matcher.Matcher
		hostStorageAdapterMatcher              matcher.Matcher
		hostStoragePathMatcher                 matcher.Matcher
		hostCPUInstanceMatcher                 matcher.Matcher
		vsanClusterMatcher                     matcher.Matcher
		vsanHostMatcher                        matcher.Matcher
		vsanVMMatcher                          matcher.Matcher
		vsphereTagCategoryMatcher              matcher.Matcher
		customAttributeMatcher                 matcher.Matcher
		vmDiskPerfSamples                      map[string]*vmDiskPerfSample
		vmNICPerfSamples                       map[string]*vmNICPerfSample
		hostNICPerfSamples                     map[string]*hostNICPerfSample
		hostDiskPerfSamples                    map[string]*hostDiskPerfSample
		hostStorageAdapterPerfSamples          map[string]*hostStorageAdapterPerfSample
		hostStorageAdapterAggregatePerfSamples map[string]*hostStorageAdapterAggregatePerfSample
		hostStoragePathPerfSamples             map[string]*hostStoragePathPerfSample
		hostStoragePathAggregatePerfSamples    map[string]*hostStoragePathAggregatePerfSample
		hostCPUInstancePerfSamples             map[string]*hostCPUInstancePerfSample
		hostPowerPerfSamples                   map[string]*hostPowerPerfSample
		vmPowerPerfSamples                     map[string]*vmPowerPerfSample
		vsanMetrics                            *scrapepkg.VSANMetrics
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

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) error {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	mx, err := c.collectLocked()
	if err != nil {
		return fmt.Errorf("collect vSphere metrics: %w", err)
	}

	c.writeMetrics(mx)
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
	if c.charts == nil {
		c.charts = inventoryChartsTmpl.Copy()
	}
}

func (c *Collector) resetRuntimeStateForInit() {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	c.charts = inventoryChartsTmpl.Copy()
	c.discoverer = nil
	c.scraper = nil
	c.dsPropertyCollector = nil
	c.clusterPropertyCollector = nil
	c.rpPropertyCollector = nil
	c.resources = nil
	c.discoveredHosts = make(map[string]int)
	c.discoveredVMs = make(map[string]int)
	c.discoveredDatastores = make(map[string]int)
	c.discoveredClusters = make(map[string]int)
	c.discoveredResourcePools = make(map[string]int)
	c.charted = make(map[string]bool)
	c.datastorePerfReceived = make(map[string]bool)
	c.datastorePerfCharted = make(map[string]bool)
	c.clusterPerfReceived = make(map[string]bool)
	c.clusterPerfCharted = make(map[string]bool)
	c.datastoreClusterMatcher = nil
	c.vmDiskMatcher = nil
	c.vmNICMatcher = nil
	c.hostNICMatcher = nil
	c.hostDiskMatcher = nil
	c.hostStorageAdapterMatcher = nil
	c.hostStoragePathMatcher = nil
	c.hostCPUInstanceMatcher = nil
	c.vsanClusterMatcher = nil
	c.vsanHostMatcher = nil
	c.vsanVMMatcher = nil
	c.vsphereTagCategoryMatcher = nil
	c.customAttributeMatcher = nil
	c.vmDiskPerfSamples = nil
	c.vmNICPerfSamples = nil
	c.hostNICPerfSamples = nil
	c.hostDiskPerfSamples = nil
	c.hostStorageAdapterPerfSamples = nil
	c.hostStorageAdapterAggregatePerfSamples = nil
	c.hostStoragePathPerfSamples = nil
	c.hostStoragePathAggregatePerfSamples = nil
	c.hostCPUInstancePerfSamples = nil
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
