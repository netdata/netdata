// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

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
	module.Register("varnish", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Varnish {
	return &Varnish{
		Config: Config{
			BinaryPath: "/usr/bin/varnishstat",
			Timeout:    web.Duration(time.Second * 2),
		},
		seenBackends: make(map[string]bool),
		seenStorages: make(map[string]bool),

		charts: varnishCharts.Copy(),
	}

}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string       `yaml:"binary_path,omitempty" json:"binary_path"`
}

type (
	Varnish struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec varnishStatBinary

		seenBackends map[string]bool
		seenStorages map[string]bool
	}
	varnishStatBinary interface {
		varnishStatistics() ([]byte, error)
	}
)

func (v *Varnish) Configuration() any {
	return v.Config
}

func (v *Varnish) Init() error {
	if err := v.validateConfig(); err != nil {
		v.Errorf("config validation: %s", err)
		return err
	}

	iw, err := v.initIwExec()
	if err != nil {
		v.Errorf("iw dev exec initialization: %v", err)
		return err
	}
	v.exec = iw

	return nil
}

func (v *Varnish) Check() error {
	mx, err := v.collect()
	if err != nil {
		v.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (v *Varnish) Charts() *module.Charts {
	return v.charts
}

func (v *Varnish) Collect() map[string]int64 {
	mx, err := v.collect()
	if err != nil {
		v.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (v *Varnish) Cleanup() {}
