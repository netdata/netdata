// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
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
			UpdateEvery:          defaultUpdateEvery,
			AutoDetectionRetry:   defaultAutoDetectRetry,
			Cloud:                defaultCloud,
			DiscoveryEvery:       defaultDiscoveryEvery,
			QueryOffset:          defaultQueryOffset,
			Timeout:              defaultTimeout,
			MaxConcurrency:       defaultMaxConcurrency,
			MaxBatchResources:    defaultMaxBatchResource,
			MaxMetricsPerQuery:   defaultMaxMetricsQuery,
			ProfileSelectionMode: profileSelectionModeAuto,
		},
		store:              store,
		now:                time.Now,
		newResourceGraph:   defaultNewResourceGraphClient,
		newMetricsClient:   defaultNewMetricsClient,
		loadProfileCatalog: azureprofiles.DefaultCatalog,
		nextCollectByGrain: make(map[string]time.Time),
	}
	return c
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore

	resourceGraph resourceGraphClient
	queryExecutor *queryExecutor
	observations  *observationState

	runtime *collectorRuntime

	discovery discoveryState

	now func() time.Time

	newResourceGraph   func(subscriptionID string, cred azcore.TokenCredential, cloud azcloud.Configuration) (resourceGraphClient, error)
	newMetricsClient   func(endpoint string, cred azcore.TokenCredential, cloud azcloud.Configuration) (metricsQueryClient, error)
	loadProfileCatalog func() (azureprofiles.Catalog, error)

	nextCollectByGrain map[string]time.Time
}

func (c *Collector) Init(ctx context.Context) error {
	result, err := c.prepareInitResult(ctx)
	if err != nil {
		return err
	}

	if err := c.initInstruments(result.runtime); err != nil {
		return err
	}

	c.Config = result.config
	c.resourceGraph = result.resourceGraph
	c.queryExecutor = result.queryExecutor
	c.observations = newObservationState(result.runtime.Instruments)
	c.runtime = result.runtime

	return nil
}

func (c *Collector) Configuration() any { return c.Config }

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
	if c.queryExecutor != nil {
		c.queryExecutor.reset()
	}
	if c.observations != nil {
		c.observations.reset()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string {
	if c.runtime == nil {
		return ""
	}
	return c.runtime.ChartTemplateYAML
}
