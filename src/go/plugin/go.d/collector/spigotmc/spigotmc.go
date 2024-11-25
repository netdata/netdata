// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("spigotmc", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *SpigotMC {
	return &SpigotMC{
		Config: Config{
			Address: "127.0.0.1:25575",
			Timeout: confopt.Duration(time.Second * 1),
		},
		charts:  charts.Copy(),
		newConn: newRconConn,
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Password    string           `yaml:"password" json:"password"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type SpigotMC struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(Config) rconConn
	conn    rconConn
}

func (s *SpigotMC) Configuration() any {
	return s.Config
}

func (s *SpigotMC) Init() error {
	if s.Address == "" {
		return errors.New("config: 'address' required but not set")
	}
	if s.Password == "" {
		return errors.New("config: 'password' required but not set")
	}
	return nil
}

func (s *SpigotMC) Check() error {
	mx, err := s.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *SpigotMC) Charts() *module.Charts {
	return s.charts
}

func (s *SpigotMC) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *SpigotMC) Cleanup() {
	if s.conn != nil {
		if err := s.conn.disconnect(); err != nil {
			s.Warningf("error on disconnect: %s", err)
		}
		s.conn = nil
	}
}
