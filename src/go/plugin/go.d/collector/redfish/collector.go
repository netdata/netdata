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
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/redfishfunc"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

const maxLoggedDiagnostics = 256

func init() {
	collectorapi.Register("redfish", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: redfishMethods,
		MethodHandler:   redfishFunctionHandler,
	})
}

type endpointClient interface {
	Check(context.Context) error
	Acquire(context.Context) (acquisition.Result, error)
	Close()
}

type collectionResult struct {
	AuthMethod  string
	ObservedAt  time.Time
	Metrics     cycleMetrics
	Hardware    []measurement.Observation
	Diagnostics []string
	Complete    bool
	Snapshot    *redfishfunc.Snapshot
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store            metrix.CollectorStore
	metrics          *collectorMetrics
	hardware         *hardwareMetrics
	funcRouter       funcapi.MethodHandler
	functionSnapshot atomic.Pointer[redfishfunc.Snapshot]

	endpointKey string

	client             endpointClient
	measurement        *measurement.Projector
	httpClient         *http.Client
	newClient          func(acquisition.Options, *http.Client) (endpointClient, error)
	now                func() time.Time
	warningMu          sync.Mutex
	warnedDiagnostics  map[string]struct{}
	diagnosticOverflow bool
	authSelectionOnce  sync.Once
}

// New returns one independently owned endpoint collector.
func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{
		Config: Config{
			UpdateEvery:           defaultUpdateEvery,
			AuthMethod:            defaultAuthMethod,
			Timeout:               defaultTimeout,
			MaxConcurrentRequests: defaultMaxConcurrentRequests,
			Collect:               defaultCollect,
		},
		store:    store,
		metrics:  newCollectorMetrics(store),
		hardware: newHardwareMetrics(store),
		newClient: func(opts acquisition.Options, client *http.Client) (endpointClient, error) {
			return acquisition.New(opts, client)
		},
		now: time.Now,
	}
	c.funcRouter = redfishfunc.NewRouter(functionDeps{
		snapshot: &c.functionSnapshot,
	})
	return c
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(ctx context.Context) error {
	c.Config.applyDefaults()
	if c.Name == "" {
		return errors.New("config validation: job name is required")
	}
	if len(c.Name) > measurement.MaxLabelValueBytes {
		return fmt.Errorf("config validation: job name must not exceed %d bytes", measurement.MaxLabelValueBytes)
	}
	if err := c.Config.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	root, origin, _ := acquisition.NormalizeServiceRoot(c.URL)
	if root.Scheme == "http" {
		c.Warningf("Redfish endpoint %s uses unencrypted HTTP; credentials and metrics can be intercepted", origin)
	} else if c.TLSSkipVerify {
		c.Warningf("Redfish endpoint %s has TLS certificate verification disabled", origin)
	}
	c.endpointKey = identity.Key("netdata:redfish:endpoint:v1", origin, identity.EndpointKeyHexChars)
	var err error
	c.httpClient, err = newHTTPClient(ctx, c.Config)
	if err != nil {
		return fmt.Errorf("init HTTP client: %w", err)
	}
	c.client, err = c.newClient(
		acquisition.Options{
			URL:                   c.URL,
			AuthMethod:            c.AuthMethod,
			Username:              c.Username,
			Password:              c.Password,
			MaxConcurrentRequests: c.MaxConcurrentRequests,
			Collect:               c.Config.Collect,
		},
		c.httpClient,
	)
	if err != nil {
		c.httpClient.CloseIdleConnections()
		c.httpClient = nil
		return fmt.Errorf("init Redfish client: %w", err)
	}
	c.measurement = measurement.New(origin, c.Name, acquisition.ReadingProvenanceResolver(root, origin))
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("Redfish client is not initialized")
	}
	if err := c.client.Check(ctx); err != nil {
		return fmt.Errorf("Redfish endpoint check: %w", err)
	}

	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	if c.client == nil {
		c.functionSnapshot.Store(nil)
		return errors.New("Redfish client is not initialized")
	}

	cycleCtx, cancel := context.WithTimeout(ctx, time.Duration(c.UpdateEvery)*time.Second)
	defer cancel()
	result, err := c.collect(cycleCtx)
	if result.AuthMethod != "" {
		c.authSelectionOnce.Do(func() { c.Infof("Redfish authentication method selected: %s", result.AuthMethod) })
	}
	if err := ctx.Err(); err != nil {
		c.functionSnapshot.Store(nil)
		return err
	}
	c.functionSnapshot.Store(result.Snapshot)
	c.warnCollectionDiagnostics(result.Diagnostics)
	c.metrics.observe(c.endpointKey, c.Name, result.Metrics)
	c.hardware.observe(result.Hardware)

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
		diagnostic = acquisition.BoundDiagnostic(strings.TrimSpace(diagnostic))
		if diagnostic == "" {
			continue
		}
		if _, ok := c.warnedDiagnostics[diagnostic]; ok {
			continue
		}
		if len(c.warnedDiagnostics) >= maxLoggedDiagnostics {
			if !c.diagnosticOverflow {
				c.diagnosticOverflow = true
				c.Warningf(
					"Redfish collection diagnostics exceeded the fixed logging bound; additional distinct diagnostics are suppressed",
				)
			}
			continue
		}
		c.warnedDiagnostics[diagnostic] = struct{}{}
		c.Warningf("%s", diagnostic)
	}
}

func (c *Collector) Cleanup(ctx context.Context) {
	c.functionSnapshot.Store(nil)
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	c.measurement = nil
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
		c.httpClient = nil
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
