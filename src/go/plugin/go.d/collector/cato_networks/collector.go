// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
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

	markerStore eventsMarkerPersistence
	eventMarker string

	markerStoreAvailable bool

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

type eventsMarkerPersistence interface {
	read() (string, error)
	write(string) error
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

	c.markerStore = newEventsMarkerStore(c.Events.MarkerFile, c.AccountID, c.URL, c.Vnode)
	c.markerStoreAvailable = c.markerStore != nil
	if c.markerStore != nil {
		marker, err := c.markerStore.read()
		if err != nil {
			c.markerStoreAvailable = false
			c.Warningf("events marker read failed, continuing without persisted marker: %v", err)
		}
		c.eventMarker = marker
	} else if c.eventsEnabled() {
		c.Warningf("events marker persistence disabled because Netdata varlib is unavailable and events.marker_file is not configured; events counters may reset across restarts")
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

	var events eventsCollection
	if write && c.eventsEnabled() {
		events, err = c.collectEvents(ctx)
		if err != nil {
			c.warnRecoverable(warningKeyEvents, classifyCatoError(err), "events feed collection incomplete, error_class=%s", classifyCatoError(err))
		} else {
			c.clearRecoverableWarning(warningKeyEvents)
			c.clearRecoverableWarning(warningKeyEventAccountErr)
			c.health.MarkerPersistenceAvailable = c.markerStoreAvailable
		}
	}

	now := c.now()
	var topo *topology.Data
	if c.topologyEnabled() {
		topo = buildTopology(c.AccountID, sites, order, now)
	}

	c.mu.Lock()
	c.topology = topo
	c.mu.Unlock()

	if write {
		c.writeMetrics(sites, order, events.counts)
		c.commitEventsMarker(ctx, events.marker)
	}

	return nil
}

func (c *Collector) commitEventsMarker(ctx context.Context, marker string) {
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return
	}
	c.eventMarker = marker
	if c.markerStore == nil {
		return
	}
	if err := c.writeEventsMarkerWithRetry(ctx, marker); err != nil {
		c.markerStoreAvailable = false
		c.health.MarkerPersistenceAvailable = false
		c.markOperationFailure(operationEventMarker, err)
		c.warnRecoverable(warningKeyEventMarker, classifyCatoError(err), "events marker write failed, error_class=%s", classifyCatoError(err))
		return
	}
	c.markerStoreAvailable = true
	c.health.MarkerPersistenceAvailable = true
	c.markOperationSuccess(operationEventMarker)
	c.clearRecoverableWarning(warningKeyEventMarker)
}

func (c *Collector) writeEventsMarkerWithRetry(ctx context.Context, marker string) error {
	attempts := c.Retry.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = c.markerStore.write(marker); err == nil {
			return nil
		}
		if attempt == attempts || !isRetryableMarkerWriteError(err) {
			return err
		}
		if sleepErr := sleepContext(ctx, retryWait(c.Retry.WaitMin.Duration(), c.Retry.WaitMax.Duration(), attempt)); sleepErr != nil {
			return sleepErr
		}
	}
	return err
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

func isRetryableMarkerWriteError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsTimeout(err) {
		return true
	}
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}
