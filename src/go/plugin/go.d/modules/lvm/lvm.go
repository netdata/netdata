// SPDX-License-Identifier: GPL-3.0-or-later

package lvm

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
	module.Register("lvm", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *LVM {
	return &LVM{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:       &module.Charts{},
		lvmThinPools: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	LVM struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec lvmCLI

		lvmThinPools map[string]bool
	}
	lvmCLI interface {
		lvsReportJson() ([]byte, error)
	}
)

func (l *LVM) Configuration() any {
	return l.Config
}

func (l *LVM) Init() error {
	lvmExec, err := l.initLVMCLIExec()
	if err != nil {
		l.Errorf("lvm exec initialization: %v", err)
		return err
	}
	l.exec = lvmExec

	return nil
}

func (l *LVM) Check() error {
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

func (l *LVM) Charts() *module.Charts {
	return l.charts
}

func (l *LVM) Collect() map[string]int64 {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (l *LVM) Cleanup() {}
