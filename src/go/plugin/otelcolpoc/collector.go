// SPDX-License-Identifier: GPL-3.0-or-later
// Package otelcolpoc adapts curated OTel jobs to the existing Agent lifecycle.
package otelcolpoc

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed hostmetrics_schema.json
var hostSchema string

//go:embed filelogs_schema.json
var logsSchema string

//go:embed charts.yaml
var charts string

func Registry(worker, state, endpoint string) collectorapi.Registry {
	registry := collectorapi.Registry{}
	for kind, schema := range map[string]string{"hostmetrics": hostSchema, "filelogs": logsSchema} {
		registry.Register(kind, collectorapi.Creator{
			CreateV2:        func() collectorapi.CollectorV2 { return New(kind, worker, state, endpoint) },
			Config:          func() any { c := defaultConfig(kind); return &c },
			JobConfigSchema: schema,
			Defaults: collectorapi.Defaults{
				UpdateEvery: 1,
			},
		})
	}
	return registry
}

type Config struct {
	Name               string   `yaml:"name"                          json:"name"`
	UpdateEvery        int      `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	ServiceName        string   `yaml:"service_name"                  json:"service_name"`
	CollectionInterval string   `yaml:"collection_interval,omitempty" json:"collection_interval,omitempty"`
	Scrapers           []string `yaml:"scrapers,omitempty"            json:"scrapers,omitempty"`
	Include            []string `yaml:"include,omitempty"             json:"include,omitempty"`
	StartAt            string   `yaml:"start_at,omitempty"            json:"start_at,omitempty"`
}

func defaultConfig(kind string) Config {
	c := Config{
		ServiceName: "otel-orchestrator-poc",
	}
	if kind == "hostmetrics" {
		c.CollectionInterval = "2s"
		c.Scrapers = []string{"cpu", "memory"}
	} else {
		c.StartAt = "end"
	}
	return c
}

type Collector struct {
	collectorapi.Base
	Config                        `yaml:",inline" json:""`
	kind, worker, state, endpoint string
	store                         metrix.CollectorStore
	started                       atomic.Int64
	payload                       []byte
}

var _ collectorapi.CollectorV2Runner = (*Collector)(nil)

func New(kind, worker, state, endpoint string) *Collector {
	return &Collector{
		Config:   defaultConfig(kind),
		kind:     kind,
		worker:   worker,
		state:    state,
		endpoint: endpoint,
		store:    metrix.NewCollectorStore(),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (*Collector) ChartTemplateYAML() string            { return charts }
func (*Collector) Cleanup(context.Context)              {}

func (c *Collector) Init(context.Context) error {
	if c.kind != "hostmetrics" && c.kind != "filelogs" {
		return errors.New("unsupported job kind")
	}
	if c.Name == "" {
		return errors.New("job name is required")
	}
	if c.ServiceName == "" {
		return errors.New("service_name is required")
	}
	if !filepath.IsAbs(c.worker) || !filepath.IsAbs(c.state) {
		return errors.New("worker and state paths must be absolute")
	}
	if c.kind == "hostmetrics" {
		interval, err := time.ParseDuration(c.CollectionInterval)
		if err != nil || interval < time.Second {
			return errors.New("collection_interval must be at least 1s")
		}
		if len(c.Scrapers) == 0 {
			return errors.New("at least one scraper is required")
		}
		for _, scraper := range c.Scrapers {
			if scraper != "cpu" && scraper != "memory" {
				return errors.New("supported scrapers: cpu, memory")
			}
		}
		if len(c.Include) != 0 || c.StartAt != "" {
			return errors.New("file log options are not supported by hostmetrics")
		}
	} else {
		if len(c.Include) == 0 {
			return errors.New("include requires an absolute file path or glob")
		}
		for _, path := range c.Include {
			if !filepath.IsAbs(path) {
				return errors.New("include paths must be absolute")
			}
		}
		if c.StartAt != "beginning" && c.StartAt != "end" {
			return errors.New("start_at must be beginning or end")
		}
		if len(c.Scrapers) != 0 || c.CollectionInterval != "" {
			return errors.New("hostmetrics options are not supported by filelogs")
		}
	}
	var err error
	c.payload, err = json.Marshal(c.nativeConfig())
	return err
}

// Check uses the actual distribution's validation without starting pipelines or
// opening the log offset database, which can still belong to the accepted job.
func (c *Collector) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s, err := startWorker(c.worker, true)
	if err != nil {
		return err
	}
	defer s.close()
	if err := s.write(ctx, c.payload); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.exited:
		if s.waitErr != nil {
			return fmt.Errorf("Collector configuration validation failed: %w", s.waitErr)
		}
		return nil
	}
}

func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.kind == "filelogs" {
		if err := os.MkdirAll(c.storageDir(), 0700); err != nil {
			return err
		}
	}
	s, err := startWorker(c.worker, false)
	if err != nil {
		return err
	}
	defer s.close()
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.write(startup, c.payload); err != nil {
		return err
	}
	select {
	case <-startup.Done():
		return startup.Err()
	case <-s.exited:
		return fmt.Errorf("Collector exited before readiness: %v", s.waitErr)
	case err := <-s.ready:
		if err != nil {
			return err
		}
	}
	c.started.Store(time.Now().UnixNano())
	defer c.started.Store(0)
	ready()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.exited:
		return fmt.Errorf("Collector exited: %v", s.waitErr)
	}
}

func (c *Collector) Collect(context.Context) error {
	started := c.started.Load()
	if started == 0 {
		return errors.New("Collector is not running")
	}
	c.store.Write().SnapshotMeter("").Gauge("worker_uptime").Observe(time.Since(time.Unix(0, started)).Seconds())
	return nil
}
func (c *Collector) storageDir() string {
	key := sha256.Sum256([]byte(c.kind + ":" + c.Name))
	return filepath.Join(c.state, fmt.Sprintf("%x", key))
}

func (c *Collector) nativeConfig() map[string]any {
	extensions := map[string]any{"netdata_worker": map[string]any{}}
	extensionIDs := []string{"netdata_worker"}
	signal, receiver := "metrics", "host_metrics"
	var receiverConfig any
	if c.kind == "hostmetrics" {
		scrapers := map[string]any{}
		for _, name := range c.Scrapers {
			scrapers[name] = map[string]any{}
		}
		receiverConfig = map[string]any{"collection_interval": c.CollectionInterval, "scrapers": scrapers}
	} else {
		signal, receiver = "logs", "file_log"
		extensions["file_storage"] = map[string]any{"directory": c.storageDir(), "create_directory": true}
		extensionIDs = append(extensionIDs, "file_storage")
		receiverConfig = map[string]any{"include": c.Include, "start_at": c.StartAt, "storage": "file_storage", "include_file_path": true}
	}
	return map[string]any{
		"extensions": extensions,
		"receivers":  map[string]any{receiver: receiverConfig},
		"processors": map[string]any{"resource": map[string]any{"attributes": []any{
			map[string]any{"key": "service.name", "value": c.ServiceName, "action": "upsert"},
			map[string]any{"key": "netdata.poc.job", "value": c.kind + ":" + c.Name, "action": "upsert"},
		}}},
		"exporters": map[string]any{
			"otlp_grpc": map[string]any{"endpoint": c.endpoint, "tls": map[string]any{"insecure": true}},
		},
		"service": map[string]any{
			"extensions": extensionIDs,
			"telemetry":  map[string]any{"metrics": map[string]any{"level": "none"}},
			"pipelines": map[string]any{
				signal: map[string]any{
					"receivers":  []string{receiver},
					"processors": []string{"resource"},
					"exporters":  []string{"otlp_grpc"},
				},
			},
		},
	}
}
