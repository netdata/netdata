//go:build cgo
// +build cgo

package jmx

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/jmx/contexts"
	jmxproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/jmx"
)

func defaultConfig() Config {
	return Config{
		Config: framework.Config{
			UpdateEvery:          int(defaultUpdateEvery / time.Second),
			ObsoletionIterations: 60,
		},
		Vnode: "",

		JMXURL:       "",
		JMXUsername:  "",
		JMXPassword:  "",
		JMXClasspath: "",
		JavaExecPath: "",

		JMXTimeout:    confopt.Duration(defaultScrapeTimeout),
		InitTimeout:   confopt.Duration(defaultInitTimeout),
		ShutdownDelay: confopt.Duration(defaultShutdownDelay),

		ClusterName:  "",
		CellName:     "",
		NodeName:     "",
		ServerName:   "",
		ServerType:   "",
		CustomLabels: map[string]string{},

		CollectJVMMetrics:         framework.AutoBoolEnabled,
		CollectThreadPoolMetrics:  framework.AutoBoolEnabled,
		CollectJDBCMetrics:        framework.AutoBoolEnabled,
		CollectJCAMetrics:         framework.AutoBoolEnabled,
		CollectJMSMetrics:         framework.AutoBoolEnabled,
		CollectWebAppMetrics:      framework.AutoBoolEnabled,
		CollectSessionMetrics:     framework.AutoBoolEnabled,
		CollectTransactionMetrics: framework.AutoBoolEnabled,
		CollectClusterMetrics:     framework.AutoBoolEnabled,
		CollectServletMetrics:     framework.AutoBoolEnabled,
		CollectEJBMetrics:         framework.AutoBoolEnabled,
		CollectJDBCAdvanced:       framework.AutoBoolDisabled,

		MaxThreadPools:     50,
		MaxJDBCPools:       50,
		MaxJCAPools:        50,
		MaxJMSDestinations: 50,
		MaxApplications:    100,
		MaxServlets:        50,
		MaxEJBs:            50,

		CollectPoolsMatching:    "",
		CollectJMSMatching:      "",
		CollectAppsMatching:     "",
		CollectServletsMatching: "",
		CollectEJBsMatching:     "",

		MaxRetries:              3,
		RetryBackoffMultiplier:  2.0,
		CircuitBreakerThreshold: 5,
		HelperRestartMax:        3,
	}
}

// Init initialises the collector.
func (c *Collector) Init(context.Context) error {
	c.SetImpl(c)
	c.RegisterContexts(contexts.GetAllContexts()...)

	cfg := &c.Config

	if cfg.JMXURL == "" {
		return fmt.Errorf("jmx_url is required")
	}

	if cfg.UpdateEvery <= 0 {
		c.Config.UpdateEvery = int(defaultUpdateEvery / time.Second)
	}

	if cfg.JMXTimeout <= 0 {
		c.Config.JMXTimeout = confopt.Duration(defaultScrapeTimeout)
	}
	if cfg.InitTimeout <= 0 {
		c.Config.InitTimeout = confopt.Duration(defaultInitTimeout)
	}
	if cfg.ShutdownDelay <= 0 {
		c.Config.ShutdownDelay = confopt.Duration(defaultShutdownDelay)
	}

	clientCfg := jmxproto.Config{
		JMXURL:         cfg.JMXURL,
		JMXUsername:    cfg.JMXUsername,
		JMXPassword:    cfg.JMXPassword,
		JMXClasspath:   cfg.JMXClasspath,
		JavaExecPath:   cfg.JavaExecPath,
		InitTimeout:    cfg.InitTimeout.Duration(),
		CommandTimeout: cfg.JMXTimeout.Duration(),
		ShutdownDelay:  cfg.ShutdownDelay.Duration(),
	}

	client, err := jmxproto.NewClient(clientCfg, c)
	if err != nil {
		return err
	}
	if err := client.Start(context.Background()); err != nil {
		client.Shutdown()
		return err
	}
	c.client = client

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

	if cfg.CollectAppsMatching != "" {
		sel, err := matcher.NewSimplePatternsMatcher(cfg.CollectAppsMatching)
		if err != nil {
			return err
		}
		c.appSelector = sel
	}

	c.identity = common.Identity{
		Cluster: cfg.ClusterName,
		Cell:    cfg.CellName,
		Node:    cfg.NodeName,
		Server:  cfg.ServerName,
	}
	labels := c.identity.Labels()
	if cfg.ServerType != "" {
		labels["server_type"] = cfg.ServerType
	}
	for k, v := range cfg.CustomLabels {
		if k == "" || v == "" {
			continue
		}
		labels[k] = v
	}
	c.SetGlobalLabels(labels)

	return nil
}

// Cleanup releases collector resources.
func (c *Collector) Cleanup(context.Context) {
	if c.client != nil {
		c.client.Shutdown()
	}
}
