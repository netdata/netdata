// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_mp

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("websphere_mp", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WebSphereMicroProfile {
	return &WebSphereMicroProfile{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://localhost:9443",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 10),
				},
			},
			UpdateEvery: 5,

			// MicroProfile specific defaults
			MetricsEndpoint: "/metrics",

			// Collection flags
			CollectJVMMetrics:    true,
			CollectRESTMetrics:   true,

			// Cardinality limits
			MaxRESTEndpoints: 50,
		},

		charts:           &module.Charts{},
		seenMetrics:      make(map[string]bool),
		collectedMetrics: make(map[string]bool),
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`

	// Server identification
	CellName   string `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName   string `yaml:"node_name,omitempty" json:"node_name"`
	ServerName string `yaml:"server_name,omitempty" json:"server_name"`

	// MicroProfile specific
	MetricsEndpoint string `yaml:"metrics_endpoint,omitempty" json:"metrics_endpoint"`

	// Collection flags
	CollectJVMMetrics    bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
	CollectRESTMetrics   bool `yaml:"collect_rest_metrics" json:"collect_rest_metrics"`

	// Cardinality control
	MaxRESTEndpoints int `yaml:"max_rest_endpoints,omitempty" json:"max_rest_endpoints"`

	// Filtering
	CollectRESTMatching   string `yaml:"collect_rest_matching,omitempty" json:"collect_rest_matching"`
}

type WebSphereMicroProfile struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
	metricsURL string

	// For tracking dynamic metrics
	seenMetrics      map[string]bool
	collectedMetrics map[string]bool
	lastMetricCount  int

	// Selectors
	restSelector   matcher.Matcher

	// Cached server info
	serverVersion string
	serverType    string // "Liberty MicroProfile"

	// Metric name patterns
	jvmPattern    *regexp.Regexp
	restPattern   *regexp.Regexp
}

func (w *WebSphereMicroProfile) Configuration() any {
	return w.Config
}

func (w *WebSphereMicroProfile) Init(context.Context) error {
	if w.HTTPConfig.RequestConfig.URL == "" {
		return errors.New("websphere_mp URL is required")
	}

	// Set default metrics endpoint for MicroProfile
	if w.MetricsEndpoint == "" {
		w.MetricsEndpoint = "/metrics"
	}

	// Build metrics URL with smart detection
	if err := w.buildMetricsURL(); err != nil {
		return fmt.Errorf("failed to build metrics URL: %w", err)
	}

	// Validate cardinality limits
	if w.MaxRESTEndpoints < 0 {
		w.MaxRESTEndpoints = 0
	}

	// Initialize selectors
	if w.CollectRESTMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectRESTMatching)
		if err != nil {
			return fmt.Errorf("invalid REST selector pattern '%s': %v", w.CollectRESTMatching, err)
		}
		w.restSelector = m
	}

	// Initialize metric patterns
	// Liberty MicroProfile JVM metrics use patterns like: memory_, gc_, thread_, classloader_, jvm_
	w.jvmPattern = regexp.MustCompile(`^(?:jvm_|base_jvm_|memory_|gc_|thread_|classloader_|cpu_).*`)
	w.restPattern = regexp.MustCompile(`^(?:REST_request_|base_REST_).*`)

	httpClient, err := web.NewHTTPClient(w.HTTPConfig.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	w.httpClient = httpClient

	w.Debugf("initialized websphere_mp collector: base_url=%s, metrics_endpoint=%s, final_url=%s", w.HTTPConfig.RequestConfig.URL, w.MetricsEndpoint, w.metricsURL)

	return nil
}

func (w *WebSphereMicroProfile) buildMetricsURL() error {
	baseURL := strings.TrimRight(w.HTTPConfig.RequestConfig.URL, "/")

	// Parse the base URL to check if it already contains a metrics path
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}

	// Check if the URL already contains the metrics endpoint path
	if strings.HasSuffix(u.Path, w.MetricsEndpoint) {
		// URL already contains the full path
		w.metricsURL = baseURL
		w.Debugf("using full metrics URL as provided: %s", w.metricsURL)
		return nil
	}

	// Check if URL has any path that might be the metrics endpoint
	if u.Path != "" && u.Path != "/" {
		// URL has a path but it's not our default metrics endpoint
		// Assume it's a complete custom endpoint
		w.metricsURL = baseURL
		w.Debugf("using complete custom metrics URL: %s", w.metricsURL)
		return nil
	}

	// Append the metrics endpoint to the base URL
	w.metricsURL = baseURL + w.MetricsEndpoint
	w.Debugf("built metrics URL by appending endpoint: %s", w.metricsURL)

	return nil
}

func (w *WebSphereMicroProfile) Check(ctx context.Context) error {
	mx, err := w.collect(ctx)
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (w *WebSphereMicroProfile) Charts() *module.Charts {
	return w.charts
}

func (w *WebSphereMicroProfile) Collect(ctx context.Context) map[string]int64 {
	mx, err := w.collect(ctx)
	if err != nil {
		w.Errorf("failed to collect metrics: %v", err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (w *WebSphereMicroProfile) Cleanup(context.Context) {
	if w.httpClient != nil {
		w.httpClient.CloseIdleConnections()
	}
}

// cleanName converts metric names to chart-safe identifiers
func cleanName(name string) string {
	// Replace non-alphanumeric characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	cleaned := reg.ReplaceAllString(name, "_")

	// Remove multiple consecutive underscores
	reg2 := regexp.MustCompile(`_+`)
	cleaned = reg2.ReplaceAllString(cleaned, "_")

	// Trim leading/trailing underscores
	return strings.Trim(cleaned, "_")
}
