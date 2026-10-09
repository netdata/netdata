// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"sort"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/java/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/java/javafunc"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var charts string

// The controlled exporter sends every second; five missing exports make coverage stale.
const sourceMaxAge = 5 * time.Second

func Register(registry collectorapi.Registry, runtime RuntimeConfig) {
	registry.Register("java", collectorapi.Creator{
		Defaults:        collectorapi.Defaults{UpdateEvery: 1},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
		CreateV2:        func() collectorapi.CollectorV2 { return New(runtime) },
		SharedFunctions: javafunc.Methods,
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return javafunc.New(job.Collector().(*Collector))
		},
	})
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	runtime     RuntimeConfig
	store       metrix.CollectorStore
	metrics     instruments
	ingress     *ingest.Store
	now         func() time.Time
	helper      helper
	isTerminal  func() bool
	credentials map[[sha256.Size]byte]admission

	mu      sync.Mutex
	targets map[string]targetState
}

func New(runtime RuntimeConfig) *Collector {
	if runtime.ProcDir == "" {
		runtime.ProcDir = "/proc"
	}
	store := metrix.NewCollectorStore()
	return &Collector{
		Config:      Config{Name: "java", UpdateEvery: 1},
		runtime:     runtime,
		store:       store,
		metrics:     newInstruments(store),
		ingress:     ingest.New(),
		now:         time.Now,
		helper:      nativeHelper{},
		isTerminal:  terminal.IsTerminal,
		credentials: make(map[[sha256.Size]byte]admission),
		targets:     make(map[string]targetState),
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	return c.runtime.validate()
}

func (c *Collector) Check(context.Context) error { return c.Config.validate() }

func (c *Collector) Run(ctx context.Context, ready func()) error { return c.run(ctx, ready) }

func (c *Collector) Collect(context.Context) error {
	for _, app := range c.ingress.Snapshot(c.now(), sourceMaxAge) {
		c.metrics.write(app, c.displayName(app.Application))
		c.mu.Lock()
		for key, target := range c.targets {
			if target.Instance() == app.Instance {
				target.LastSeen, target.Runtime = app.LastSeen, app.Runtime
				c.targets[key] = target
			}
		}
		c.mu.Unlock()
	}
	return nil
}

// Run joins its receiver and discovery workers before framework cleanup.
func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }

func (c *Collector) Applications() []javafunc.Application {
	c.mu.Lock()
	targets := make([]targetState, 0, len(c.targets))
	for _, target := range c.targets {
		targets = append(targets, target)
	}
	c.mu.Unlock()
	observed := make(map[string]ingest.Application)
	for _, app := range c.ingress.Snapshot(c.now(), sourceMaxAge) {
		observed[app.Instance] = app
	}
	rows := make([]javafunc.Application, 0, len(targets))
	for _, target := range targets {
		instance := target.Instance()
		app := observed[instance]
		families := make(map[string]bool)
		for _, sample := range app.Samples {
			families[sample.Name] = true
		}
		row := javafunc.Application{
			Name:       c.displayName(target.Application),
			Instance:   instance,
			PID:        target.PID,
			Runtime:    target.Runtime,
			Location:   target.Location,
			Status:     target.Status,
			Detail:     target.Detail,
			JVM:        "Not observed",
			HTTP:       "Not observed",
			Pools:      "Not observed",
			LastSample: "Never",
		}
		if !target.LastSeen.IsZero() {
			row.LastSample = target.LastSeen.UTC().Format(time.RFC3339)
		}
		if families["jvm_memory_used_bytes"] {
			row.JVM = "Collecting"
		}
		if families["http_server_request_duration_seconds"] {
			row.HTTP = "Collecting"
		}
		if families["hikari_connections"] && families["hikari_pending_requests"] && families["hikari_limit"] {
			row.Pools = "Collecting"
		}
		if target.Status == "Attached" {
			switch {
			case row.JVM == "Collecting" && row.HTTP == "Collecting" && row.Pools == "Collecting":
				row.Status = "Collecting"
			case len(app.Samples) > 0:
				row.Status = "Partial coverage"
			case !target.LastSeen.IsZero():
				row.Status = "No fresh data"
			default:
				row.Status = "Waiting for data"
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Instance < rows[j].Instance })
	return rows
}
