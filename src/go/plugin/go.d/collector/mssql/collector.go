// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mssql/mssqlfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"

	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/microsoft/go-mssqldb/azuread"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("mssql", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: mssqlfunc.Methods,
		MethodHandler:   mssqlFunctionHandler,
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			DSN:                 "sqlserver://localhost:1433",
			Timeout:             confopt.Duration(time.Second * 5),
			CollectDisabledJobs: false,
			Functions: mssqlfunc.FunctionsConfig{
				TopQueries: mssqlfunc.TopQueriesConfig{
					Limit:          500,
					TimeWindowDays: 7,
				},
			},
		},

		charts: instanceCharts.Copy(),

		seenDatabases:        make(map[string]bool),
		seenDatabasesWithLog: make(map[string]bool),
		seenWaitTypes:        make(map[string]bool),
		seenLockTypes:        make(map[string]bool),
		seenLockStatsTypes:   make(map[string]bool),
		jobChartIDs:          make(map[string]string),
		activeJobs:           make(map[string]string),
		seenReplications:     make(map[string]bool),

		seenAGs:                make(map[string]bool),
		seenAGReplicas:         make(map[string]bool),
		seenAGDatabaseReplicas: make(map[string]bool),
		seenAGClusterMembers:   make(map[string]bool),
		seenAGPageRepairDBs:    make(map[string]bool),
	}
}

type Config struct {
	Vnode               string                    `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery         int                       `yaml:"update_every,omitempty" json:"update_every"`
	DSN                 string                    `yaml:"dsn" json:"dsn"`
	Timeout             confopt.Duration          `yaml:"timeout,omitempty" json:"timeout"`
	CollectDisabledJobs bool                      `yaml:"collect_disabled_jobs" json:"collect_disabled_jobs"`
	CloudAuth           cloudauth.Config          `yaml:"cloud_auth" json:"cloud_auth"`
	Functions           mssqlfunc.FunctionsConfig `yaml:"functions,omitempty" json:"functions"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	// Metrics and Functions use separate single-connection pools so a slow diagnostic
	// query never delays metric collection. db is opened lazily by the first collect;
	// functionDB is created in Init and never replaced by a request.
	db         *sql.DB
	functionDB *sql.DB

	serverPropertiesMu     sync.RWMutex
	serverPropertiesLoaded bool
	version                string
	majorVersion           int // parsed from version string (11=2012, 12=2014, 13=2016, etc.)
	engineEdition          int

	seenDatabases        map[string]bool
	seenDatabasesWithLog map[string]bool
	seenWaitTypes        map[string]bool
	seenLockTypes        map[string]bool
	seenLockStatsTypes   map[string]bool
	jobChartIDs          map[string]string
	activeJobs           map[string]string
	seenReplications     map[string]bool

	hadrEnabled bool // true if Always On AG is enabled on this instance
	hadrChecked bool // true after the HADR check has been performed

	seenAGs                map[string]bool // key: ag_name
	seenAGReplicas         map[string]bool // key: ag_name + "_" + replica_server_name
	seenAGDatabaseReplicas map[string]bool // key: ag_name + "_" + replica_server_name + "_" + db_name
	seenAGClusterMembers   map[string]bool // key: member_name
	seenAGPageRepairDBs    map[string]bool // key: database_name
	agClusterChartAdded    bool            // true after cluster quorum chart has been added

	funcRouter funcapi.MethodHandler
}

const engineEditionAzureSQLDatabase = 5

func (c *Collector) currentEngineEdition() int {
	c.serverPropertiesMu.RLock()
	edition := c.engineEdition
	c.serverPropertiesMu.RUnlock()
	return edition
}

func (c *Collector) currentMajorVersion() int {
	c.serverPropertiesMu.RLock()
	major := c.majorVersion
	c.serverPropertiesMu.RUnlock()
	return major
}

func (c *Collector) serverProperties() (string, int, int, bool) {
	c.serverPropertiesMu.RLock()
	version := c.version
	major := c.majorVersion
	edition := c.engineEdition
	loaded := c.serverPropertiesLoaded
	c.serverPropertiesMu.RUnlock()
	return version, major, edition, loaded
}

func (c *Collector) setServerProperties(version string, edition int) {
	c.serverPropertiesMu.Lock()
	c.serverPropertiesLoaded = true
	c.version = version
	c.majorVersion = parseMajorVersion(version)
	c.engineEdition = edition
	c.serverPropertiesMu.Unlock()
}

func (c *Collector) ensureEngineEdition(ctx context.Context) (int, error) {
	if _, _, edition, loaded := c.serverProperties(); loaded {
		return edition, nil
	}

	version, edition, err := queryServerProperties(ctx, c.functionDB)
	if err != nil {
		return 0, err
	}

	c.serverPropertiesMu.Lock()
	if !c.serverPropertiesLoaded {
		c.serverPropertiesLoaded = true
		c.version = version
		c.majorVersion = parseMajorVersion(version)
		c.engineEdition = edition
	}
	edition = c.engineEdition
	c.serverPropertiesMu.Unlock()
	return edition, nil
}

func queryServerProperties(ctx context.Context, db *sql.DB) (string, int, error) {
	var version string
	var edition int
	if err := db.QueryRowContext(ctx, queryVersion).Scan(&version, &edition); err != nil {
		return "", 0, err
	}
	return version, edition, nil
}

func (c *Collector) isAzureSQLDatabase() bool {
	return c.currentEngineEdition() == engineEditionAzureSQLDatabase
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.DSN == "" {
		return errors.New("config: dsn not set")
	}
	if err := c.CloudAuth.Validate(); err != nil {
		return err
	}

	db, err := c.newConnectionPool()
	if err != nil {
		return err
	}
	c.functionDB = db

	c.funcRouter = mssqlfunc.NewRouter(functionDeps{collector: c}, c.Logger, c.Functions)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	// functionDB stays set after Close so an in-flight request fails with a closed-database
	// error instead of a nil dereference; sql.DB.Close tolerates a repeated Cleanup.
	if c.functionDB != nil {
		if err := c.functionDB.Close(); err != nil {
			c.Errorf("cleanup: error closing Function database connection: %v", err)
		}
	}
	if c.db == nil {
		return
	}
	if err := c.db.Close(); err != nil {
		c.Errorf("cleanup: error closing database connection: %v", err)
	}
	c.db = nil
}
