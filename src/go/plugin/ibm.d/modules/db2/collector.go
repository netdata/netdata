//go:build cgo

package db2

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

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

	Config `yaml:",inline" json:",inline"`

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
	databaseSelector matcher.Matcher

	connectionInclude matcher.Matcher
	connectionExclude matcher.Matcher

	bufferpoolInclude matcher.Matcher
	bufferpoolExclude matcher.Matcher

	tablespaceInclude matcher.Matcher
	tablespaceExclude matcher.Matcher

	tableInclude matcher.Matcher
	tableExclude matcher.Matcher

	indexInclude matcher.Matcher
	indexExclude matcher.Matcher

	// DB2 version info
	version      string
	edition      string
	versionMajor int
	versionMinor int
	serverInfo   serverInfo

	// Filtering mode flags
	databaseFilterMode bool

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

	warnMu sync.Mutex
	warns  map[string]time.Time
}

const warnThrottleInterval = 10 * time.Minute

func compileMatcher(patterns []string) (matcher.Matcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	expr := strings.TrimSpace(strings.Join(patterns, " "))
	if expr == "" {
		return nil, nil
	}

	return matcher.NewSimplePatternsMatcher(expr)
}

func (c *Collector) warnOnce(key string, format string, args ...interface{}) {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()

	if c.warns == nil {
		c.warns = make(map[string]time.Time)
	}

	now := time.Now()
	if last, ok := c.warns[key]; ok && now.Sub(last) < warnThrottleInterval {
		return
	}

	c.Warningf(format, args...)
	c.warns[key] = now
}

func (c *Collector) clearWarnOnce(key string) {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()

	if c.warns == nil {
		return
	}
	delete(c.warns, key)
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

func (c *Collector) matchIncludeExclude(include, exclude matcher.Matcher, values ...string) bool {
	matched := false
	matchedMap := make(map[string]bool)

	if include == nil {
		matched = true
	} else {
		for _, v := range values {
			if v == "" {
				continue
			}
			if include.MatchString(v) {
				matched = true
				matchedMap[v] = true
			}
		}
	}

	if !matched {
		return false
	}

	if exclude != nil {
		for _, v := range values {
			if v == "" {
				continue
			}
			if exclude.MatchString(v) {
				if include != nil && matchedMap[v] {
					continue
				}
				return false
			}
		}
	}

	return true
}

func (c *Collector) allowConnection(id string, meta *connectionMetrics) bool {
	appName := ""
	host := ""
	ip := ""
	if meta != nil {
		appName = meta.applicationName
		host = meta.clientHostname
		ip = meta.clientIP
	}
	return c.matchIncludeExclude(c.connectionInclude, c.connectionExclude, id, appName, host, ip)
}

func (c *Collector) allowBufferpool(name string) bool {
	return c.matchIncludeExclude(c.bufferpoolInclude, c.bufferpoolExclude, name)
}

func (c *Collector) allowTablespace(name, contentType, state string) bool {
	return c.matchIncludeExclude(c.tablespaceInclude, c.tablespaceExclude, name, contentType, state)
}

func (c *Collector) allowTable(key string) bool {
	return c.matchIncludeExclude(c.tableInclude, c.tableExclude, key)
}

func (c *Collector) allowIndex(key string) bool {
	return c.matchIncludeExclude(c.indexInclude, c.indexExclude, key)
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
	c.exportBufferpoolMetrics(*c.mx)
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
