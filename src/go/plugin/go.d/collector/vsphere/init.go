// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
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
	if c.DiscoveryInterval.Duration() <= 0 {
		return errors.New("config option discovery_interval must be greater than zero")
	}
	if len(c.TagCategories) > 0 {
		m, err := match.NewPatternListMatcher("tag_categories", c.TagCategories)
		if err != nil {
			return err
		}
		c.vsphereTagCategoryMatcher = m
	}
	if len(c.CustomAttributes) > 0 {
		m, err := match.NewPatternListMatcher("custom_attributes", c.CustomAttributes)
		if err != nil {
			return err
		}
		c.customAttributeMatcher = m
	}
	if err := c.validateDatastoreClusterConfig(); err != nil {
		return err
	}
	if err := c.validateVSANConfig(); err != nil {
		return err
	}
	return nil
}

func (c *Collector) validateDatastoreClusterConfig() error {
	if !c.CollectDatastoreClusters {
		return nil
	}
	if len(c.DatastoreClustersInclude) == 0 {
		c.DatastoreClustersInclude = match.DatastoreClusterIncludes{"/*"}
	}
	m, err := c.DatastoreClustersInclude.Parse()
	if err != nil {
		return err
	}
	c.datastoreClusterMatcher = m
	return nil
}

func (c *Collector) validateVSANConfig() error {
	if !c.CollectVSAN {
		return nil
	}
	if len(c.VSANClustersInclude) == 0 {
		c.VSANClustersInclude = match.VSANClusterIncludes{"/*"}
	}
	vsanClusterMatcher, err := c.VSANClustersInclude.Parse()
	if err != nil {
		return err
	}
	c.vsanClusterMatcher = vsanClusterMatcher
	if len(c.VSANHostsInclude) == 0 {
		c.VSANHostsInclude = match.VSANHostIncludes{"/*"}
	}
	vsanHostMatcher, err := c.VSANHostsInclude.Parse()
	if err != nil {
		return err
	}
	c.vsanHostMatcher = vsanHostMatcher
	if len(c.VSANVMsInclude) == 0 {
		c.VSANVMsInclude = match.VSANVMIncludes{"/*"}
	}
	vsanVMMatcher, err := c.VSANVMsInclude.Parse()
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
	d.CollectVSAN = c.CollectVSAN
	d.CollectNetworkTopology = c.CollectNetworkTopology
	d.DatastoreClusterMatcher = c.datastoreClusterMatcher
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
