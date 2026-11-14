// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	CheckEvery  int              `yaml:"check_every"       json:"check_every"`
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	Name        string           `yaml:"name"              json:"name"`
	UpdateEvery int              `yaml:"update_every"      json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout"           json:"timeout"`
	URL         string           `yaml:"url"               json:"url"`
	SaveReport  bool             `yaml:"save_report"       json:"save_report"`
}

// Executor owns process-wide admission and supervised completion. Drained=false
// requires process fail-stop even if the collector has already retired.
type Executor interface {
	Check(context.Context, model.Kind) error
	monitor.Executor
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
	attempt     syntheticjob.Instruments
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
			CheckEvery:  1800,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(3 * time.Minute),
		},
		deps:    deps,
		store:   store,
		attempt: syntheticjob.New(meter),
		metrics: newMetrics(meter),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }
