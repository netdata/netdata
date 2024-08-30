// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

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
	module.Register("nvidia_smi", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *NvidiaSmi {
	return &NvidiaSmi{
		Config: Config{
			Timeout:  web.Duration(time.Second * 10),
			LoopMode: true,
		},
		binName: "nvidia-smi",
		charts:  &module.Charts{},
		gpus:    make(map[string]bool),
		migs:    make(map[string]bool),
	}

}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string       `yaml:"binary_path" json:"binary_path"`
	LoopMode    bool         `yaml:"loop_mode,omitempty" json:"loop_mode"`
}

type NvidiaSmi struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec    nvidiaSmiBinary
	binName string

	gpus map[string]bool
	migs map[string]bool
}

func (nv *NvidiaSmi) Configuration() any {
	return nv.Config
}

func (nv *NvidiaSmi) Init() error {
	if nv.exec == nil {
		smi, err := nv.initNvidiaSmiExec()
		if err != nil {
			nv.Error(err)
			return err
		}
		nv.exec = smi
	}

	return nil
}

func (nv *NvidiaSmi) Check() error {
	mx, err := nv.collect()
	if err != nil {
		nv.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (nv *NvidiaSmi) Charts() *module.Charts {
	return nv.charts
}

func (nv *NvidiaSmi) Collect() map[string]int64 {
	mx, err := nv.collect()
	if err != nil {
		nv.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (nv *NvidiaSmi) Cleanup() {
	if nv.exec != nil {
		if err := nv.exec.stop(); err != nil {
			nv.Errorf("cleanup: %v", err)
		}
		nv.exec = nil
	}
}
