// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"bytes"
	"crypto/tls"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
	vsantypes "github.com/vmware/govmomi/vsan/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/discover"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestScraper_calcMaxQuery(t *testing.T) {
	tests := map[string]struct {
		version string
		want    int
	}{
		"vcenter 5.5":        {version: "5.5.0", want: 64},
		"vcenter 6.0":        {version: "6.0.0", want: 64},
		"vcenter 6.5":        {version: "6.5.0", want: 256},
		"vcenter 7.0":        {version: "7.0.0", want: 256},
		"vcenter 8.0":        {version: "8.0.0", want: 256},
		"unparsable version": {version: "not-a-version", want: 64},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := New(mockClient{version: tt.version})
			assert.Equal(t, tt.want, s.maxQuery)
		})
	}
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

func TestScraper_ScrapeMetricsErrorIsRateLimited(t *testing.T) {
	var buf bytes.Buffer
	s := New(mockClient{
		version: "8.0.0",
		perfErr: errors.New("query failed"),
	})
	s.Logger = logger.NewWithWriter(&buf)
	hosts := rs.Hosts{
		"host-1": &rs.Host{
			ID:         "host-1",
			PowerState: string(types.HostSystemPowerStatePoweredOn),
			MetricList: performance.MetricList{
				{CounterId: 1},
			},
			Ref: types.ManagedObjectReference{Type: "HostSystem", Value: "host-1"},
		},
	}

	s.ScrapeHosts(hosts)
	s.ScrapeHosts(hosts)

	assert.Equal(t, 1, strings.Count(buf.String(), "scrape vSphere performance metrics"))
}

func TestScraper_ScrapeVSANRecordsEmptyHealthAsUnknown(t *testing.T) {
	s := New(mockClient{version: "8.0.0"})
	clusters := rs.Clusters{
		"domain-c1": &rs.Cluster{
			ID:          "domain-c1",
			VSANEnabled: true,
			VSANUUID:    "cluster-uuid",
			Ref:         types.ManagedObjectReference{Type: "ClusterComputeResource", Value: "domain-c1"},
		},
	}

	got := s.ScrapeVSAN(clusters, nil, nil)

	require.Contains(t, got.Health, "domain-c1")
	assert.Empty(t, got.Health["domain-c1"])
}

func Test_newHostsPerfQuerySpecsSkipsNonPoweredHosts(t *testing.T) {
	hosts := rs.Hosts{
		"host-1": &rs.Host{
			ID:         "host-1",
			PowerState: string(types.HostSystemPowerStatePoweredOn),
			MetricList: performance.MetricList{{CounterId: 1}},
			Ref:        types.ManagedObjectReference{Type: "HostSystem", Value: "host-1"},
		},
		"host-2": &rs.Host{
			ID:         "host-2",
			PowerState: string(types.HostSystemPowerStatePoweredOff),
			MetricList: performance.MetricList{{CounterId: 1}},
			Ref:        types.ManagedObjectReference{Type: "HostSystem", Value: "host-2"},
		},
	}

	pqs := newHostsPerfQuerySpecs(hosts)

	require.Len(t, pqs, 1)
	assert.Equal(t, "host-1", pqs[0].Entity.Value)
}

func Test_newVMsPerfQuerySpecsSkipsNonPoweredVMs(t *testing.T) {
	vms := rs.VMs{
		"vm-1": &rs.VM{
			ID:         "vm-1",
			PowerState: string(types.VirtualMachinePowerStatePoweredOn),
			MetricList: performance.MetricList{{CounterId: 1}},
			Ref:        types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-1"},
		},
		"vm-2": &rs.VM{
			ID:         "vm-2",
			PowerState: string(types.VirtualMachinePowerStateSuspended),
			MetricList: performance.MetricList{{CounterId: 1}},
			Ref:        types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-2"},
		},
	}

	pqs := newVMsPerfQuerySpecs(vms)

	require.Len(t, pqs, 1)
	assert.Equal(t, "vm-1", pqs[0].Entity.Value)
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

type mockClient struct {
	version string
	perfErr error
}

func (c mockClient) Version() string { return c.version }

func (c mockClient) PerformanceMetrics([]types.PerfQuerySpec) ([]performance.EntityMetric, error) {
	return nil, c.perfErr
}

func (c mockClient) VSANPerfMetrics(types.ManagedObjectReference, []vsantypes.VsanPerfQuerySpec) ([]vsantypes.VsanPerfEntityMetricCSV, error) {
	return nil, nil
}

func (c mockClient) VSANSpaceUsage(types.ManagedObjectReference) (*vsantypes.VsanSpaceUsage, error) {
	return nil, nil
}

func (c mockClient) VSANHealth(types.ManagedObjectReference) (string, error) {
	return "", nil
}
