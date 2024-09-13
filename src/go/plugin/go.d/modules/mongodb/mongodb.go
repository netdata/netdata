// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("mongodb", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Mongo {
	return &Mongo{
		Config: Config{
			URI:     "mongodb://localhost:27017",
			Timeout: confopt.Duration(time.Second),
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
	UpdateEvery int                `yaml:"update_every,omitempty" json:"update_every"`
	URI         string             `yaml:"uri" json:"uri"`
	Timeout     confopt.Duration   `yaml:"timeout,omitempty" json:"timeout"`
	Databases   matcher.SimpleExpr `yaml:"databases,omitempty" json:"databases"`
}

type Mongo struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                *module.Charts
	addShardingChartsOnce *sync.Once

	conn mongoConn

	dbSelector     matcher.Matcher
	optionalCharts map[string]bool
	databases      map[string]bool
	replSetMembers map[string]bool
	shards         map[string]bool
}

func (m *Mongo) Configuration() any {
	return m.Config
}

func (m *Mongo) Init() error {
	if err := m.verifyConfig(); err != nil {
		m.Errorf("config validation: %v", err)
		return err
	}

	if err := m.initDatabaseSelector(); err != nil {
		m.Errorf("init database selector: %v", err)
		return err
	}

	return nil
}

func (m *Mongo) Check() error {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
