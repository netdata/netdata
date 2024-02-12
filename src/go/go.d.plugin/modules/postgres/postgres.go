// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	_ "embed"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/metrics"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/jackc/pgx/v4/stdlib"
	_ "github.com/jackc/pgx/v4/stdlib"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("postgres", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Postgres {
	return &Postgres{
		Config: Config{
			Timeout:            web.Duration{Duration: time.Second * 2},
			DSN:                "postgres://postgres:postgres@127.0.0.1:5432/postgres",
			XactTimeHistogram:  []float64{.1, .5, 1, 2.5, 5, 10},
			QueryTimeHistogram: []float64{.1, .5, 1, 2.5, 5, 10},
			// charts: 20 x table, 4 x index.
			// https://discord.com/channels/847502280503590932/1022693928874549368
			MaxDBTables:  50,
			MaxDBIndexes: 250,
		},
		charts:  baseCharts.Copy(),
		dbConns: make(map[string]*dbConn),
		mx: &pgMetrics{
			dbs:       make(map[string]*dbMetrics),
			indexes:   make(map[string]*indexMetrics),
			tables:    make(map[string]*tableMetrics),
			replApps:  make(map[string]*replStandbyAppMetrics),
			replSlots: make(map[string]*replSlotMetrics),
		},
		recheckSettingsEvery:              time.Minute * 30,
		doSlowEvery:                       time.Minute * 5,
		addXactQueryRunningTimeChartsOnce: &sync.Once{},
		addWALFilesChartsOnce:             &sync.Once{},
	}
}

type Config struct {
	DSN                string       `yaml:"dsn"`
	Timeout            web.Duration `yaml:"timeout"`
	DBSelector         string       `yaml:"collect_databases_matching"`
	XactTimeHistogram  []float64    `yaml:"transaction_time_histogram"`
	QueryTimeHistogram []float64    `yaml:"query_time_histogram"`
	MaxDBTables        int64        `yaml:"max_db_tables"`
	MaxDBIndexes       int64        `yaml:"max_db_indexes"`
}

type (
	Postgres struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		db      *sql.DB
		dbConns map[string]*dbConn

		superUser      *bool
		pgIsInRecovery *bool
		pgVersion      int

		addXactQueryRunningTimeChartsOnce *sync.Once
		addWALFilesChartsOnce             *sync.Once

		dbSr matcher.Matcher

		mx *pgMetrics

		recheckSettingsTime  time.Time
		recheckSettingsEvery time.Duration

		doSlowTime  time.Time
		doSlowEvery time.Duration
	}
	dbConn struct {
		db         *sql.DB
		connStr    string
		connErrors int
	}
)

func (p *Postgres) Init() bool {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}

	sr, err := p.initDBSelector()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}
	p.dbSr = sr

	p.mx.xactTimeHist = metrics.NewHistogramWithRangeBuckets(p.XactTimeHistogram)
	p.mx.queryTimeHist = metrics.NewHistogramWithRangeBuckets(p.QueryTimeHistogram)

	return true
}

func (p *Postgres) Check() bool {
	return len(p.Collect()) > 0
}

func (p *Postgres) Charts() *module.Charts {
	return p.charts
}

func (p *Postgres) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *Postgres) Cleanup() {
	if p.db == nil {
		return
	}
	if err := p.db.Close(); err != nil {
		p.Warningf("cleanup: error on closing the Postgres database [%s]: %v", p.DSN, err)
	}
	p.db = nil

	for dbname, conn := range p.dbConns {
		delete(p.dbConns, dbname)
		if conn.connStr != "" {
			stdlib.UnregisterConnConfig(conn.connStr)
		}
		if conn.db != nil {
			_ = conn.db.Close()
		}
	}
}
