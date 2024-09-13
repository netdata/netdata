// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

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
	module.Register("megacli", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *MegaCli {
	return &MegaCli{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:   &module.Charts{},
		adapters: make(map[string]bool),
		drives:   make(map[string]bool),
		bbu:      make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	MegaCli struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec megaCli

		adapters map[string]bool
		drives   map[string]bool
		bbu      map[string]bool
	}
	megaCli interface {
		physDrivesInfo() ([]byte, error)
		bbuInfo() ([]byte, error)
	}
)

func (m *MegaCli) Configuration() any {
	return m.Config
}

func (m *MegaCli) Init() error {
	lvmExec, err := m.initMegaCliExec()
	if err != nil {
		m.Errorf("megacli exec initialization: %v", err)
		return err
	}
	m.exec = lvmExec

	return nil
}

func (m *MegaCli) Check() error {
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

func (m *MegaCli) Charts() *module.Charts {
	return m.charts
}

func (m *MegaCli) Collect() map[string]int64 {
	mx, err := m.collect()
	if err != nil {
		m.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (m *MegaCli) Cleanup() {}
