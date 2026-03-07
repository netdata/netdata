// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("azure_monitor", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: defaultAutoDetectRetry,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{
		Config: Config{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: defaultAutoDetectRetry,
			Cloud:              defaultCloud,
			DiscoveryEvery:     defaultDiscoveryEvery,
			QueryOffset:        defaultQueryOffset,
			MaxConcurrency:     defaultMaxConcurrency,
			MaxBatchResources:  defaultMaxBatchResource,
			MaxMetricsPerQuery: defaultMaxMetricsQuery,
			Auth: AuthConfig{
				Mode: authModeDefault,
			},
		},
		store:              store,
		now:                time.Now,
		newResourceGraph:   defaultNewResourceGraphClient,
		newMetricsClient:   defaultNewMetricsClient,
		loadProfileCatalog: loadProfileCatalog,
		nextCollectByGrain: make(map[string]time.Time),
		metricsClients:     make(map[string]metricsQueryClient),
		accumulators:       make(map[string]float64),
		lastObserved:       make(map[string]lastObservation),
	}
	return c
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store         metrix.CollectorStore
	chartTemplate string

	credential    azcore.TokenCredential
	cloudCfg      azcloud.Configuration
	resourceGraph resourceGraphClient

	runtimePlan *runtimePlan
	profiles    profileCatalog

	discovery discoveryState

	now func() time.Time

	newResourceGraph   func(subscriptionID string, cred azcore.TokenCredential, cloud azcloud.Configuration) (resourceGraphClient, error)
	newMetricsClient   func(endpoint string, cred azcore.TokenCredential, cloud azcloud.Configuration) (metricsQueryClient, error)
	loadProfileCatalog func() (profileCatalog, error)

	metricsClientsMu syncMapMutex
	metricsClients   map[string]metricsQueryClient

	accumulators       map[string]float64
	lastObserved       map[string]lastObservation
	nextCollectByGrain map[string]time.Time
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	c.Config.applyDefaults()

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return fmt.Errorf("load profiles catalog: %w", err)
	}
	c.profiles = catalog
	if len(c.Config.Profiles) == 0 {
		c.Config.Profiles = c.profiles.defaultProfileNames()
	}
	if err := c.Config.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	cfg, err := cloudConfigFromName(c.Cloud)
	if err != nil {
		return err
	}
	c.cloudCfg = cfg

	cred, err := c.createCredential()
	if err != nil {
		return fmt.Errorf("create azure credential: %w", err)
	}
	c.credential = cred

	rgClient, err := c.newResourceGraph(c.SubscriptionID, c.credential, c.cloudCfg)
	if err != nil {
		return fmt.Errorf("create resource graph client: %w", err)
	}
	c.resourceGraph = rgClient

	plan, err := c.buildRuntimePlan()
	if err != nil {
		return fmt.Errorf("build runtime plan: %w", err)
	}
	c.runtimePlan = plan
	c.chartTemplate = plan.ChartTemplateYAML

	if err := c.initInstruments(plan); err != nil {
		return err
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	resources, err := c.refreshDiscovery(ctx, true)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return errors.New("no Azure resources discovered for the configured profiles")
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {
	c.metricsClientsMu.Lock()
	defer c.metricsClientsMu.Unlock()
	c.metricsClients = make(map[string]metricsQueryClient)
	c.accumulators = make(map[string]float64)
	c.lastObserved = make(map[string]lastObservation)
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return c.chartTemplate }

func (c *Collector) createCredential() (azcore.TokenCredential, error) {
	coreOpts := azcore.ClientOptions{Cloud: c.cloudCfg}

	switch stringsLowerTrim(c.Auth.Mode) {
	case authModeServicePrincipal:
		opts := &azidentity.ClientSecretCredentialOptions{ClientOptions: coreOpts}
		return azidentity.NewClientSecretCredential(c.Auth.TenantID, c.Auth.ClientID, c.Auth.ClientSecret, opts)
	case authModeManagedIdentity:
		miOpts := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: coreOpts}
		if stringsTrim(c.Auth.ManagedIdentityClientID) != "" {
			miOpts.ID = azidentity.ClientID(stringsTrim(c.Auth.ManagedIdentityClientID))
		}
		return azidentity.NewManagedIdentityCredential(miOpts)
	case authModeDefault, "":
		opts := &azidentity.DefaultAzureCredentialOptions{ClientOptions: coreOpts}
		return azidentity.NewDefaultAzureCredential(opts)
	default:
		return nil, fmt.Errorf("unsupported auth.mode: %q", c.Auth.Mode)
	}
}

func defaultNewResourceGraphClient(subscriptionID string, cred azcore.TokenCredential, cloudCfg azcloud.Configuration) (resourceGraphClient, error) {
	_ = subscriptionID
	client, err := armresourcegraph.NewClient(cred, armClientOptions{Cloud: cloudCfg}.toARM())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func defaultNewMetricsClient(endpoint string, cred azcore.TokenCredential, cloudCfg azcloud.Configuration) (metricsQueryClient, error) {
	client, err := azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: azcore.ClientOptions{Cloud: cloudCfg}})
	if err != nil {
		return nil, err
	}
	return client, nil
}
