// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dovecot", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Dovecot {
	return &Dovecot{
		Config: Config{
			Address: "127.0.0.1:24242",
			Timeout: confopt.Duration(time.Second * 1),
		},
		newConn: newDovecotConn,
		charts:  charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
}

type Dovecot struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config) dovecotConn
	conn    dovecotConn
}

func (d *Dovecot) Configuration() any {
	return d.Config
}

func (d *Dovecot) Init() error {
	if d.Address == "" {
		d.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (d *Dovecot) Check() error {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (d *Dovecot) Charts() *module.Charts {
	return d.charts
}

func (d *Dovecot) Collect() map[string]int64 {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (d *Dovecot) Cleanup() {
	if d.conn != nil {
		d.conn.disconnect()
		d.conn = nil
	}
}
