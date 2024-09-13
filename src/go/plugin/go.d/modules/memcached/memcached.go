// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

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
	module.Register("memcached", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Memcached {
	return &Memcached{
		Config: Config{
			Address: "127.0.0.1:11211",
			Timeout: confopt.Duration(time.Second * 1),
		},
		newMemcachedConn: newMemcachedConn,
		charts:           charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
}

type (
	Memcached struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		newMemcachedConn func(Config) memcachedConn
		conn             memcachedConn
	}
	memcachedConn interface {
		connect() error
		disconnect()
		queryStats() ([]byte, error)
	}
)

func (m *Memcached) Configuration() any {
	return m.Config
}

func (m *Memcached) Init() error {
	if m.Address == "" {
		m.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	return nil
}

func (m *Memcached) Check() error {
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

func (m *Memcached) Charts() *module.Charts {
	return m.charts
}

func (m *Memcached) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (m *Memcached) Cleanup() {
	if m.conn != nil {
		m.conn.disconnect()
		m.conn = nil
	}
}
