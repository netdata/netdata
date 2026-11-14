// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	CheckEvery  int              `yaml:"check_every"     json:"check_every"`
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	Name        string           `yaml:"name"            json:"name"`
	UpdateEvery int              `yaml:"update_every"    json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout"         json:"timeout"`
	URL         string           `yaml:"url"             json:"url"`
	SaveReport  bool             `yaml:"save_report"     json:"save_report"`
}

// Dependencies names the shared synthetic job dependencies for callers outside the collector tree.
type Dependencies = syntheticjob.Dependencies

type Collector struct {
	collectorapi.Base
	Config           `yaml:",inline" json:""`
	syntheticjob.Job `yaml:"-"       json:"-"`
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2:               func() collectorapi.CollectorV2 { return New(deps) },
		Config:                 func() any { return &Config{} },
		JobConfigSchema:        schema,
		StoreFirst:             true,
		ActivationDependencies: deps.ActivationDependencies,
	}
}
func New(deps Dependencies) *Collector {
	return &Collector{
		Config: Config{
			CheckEvery:  1800,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(3 * time.Minute),
		},
		Job: syntheticjob.New(deps, newMetrics),
	}
}
func (c *Collector) Configuration() any        { return c.Config }
func (c *Collector) ChartTemplateYAML() string { return charts }
