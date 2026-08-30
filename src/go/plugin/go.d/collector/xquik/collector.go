// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("xquik", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
			HTTPConfig: HTTPConfig{
				URL:     defaultEndpoint,
				Timeout: defaultTimeout,
			},
		},
		store:     store,
		metrics:   newProfileMetrics(store),
		newClient: newProfileClient,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics profileMetrics

	httpClient *http.Client
	client     profileClient
	newClient  func(Config, *http.Client) profileClient
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	c.Config.applyDefaults()
	if err := c.Config.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	clientConfig := web.ClientConfig{
		Timeout:           c.Timeout,
		NotFollowRedirect: true,
		ProxyURL:          c.ProxyURL,
		TLSConfig:         c.TLSConfig,
	}
	httpClient, err := web.NewHTTPClient(clientConfig)
	if err != nil {
		return fmt.Errorf("init HTTP client: %w", err)
	}
	c.httpClient = httpClient
	c.client = c.newClient(c.Config, httpClient)
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("Xquik client is not initialized")
	}
	if _, err := c.client.Profile(ctx, c.User); err != nil {
		return fmt.Errorf("check Xquik profile: %w", err)
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	if c.client == nil {
		return errors.New("Xquik client is not initialized")
	}
	p, err := c.client.Profile(ctx, c.User)
	if err != nil {
		return fmt.Errorf("collect Xquik profile: %w", err)
	}
	c.metrics.write(p)
	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
