// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"database/sql"
	_ "embed"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("proxysql", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *ProxySQL {
	return &ProxySQL{
		Config: Config{
			DSN:     "stats:stats@tcp(127.0.0.1:6032)/",
			Timeout: web.Duration(time.Second),
		},

		charts: baseCharts.Copy(),
		once:   &sync.Once{},
		cache: &cache{
			commands: make(map[string]*commandCache),
			users:    make(map[string]*userCache),
			backends: make(map[string]*backendCache),
		},
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	DSN         string       `yaml:"dsn" json:"dsn"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type ProxySQL struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	db *sql.DB

	once  *sync.Once
	cache *cache
}

func (p *ProxySQL) Configuration() any {
	return p.Config
}

func (p *ProxySQL) Init() error {
	if p.DSN == "" {
		p.Error("dsn not set")
		return errors.New("dsn not set")
	}

	p.Debugf("using DSN [%s]", p.DSN)

	return nil
}

func (p *ProxySQL) Check() error {
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

func (p *ProxySQL) Charts() *module.Charts {
	return p.charts
}

func (p *ProxySQL) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *ProxySQL) Cleanup() {
	if p.db == nil {
		return
	}
	if err := p.db.Close(); err != nil {
		p.Errorf("cleanup: error on closing the ProxySQL instance [%s]: %v", p.DSN, err)
	}
	p.db = nil
}
