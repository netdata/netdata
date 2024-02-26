// SPDX-License-Identifier: GPL-3.0-or-later

package example

import (
	_ "embed"
	"math/rand"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("example", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery:        module.UpdateEvery,
			AutoDetectionRetry: module.AutoDetectionRetry,
			Priority:           module.Priority,
			Disabled:           true,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Example {
	return &Example{
		Config: Config{
			Charts: ConfigCharts{
				Num:  1,
				Dims: 4,
			},
			HiddenCharts: ConfigCharts{
				Num:  0,
				Dims: 4,
			},
		},

		randInt:       func() int64 { return rand.Int63n(100) },
		collectedDims: make(map[string]bool),
	}
}

type (
	Config struct {
		Charts       ConfigCharts `yaml:"charts"`
		HiddenCharts ConfigCharts `yaml:"hidden_charts"`
	}
	ConfigCharts struct {
		Type     string `yaml:"type"`
		Num      int    `yaml:"num"`
		Contexts int    `yaml:"contexts"`
		Dims     int    `yaml:"dimensions"`
		Labels   int    `yaml:"labels"`
	}
)

type Example struct {
	module.Base // should be embedded by every module
	Config      `yaml:",inline"`

	randInt       func() int64
	charts        *module.Charts
	collectedDims map[string]bool
}

func (e *Example) Init() bool {
	err := e.validateConfig()
	if err != nil {
		e.Errorf("config validation: %v", err)
		return false
	}

	charts, err := e.initCharts()
	if err != nil {
		e.Errorf("charts init: %v", err)
		return false
	}
	e.charts = charts
	return true
}

func (e *Example) Check() bool {
	return len(e.Collect()) > 0
}

func (e *Example) Charts() *module.Charts {
	return e.charts
}

func (e *Example) Collect() map[string]int64 {
	mx, err := e.collect()
	if err != nil {
		e.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (e *Example) Cleanup() {}
