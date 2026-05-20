// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

func (c *Collector) validateConfig() error {
	const minRecommendedUpdateEvery = 20

	if c.URL == "" {
		return errors.New("config option url is required")
	}
	if c.Username == "" || c.Password == "" {
		return errors.New("config options username and password are required")
	}
	if c.UpdateEvery < minRecommendedUpdateEvery {
		c.Warningf("config option update_every=%d is lower than recommended minimum %d", c.UpdateEvery, minRecommendedUpdateEvery)
	}
	if err := validateStringAllowlist("vm_guest_labels", c.VMGuestLabels, validVMGuestLabels); err != nil {
		return err
	}
	if len(c.VSphereTagCategories) > 0 {
		m, err := newUserMetadataPatternMatcher("vsphere_tag_categories", c.VSphereTagCategories)
		if err != nil {
			return err
		}
		c.vsphereTagCategoryMatcher = m
	}
	if len(c.CustomAttributes) > 0 {
		m, err := newUserMetadataPatternMatcher("custom_attributes", c.CustomAttributes)
		if err != nil {
			return err
		}
		c.customAttributeMatcher = m
	}
	if len(c.DatastoreClustersInclude) == 0 {
		c.DatastoreClustersInclude = []string{"/*"}
	}
	dcMatcher, err := newSimplePatternsMatcher("datastore_cluster_include", c.DatastoreClustersInclude)
	if err != nil {
		return err
	}
	c.datastoreClusterMatcher = dcMatcher
	if len(c.VMDisksInclude) == 0 {
		c.VMDisksInclude = []string{"*"}
	}
	m, err := newSimplePatternsMatcher("vm_disk_include", c.VMDisksInclude)
	if err != nil {
		return err
	}
	c.vmDiskMatcher = m
	if len(c.VMNICsInclude) == 0 {
		c.VMNICsInclude = []string{"*"}
	}
	nicMatcher, err := newSimplePatternsMatcher("vm_nic_include", c.VMNICsInclude)
	if err != nil {
		return err
	}
	c.vmNICMatcher = nicMatcher
	if len(c.HostNICsInclude) == 0 {
		c.HostNICsInclude = []string{"*"}
	}
	hostNICMatcher, err := newSimplePatternsMatcher("host_nic_include", c.HostNICsInclude)
	if err != nil {
		return err
	}
	c.hostNICMatcher = hostNICMatcher
	if len(c.HostDisksInclude) == 0 {
		c.HostDisksInclude = []string{"*"}
	}
	hostDiskMatcher, err := newSimplePatternsMatcher("host_disk_include", c.HostDisksInclude)
	if err != nil {
		return err
	}
	c.hostDiskMatcher = hostDiskMatcher
	if len(c.HostStorageAdaptersInclude) == 0 {
		c.HostStorageAdaptersInclude = []string{"*"}
	}
	hostStorageAdapterMatcher, err := newSimplePatternsMatcher("host_storage_adapter_include", c.HostStorageAdaptersInclude)
	if err != nil {
		return err
	}
	c.hostStorageAdapterMatcher = hostStorageAdapterMatcher
	if len(c.HostStoragePathsInclude) == 0 {
		c.HostStoragePathsInclude = []string{"*"}
	}
	hostStoragePathMatcher, err := newSimplePatternsMatcher("host_storage_path_include", c.HostStoragePathsInclude)
	if err != nil {
		return err
	}
	c.hostStoragePathMatcher = hostStoragePathMatcher
	if len(c.HostCPUInstancesInclude) == 0 {
		c.HostCPUInstancesInclude = []string{"*"}
	}
	hostCPUInstanceMatcher, err := newSimplePatternsMatcher("host_cpu_instance_include", c.HostCPUInstancesInclude)
	if err != nil {
		return err
	}
	c.hostCPUInstanceMatcher = hostCPUInstanceMatcher
	if len(c.VSANClustersInclude) == 0 {
		c.VSANClustersInclude = []string{"/*"}
	}
	vsanClusterMatcher, err := newSimplePatternsMatcher("vsan_cluster_include", c.VSANClustersInclude)
	if err != nil {
		return err
	}
	c.vsanClusterMatcher = vsanClusterMatcher
	if len(c.VSANHostsInclude) == 0 {
		c.VSANHostsInclude = []string{"/*"}
	}
	vsanHostMatcher, err := newSimplePatternsMatcher("vsan_host_include", c.VSANHostsInclude)
	if err != nil {
		return err
	}
	c.vsanHostMatcher = vsanHostMatcher
	if len(c.VSANVMsInclude) == 0 {
		c.VSANVMsInclude = []string{"/*"}
	}
	vsanVMMatcher, err := newSimplePatternsMatcher("vsan_vm_include", c.VSANVMsInclude)
	if err != nil {
		return err
	}
	c.vsanVMMatcher = vsanVMMatcher
	return nil
}

func (c *Collector) initClient() (*client.Client, error) {
	config := client.Config{
		URL:       c.URL,
		User:      c.Username,
		Password:  c.Password,
		Timeout:   c.Timeout.Duration(),
		TLSConfig: c.ClientConfig.TLSConfig,
	}
	return client.New(config)
}

func (c *Collector) initDiscoverer(cli *client.Client) error {
	d := discover.New(cli)
	d.Logger = c.Logger
	d.CollectDatastoreClusters = c.CollectDatastoreClusters
	d.CollectVMDisks = c.CollectVMDisks
	d.CollectVMDiskPerformance = c.CollectVMDiskPerformance
	d.CollectVMNICPerformance = c.CollectVMNICPerformance
	d.CollectHostNICPerformance = c.CollectHostNICPerformance
	d.CollectHostDiskPerformance = c.CollectHostDiskPerformance
	d.CollectHostStorageAdapterPerformance = c.CollectHostStorageAdapterPerformance
	d.CollectHostStoragePathPerformance = c.CollectHostStoragePathPerformance
	d.CollectHostCPUInstancePerformance = c.CollectHostCPUInstancePerformance
	d.CollectPowerMetrics = c.CollectPowerMetrics
	d.CollectVSAN = c.CollectVSAN
	d.CollectNetworkTopology = c.CollectNetworkTopology
	d.TagCategoryMatcher = c.vsphereTagCategoryMatcher
	d.CustomAttributeMatcher = c.customAttributeMatcher

	hm, err := c.HostsInclude.Parse()
	if err != nil {
		return fmt.Errorf("parse config option host_include: %w", err)
	}
	if hm != nil {
		d.HostMatcher = hm
	}
	vmm, err := c.VMsInclude.Parse()
	if err != nil {
		return fmt.Errorf("parse config option vm_include: %w", err)
	}
	if vmm != nil {
		d.VMMatcher = vmm
	}
	dsm, err := c.DatastoresInclude.Parse()
	if err != nil {
		return fmt.Errorf("parse config option datastore_include: %w", err)
	}
	if dsm != nil {
		d.DatastoreMatcher = dsm
	}

	cm, err := c.ClustersInclude.Parse()
	if err != nil {
		return fmt.Errorf("parse config option cluster_include: %w", err)
	}
	if cm != nil {
		d.ClusterMatcher = cm
	}

	c.discoverer = d
	return nil
}

func (c *Collector) initScraper(cli *client.Client) {
	ms := scrape.New(cli)
	ms.Logger = c.Logger
	c.scraper = ms
	c.dsPropertyCollector = cli
	c.clusterPropertyCollector = cli
	c.rpPropertyCollector = cli
}
