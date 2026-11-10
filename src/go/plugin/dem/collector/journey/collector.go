// SPDX-License-Identifier: GPL-3.0-or-later

package journey

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/attemptmetrics"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
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
	CheckEvery          int              `yaml:"check_every"           json:"check_every"`
	Vnode               string           `yaml:"vnode,omitempty"     json:"vnode,omitempty"`
	Name                string           `yaml:"name"                  json:"name"`
	UpdateEvery         int              `yaml:"update_every"          json:"update_every"`
	Timeout             confopt.Duration `yaml:"timeout"               json:"timeout"`
	Script              string           `yaml:"script,omitempty"      json:"script,omitempty"`
	ScriptPath          string           `yaml:"script_path,omitempty" json:"script_path,omitempty"`
	BrowserMode         string           `yaml:"browser_mode" json:"browser_mode"`
	Secrets             []Secret         `yaml:"secrets,omitempty"     json:"secrets,omitempty"`
	ScreenshotOnFailure bool             `yaml:"screenshot_on_failure" json:"screenshot_on_failure"`
}

// Executor owns process-wide admission and supervised completion. Drained=false
// requires process fail-stop even if the collector has already retired.
type Executor interface {
	Check(context.Context, model.Kind) error
	monitor.Executor
	Rehearse(context.Context, model.Request) error
}

type Dependencies struct {
	Executor Executor
	Registry *syntheticregistry.Registry
}
type Collector struct {
	collectorapi.Base
	Config      `yaml:",inline" json:""`
	deps        Dependencies
	store       metrix.CollectorStore
	attempt     attemptmetrics.Instruments
	metrics     metrics
	request     model.Request
	initialized bool
	vnodeGUID   string
	// Assigned before readiness; Run owns cancellation and physical completion.
	session *monitor.Session
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New(deps) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(deps Dependencies) *Collector {
	store := metrix.NewCollectorStore()
	meter := store.Write().SnapshotMeter("")
	return &Collector{
		Config: Config{
			BrowserMode: "reuse",
			CheckEvery:  900,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(2 * time.Minute),
		},
		deps:    deps,
		store:   store,
		attempt: attemptmetrics.New(meter),
		metrics: newMetrics(meter),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }

func (c *Collector) Test(ctx context.Context) error {
	if !c.initialized {
		return errors.New("collector is not initialized")
	}
	return c.deps.Executor.Rehearse(ctx, c.request)
}
