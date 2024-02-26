// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	_ "embed"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("mongodb", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Mongo {
	return &Mongo{
		Config: Config{
			Timeout: 2,
			URI:     "mongodb://localhost:27017",
			Databases: matcher.SimpleExpr{
				Includes: []string{},
				Excludes: []string{},
			},
		},

		conn: &mongoClient{},

		charts:                chartsServerStatus.Copy(),
		addShardingChartsOnce: &sync.Once{},

		optionalCharts: make(map[string]bool),
		replSetMembers: make(map[string]bool),
		databases:      make(map[string]bool),
		shards:         make(map[string]bool),
	}
}

type Config struct {
	URI       string             `yaml:"uri"`
	Timeout   time.Duration      `yaml:"timeout"`
	Databases matcher.SimpleExpr `yaml:"databases"`
}

type Mongo struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	conn mongoConn

	dbSelector matcher.Matcher

	addShardingChartsOnce *sync.Once

	optionalCharts map[string]bool
	databases      map[string]bool
	replSetMembers map[string]bool
	shards         map[string]bool
}

func (m *Mongo) Init() bool {
	if err := m.verifyConfig(); err != nil {
		m.Errorf("config validation: %v", err)
		return false
	}

	if err := m.initDatabaseSelector(); err != nil {
		m.Errorf("init database selector: %v", err)
		return false
	}

	return true
}

func (m *Mongo) Check() bool {
	return len(m.Collect()) > 0
}

func (m *Mongo) Charts() *module.Charts {
	return m.charts
}

func (m *Mongo) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
	}

	if len(mx) == 0 {
		m.Warning("no values collected")
		return nil
	}

	return mx
}

func (m *Mongo) Cleanup() {
	if m.conn == nil {
		return
	}
	if err := m.conn.close(); err != nil {
		m.Warningf("cleanup: error on closing mongo conn: %v", err)
	}
}
