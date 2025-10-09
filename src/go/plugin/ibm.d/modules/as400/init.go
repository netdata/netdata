//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/as400/contexts"
	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

func defaultConfig() Config {
	return Config{
		Config: framework.Config{
			UpdateEvery: 10,
		},
		Vnode:         "",
		DSN:           "",
		Timeout:       confopt.Duration(2 * time.Second),
		MaxDbConns:    1,
		MaxDbLifeTime: confopt.Duration(10 * time.Minute),

		Hostname:        "",
		Port:            8471,
		Username:        "",
		Password:        "",
		Database:        "*SYSBAS",
		ConnectionType:  "odbc",
		ODBCDriver:      "IBM i Access ODBC Driver",
		UseSSL:          false,
		ResetStatistics: false,

		CollectDiskMetrics:         framework.AutoBoolAuto,
		CollectSubsystemMetrics:    framework.AutoBoolAuto,
		CollectJobQueueMetrics:     framework.AutoBoolAuto,
		CollectActiveJobs:          framework.AutoBoolAuto,
		CollectHTTPServerMetrics:   framework.AutoBoolAuto,
		CollectMessageQueueMetrics: framework.AutoBoolAuto,
		CollectOutputQueueMetrics:  framework.AutoBoolAuto,
		CollectPlanCacheMetrics:    framework.AutoBoolAuto,

		MaxDisks:         100,
		MaxSubsystems:    100,
		MaxJobQueues:     100,
		MaxMessageQueues: 100,
		MaxOutputQueues:  100,
		MaxActiveJobs:    100,

		DiskSelector:      "",
		SubsystemSelector: "",
		JobQueueSelector:  "",
	}
}

func (c *Collector) Init(ctx context.Context) error {
	c.initOnce()

	// Propagate job-level configuration into framework collector
	if c.Config.ObsoletionIterations != 0 {
		c.Collector.Config.ObsoletionIterations = c.Config.ObsoletionIterations
	}
	if c.Config.UpdateEvery != 0 {
		c.Collector.Config.UpdateEvery = c.Config.UpdateEvery
	}
	if c.Config.CollectionGroups != nil {
		c.Collector.Config.CollectionGroups = c.Config.CollectionGroups
	}

	// Register generated contexts before base init
	c.RegisterContexts(contexts.GetAllContexts()...)

	// Initialise base collector
	if err := c.Collector.Init(ctx); err != nil {
		return err
	}

	// Collector implements the framework interface itself
	c.SetImpl(c)

	// Prepare DSN from individual parameters if necessary
	c.Debugf("initial config: DSN=%q hostname=%q username=%q password_set=%t port=%d driver=%q",
		c.DSN, c.Hostname, c.Username, c.Password != "", c.Port, c.ODBCDriver)
	if err := c.buildDSNIfNeeded(ctx); err != nil {
		return err
	}
	if err := c.verifyConfig(); err != nil {
		return err
	}

	clientCfg := as400proto.Config{
		DSN:          c.DSN,
		Timeout:      time.Duration(c.Timeout),
		MaxOpenConns: c.MaxDbConns,
		ConnMaxLife:  time.Duration(c.MaxDbLifeTime),
	}
	c.client = as400proto.NewClient(clientCfg)

	if c.ResetStatistics {
		c.Warningf("reset_statistics is enabled; IBM i statistics will be reset on each collection iteration")
	}

	// Compile selectors if provided
	if c.DiskSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.DiskSelector)
		if err != nil {
			return fmt.Errorf("invalid disk selector pattern '%s': %v", c.DiskSelector, err)
		}
		c.diskSelector = m
	}
	if c.SubsystemSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.SubsystemSelector)
		if err != nil {
			return fmt.Errorf("invalid subsystem selector pattern '%s': %v", c.SubsystemSelector, err)
		}
		c.subsystemSelector = m
	}
	if c.JobQueueSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.JobQueueSelector)
		if err != nil {
			return fmt.Errorf("invalid job queue selector pattern '%s': %v", c.JobQueueSelector, err)
		}
		c.jobQueueSelector = m
	}

	// Detect IBM i version on first init to drive feature toggles
	if err := c.client.Connect(ctx); err == nil {
		if err := c.detectIBMiVersion(ctx); err != nil {
			c.Warningf("failed to detect IBM i version: %v", err)
		} else {
			c.Infof("detected IBM i version: %s", c.osVersion)
		}

		c.logVersionInformation()
		c.setConfigurationDefaults()
		c.detectAvailableFeatures(ctx)
		c.collectSystemInfo(ctx)
		c.applyGlobalLabels()
	}

	return nil
}

func (c *Collector) applyGlobalLabels() {
	labels := map[string]string{
		"ibmi_version":       c.osVersion,
		"technology_refresh": c.technologyRefresh,
		"system_name":        c.systemName,
		"serial_number":      c.serialNumber,
		"model":              c.model,
	}
	c.SetGlobalLabels(labels)
}

func (c *Collector) Check(ctx context.Context) error {
	c.initOnce()
	if err := c.client.Connect(ctx); err != nil {
		return err
	}
	if err := c.client.Ping(ctx); err != nil {
		return err
	}
	// Perform lightweight query to ensure connectivity
	return c.collect(ctx)
}
