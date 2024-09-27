// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

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
	module.Register("hddtemp", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *HddTemp {
	return &HddTemp{
		Config: Config{
			Address: "127.0.0.1:7634",
			Timeout: confopt.Duration(time.Second * 1),
		},
		charts:    &module.Charts{},
		disks:     make(map[string]bool),
		disksTemp: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
}

type HddTemp struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	conn hddtempConn

	disks     map[string]bool
	disksTemp map[string]bool
}

func (h *HddTemp) Configuration() any {
	return h.Config
}

func (h *HddTemp) Init() error {
	if h.Address == "" {
		h.Error("config: 'address' not set")
		return errors.New("address not set")
	}

	h.conn = newHddTempConn(h.Config)

	return nil
}

func (h *HddTemp) Check() error {
	mx, err := h.collect()
	if err != nil {
		h.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (h *HddTemp) Charts() *module.Charts {
	return h.charts
}

func (h *HddTemp) Collect() map[string]int64 {
	mx, err := h.collect()
	if err != nil {
		h.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (h *HddTemp) Cleanup() {}
