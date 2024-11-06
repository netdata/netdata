// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/freeradius/api"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("freeradius", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *FreeRADIUS {
	return &FreeRADIUS{
		Config: Config{
			Address: "127.0.0.1",
			Port:    18121,
			Secret:  "adminsecret",
			Timeout: confopt.Duration(time.Second),
		},
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Port        int              `yaml:"port" json:"port"`
	Secret      string           `yaml:"secret" json:"secret"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
}

type (
	FreeRADIUS struct {
		module.Base
		Config `yaml:",inline" json:""`

		client
	}
	client interface {
		Status() (*api.Status, error)
	}
)

func (f *FreeRADIUS) Configuration() any {
	return f.Config
}

func (f *FreeRADIUS) Init() error {
	if err := f.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	f.client = api.New(api.Config{
		Address: f.Address,
		Port:    f.Port,
		Secret:  f.Secret,
		Timeout: f.Timeout.Duration(),
	})

	return nil
}

func (f *FreeRADIUS) Check() error {
	mx, err := f.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (f *FreeRADIUS) Charts() *Charts {
	return charts.Copy()
}

func (f *FreeRADIUS) Collect() map[string]int64 {
	mx, err := f.collect()
	if err != nil {
		f.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (f *FreeRADIUS) Cleanup() {}
