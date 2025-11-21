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
		MaxTablespaces: 50,
		MaxConnections: 50,
		MaxTables:      25,
		MaxIndexes:     50,

		BackupHistoryDays: 30,

		CollectDatabasesMatching: "",

		IncludeConnections: []string{
			"db2sysc*",
			"db2agent*",
			"db2hadr*",
			"db2acd*",
			"db2bmgr*",
		},
		ExcludeConnections: []string{
			"*TEMP*",
		},

		IncludeBufferpools: []string{
			"IBMDEFAULTBP",
			"IBMSYSTEMBP*",
			"IBMHADRBP*",
		},
		ExcludeBufferpools: nil,

		IncludeTablespaces: []string{
			"SYSCATSPACE",
			"TEMPSPACE*",
			"SYSTOOLSPACE",
		},
		ExcludeTablespaces: []string{
			"TEMPSPACE2",
		},

		IncludeTables: nil,
		ExcludeTables: nil,

		IncludeIndexes: nil,
		ExcludeIndexes: nil,
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

	connInclude, err := compileMatcher(c.IncludeConnections)
	if err != nil {
		return fmt.Errorf("invalid include_connections patterns: %w", err)
	}
	connExclude, err := compileMatcher(c.ExcludeConnections)
	if err != nil {
		return fmt.Errorf("invalid exclude_connections patterns: %w", err)
	}
	c.connectionInclude = connInclude
	c.connectionExclude = connExclude

	bpInclude, err := compileMatcher(c.IncludeBufferpools)
	if err != nil {
		return fmt.Errorf("invalid include_bufferpools patterns: %w", err)
	}
	bpExclude, err := compileMatcher(c.ExcludeBufferpools)
	if err != nil {
		return fmt.Errorf("invalid exclude_bufferpools patterns: %w", err)
	}
	c.bufferpoolInclude = bpInclude
	c.bufferpoolExclude = bpExclude

	tspInclude, err := compileMatcher(c.IncludeTablespaces)
	if err != nil {
		return fmt.Errorf("invalid include_tablespaces patterns: %w", err)
	}
	tspExclude, err := compileMatcher(c.ExcludeTablespaces)
	if err != nil {
		return fmt.Errorf("invalid exclude_tablespaces patterns: %w", err)
	}
	c.tablespaceInclude = tspInclude
	c.tablespaceExclude = tspExclude

	tblInclude, err := compileMatcher(c.IncludeTables)
	if err != nil {
		return fmt.Errorf("invalid include_tables patterns: %w", err)
	}
	tblExclude, err := compileMatcher(c.ExcludeTables)
	if err != nil {
		return fmt.Errorf("invalid exclude_tables patterns: %w", err)
	}
	c.tableInclude = tblInclude
	c.tableExclude = tblExclude

	idxInclude, err := compileMatcher(c.IncludeIndexes)
	if err != nil {
		return fmt.Errorf("invalid include_indexes patterns: %w", err)
	}
	idxExclude, err := compileMatcher(c.ExcludeIndexes)
	if err != nil {
		return fmt.Errorf("invalid exclude_indexes patterns: %w", err)
	}
	c.indexInclude = idxInclude
	c.indexExclude = idxExclude

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
