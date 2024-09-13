// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

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
	module.Register("zfspool", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *ZFSPool {
	return &ZFSPool{
		Config: Config{
			BinaryPath: "/usr/bin/zpool",
			Timeout:    confopt.Duration(time.Second * 2),
		},
		charts:     &module.Charts{},
		seenZpools: make(map[string]bool),
		seenVdevs:  make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string           `yaml:"binary_path,omitempty" json:"binary_path"`
}

type (
	ZFSPool struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec zpoolCLI

		seenZpools map[string]bool
		seenVdevs  map[string]bool
	}
	zpoolCLI interface {
		list() ([]byte, error)
		listWithVdev(pool string) ([]byte, error)
	}
)

func (z *ZFSPool) Configuration() any {
	return z.Config
}

func (z *ZFSPool) Init() error {
	if err := z.validateConfig(); err != nil {
		z.Errorf("config validation: %s", err)
		return err
	}

	zpoolExec, err := z.initZPoolCLIExec()
	if err != nil {
		z.Errorf("zpool exec initialization: %v", err)
		return err
	}
	z.exec = zpoolExec

	return nil
}

func (z *ZFSPool) Check() error {
	mx, err := z.collect()
	if err != nil {
		z.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (z *ZFSPool) Charts() *module.Charts {
	return z.charts
}

func (z *ZFSPool) Collect() map[string]int64 {
	mx, err := z.collect()
	if err != nil {
		z.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (z *ZFSPool) Cleanup() {}
