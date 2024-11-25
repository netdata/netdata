// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	_ "embed"
	"errors"
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("intelgpu", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *IntelGPU {
	return &IntelGPU{
		ndsudoName: "ndsudo",
		charts:     charts.Copy(),
		engines:    make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Device      string `yaml:"device,omitempty" json:"device"`
}

type (
	IntelGPU struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec       intelGpuTop
		ndsudoName string

		engines map[string]bool
	}
	intelGpuTop interface {
		queryGPUSummaryJson() ([]byte, error)
		stop() error
	}
)

func (ig *IntelGPU) Configuration() any {
	return ig.Config
}

func (ig *IntelGPU) Init() error {
	topExec, err := ig.initIntelGPUTopExec()
	if err != nil {
		return fmt.Errorf("init intelgpu top exec: %v", err)
	}

	ig.exec = topExec

	return nil
}

func (ig *IntelGPU) Check() error {
	mx, err := ig.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (ig *IntelGPU) Charts() *module.Charts {
	return ig.charts
}

func (ig *IntelGPU) Collect() map[string]int64 {
	mx, err := ig.collect()
	if err != nil {
		ig.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (ig *IntelGPU) Cleanup() {
	if ig.exec != nil {
		if err := ig.exec.stop(); err != nil {
			ig.Error(err)
		}
		ig.exec = nil
	}
}
