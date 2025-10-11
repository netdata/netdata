//go:build cgo

package db2

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
	db2proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/db2"
)

func defaultConfig() Config {
	return Config{
		Config: framework.Config{
			UpdateEvery: 5,
		},
		Vnode:         "",
		DSN:           "",
		Timeout:       confopt.Duration(2 * time.Second),
		MaxDbConns:    1,
		MaxDbLifeTime: confopt.Duration(10 * time.Minute),

		CollectDatabaseMetrics:   confopt.AutoBoolAuto,
		CollectBufferpoolMetrics: confopt.AutoBoolAuto,
		CollectTablespaceMetrics: confopt.AutoBoolAuto,
		CollectConnectionMetrics: confopt.AutoBoolAuto,
		CollectLockMetrics:       confopt.AutoBoolAuto,
		CollectTableMetrics:      confopt.AutoBoolAuto,
		CollectIndexMetrics:      confopt.AutoBoolAuto,

		CollectMemoryMetrics:  true,
		CollectWaitMetrics:    true,
		CollectTableIOMetrics: true,

		MaxDatabases:   10,
		MaxBufferpools: 20,
		MaxTablespaces: 100,
		MaxConnections: 200,
		MaxTables:      50,
		MaxIndexes:     100,

		BackupHistoryDays: 30,

		CollectDatabasesMatching:   "",
		CollectBufferpoolsMatching: "",
		CollectTablespacesMatching: "",
		CollectConnectionsMatching: "",
		CollectTablesMatching:      "",
		CollectIndexesMatching:     "",
	}
}

func (c *Collector) Init(ctx context.Context) error {
	c.initOnce()

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

	if err := c.verifyConfig(); err != nil {
		return err
	}

	originalDSN := c.DSN
	c.DSN = dbdriver.EnsureDriver(c.DSN, "IBM DB2 ODBC DRIVER")
	if c.DSN != originalDSN {
		c.Debugf("DSN missing driver keyword; prepended default driver mapping")
	}

	clientCfg := db2proto.Config{
		DSN:          c.DSN,
		Timeout:      time.Duration(c.Timeout),
		MaxOpenConns: c.MaxDbConns,
		ConnMaxLife:  time.Duration(c.MaxDbLifeTime),
	}
	c.client = db2proto.NewClient(clientCfg)

	if c.CollectDatabasesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectDatabasesMatching)
		if err != nil {
			return fmt.Errorf("invalid database selector pattern '%s': %v", c.CollectDatabasesMatching, err)
		}
		c.databaseSelector = m
	}
	if c.CollectBufferpoolsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectBufferpoolsMatching)
		if err != nil {
			return fmt.Errorf("invalid bufferpool selector pattern '%s': %v", c.CollectBufferpoolsMatching, err)
		}
		c.bufferpoolSelector = m
	}
	if c.CollectTablespacesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectTablespacesMatching)
		if err != nil {
			return fmt.Errorf("invalid tablespace selector pattern '%s': %v", c.CollectTablespacesMatching, err)
		}
		c.tablespaceSelector = m
	}
	if c.CollectConnectionsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectConnectionsMatching)
		if err != nil {
			return fmt.Errorf("invalid connection selector pattern '%s': %v", c.CollectConnectionsMatching, err)
		}
		c.connectionSelector = m
	}
	if c.CollectTablesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectTablesMatching)
		if err != nil {
			return fmt.Errorf("invalid table selector pattern '%s': %v", c.CollectTablesMatching, err)
		}
		c.tableSelector = m
	}
	if c.CollectIndexesMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.CollectIndexesMatching)
		if err != nil {
			return fmt.Errorf("invalid index selector pattern '%s': %v", c.CollectIndexesMatching, err)
		}
		c.indexSelector = m
	}

	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	if err := c.detectDB2Edition(ctx); err != nil {
		c.Warningf("failed to detect DB2 edition: %v", err)
	}
	c.logVersionInformation()
	c.setConfigurationDefaults()
	c.detectColumnOrganizedSupport(ctx)
	c.applyGlobalLabels()

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.client != nil {
		_ = c.client.Close()
	}
	c.db = nil
}

func (c *Collector) verifyConfig() error {
	if c.DSN == "" {
		return errors.New("DSN is required but not set")
	}
	return nil
}
