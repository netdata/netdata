// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	vsanapi "github.com/vmware/govmomi/vsan"
	vsanmethods "github.com/vmware/govmomi/vsan/methods"
	vsantypes "github.com/vmware/govmomi/vsan/types"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

const (
	datacenter       = "Datacenter"
	folder           = "Folder"
	computeResource  = "ComputeResource"
	hostSystem       = "HostSystem"
	virtualMachine   = "VirtualMachine"
	datastoreType    = "Datastore"
	networkType      = "Network"
	storagePodType   = "StoragePod"
	resourcePoolType = "ResourcePool"

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
	client   *govmomi.Client
	root     *view.ContainerView
	perf     *performance.Manager
	userInfo *url.Userinfo
	rest     *rest.Client
	tags     *tags.Manager
	vsan     *vsanapi.Client
	lazyMu   sync.Mutex
}

func newSoapClient(config Config) (*soap.Client, error) {
	soapURL, err := soap.ParseURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse config option url for vSphere SOAP endpoint: %w", err)
	}
	if soapURL == nil {
		return nil, errors.New("parse config option url for vSphere SOAP endpoint: empty SOAP URL")
	}
	soapURL.User = url.UserPassword(config.User, config.Password)
	soapClient := soap.NewClient(soapURL, config.TLSConfig.InsecureSkipVerify)

	tlsConfig, err := tlscfg.NewTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("build TLS configuration from tls_* options: %w", err)
	}
	if tlsConfig != nil && len(tlsConfig.Certificates) > 0 {
		soapClient.SetCertificate(tlsConfig.Certificates[0])
	}
	if config.TLSConfig.TLSCA != "" {
		if err := soapClient.SetRootCAs(config.TLSConfig.TLSCA); err != nil {
			return nil, fmt.Errorf("load tls_ca certificate bundle %q for vSphere SOAP client: %w", config.TLSConfig.TLSCA, err)
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

var createContainerView = newContainerView

func newPerformanceManager(client *vim25.Client) *performance.Manager {
	perfManager := performance.NewManager(client)
	perfManager.Sort = true
	return perfManager
}

func New(config Config) (*Client, error) {
	ctx := context.Background()
	soapClient, err := newSoapClient(config)
	if err != nil {
		return nil, fmt.Errorf("initialize vSphere SOAP client: %w", err)
	}

	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, fmt.Errorf("initialize vSphere vim25 client and retrieve service content: %w", err)
	}

	vmomiClient := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	userInfo := url.UserPassword(config.User, config.Password)
	addKeepAlive(vmomiClient, userInfo)

	err = vmomiClient.Login(ctx, userInfo)
	if err != nil {
		return nil, fmt.Errorf("login to vSphere API with configured username: %w", err)
	}

	containerView, err := createContainerView(ctx, vmomiClient)
	if err != nil {
		return nil, fmt.Errorf("create root vSphere container view: %w", errors.Join(err, vmomiClient.Logout(ctx)))
	}

	perfManager := newPerformanceManager(vimClient)

	client := &Client{
		client:   vmomiClient,
		perf:     perfManager,
		root:     containerView,
		userInfo: userInfo,
	}

	return client, nil
}

func (c *Client) IsSessionActive() (bool, error) {
	active, err := c.client.SessionManager.SessionIsActive(context.Background())
	if err != nil {
		return false, fmt.Errorf("check vSphere SOAP session activity: %w", err)
	}
	return active, nil
}

func (c *Client) Version() string {
	return c.client.ServiceContent.About.Version
}

func (c *Client) InstanceUUID() string {
	return c.client.ServiceContent.About.InstanceUuid
}

func (c *Client) Login(userinfo *url.Userinfo) error {
	if err := c.client.Login(context.Background(), userinfo); err != nil {
		return fmt.Errorf("login to vSphere SOAP API: %w", err)
	}
	return nil
}

func (c *Client) Logout() error {
	if err := c.client.Logout(context.Background()); err != nil {
		return fmt.Errorf("logout from vSphere SOAP API: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	ctx := context.Background()
	var err error
	if c.root != nil {
		if e := c.root.Destroy(ctx); e != nil {
			err = errors.Join(err, fmt.Errorf("destroy root vSphere container view: %w", e))
		}
		c.root = nil
	}
	c.lazyMu.Lock()
	if c.rest != nil {
		if e := c.rest.Logout(ctx); e != nil {
			err = errors.Join(err, fmt.Errorf("logout from vSphere REST API: %w", e))
		}
		c.rest = nil
		c.tags = nil
	}
	c.vsan = nil
	c.userInfo = nil
	c.lazyMu.Unlock()
	if c.client != nil {
		if e := c.client.Logout(ctx); e != nil {
			err = errors.Join(err, fmt.Errorf("logout from vSphere SOAP API: %w", e))
		}
	}
	return err
}

func (c *Client) PerformanceMetrics(pqs []types.PerfQuerySpec) ([]performance.EntityMetric, error) {
	metrics, err := c.perf.Query(context.Background(), pqs)
	if err != nil {
		return nil, fmt.Errorf("query vSphere performance manager for %d perf query specs: %w", len(pqs), err)
	}
	series, err := c.perf.ToMetricSeries(context.Background(), metrics)
	if err != nil {
		return nil, fmt.Errorf("convert vSphere performance samples for %d perf query specs: %w", len(pqs), err)
	}
	return series, nil
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

func (c *Client) Datastores(pathSet ...string) (datastores []mo.Datastore, err error) {
	err = c.root.Retrieve(context.Background(), []string{datastoreType}, pathSet, &datastores)
	return
}

func (c *Client) Networks(pathSet ...string) (networks []mo.Network, err error) {
	err = c.root.Retrieve(context.Background(), []string{networkType}, pathSet, &networks)
	return
}

func (c *Client) StoragePods(pathSet ...string) (pods []mo.StoragePod, err error) {
	err = c.root.Retrieve(context.Background(), []string{storagePodType}, pathSet, &pods)
	return
}

func (c *Client) DatastoresByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo.Datastore, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var datastores []mo.Datastore
	pc := property.DefaultCollector(c.client.Client)
	err := pc.Retrieve(context.Background(), refs, pathSet, &datastores)
	if err != nil {
		return nil, fmt.Errorf("retrieve datastore properties for %d refs pathSet=%v: %w", len(refs), pathSet, err)
	}
	return datastores, nil
}

func (c *Client) ClustersByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo.ClusterComputeResource, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var clusters []mo.ClusterComputeResource
	pc := property.DefaultCollector(c.client.Client)
	err := pc.Retrieve(context.Background(), refs, pathSet, &clusters)
	if err != nil {
		return nil, fmt.Errorf("retrieve cluster properties for %d refs pathSet=%v: %w", len(refs), pathSet, err)
	}
	return clusters, nil
}

func (c *Client) ResourcePools(pathSet ...string) (pools []mo.ResourcePool, err error) {
	err = c.root.Retrieve(context.Background(), []string{resourcePoolType}, pathSet, &pools)
	return
}

func (c *Client) ResourcePoolsByRef(refs []types.ManagedObjectReference, pathSet ...string) ([]mo.ResourcePool, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var pools []mo.ResourcePool
	pc := property.DefaultCollector(c.client.Client)
	err := pc.Retrieve(context.Background(), refs, pathSet, &pools)
	if err != nil {
		return nil, fmt.Errorf("retrieve resource pool properties for %d refs pathSet=%v: %w", len(refs), pathSet, err)
	}
	return pools, nil
}

func (c *Client) CustomFields() ([]types.CustomFieldDef, error) {
	m, err := object.GetCustomFieldsManager(c.client.Client)
	if err != nil {
		return nil, fmt.Errorf("get vSphere custom fields manager: %w", err)
	}
	fields, err := m.Field(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list vSphere custom field definitions: %w", err)
	}
	return fields, nil
}

func (c *Client) TagsByRef(refs []types.ManagedObjectReference) (map[types.ManagedObjectReference]map[string][]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	ctx := context.Background()
	manager, err := c.tagManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize vSphere tag manager: %w", err)
	}

	categories, err := manager.GetCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vSphere tag categories: %w", err)
	}
	categoriesByID := make(map[string]string, len(categories))
	for _, category := range categories {
		categoriesByID[category.ID] = category.Name
	}

	tagList, err := manager.GetTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vSphere tags: %w", err)
	}
	tagsByID := make(map[string]tags.Tag, len(tagList))
	for _, tag := range tagList {
		tagsByID[tag.ID] = tag
	}

	out := make(map[types.ManagedObjectReference]map[string][]string)
	for i := 0; i < len(refs); i += maxTagAssociationBatchSize {
		end := min(i+maxTagAssociationBatchSize, len(refs))
		batch := make([]mo.Reference, 0, end-i)
		for _, ref := range refs[i:end] {
			batch = append(batch, ref)
		}

		attached, err := manager.ListAttachedTagsOnObjects(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("list vSphere tag attachments for refs batch offset=%d size=%d: %w", i, len(batch), err)
		}
		for _, objectTags := range attached {
			ref := objectTags.ObjectID.Reference()
			for _, tagID := range objectTags.TagIDs {
				tag, ok := tagsByID[tagID]
				if !ok || tag.Name == "" {
					continue
				}
				category := categoriesByID[tag.CategoryID]
				if category == "" {
					continue
				}
				if out[ref] == nil {
					out[ref] = make(map[string][]string)
				}
				out[ref][category] = append(out[ref][category], tag.Name)
			}
		}
	}

	return out, nil
}

func (c *Client) tagManager(ctx context.Context) (*tags.Manager, error) {
	c.lazyMu.Lock()
	defer c.lazyMu.Unlock()

	if c.tags != nil {
		return c.tags, nil
	}
	restClient := rest.NewClient(c.client.Client)
	if err := restClient.Login(ctx, c.userInfo); err != nil {
		return nil, fmt.Errorf("login to vSphere REST API for tag collection: %w", err)
	}
	c.rest = restClient
	c.tags = tags.NewManager(restClient)
	return c.tags, nil
}

func (c *Client) VSANPerfMetrics(cluster types.ManagedObjectReference, specs []vsantypes.VsanPerfQuerySpec) ([]vsantypes.VsanPerfEntityMetricCSV, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	ctx := context.Background()
	cli, err := c.vsanClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize vSAN client for cluster %s: %w", cluster.Value, err)
	}
	metrics, err := cli.VsanPerfQueryPerf(ctx, &cluster, specs)
	if err != nil {
		return nil, fmt.Errorf("query vSAN performance metrics for cluster %s with %d specs: %w", cluster.Value, len(specs), err)
	}
	return metrics, nil
}

func (c *Client) VSANSpaceUsage(cluster types.ManagedObjectReference) (*vsantypes.VsanSpaceUsage, error) {
	ctx := context.Background()
	cli, err := c.vsanClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize vSAN client for cluster %s: %w", cluster.Value, err)
	}
	req := vsantypes.VsanQuerySpaceUsage{
		This:    vsanSpaceReportSystemInstance,
		Cluster: cluster,
	}
	res, err := vsanmethods.VsanQuerySpaceUsage(ctx, cli, &req)
	if err != nil {
		return nil, fmt.Errorf("query vSAN space usage for cluster %s: %w", cluster.Value, err)
	}
	return &res.Returnval, nil
}

func (c *Client) VSANHealth(cluster types.ManagedObjectReference) (string, error) {
	ctx := context.Background()
	cli, err := c.vsanClient(ctx)
	if err != nil {
		return "", fmt.Errorf("initialize vSAN client for cluster %s: %w", cluster.Value, err)
	}
	fetchFromCache := true
	req := vsantypes.VsanQueryVcClusterHealthSummary{
		This:           vsanClusterHealthSystemInstance,
		Cluster:        &cluster,
		Fields:         []string{"overallHealth", "overallHealthDescription"},
		FetchFromCache: &fetchFromCache,
	}
	res, err := vsanmethods.VsanQueryVcClusterHealthSummary(ctx, cli, &req)
	if err != nil {
		return "", fmt.Errorf("query vSAN health summary for cluster %s: %w", cluster.Value, err)
	}
	return res.Returnval.OverallHealth, nil
}

func (c *Client) vsanClient(ctx context.Context) (*vsanapi.Client, error) {
	c.lazyMu.Lock()
	defer c.lazyMu.Unlock()

	if c.vsan != nil {
		return c.vsan, nil
	}
	cli, err := vsanapi.NewClient(ctx, c.client.Client)
	if err != nil {
		return nil, fmt.Errorf("create govmomi vSAN API client: %w", err)
	}
	c.vsan = cli
	return c.vsan, nil
}

func (c *Client) CounterInfoByName() (map[string]*types.PerfCounterInfo, error) {
	counters, err := c.perf.CounterInfoByName(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list vSphere performance counter registry: %w", err)
	}
	return counters, nil
}

const maxTagAssociationBatchSize = 2000

var (
	vsanSpaceReportSystemInstance = types.ManagedObjectReference{
		Type:  "VsanSpaceReportSystem",
		Value: "vsan-cluster-space-report-system",
	}
	vsanClusterHealthSystemInstance = types.ManagedObjectReference{
		Type:  "VsanVcClusterHealthSystem",
		Value: "vsan-cluster-health-system",
	}
)
