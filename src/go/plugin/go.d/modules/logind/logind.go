// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package logind

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("logind", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			Priority: 59999, // copied from the python collector
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Logind {
	return &Logind{
		Config: Config{
			Timeout: web.Duration(time.Second),
		},
		newLogindConn: func(cfg Config) (logindConnection, error) {
			return newLogindConnection(cfg.Timeout.Duration())
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Logind struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	conn          logindConnection
	newLogindConn func(config Config) (logindConnection, error)
}

func (l *Logind) Configuration() any {
	return l.Config
}

func (l *Logind) Init() error {
	return nil
}

func (l *Logind) Check() error {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (l *Logind) Charts() *module.Charts {
	return l.charts
}

func (l *Logind) Collect() map[string]int64 {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (l *Logind) Cleanup() {
	if l.conn != nil {
		l.conn.Close()
	}
}
