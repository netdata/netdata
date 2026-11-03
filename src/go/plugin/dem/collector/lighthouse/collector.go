// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/attemptmetrics"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	Name        string           `yaml:"name"         json:"name"`
	UpdateEvery int              `yaml:"update_every" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout"      json:"timeout"`
	URL         string           `yaml:"url"          json:"url"`
	SaveReport  bool             `yaml:"save_report"  json:"save_report"`
}

// Executor owns process-wide admission and supervised completion. Drained=false
// requires process fail-stop even if the collector has already retired.
type Executor interface {
	Check(context.Context, model.Kind) error
	Execute(context.Context, model.Request, func(string)) model.Execution
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
	// Assigned before readiness; retained after retirement for in-flight callbacks.
	registration *syntheticregistry.Registration
	terminal     chan error
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1800,
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
			UpdateEvery: 1800,
			Timeout:     confopt.Duration(3 * time.Minute),
		},
		deps:     deps,
		store:    store,
		attempt:  attemptmetrics.New(meter),
		metrics:  newMetrics(meter),
		terminal: make(chan error, 1),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }
