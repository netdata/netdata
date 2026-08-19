// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("panos", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(3 * time.Second),
				},
			},
		},
		store:               store,
		metrics:             newCollectorMetrics(store),
		routingEngine:       routingEngineUnknown,
		newAPIClient:        newPangoAPIClient,
		advancedBGPCommands: advancedBGPPeerCommands,
		now:                 time.Now,
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	APIKey             string `yaml:"api_key,omitempty" json:"api_key"`
	Vsys               string `yaml:"vsys,omitempty" json:"vsys"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	apiClient panosAPIClient

	routingEngine routingEngine
	bgpCommand    string
	noBGPProbedAt time.Time

	newAPIClient        func(Config) (panosAPIClient, error)
	advancedBGPCommands []string
	now                 func() time.Time
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}

	client, err := c.newAPIClient(c.Config)
	if err != nil {
		return fmt.Errorf("init PAN-OS API client: %w", err)
	}
	c.apiClient = client

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.apiClient == nil {
		return errors.New("PAN-OS API client not initialized")
	}
	defer c.logSystemInfo()

	if err := contextError(ctx); err != nil {
		return err
	}
	if _, err := c.querySystemInfo(ctx); err != nil {
		return fmt.Errorf("check system info: %w", err)
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	hasMetrics, err := c.collect(ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if !hasMetrics {
		return err
	}
	if err != nil {
		c.Limit(logKeyCollectPartialError, 1, recurringLogEvery).
			Warningf("PAN-OS partial collection error: %v", err)
	}
	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.apiClient != nil {
		c.apiClient.closeIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("config: url not configured")
	}
	if c.APIKey == "" && (c.Username == "" || c.Password == "") {
		return errors.New("config: api_key or username/password must be set")
	}
	if c.ForceHTTP2 {
		return errors.New("config: force_http2 is not supported by the panos collector")
	}
	if c.BearerTokenFile != "" {
		return errors.New("config: bearer_token_file is not supported by the panos collector")
	}
	if c.Method != "" {
		return errors.New("config: method is not supported by the panos collector")
	}
	if c.Body != "" {
		return errors.New("config: body is not supported by the panos collector")
	}
	if c.NotFollowRedirect {
		return errors.New("config: not_follow_redirects is not supported by the panos collector")
	}
	if c.ProxyUsername != "" || c.ProxyPassword != "" {
		return errors.New("config: proxy_username/proxy_password are not supported; include proxy credentials in proxy_url")
	}
	if (c.TLSCert != "" && c.TLSKey == "") || (c.TLSKey != "" && c.TLSCert == "") {
		return errors.New("config: tls_cert and tls_key must both be set")
	}
	return nil
}

const (
	recurringLogEvery         = time.Hour
	logKeyCollectPartialError = "panos:collect:partial_error"
	logKeySystemInfo          = "panos:system_info"
	logKeyPanorama            = "panos:panorama"
)

func (c *Collector) logSystemInfo() {
	if c.apiClient == nil {
		return
	}

	info := c.apiClient.systemInfo()
	if len(info) == 0 {
		return
	}

	hostname := firstNonEmpty(info["hostname"], info["devicename"])
	model := info["model"]
	swVersion := info["sw-version"]
	serial := info["serial"]
	haState := firstNonEmpty(info["ha-state"], info["state"])

	parts := make([]string, 0, 4)
	if hostname != "" {
		parts = append(parts, "hostname="+hostname)
	}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if swVersion != "" {
		parts = append(parts, "sw_version="+swVersion)
	}
	if serial != "" {
		parts = append(parts, "serial="+serial)
	}
	if haState != "" {
		parts = append(parts, "ha_state="+haState)
	}

	if len(parts) > 0 {
		c.Limit(logKeySystemInfo, 1, 0).
			Infof("connected to PAN-OS device: %s", strings.Join(parts, ", "))
	}
	if strings.Contains(strings.ToLower(model), "panorama") {
		c.Limit(logKeyPanorama, 1, 0).
			Warningf("PAN-OS device appears to be Panorama (model=%s); Panorama target proxy mode is not supported by this collector version", model)
	}
}
