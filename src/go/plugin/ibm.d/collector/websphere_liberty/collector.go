// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_liberty

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("websphere_liberty", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WebSphere {
	return &WebSphere{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://localhost:9443",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},
			UpdateEvery: 5,

			// Metric collection flags
			CollectJVMMetrics:            true,
			CollectThreadPoolMetrics:     true,
			CollectConnectionPoolMetrics: true,
			CollectWebAppMetrics:         true,
			CollectSessionMetrics:        true,

			// Cardinality limits
			MaxThreadPools:     20,
			MaxConnectionPools: 20,
			MaxApplications:    50,
		},

		charts:         &module.Charts{},
		collectedApps:  make(map[string]bool),
		collectedPools: make(map[string]bool),
		seenApps:       make(map[string]bool),
		seenPools:      make(map[string]bool),
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`

	// Server identification (for clustered environments)
	CellName   string `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName   string `yaml:"node_name,omitempty" json:"node_name"`
	ServerName string `yaml:"server_name,omitempty" json:"server_name"`

	// Liberty specific
	MetricsEndpoint string `yaml:"metrics_endpoint,omitempty" json:"metrics_endpoint"`

	// Collection flags
	CollectJVMMetrics            bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
	CollectThreadPoolMetrics     bool `yaml:"collect_threadpool_metrics" json:"collect_threadpool_metrics"`
	CollectConnectionPoolMetrics bool `yaml:"collect_connectionpool_metrics" json:"collect_connectionpool_metrics"`
	CollectWebAppMetrics         bool `yaml:"collect_webapp_metrics" json:"collect_webapp_metrics"`
	CollectSessionMetrics        bool `yaml:"collect_session_metrics" json:"collect_session_metrics"`

	// Cardinality control
	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`
	MaxConnectionPools int `yaml:"max_connectionpools,omitempty" json:"max_connectionpools"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications"`

	// Filtering
	CollectAppsMatching  string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`
	CollectPoolsMatching string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`
}

type WebSphere struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	// For tracking dynamic instances
	collectedApps  map[string]bool
	collectedPools map[string]bool
	seenApps       map[string]bool
	seenPools      map[string]bool

	// Selectors
	appSelector  matcher.Matcher
	poolSelector matcher.Matcher

	// Cached server info
	serverVersion string
	serverType    string // "Liberty" or "Traditional"
}

func (w *WebSphere) Configuration() any {
	return w.Config
}

func (w *WebSphere) Init(context.Context) error {
	if w.HTTPConfig.RequestConfig.URL == "" {
		return errors.New("websphere_liberty URL is required")
	}

	// Set default metrics endpoint for Liberty
	if w.MetricsEndpoint == "" {
		w.MetricsEndpoint = "/ibm/api/metrics"
	}

	// Validate cardinality limits
	if w.MaxThreadPools < 0 {
		w.MaxThreadPools = 0
	}
	if w.MaxConnectionPools < 0 {
		w.MaxConnectionPools = 0
	}
	if w.MaxApplications < 0 {
		w.MaxApplications = 0
	}

	// Initialize selectors
	if w.CollectAppsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectAppsMatching)
		if err != nil {
			return fmt.Errorf("invalid app selector pattern '%s': %v", w.CollectAppsMatching, err)
		}
		w.appSelector = m
	}

	if w.CollectPoolsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectPoolsMatching)
		if err != nil {
			return fmt.Errorf("invalid pool selector pattern '%s': %v", w.CollectPoolsMatching, err)
		}
		w.poolSelector = m
	}

	httpClient, err := web.NewHTTPClient(w.HTTPConfig.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	w.httpClient = httpClient

	w.Debugf("initialized websphere_liberty collector: url=%s, metrics_endpoint=%s", w.HTTPConfig.RequestConfig.URL, w.MetricsEndpoint)

	return nil
}

func (w *WebSphere) Check(ctx context.Context) error {
	mx, err := w.collect(ctx)
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (w *WebSphere) Charts() *module.Charts {
	return w.charts
}

func (w *WebSphere) Collect(ctx context.Context) map[string]int64 {
	mx, err := w.collect(ctx)
	if err != nil {
		w.Error(err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (w *WebSphere) Cleanup(context.Context) {
	if w.httpClient != nil {
		w.httpClient.CloseIdleConnections()
	}
}
