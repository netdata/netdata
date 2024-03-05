// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nvidia_smi", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			Disabled:    true,
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *NvidiaSMI {
	return &NvidiaSMI{
		Config: Config{
			Timeout:      web.Duration(time.Second * 10),
			UseCSVFormat: true,
		},
		binName: "nvidia-smi",
		charts:  &module.Charts{},
		gpus:    make(map[string]bool),
		migs:    make(map[string]bool),
	}

}

type Config struct {
	UpdateEvery  int          `yaml:"update_every" json:"update_every"`
	Timeout      web.Duration `yaml:"timeout" json:"timeout"`
	BinaryPath   string       `yaml:"binary_path" json:"binary_path"`
	UseCSVFormat bool         `yaml:"use_csv_format" json:"use_csv_format"`
}

type (
	NvidiaSMI struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec    nvidiaSMI
		binName string

		gpuQueryProperties []string

		gpus map[string]bool
		migs map[string]bool
	}
	nvidiaSMI interface {
		queryGPUInfoXML() ([]byte, error)
		queryGPUInfoCSV(properties []string) ([]byte, error)
		queryHelpQueryGPU() ([]byte, error)
	}
)

func (nv *NvidiaSMI) Configuration() any {
	return nv.Config
}

func (nv *NvidiaSMI) Init() error {
	if nv.exec == nil {
		smi, err := nv.initNvidiaSMIExec()
		if err != nil {
			nv.Error(err)
			return err
		}
		nv.exec = smi
	}

	return nil
}

func (nv *NvidiaSMI) Check() error {
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

func (nv *NvidiaSMI) Charts() *module.Charts {
	return nv.charts
}

func (nv *NvidiaSMI) Collect() map[string]int64 {
	mx, err := nv.collect()
	if err != nil {
		nv.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (nv *NvidiaSMI) Cleanup() {}
