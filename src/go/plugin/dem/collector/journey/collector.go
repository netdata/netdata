// SPDX-License-Identifier: GPL-3.0-or-later

package journey

import (
	"context"
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

type Secret struct {
	Name  string `yaml:"name"  json:"name"`
	Value string `yaml:"value" json:"value"`
}

type Config struct {
	BrowserErrorSummary bool             `yaml:"browser_error_summary" json:"browser_error_summary"`
	CheckEvery          int              `yaml:"check_every"           json:"check_every"`
	Vnode               string           `yaml:"vnode,omitempty"       json:"vnode,omitempty"`
	Name                string           `yaml:"name"                  json:"name"`
	UpdateEvery         int              `yaml:"update_every"          json:"update_every"`
	Timeout             confopt.Duration `yaml:"timeout"               json:"timeout"`
	Script              string           `yaml:"script,omitempty"      json:"script,omitempty"`
	ScriptPath          string           `yaml:"script_path,omitempty" json:"script_path,omitempty"`
	Secrets             []Secret         `yaml:"secrets,omitempty"     json:"secrets,omitempty"`
	TraceOnFailure      bool             `yaml:"trace_on_failure"      json:"trace_on_failure"`
	ScreenshotOnFailure bool             `yaml:"screenshot_on_failure" json:"screenshot_on_failure"`
}

// Executor and Dependencies name the shared synthetic job types for callers outside the collector tree.
type (
	Executor     = syntheticjob.Executor
	Dependencies = syntheticjob.Dependencies
)

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
			CheckEvery:  900,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(2 * time.Minute),
		},
		Job: syntheticjob.New(deps, newMetrics),
	}
}
func (c *Collector) Configuration() any        { return c.Config }
func (c *Collector) ChartTemplateYAML() string { return charts }

// Test rehearses the submitted configuration once without activating it.
func (c *Collector) Test(ctx context.Context) error { return c.Rehearse(ctx) }
