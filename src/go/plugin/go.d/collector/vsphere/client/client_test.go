// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"crypto/tls"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

func TestNew(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	v, err := client.IsSessionActive()
	assert.NoError(t, err)
	assert.True(t, v)
}

func TestClient_Version(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	assert.NotEmpty(t, client.Version())
}

func TestClient_CounterInfoByName(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	v, err := client.CounterInfoByName()
	assert.NoError(t, err)
	assert.IsType(t, map[string]*types.PerfCounterInfo{}, v)
	assert.NotEmpty(t, v)
}

func TestClient_IsSessionActive(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	v, err := client.IsSessionActive()
	assert.NoError(t, err)
	assert.True(t, v)
}

func TestClient_Login(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	assert.NoError(t, client.Logout())

	err := client.Login(url.UserPassword("admin", "password"))
	assert.NoError(t, err)

	ok, err := client.IsSessionActive()
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestClient_Logout(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	assert.NoError(t, client.Logout())

	v, err := client.IsSessionActive()
	assert.NoError(t, err)
	assert.False(t, v)
}

func TestClient_Datacenters(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	dcs, err := client.Datacenters()
	assert.NoError(t, err)
	assert.NotEmpty(t, dcs)
}

func TestClient_Folders(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	folders, err := client.Folders()
	assert.NoError(t, err)
	assert.NotEmpty(t, folders)
}

func TestClient_ComputeResources(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	computes, err := client.ComputeResources()
	assert.NoError(t, err)
	assert.NotEmpty(t, computes)
}

func TestClient_Hosts(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	hosts, err := client.Hosts()
	assert.NoError(t, err)
	assert.NotEmpty(t, hosts)
}

func TestClient_VirtualMachines(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	vms, err := client.VirtualMachines()
	assert.NoError(t, err)
	assert.NotEmpty(t, vms)
}

func TestClient_PerformanceMetrics(t *testing.T) {
	client, teardown := prepareClient(t)
	defer teardown()

	hosts, err := client.Hosts()
	require.NoError(t, err)
	metrics, err := client.PerformanceMetrics(hostsPerfQuerySpecs(hosts))
	require.NoError(t, err)
	assert.True(t, len(metrics) > 0)
}

func prepareClient(t *testing.T) (client *Client, teardown func()) {
	model, srv := createSim(t)
	teardown = func() { model.Remove(); srv.Close() }
	return newClient(t, srv.URL), teardown
}

func newClient(t *testing.T, vCenterURL *url.URL) *Client {
	client, err := New(Config{
		URL:       vCenterURL.String(),
		User:      "admin",
		Password:  "password",
		Timeout:   time.Second * 3,
		TLSConfig: tlscfg.TLSConfig{InsecureSkipVerify: true},
	})
	require.NoError(t, err)
	return client
}

func createSim(t *testing.T) (*simulator.Model, *simulator.Server) {
	model := simulator.VPX()
	err := model.Create()
	require.NoError(t, err)
	model.Service.TLS = new(tls.Config)
	return model, model.Service.NewServer()
}

func hostsPerfQuerySpecs(hosts []mo.HostSystem) []types.PerfQuerySpec {
	var pqs []types.PerfQuerySpec
	for _, host := range hosts {
		pq := types.PerfQuerySpec{
			Entity:     host.Reference(),
			MaxSample:  1,
			MetricId:   []types.PerfMetricId{{CounterId: 32, Instance: ""}},
			IntervalId: 20,
			Format:     "normal",
		}
		pqs = append(pqs, pq)
	}
	return pqs
}
