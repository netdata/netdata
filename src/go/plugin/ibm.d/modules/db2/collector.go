//go:build cgo

package db2

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	db2proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/db2"
)

type serverInfo struct {
	instanceName string
	hostName     string
	version      string
	platform     string
}

// Collector implements the DB2 module on top of the ibm.d framework.
type Collector struct {
	framework.Collector

	Config

	client *db2proto.Client
	db     *sql.DB

	mx *metricsData

	// Metadata caches
	databases   map[string]*databaseMetrics
	bufferpools map[string]*bufferpoolMetrics
	tablespaces map[string]*tablespaceMetrics
	connections map[string]*connectionMetrics
	tables      map[string]*tableMetrics
	indexes     map[string]*indexMetrics
	memoryPools map[string]*memoryPoolMetrics
	memorySets  map[string]*memorySetInstanceMetrics
	prefetchers map[string]*prefetcherInstanceMetrics

	// Selectors
	databaseSelector   matcher.Matcher
	bufferpoolSelector matcher.Matcher
	tablespaceSelector matcher.Matcher
	connectionSelector matcher.Matcher
	tableSelector      matcher.Matcher
	indexSelector      matcher.Matcher

	// DB2 version info
	version      string
	edition      string
	versionMajor int
	versionMinor int
	serverInfo   serverInfo

	// Filtering mode flags
	databaseFilterMode   bool
	bufferpoolFilterMode bool
	tablespaceFilterMode bool
	connectionFilterMode bool
	tableFilterMode      bool
	indexFilterMode      bool

	// Resilience tracking
	disabledMetrics  map[string]bool
	disabledFeatures map[string]bool

	// Edition flags
	isDB2ForAS400 bool
	isDB2ForZOS   bool
	isDB2Cloud    bool

	// Memory set iteration state
	currentMemorySetHostName string
	currentMemorySetDBName   string
	currentMemorySetType     string
	currentMemorySetMember   int64

	// Prefetcher iteration state
	currentBufferPoolName string

	once     sync.Once
	metaOnce sync.Once
}

func (c *Collector) initOnce() {
	c.once.Do(func() {
		c.disabledMetrics = make(map[string]bool)
		c.disabledFeatures = make(map[string]bool)
		c.resetCaches()
	})
}

func (c *Collector) resetCaches() {
	c.mx = &metricsData{
		databases:   make(map[string]databaseInstanceMetrics),
		bufferpools: make(map[string]bufferpoolInstanceMetrics),
		tablespaces: make(map[string]tablespaceInstanceMetrics),
		connections: make(map[string]connectionInstanceMetrics),
		tables:      make(map[string]tableInstanceMetrics),
		indexes:     make(map[string]indexInstanceMetrics),
		memoryPools: make(map[string]memoryPoolInstanceMetrics),
		memorySets:  make(map[string]memorySetInstanceMetrics),
		tableIOs:    make(map[string]tableIOInstanceMetrics),
		prefetchers: make(map[string]prefetcherInstanceMetrics),
	}

	c.databases = make(map[string]*databaseMetrics)
	c.bufferpools = make(map[string]*bufferpoolMetrics)
	c.tablespaces = make(map[string]*tablespaceMetrics)
	c.connections = make(map[string]*connectionMetrics)
	c.tables = make(map[string]*tableMetrics)
	c.indexes = make(map[string]*indexMetrics)
	c.memoryPools = make(map[string]*memoryPoolMetrics)
	c.memorySets = make(map[string]*memorySetInstanceMetrics)
	c.prefetchers = make(map[string]*prefetcherInstanceMetrics)
}

// CollectOnce implements framework.CollectorImpl.
func (c *Collector) CollectOnce() error {
	c.initOnce()

	ctx := context.Background()
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.metaOnce.Do(func() {
		if err := c.detectDB2Edition(ctx); err != nil {
			c.Warningf("failed to detect DB2 edition: %v", err)
		}
		c.logVersionInformation()
		c.setConfigurationDefaults()
		c.detectColumnOrganizedSupport(ctx)
		c.applyGlobalLabels()
	})

	c.resetCaches()

	metricsMap, err := c.collect(ctx)
	if err != nil {
		return err
	}

	c.exportSystemMetrics()
	c.exportDatabaseMetrics()
	c.exportBufferpoolMetrics()
	c.exportTablespaceMetrics()
	c.exportConnectionMetrics()
	c.exportTableMetrics()
	c.exportIndexMetrics()
	c.exportMemoryPoolMetrics()
	c.exportTableIOMetrics()
	c.exportMemorySetMetrics()
	c.exportPrefetcherMetrics()
	c.applyGlobalLabels()

	_ = metricsMap

	return nil
}

func (c *Collector) ensureConnected(ctx context.Context) error {
	if c.client == nil {
		return errors.New("db2 collector: client not initialised")
	}

	if err := c.client.Connect(ctx); err != nil {
		return err
	}
	c.db = c.client.DB()

	if err := c.client.Ping(ctx); err != nil {
		_ = c.client.Close()
		if err := c.client.Connect(ctx); err != nil {
			return err
		}
		c.db = c.client.DB()
		if err := c.client.Ping(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (c *Collector) applyGlobalLabels() {
	labels := map[string]string{
		"db2_version": c.version,
		"db2_edition": c.edition,
	}
	if c.serverInfo.instanceName != "" {
		labels["db2_instance"] = c.serverInfo.instanceName
	}
	if c.serverInfo.hostName != "" {
		labels["db2_host"] = c.serverInfo.hostName
	}
	c.SetGlobalLabels(labels)
}
