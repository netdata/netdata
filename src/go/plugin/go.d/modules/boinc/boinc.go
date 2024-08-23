// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("boinc", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Boinc {
	return &Boinc{
		Config: Config{
			Address: "127.0.0.1:31416",
			Timeout: web.Duration(time.Second * 1),
		},
		newConn: newBoincConn,
		charts:  charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Timeout     web.Duration `yaml:"timeout" json:"timeout"`
	Password    string       `yaml:"password" json:"password"`
}

type Boinc struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config, *logger.Logger) boincConn
	conn    boincConn
}

func (b *Boinc) Configuration() any {
	return b.Config
}

func (b *Boinc) Init() error {
	if b.Address == "" {
		b.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (b *Boinc) Check() error {
	mx, err := b.collect()
	if err != nil {
		b.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (b *Boinc) Charts() *module.Charts {
	return b.charts
}

func (b *Boinc) Collect() map[string]int64 {
	mx, err := b.collect()
	if err != nil {
		b.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (b *Boinc) Cleanup() {
	if b.conn != nil {
		b.conn.disconnect()
		b.conn = nil
	}
}
