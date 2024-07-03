// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/vsphere/discover"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/vsphere/scrape"
)

func (vs *VSphere) validateConfig() error {
	const minRecommendedUpdateEvery = 20

	if vs.URL == "" {
		return errors.New("URL is not set")
	}
	if vs.Username == "" || vs.Password == "" {
		return errors.New("username or password not set")
	}
	if vs.UpdateEvery < minRecommendedUpdateEvery {
		vs.Warningf("update_every is to low, minimum recommended is %d", minRecommendedUpdateEvery)
	}
	return nil
}

func (vs *VSphere) initClient() (*client.Client, error) {
	config := client.Config{
		URL:       vs.URL,
		User:      vs.Username,
		Password:  vs.Password,
		Timeout:   vs.Timeout.Duration(),
		TLSConfig: vs.Client.TLSConfig,
	}
	return client.New(config)
}

func (vs *VSphere) initDiscoverer(c *client.Client) error {
	d := discover.New(c)
	d.Logger = vs.Logger

	hm, err := vs.HostsInclude.Parse()
	if err != nil {
		return err
	}
	if hm != nil {
		d.HostMatcher = hm
	}
	vmm, err := vs.VMsInclude.Parse()
	if err != nil {
		return err
	}
	if vmm != nil {
		d.VMMatcher = vmm
	}

	vs.discoverer = d
	return nil
}

func (vs *VSphere) initScraper(c *client.Client) {
	ms := scrape.New(c)
	ms.Logger = vs.Logger
	vs.scraper = ms
}
