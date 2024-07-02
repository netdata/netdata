// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"database/sql"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/blang/semver/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pgbouncer", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *PgBouncer {
	return &PgBouncer{
		Config: Config{
			Timeout: web.Duration(time.Second),
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
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string       `yaml:"dsn" json:"dsn"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type PgBouncer struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db *sql.DB

	version              *semver.Version
	recheckSettingsTime  time.Time
	recheckSettingsEvery time.Duration
	maxClientConn        int64

	metrics *metrics
}

func (p *PgBouncer) Configuration() any {
	return p.Config
}

func (p *PgBouncer) Init() error {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return err
	}

	return nil
}

func (p *PgBouncer) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
