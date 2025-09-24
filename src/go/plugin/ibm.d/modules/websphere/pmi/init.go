//go:build cgo
// +build cgo

package pmi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/pmi/contexts"
	pmiproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/pmi"
)

func defaultConfig() Config {
	return Config{
		Config: framework.Config{
			UpdateEvery:          5,
			ObsoletionIterations: 60,
		},
		Vnode: "",
		HTTPConfig: web.HTTPConfig{
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(5 * time.Second),
			},
		},
		PMIStatsType:              "extended",
		PMIRefreshRate:            60,
		PMICustomStatsPaths:       nil,
		ClusterName:               "",
		CellName:                  "",
		NodeName:                  "",
		ServerType:                "",
		CustomLabels:              map[string]string{},
		CollectJVMMetrics:         nil,
		CollectThreadPoolMetrics:  nil,
		CollectJDBCMetrics:        nil,
		CollectJCAMetrics:         nil,
		CollectJMSMetrics:         nil,
		CollectWebAppMetrics:      nil,
		CollectSessionMetrics:     nil,
		CollectTransactionMetrics: nil,
		CollectClusterMetrics:     nil,
		CollectServletMetrics:     nil,
		CollectEJBMetrics:         nil,
		CollectJDBCAdvanced:       nil,
		MaxThreadPools:            50,
		MaxJDBCPools:              50,
		MaxJCAPools:               50,
		MaxJMSDestinations:        50,
		MaxApplications:           100,
		MaxServlets:               50,
		MaxEJBs:                   50,
		CollectAppsMatching:       "",
		CollectPoolsMatching:      "",
		CollectJMSMatching:        "",
		CollectServletsMatching:   "",
		CollectEJBsMatching:       "",
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
	if cfg.URL == "" {
		return fmt.Errorf("url is required")
	}

	if cfg.ClientConfig.Timeout == 0 {
		c.Config.ClientConfig.Timeout = confopt.Duration(5 * time.Second)
	}

	httpCfg := c.HTTPConfig
	httpCfg.RequestConfig.URL = strings.TrimSpace(cfg.URL)

	client, err := pmiproto.NewClient(pmiproto.Config{
		URL:        httpCfg.RequestConfig.URL,
		StatsType:  cfg.PMIStatsType,
		HTTPConfig: httpCfg,
	})
	if err != nil {
		return err
	}
	c.client = client

	if cfg.CollectAppsMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectAppsMatching)
		if err != nil {
			return err
		}
		c.appSelector = sel
	}
	if cfg.CollectPoolsMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectPoolsMatching)
		if err != nil {
			return err
		}
		c.poolSelector = sel
	}
	if cfg.CollectJMSMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectJMSMatching)
		if err != nil {
			return err
		}
		c.jmsSelector = sel
	}
	if cfg.CollectServletsMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectServletsMatching)
		if err != nil {
			return err
		}
		c.servletSelector = sel
	}
	if cfg.CollectEJBsMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectEJBsMatching)
		if err != nil {
			return err
		}
		c.ejbSelector = sel
	}

	c.identity = common.Identity{
		Cluster: cfg.ClusterName,
		Cell:    cfg.CellName,
		Node:    cfg.NodeName,
		Server:  cfg.ServerType,
	}

	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.client != nil {
		c.client.Close()
	}
}
