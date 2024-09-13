// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

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
	module.Register("nvme", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *NVMe {
	return &NVMe{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},

		charts:           &module.Charts{},
		devicePaths:      make(map[string]bool),
		listDevicesEvery: time.Minute * 10,
	}

}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	NVMe struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec nvmeCLI

		devicePaths      map[string]bool
		listDevicesTime  time.Time
		listDevicesEvery time.Duration
		forceListDevices bool
	}
	nvmeCLI interface {
		list() (*nvmeDeviceList, error)
		smartLog(devicePath string) (*nvmeDeviceSmartLog, error)
	}
)

func (n *NVMe) Configuration() any {
	return n.Config
}

func (n *NVMe) Init() error {
	nvmeExec, err := n.initNVMeCLIExec()
	if err != nil {
		n.Errorf("init nvme-cli exec: %v", err)
		return err
	}
	n.exec = nvmeExec

	return nil
}

func (n *NVMe) Check() error {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (n *NVMe) Charts() *module.Charts {
	return n.charts
}

func (n *NVMe) Collect() map[string]int64 {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (n *NVMe) Cleanup() {}
