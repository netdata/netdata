// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"crypto/tls"
	"net/url"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/modules/vsphere/client"
	"github.com/netdata/go.d.plugin/modules/vsphere/discover"
	rs "github.com/netdata/go.d.plugin/modules/vsphere/resources"
	"github.com/netdata/go.d.plugin/pkg/tlscfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"
)

func TestNew(t *testing.T) {
}

func TestScraper_ScrapeVMs(t *testing.T) {
	s, res, teardown := prepareScraper(t)
	defer teardown()

	metrics := s.ScrapeVMs(res.VMs)
	assert.Len(t, metrics, len(res.VMs))
}

func TestScraper_ScrapeHosts(t *testing.T) {
	s, res, teardown := prepareScraper(t)
	defer teardown()

	metrics := s.ScrapeHosts(res.Hosts)
	assert.Len(t, metrics, len(res.Hosts))
}

func prepareScraper(t *testing.T) (s *Scraper, res *rs.Resources, teardown func()) {
	model, srv := createSim(t)
	teardown = func() { model.Remove(); srv.Close() }

	c := newClient(t, srv.URL)
	d := discover.New(c)
	res, err := d.Discover()
	require.NoError(t, err)

	return New(c), res, teardown
}

func newClient(t *testing.T, vCenterURL *url.URL) *client.Client {
	c, err := client.New(client.Config{
		URL:       vCenterURL.String(),
		User:      "admin",
		Password:  "password",
		Timeout:   time.Second * 3,
		TLSConfig: tlscfg.TLSConfig{InsecureSkipVerify: true},
	})
	require.NoError(t, err)
	return c
}

func createSim(t *testing.T) (*simulator.Model, *simulator.Server) {
	model := simulator.VPX()
	err := model.Create()
	require.NoError(t, err)
	model.Service.TLS = new(tls.Config)
	return model, model.Service.NewServer()
}
