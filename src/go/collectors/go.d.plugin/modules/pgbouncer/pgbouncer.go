// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"database/sql"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/blang/semver/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pgbouncer", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *PgBouncer {
	return &PgBouncer{
		Config: Config{
			Timeout: web.Duration{Duration: time.Second},
			DSN:     "postgres://postgres:postgres@127.0.0.1:6432/pgbouncer",
		},
		charts:               globalCharts.Copy(),
		recheckSettingsEvery: time.Minute * 5,
		metrics: &metrics{
			dbs: make(map[string]*dbMetrics),
		},
	}
}

type Config struct {
	DSN     string       `yaml:"dsn"`
	Timeout web.Duration `yaml:"timeout"`
}

type PgBouncer struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	db      *sql.DB
	version *semver.Version

	recheckSettingsTime  time.Time
	recheckSettingsEvery time.Duration
	maxClientConn        int64

	metrics *metrics
}

func (p *PgBouncer) Init() bool {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}

	return true
}

func (p *PgBouncer) Check() bool {
	return len(p.Collect()) > 0
}

func (p *PgBouncer) Charts() *module.Charts {
	return p.charts
}

func (p *PgBouncer) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *PgBouncer) Cleanup() {
	if p.db == nil {
		return
	}
	if err := p.db.Close(); err != nil {
		p.Warningf("cleanup: error on closing the PgBouncer database [%s]: %v", p.DSN, err)
	}
	p.db = nil
}
