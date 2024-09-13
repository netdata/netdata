// SPDX-License-Identifier: GPL-3.0-or-later

package tor

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
	module.Register("tor", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Tor {
	return &Tor{
		Config: Config{
			Address: "127.0.0.1:9051",
			Timeout: confopt.Duration(time.Second * 1),
		},
		newConn: newControlConn,
		charts:  charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
	Password    string           `yaml:"password" json:"password"`
}

type Tor struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config) controlConn
	conn    controlConn
}

func (t *Tor) Configuration() any {
	return t.Config
}

func (t *Tor) Init() error {
	if t.Address == "" {
		t.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (t *Tor) Check() error {
	mx, err := t.collect()
	if err != nil {
		t.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (t *Tor) Charts() *module.Charts {
	return t.charts
}

func (t *Tor) Collect() map[string]int64 {
	mx, err := t.collect()
	if err != nil {
		t.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (t *Tor) Cleanup() {
	if t.conn != nil {
		t.conn.disconnect()
		t.conn = nil
	}
}
