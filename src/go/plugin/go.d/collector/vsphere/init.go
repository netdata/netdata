// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

func (c *Collector) validateConfig() error {
	const minRecommendedUpdateEvery = 20

	if c.URL == "" {
		return errors.New("URL is not set")
	}
	if c.Username == "" || c.Password == "" {
		return errors.New("username or password not set")
	}
	if c.UpdateEvery < minRecommendedUpdateEvery {
		c.Warningf("update_every is to low, minimum recommended is %d", minRecommendedUpdateEvery)
	}
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

	hm, err := c.HostsInclude.Parse()
	if err != nil {
		return err
	}
	if hm != nil {
		d.HostMatcher = hm
	}
	vmm, err := c.VMsInclude.Parse()
	if err != nil {
		return err
	}
	if vmm != nil {
		d.VMMatcher = vmm
	}

	c.discoverer = d
	return nil
}

func (c *Collector) initScraper(cli *client.Client) {
	ms := scrape.New(cli)
	ms.Logger = c.Logger
	c.scraper = ms
}
