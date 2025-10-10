//go:build cgo
// +build cgo

package mp

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/mp/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/openmetrics"
)

func defaultConfig() Config {
	return Config{
		Config: framework.Config{
			UpdateEvery:          1,
			ObsoletionIterations: 60,
		},
		Vnode:               "",
		CellName:            "",
		NodeName:            "",
		ServerName:          "",
		MetricsEndpoint:     "/metrics",
		CollectJVMMetrics:   framework.AutoBoolEnabled,
		CollectRESTMetrics:  framework.AutoBoolEnabled,
		MaxRESTEndpoints:    50,
		CollectRESTMatching: "",
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL: "https://localhost:9443",
			},
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(defaultTimeout),
			},
		},
	}
}

func (c *Collector) Init(ctx context.Context) error {
	// Propagate job-level overrides into the embedded framework collector before init
	if c.Config.ObsoletionIterations != 0 {
		c.Collector.Config.ObsoletionIterations = c.Config.ObsoletionIterations
	}
	if c.Config.UpdateEvery != 0 {
		c.Collector.Config.UpdateEvery = c.Config.UpdateEvery
	}
	if c.Config.CollectionGroups != nil {
		c.Collector.Config.CollectionGroups = c.Config.CollectionGroups
	}

	c.RegisterContexts(contexts.GetAllContexts()...)

	if err := c.Collector.Init(ctx); err != nil {
		return err
	}
	c.SetImpl(c)

	cfg := c.Config
	baseURL := strings.TrimSpace(cfg.HTTPConfig.RequestConfig.URL)
	if baseURL == "" {
		return fmt.Errorf("url is required")
	}

	if cfg.ClientConfig.Timeout == 0 {
		c.Config.ClientConfig.Timeout = confopt.Duration(defaultTimeout)
	}

	metricsURL := baseURL
	if endpoint := strings.TrimSpace(cfg.MetricsEndpoint); endpoint != "" {
		if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
			metricsURL = endpoint
		} else {
			baseNormalized := strings.TrimRight(baseURL, "/")
			endpointNormalized := strings.TrimLeft(endpoint, "/")
			if endpointNormalized == "" {
				metricsURL = baseNormalized
			} else if strings.HasSuffix(baseNormalized, "/"+endpointNormalized) {
				metricsURL = baseNormalized
			} else {
				metricsURL = baseNormalized + "/" + endpointNormalized
			}
		}
	}
	c.Config.HTTPConfig.RequestConfig.URL = metricsURL

	client, err := openmetrics.NewClient(openmetrics.Config{
		HTTPConfig: c.Config.HTTPConfig,
	})
	if err != nil {
		return err
	}
	c.client = client

	if cfg.CollectRESTMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectRESTMatching)
		if err != nil {
			return err
		}
		c.restSelector = sel
	}

	if cfg.MaxRESTEndpoints < 0 {
		c.MaxRESTEndpoints = 0
	}

	c.identity = common.Identity{
		Cell:   cfg.CellName,
		Node:   cfg.NodeName,
		Server: cfg.ServerName,
	}

	return nil
}

func (c *Collector) Cleanup(context.Context) {
}
