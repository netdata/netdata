// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

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
	module.Register("adaptec_raid", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *AdaptecRaid {
	return &AdaptecRaid{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts: &module.Charts{},
		lds:    make(map[string]bool),
		pds:    make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	AdaptecRaid struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec arcconfCli

		lds map[string]bool
		pds map[string]bool
	}
	arcconfCli interface {
		logicalDevicesInfo() ([]byte, error)
		physicalDevicesInfo() ([]byte, error)
	}
)

func (a *AdaptecRaid) Configuration() any {
	return a.Config
}

func (a *AdaptecRaid) Init() error {
	arcconfExec, err := a.initArcconfCliExec()
	if err != nil {
		a.Errorf("arcconf exec initialization: %v", err)
		return err
	}
	a.exec = arcconfExec

	return nil
}

func (a *AdaptecRaid) Check() error {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (a *AdaptecRaid) Charts() *module.Charts {
	return a.charts
}

func (a *AdaptecRaid) Collect() map[string]int64 {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (a *AdaptecRaid) Cleanup() {}
