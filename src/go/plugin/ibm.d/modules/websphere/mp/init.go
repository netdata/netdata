//go:build cgo
// +build cgo

package mp

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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
		URL:                 "https://localhost:9443",
		MetricsEndpoint:     "/metrics",
		CollectJVMMetrics:   true,
		CollectRESTMetrics:  true,
		MaxRESTEndpoints:    50,
		CollectRESTMatching: "",
		HTTPConfig: web.HTTPConfig{
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(defaultTimeout),
			},
		},
	}
}

func (c *Collector) Init(ctx context.Context) error {
	c.SetImpl(c)
	c.RegisterContexts(contexts.GetAllContexts()...)

	cfg := c.Config
	baseURL := strings.TrimSpace(cfg.URL)
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
			metricsURL = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
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
