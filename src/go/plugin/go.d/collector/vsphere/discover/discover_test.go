// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"crypto/tls"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestDiscoverer_Discover(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	res, err := d.Discover()

	require.NoError(t, err)
	assert.True(t, len(res.DataCenters) > 0)
	assert.True(t, len(res.Folders) > 0)
	assert.True(t, len(res.Clusters) > 0)
	assert.True(t, len(res.Hosts) > 0)
	assert.True(t, len(res.VMs) > 0)
	assert.True(t, isHierarchySet(res))
	assert.True(t, isMetricListsCollected(res))
}

func TestDiscoverer_discover(t *testing.T) {
	d, model, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()

	require.NoError(t, err)
	count := model.Count()
	assert.Lenf(t, raw.dcs, count.Datacenter, "datacenters")
	assert.Lenf(t, raw.folders, count.Folder-1, "folders") // minus root folder
	dummyClusters := model.Host * count.Datacenter
	assert.Lenf(t, raw.clusters, count.Cluster+dummyClusters, "clusters")
	assert.Lenf(t, raw.hosts, count.Host, "hosts")
	assert.Lenf(t, raw.vms, count.Machine, "hosts")
}

func TestDiscoverer_build(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)

	res := d.build(raw)

	assert.Lenf(t, res.DataCenters, len(raw.dcs), "datacenters")
	assert.Lenf(t, res.Folders, len(raw.folders), "folders")
	assert.Lenf(t, res.Clusters, len(raw.clusters), "clusters")
	assert.Lenf(t, res.Hosts, len(raw.hosts), "hosts")
	assert.Lenf(t, res.VMs, len(raw.vms), "hosts")
}

func TestDiscoverer_setHierarchy(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)
	res := d.build(raw)

	err = d.setHierarchy(res)

	require.NoError(t, err)
	assert.True(t, isHierarchySet(res))
}

func TestDiscoverer_removeUnmatched(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	d.HostMatcher = falseHostMatcher{}
	d.VMMatcher = falseVMMatcher{}
	raw, err := d.discover()
	require.NoError(t, err)
	res := d.build(raw)

	numVMs, numHosts := len(res.VMs), len(res.Hosts)
	assert.Equal(t, numVMs+numHosts, d.removeUnmatched(res))
	assert.Lenf(t, res.Hosts, 0, "hosts")
	assert.Lenf(t, res.VMs, 0, "vms")
}

func TestDiscoverer_collectMetricLists(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)

	res := d.build(raw)
	err = d.collectMetricLists(res)

	require.NoError(t, err)
	assert.True(t, isMetricListsCollected(res))
}

func prepareDiscovererSim(t *testing.T) (d *Discoverer, model *simulator.Model, teardown func()) {
	model, srv := createSim(t)
	teardown = func() { model.Remove(); srv.Close() }
	c := newClient(t, srv.URL)

	return New(c), model, teardown
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

func isHierarchySet(res *rs.Resources) bool {
	for _, c := range res.Clusters {
		if !c.Hier.IsSet() {
			return false
		}
	}
	for _, h := range res.Hosts {
		if !h.Hier.IsSet() {
			return false
		}
	}
	for _, v := range res.VMs {
		if !v.Hier.IsSet() {
			return false
		}
	}
	return true
}

func isMetricListsCollected(res *rs.Resources) bool {
	for _, h := range res.Hosts {
		if h.MetricList == nil {
			return false
		}
	}
	for _, v := range res.VMs {
		if v.MetricList == nil {
			return false
		}
	}
	return true
}

type falseHostMatcher struct{}

func (falseHostMatcher) Match(*rs.Host) bool { return false }

type falseVMMatcher struct{}

func (falseVMMatcher) Match(*rs.VM) bool { return false }
