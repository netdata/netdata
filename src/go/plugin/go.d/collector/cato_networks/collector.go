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
				MaxSites:             intPtr(defaultMaxSites),
				MaxInterfacesPerSite: intPtr(defaultMaxIfacesPerSite),
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
				MaxPeersPerSite:       intPtr(defaultBGPMaxPeers),
			},
			Retry: RetryConfig{
				Attempts: defaultRetryAttempts,
				WaitMin:  defaultRetryWaitMin,
				WaitMax:  defaultRetryWaitMax,
			},
		},
		store:     store,
		newClient: newSDKAPIClient,
		now:       time.Now,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore

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

type dryRunState struct {
	health    collectorHealth
	discovery discoveryState
	bgp       bgpState
	topology  *topology.Data
	warnings  map[string]string
}

type discoveryState struct {
	siteIDs           []string
	siteNames         map[string]string
	fetchedAt         time.Time
	totalSites        int
	skippedBySelector int
	skippedByLimit    int
}

type bgpState struct {
	bySite      map[string][]bgpPeerState
	nextRefresh time.Time
	nextIndex   int
}

func (c *Collector) snapshotDryRunState() dryRunState {
	c.mu.RLock()
	topo := c.topology
	c.mu.RUnlock()
	return dryRunState{
		health:    cloneCollectorHealth(c.health),
		discovery: cloneDiscoveryState(c.discovery),
		bgp:       cloneBGPState(c.bgp),
		topology:  topo,
		warnings:  cloneStringMap(c.warningStates),
	}
}

func (c *Collector) restoreDryRunState(state dryRunState) {
	c.health = state.health
	c.discovery = state.discovery
	c.bgp = state.bgp
	c.warningStates = state.warnings
	c.mu.Lock()
	c.topology = state.topology
	c.mu.Unlock()
}

func cloneDiscoveryState(src discoveryState) discoveryState {
	dst := src
	dst.siteIDs = append([]string(nil), src.siteIDs...)
	if src.siteNames != nil {
		dst.siteNames = make(map[string]string, len(src.siteNames))
		for k, v := range src.siteNames {
			dst.siteNames[k] = v
		}
	}
	return dst
}

func cloneBGPState(src bgpState) bgpState {
	dst := src
	if src.bySite != nil {
		dst.bySite = make(map[string][]bgpPeerState, len(src.bySite))
		for siteID, peers := range src.bySite {
			dst.bySite[siteID] = append([]bgpPeerState(nil), peers...)
		}
	}
	return dst
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
	return c.collect(ctx, false)
}

func (c *Collector) Collect(ctx context.Context) error {
	if err := c.collect(ctx, true); err != nil {
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

func (c *Collector) collect(ctx context.Context, write bool) (err error) {
	if !write {
		previousState := c.snapshotDryRunState()
		defer func() {
			c.restoreDryRunState(previousState)
		}()
	}
	c.beginHealthCycle()
	if write {
		defer func() {
			c.health.CollectionSuccess = err == nil
			c.updateSiteSelectionHealth()
			if err != nil {
				c.markCollectionFailure(err)
			}
			c.writeCollectorHealth()
		}()
	}

	if c.client == nil {
		return errors.New("Cato client is not initialized")
	}

	if err := c.refreshDiscovery(ctx, false); err != nil {
		return fmt.Errorf("site discovery failed, error_class=%s", classifyCatoError(err))
	}
	if len(c.discovery.siteIDs) == 0 {
		return errors.New("no Cato sites discovered")
	}

	sites, order, err := c.collectSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("account snapshot failed, error_class=%s", classifyCatoError(err))
	}
	if len(sites) == 0 {
		return errors.New("no Cato sites returned by account snapshot")
	}
	c.pruneUnselectedSites(sites, &order)

	if c.metricsEnabled() {
		if err := c.collectMetrics(ctx, sites); err != nil {
			if write {
				c.warnRecoverable(warningKeyMetrics, classifyCatoError(err), "account metrics collection incomplete, error_class=%s", classifyCatoError(err))
			}
		} else if write {
			c.clearRecoverableWarning(warningKeyMetrics)
		}
	}

	if c.bgpEnabled() {
		if err := c.collectBGP(ctx, sites, order); err != nil {
			if write {
				c.warnRecoverable(warningKeyBGP, classifyCatoError(err), "BGP status collection incomplete, error_class=%s", classifyCatoError(err))
			}
		} else if write {
			c.clearRecoverableWarning(warningKeyBGP)
		}
	}

	c.applyEntityControls(sites, &order)

	now := c.now()
	var topo *topology.Data
	if c.topologyEnabled() {
		topo = buildTopology(c.AccountID, sites, order, now)
	}

	c.mu.Lock()
	c.topology = topo
	c.mu.Unlock()

	if write {
		c.writeMetrics(sites, order)
	}

	return nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
