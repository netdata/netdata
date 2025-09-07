// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	datacenter      = "Datacenter"
	folder          = "Folder"
	computeResource = "ComputeResource"
	hostSystem      = "HostSystem"
	virtualMachine  = "VirtualMachine"

	maxIdleConnections = 32
)

type Config struct {
	URL      string
	User     string
	Password string
	tlscfg.TLSConfig
	Timeout time.Duration
}

type Client struct {
	client *govmomi.Client
	root   *view.ContainerView
	perf   *performance.Manager
}

func newSoapClient(config Config) (*soap.Client, error) {
	soapURL, err := soap.ParseURL(config.URL)
	if err != nil || soapURL == nil {
		return nil, err
	}
	soapURL.User = url.UserPassword(config.User, config.Password)
	soapClient := soap.NewClient(soapURL, config.TLSConfig.InsecureSkipVerify.Bool())

	tlsConfig, err := tlscfg.NewTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil && len(tlsConfig.Certificates) > 0 {
		soapClient.SetCertificate(tlsConfig.Certificates[0])
	}
	if config.TLSConfig.TLSCA != "" {
		if err := soapClient.SetRootCAs(config.TLSConfig.TLSCA); err != nil {
			return nil, err
		}
	}

	if t, ok := soapClient.Transport.(*http.Transport); ok {
		t.MaxIdleConnsPerHost = maxIdleConnections
		t.TLSHandshakeTimeout = config.Timeout
	}
	soapClient.Timeout = config.Timeout

	return soapClient, nil
}

func newContainerView(ctx context.Context, client *govmomi.Client) (*view.ContainerView, error) {
	viewManager := view.NewManager(client.Client)
	return viewManager.CreateContainerView(ctx, client.ServiceContent.RootFolder, []string{}, true)
}

func newPerformanceManager(client *vim25.Client) *performance.Manager {
	perfManager := performance.NewManager(client)
	perfManager.Sort = true
	return perfManager
}

func New(config Config) (*Client, error) {
	ctx := context.Background()
	soapClient, err := newSoapClient(config)
	if err != nil {
		return nil, err
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, err
	}

	vmomiClient := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	userInfo := url.UserPassword(config.User, config.Password)
	addKeepAlive(vmomiClient, userInfo)

	err = vmomiClient.Login(ctx, userInfo)
	if err != nil {
		return nil, err
	}

	containerView, err := newContainerView(ctx, vmomiClient)
	if err != nil {
		return nil, err
	}

	perfManager := newPerformanceManager(vimClient)

	client := &Client{
		client: vmomiClient,
		perf:   perfManager,
		root:   containerView,
	}

	return client, nil
}

func (c *Client) IsSessionActive() (bool, error) {
	return c.client.SessionManager.SessionIsActive(context.Background())
}

func (c *Client) Version() string {
	return c.client.ServiceContent.About.Version
}

func (c *Client) Login(userinfo *url.Userinfo) error {
	return c.client.Login(context.Background(), userinfo)
}

func (c *Client) Logout() error {
	return c.client.Logout(context.Background())
}

func (c *Client) PerformanceMetrics(pqs []types.PerfQuerySpec) ([]performance.EntityMetric, error) {
	metrics, err := c.perf.Query(context.Background(), pqs)
	if err != nil {
		return nil, err
	}
	return c.perf.ToMetricSeries(context.Background(), metrics)
}

func (c *Client) Datacenters(pathSet ...string) (dcs []mo.Datacenter, err error) {
	err = c.root.Retrieve(context.Background(), []string{datacenter}, pathSet, &dcs)
	return
}

func (c *Client) Folders(pathSet ...string) (folders []mo.Folder, err error) {
	err = c.root.Retrieve(context.Background(), []string{folder}, pathSet, &folders)
	return
}

func (c *Client) ComputeResources(pathSet ...string) (computes []mo.ComputeResource, err error) {
	err = c.root.Retrieve(context.Background(), []string{computeResource}, pathSet, &computes)
	return
}

func (c *Client) Hosts(pathSet ...string) (hosts []mo.HostSystem, err error) {
	err = c.root.Retrieve(context.Background(), []string{hostSystem}, pathSet, &hosts)
	return
}

func (c *Client) VirtualMachines(pathSet ...string) (vms []mo.VirtualMachine, err error) {
	err = c.root.Retrieve(context.Background(), []string{virtualMachine}, pathSet, &vms)
	return
}

func (c *Client) CounterInfoByName() (map[string]*types.PerfCounterInfo, error) {
	return c.perf.CounterInfoByName(context.Background())
}
