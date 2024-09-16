// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	_ "embed"
	"math/rand"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("testrandom", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: module.UpdateEvery,
			Priority:    module.Priority,
			Disabled:    true,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *TestRandom {
	return &TestRandom{
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
		UpdateEvery  int          `yaml:"update_every,omitempty" json:"update_every"`
		Charts       ConfigCharts `yaml:"charts" json:"charts"`
		HiddenCharts ConfigCharts `yaml:"hidden_charts" json:"hidden_charts"`
	}
	ConfigCharts struct {
		Type     string `yaml:"type,omitempty" json:"type"`
		Num      int    `yaml:"num" json:"num"`
		Contexts int    `yaml:"contexts" json:"contexts"`
		Dims     int    `yaml:"dimensions" json:"dimensions"`
		Labels   int    `yaml:"labels" json:"labels"`
	}
)

type TestRandom struct {
	module.Base // should be embedded by every module
	Config      `yaml:",inline"`

	randInt       func() int64
	charts        *module.Charts
	collectedDims map[string]bool
}

func (tr *TestRandom) Configuration() any {
	return tr.Config
}

func (tr *TestRandom) Init() error {
	err := tr.validateConfig()
	if err != nil {
		tr.Errorf("config validation: %v", err)
		return err
	}

	charts, err := tr.initCharts()
	if err != nil {
		tr.Errorf("charts init: %v", err)
		return err
	}
	tr.charts = charts
	return nil
}

func (tr *TestRandom) Check() error {
	return nil
}

func (tr *TestRandom) Charts() *module.Charts {
	return tr.charts
}

func (tr *TestRandom) Collect() map[string]int64 {
	mx, err := tr.collect()
	if err != nil {
		tr.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (tr *TestRandom) Cleanup() {}
