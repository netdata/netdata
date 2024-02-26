// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"database/sql"
	_ "embed"
	_ "github.com/go-sql-driver/mysql"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("proxysql", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *ProxySQL {
	return &ProxySQL{
		Config: Config{
			DSN:     "stats:stats@tcp(127.0.0.1:6032)/",
			Timeout: web.Duration{Duration: time.Second * 2},
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
	DSN     string       `yaml:"dsn"`
	MyCNF   string       `yaml:"my.cnf"`
	Timeout web.Duration `yaml:"timeout"`
}

type (
	ProxySQL struct {
		module.Base
		Config `yaml:",inline"`

		db *sql.DB

		charts *module.Charts

		once  *sync.Once
		cache *cache
	}
)

func (p *ProxySQL) Init() bool {
	if p.DSN == "" {
		p.Error("'dsn' not set")
		return false
	}

	p.Debugf("using DSN [%s]", p.DSN)
	return true
}

func (p *ProxySQL) Check() bool {
	return len(p.Collect()) > 0
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
