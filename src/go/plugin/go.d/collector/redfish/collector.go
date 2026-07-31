// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

const maxLoggedDiagnostics = 256

//go:generate go run ./chartgen

// Register composes the endpoint collector with the Redfish feature runtime.
func Register(runtime *redfishruntime.Runtime) {
	collectorapi.Register("redfish", newCreator(runtime))
}

func newCreator(runtime *redfishruntime.Runtime) collectorapi.Creator {
	if runtime == nil {
		panic("redfish Register requires a non-nil runtime")
	}
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2:       func() collectorapi.CollectorV2 { return New(runtime) },
		Config:         func() any { return &Config{} },
		AgentFunctions: redfishFunctions(runtime),
		MethodHandler:  redfishFunctionHandler(runtime),
	}
}

type endpointClient interface {
	Check(context.Context) error
	Collect(context.Context) (collectionResult, error)
	Close(context.Context) error
}

type collectionResult struct {
	ObservedAt  time.Time
	Metrics     cycleMetrics
	Inventory   []map[string]any
	Hardware    []hardwareObservation
	Diagnostics []string
	Complete    bool
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	runtime  *redfishruntime.Runtime
	store    metrix.CollectorStore
	metrics  *collectorMetrics
	hardware *hardwareMetrics

	jobName     string
	rootURL     string
	origin      string
	endpointKey string

	client             endpointClient
	httpClient         *http.Client
	unregisterProducer func()
	unregisterEndpoint func()
	newClient          func(Config, *http.Client) (endpointClient, error)
	now                func() time.Time
	warningMu          sync.Mutex
	warnedDiagnostics  map[string]struct{}
	diagnosticOverflow bool
	logRouteMu         sync.Mutex
	lastLogRoute       string
	authSelectionOnce  sync.Once
}

// New returns one endpoint collector bound to the provided feature runtime.
func New(runtime *redfishruntime.Runtime) *Collector {
	if runtime == nil {
		panic("redfish New requires a non-nil runtime")
	}
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
		},
		runtime:   runtime,
		store:     store,
		metrics:   newCollectorMetrics(store),
		hardware:  newHardwareMetrics(store),
		newClient: newEndpointClient,
		now:       time.Now,
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) SetJobName(name string) { c.jobName = name }

func (c *Collector) Init(context.Context) error {
	c.Config.applyDefaults()
	if c.jobName == "" {
		return errors.New("config validation: job name is required")
	}
	if len(c.jobName) > promotedLabelLimit {
		return fmt.Errorf("config validation: job name must not exceed %d bytes", promotedLabelLimit)
	}
	if err := c.Config.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	root, origin, _ := normalizeServiceRoot(c.URL)
	c.rootURL = root.String()
	c.origin = origin
	if root.Scheme == "http" {
		c.Warningf("Redfish endpoint %s uses unencrypted HTTP; credentials, inventory, metrics, and logs can be intercepted", origin)
	} else if c.TLSSkipVerify {
		c.Warningf("Redfish endpoint %s has TLS certificate verification disabled", origin)
	}
	c.endpointKey = stableKey("netdata:redfish:endpoint:v1", origin, endpointKeyHexChars)
	var err error
	c.unregisterEndpoint, err = c.runtime.RegisterEndpoint(
		c.endpointKey,
		stableKey("netdata:redfish:endpoint:v1", origin, digestHexChars),
	)
	if err != nil {
		return fmt.Errorf("register Redfish endpoint identity: %w", err)
	}

	c.httpClient, err = newHTTPClient(c.Config)
	if err != nil {
		c.unregisterEndpoint()
		c.unregisterEndpoint = nil
		return fmt.Errorf("init HTTP client: %w", err)
	}
	c.client, err = c.newClient(c.Config, c.httpClient)
	if err != nil {
		c.httpClient.CloseIdleConnections()
		c.httpClient = nil
		c.unregisterEndpoint()
		c.unregisterEndpoint = nil
		return fmt.Errorf("init Redfish client: %w", err)
	}
	if setter, ok := c.client.(interface{ setEndpointJob(string) }); ok {
		setter.setEndpointJob(c.jobName)
	}
	if setter, ok := c.client.(interface {
		setLogRoute(*redfishruntime.Runtime, string)
	}); ok {
		setter.setLogRoute(c.runtime, c.LogsBackend())
	}
	if c.Logs.enabled() {
		c.unregisterProducer, err = c.runtime.RegisterProducer(c.LogsBackend(), c.jobName)
		if err != nil {
			_ = c.client.Close(context.Background())
			c.client = nil
			c.httpClient.CloseIdleConnections()
			c.httpClient = nil
			c.unregisterEndpoint()
			c.unregisterEndpoint = nil
			return fmt.Errorf("register Redfish log producer: %w", err)
		}
	}
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("Redfish client is not initialized")
	}
	if err := c.client.Check(ctx); err != nil {
		return fmt.Errorf("Redfish endpoint check: %w", err)
	}
	if reporter, ok := c.client.(interface{ selectedAuthenticationMethod() string }); ok {
		if method := reporter.selectedAuthenticationMethod(); method != "" {
			c.authSelectionOnce.Do(func() {
				c.Infof("Redfish authentication method selected: %s", method)
			})
		}
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	started := c.now()
	if c.client == nil {
		return errors.New("Redfish client is not initialized")
	}

	cycleCtx, cancel := context.WithTimeout(ctx, time.Duration(c.UpdateEvery)*time.Second)
	defer cancel()
	result, err := c.client.Collect(cycleCtx)
	if result.ObservedAt.IsZero() {
		result.ObservedAt = c.now()
	}
	if result.Metrics.Duration == 0 {
		result.Metrics.Duration = c.now().Sub(started).Seconds()
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	c.applyLogRouteState(&result.Metrics)
	c.warnCollectionDiagnostics(result.Diagnostics)
	c.metrics.observe(c.endpointKey, c.jobName, result.Metrics)
	c.hardware.observe(result.Hardware)

	if !identityIntegrityError(err) && result.Metrics.Status != "unavailable" {
		for _, row := range result.Inventory {
			row["endpoint_job"] = c.jobName
			row["endpoint_key"] = c.endpointKey
		}
		if publishErr := c.runtime.PublishInventory(redfishruntime.InventorySnapshot{
			Job:         c.jobName,
			ObservedAt:  result.ObservedAt,
			Complete:    result.Complete,
			Rows:        result.Inventory,
			Diagnostics: result.Diagnostics,
		}); publishErr != nil {
			return fmt.Errorf("publish Redfish inventory: %w", publishErr)
		}
	}

	if err != nil && result.Metrics.Status == "" {
		return err
	}
	if err != nil {
		c.Limit("redfish:partial-collection", 1, time.Hour).
			Warningf("Redfish partial collection error: %v", err)
	}
	return nil
}

func (c *Collector) warnCollectionDiagnostics(diagnostics []string) {
	c.warningMu.Lock()
	defer c.warningMu.Unlock()
	if c.warnedDiagnostics == nil {
		c.warnedDiagnostics = make(map[string]struct{})
	}
	for _, diagnostic := range diagnostics {
		diagnostic = boundedDiagnostic(strings.TrimSpace(diagnostic))
		if diagnostic == "" {
			continue
		}
		if _, ok := c.warnedDiagnostics[diagnostic]; ok {
			continue
		}
		if len(c.warnedDiagnostics) >= maxLoggedDiagnostics {
			if !c.diagnosticOverflow {
				c.diagnosticOverflow = true
				c.Warningf("Redfish collection diagnostics exceeded the fixed logging bound; additional distinct diagnostics are suppressed")
			}
			continue
		}
		c.warnedDiagnostics[diagnostic] = struct{}{}
		c.Warningf("%s", diagnostic)
	}
}

func (c *Collector) applyLogRouteState(metrics *cycleMetrics) {
	if !c.Logs.enabled() {
		metrics.LogsRoute = "disabled"
		metrics.LogsAdmission = "disabled"
		c.observeLogRouteState(metrics.LogsRoute)
		return
	}
	if c.runtime.BackendAvailable(c.LogsBackend()) {
		metrics.LogsRoute = "ready"
		if metrics.LogsAdmission == "" {
			metrics.LogsAdmission = "ready"
		}
		c.observeLogRouteState(metrics.LogsRoute)
		return
	}
	metrics.LogsRoute = "unavailable"
	if metrics.LogsAdmission == "" {
		metrics.LogsAdmission = "unknown"
	}
	c.observeLogRouteState(metrics.LogsRoute)
}

func (c *Collector) observeLogRouteState(state string) {
	c.logRouteMu.Lock()
	previous := c.lastLogRoute
	if state == previous {
		c.logRouteMu.Unlock()
		return
	}
	c.lastLogRoute = state
	c.logRouteMu.Unlock()

	switch state {
	case "unavailable":
		c.Limit("redfish:log-backend-unavailable", 1, time.Hour).
			Warningf("Redfish log backend %q is unavailable or not ready; log polling is paused while metrics continue", c.LogsBackend())
	case "ready":
		if previous == "unavailable" {
			c.Infof("Redfish log backend %q is ready; log polling can resume", c.LogsBackend())
		}
	}
}

func (c *Collector) Cleanup(ctx context.Context) {
	c.runtime.RemoveInventory(c.jobName)
	if c.client != nil {
		_ = c.client.Close(ctx)
		c.client = nil
	}
	if c.unregisterProducer != nil {
		c.unregisterProducer()
		c.unregisterProducer = nil
	}
	if c.unregisterEndpoint != nil {
		c.unregisterEndpoint()
		c.unregisterEndpoint = nil
	}
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
		c.httpClient = nil
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
