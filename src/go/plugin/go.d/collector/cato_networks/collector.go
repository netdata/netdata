// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplate string

func init() {
	collectorapi.Register("cato_networks", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2:      func() collectorapi.CollectorV2 { return New() },
		Config:        func() any { return &Config{} },
		Methods:       catoMethods,
		MethodHandler: catoFunctionHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: defaultEndpoint,
				},
				ClientConfig: web.ClientConfig{
					Timeout: defaultTimeout,
				},
			},
			SiteSelector:      defaultEntitySelector,
			InterfaceSelector: defaultEntitySelector,
			Limits: LimitsConfig{
				MaxSites:             new(defaultMaxSites),
				MaxInterfacesPerSite: new(defaultMaxIfacesPerSite),
			},
			Discovery: DiscoveryConfig{
				RefreshEvery: defaultDiscoveryEvery,
				PageLimit:    defaultDiscoveryLimit,
			},
			Metrics: MetricsConfig{
				TimeFrame:        defaultMetricsTimeFrame,
				Buckets:          defaultMetricsBuckets,
				MaxSitesPerQuery: defaultMaxSitesPerQuery,
			},
			BGP: BGPConfig{
				RefreshEvery:          defaultBGPRefreshEvery,
				MaxSitesPerCollection: defaultBGPMaxSites,
				PeerSelector:          defaultEntitySelector,
				MaxPeersPerSite:       new(defaultBGPMaxPeers),
			},
			Retry: RetryConfig{
				Attempts: defaultRetryAttempts,
				WaitMin:  defaultRetryWaitMin,
				WaitMax:  defaultRetryWaitMax,
			},
		},
		store:     store,
		metrics:   newCollectorMetrics(store),
		newClient: newSDKAPIClient,
		now:       time.Now,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	httpClient *http.Client
	client     apiClient
	newClient  func(Config, *http.Client) (apiClient, error)

	mu       sync.RWMutex
	topology *topology.Data

	discovery discoveryState
	bgp       bgpState
	health    collectorHealth

	siteMatcher      *entitySelector
	interfaceMatcher *entitySelector
	bgpPeerMatcher   *entitySelector
	warningStates    map[string]string

	now func() time.Time
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	c.Config.applyDefaults()
	if err := c.Config.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	if err := c.initEntitySelectors(); err != nil {
		return err
	}

	if c.client == nil {
		httpClient, err := web.NewHTTPClient(c.ClientConfig)
		if err != nil {
			return fmt.Errorf("init http client: %w", err)
		}
		c.httpClient = httpClient

		client, err := c.newClient(c.Config, httpClient)
		if err != nil {
			return fmt.Errorf("init Cato client: %w", err)
		}
		c.client = client
	}

	c.discovery = discoveryState{}
	c.bgp = bgpState{bySite: make(map[string][]bgpPeerState)}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("Cato client is not initialized")
	}
	if err := c.client.Probe(ctx, c.AccountID); err != nil {
		return wrapCatoOperationError("Cato API probe", err)
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	if err := c.collect(ctx); err != nil {
		if ctxErr := contextErr(ctx); ctxErr != nil {
			return ctxErr
		}
		c.warnRecoverable(warningKeyCollection, classifyCatoError(err), "collection failed, error_class=%s: %v", classifyCatoError(err), err)
	} else {
		c.clearRecoverableWarning(warningKeyCollection)
	}
	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplate }

func (c *Collector) currentTopology() (*topology.Data, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.topology == nil {
		return nil, false
	}
	data := *c.topology
	data.Actors = append([]topology.Actor(nil), c.topology.Actors...)
	data.Links = append([]topology.Link(nil), c.topology.Links...)
	return &data, true
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
