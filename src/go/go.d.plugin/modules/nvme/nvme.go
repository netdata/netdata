// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
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
	})
}

func New() *NVMe {
	return &NVMe{
		Config: Config{
			BinaryPath: "nvme",
			Timeout:    web.Duration{Duration: time.Second * 2},
		},
		charts:           &module.Charts{},
		devicePaths:      make(map[string]bool),
		listDevicesEvery: time.Minute * 10,
	}

}

type Config struct {
	Timeout    web.Duration
	BinaryPath string `yaml:"binary_path"`
}

type (
	NVMe struct {
		module.Base
		Config `yaml:",inline"`

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

func (n *NVMe) Init() bool {
	if err := n.validateConfig(); err != nil {
		n.Errorf("config validation: %v", err)
		return false
	}

	v, err := n.initNVMeCLIExec()
	if err != nil {
		n.Errorf("init nvme-cli exec: %v", err)
		return false
	}
	n.exec = v

	return true
}

func (n *NVMe) Check() bool {
	return len(n.Collect()) > 0
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
