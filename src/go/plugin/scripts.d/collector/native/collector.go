// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pathvalidate"
)

//go:embed config_schema.json
var configSchema string

func init() {
	collectorapi.Register("native", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

type Config struct {
	ScriptConfig    Settings         `yaml:"config,omitempty"              json:"config,omitempty"`
	UpdateEvery     int              `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectEvery int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Manifest        string           `yaml:"manifest,omitempty"            json:"manifest,omitempty"`
	Timeout         confopt.Duration `yaml:"timeout,omitempty"             json:"timeout,omitempty"`
}

type Collector struct {
	collectorapi.Base
	Config             `yaml:",inline" json:",inline"`
	store              metrix.CollectorStore
	definition         manifest
	bound              bool
	configInput        []byte
	templates          *chartengine.TemplateSet
	validateExecutable func(string) (string, error)
	runtimeMu          sync.Mutex
	runtime            *scriptRuntime
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery: 10,
			Timeout:     confopt.Duration(5 * time.Second),
		},
		store:              metrix.NewCollectorStore(),
		validateExecutable: pathvalidate.ValidateBinaryPath,
	}
}

func (c *Collector) Configuration() any {
	cfg := c.Config
	if c.bound {
		cfg.Manifest = ""
		cfg.ScriptConfig = c.definition.config.effective(cfg.ScriptConfig)
	}
	return cfg
}
func (c *Collector) MetricStore() metrix.CollectorStore         { return c.store }
func (c *Collector) ChartTemplateSet() *chartengine.TemplateSet { return c.templates }
func (c *Collector) Cleanup(context.Context)                    {}

func (c *Collector) Init(context.Context) error {
	if c.UpdateEvery < 1 || c.Timeout.Duration() <= 0 {
		return fmt.Errorf("update_every and timeout must be positive")
	}
	if c.bound {
		if c.Manifest != "" {
			return fmt.Errorf("registered packages cannot override manifest")
		}
	} else {
		definition, templates, err := loadManifest(c.Manifest, c.validateExecutable)
		if err != nil {
			return err
		}
		c.definition, c.templates = definition, templates
	}
	var err error
	c.configInput, err = c.configurationEnvelope()
	return err
}

// Check validates local initialization only. A target's first observation may be critical.
func (c *Collector) Check(context.Context) error {
	if c.templates == nil {
		return fmt.Errorf("collector is not initialized")
	}
	return nil
}

// Run owns operational resources only after predecessor cleanup. Candidate
// validation in Init and Check never launches the script.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.definition.Mode == modePersistent {
		return c.runPersistent(ctx, ready)
	}
	ready()
	<-ctx.Done()
	return ctx.Err()
}

func (c *Collector) Collect(ctx context.Context) error {
	var result response
	var err error
	if c.definition.Mode == modePersistent {
		result, err = c.collectPersistent(ctx)
	} else {
		var data []byte
		data, err = runCommand(ctx, c.Timeout.Duration(), c.definition.Command, c.configInput)
		if err == nil {
			result, err = c.definition.decodeResponse(data)
			if err != nil {
				err = fmt.Errorf("invalid collect response: %w", err)
			}
		}
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The complete response has been validated before staging any writes.
	meter := c.store.Write().SnapshotMeter("")
	for _, sample := range result.Metrics {
		definition := c.definition.metricByName[sample.Name]
		labels := meter.LabelSet(metricLabels(sample.Labels)...)
		opts := []metrix.InstrumentOption{metrix.WithUnit(definition.Unit), metrix.WithFloat(true)}
		if definition.Type == "gauge" {
			meter.Gauge(sample.Name, opts...).Observe(*sample.Value, labels)
		} else {
			meter.Counter(sample.Name, opts...).ObserveTotal(*sample.Value, labels)
		}
	}
	for _, check := range result.Checks {
		meter.StateSet(checkMetric(check.ID), metrix.WithStateSetStates(checkStates...), metrix.WithStateSetMode(metrix.ModeEnum)).
			ObserveStateSet(metrix.StateSetPoint{
				States: map[string]bool{check.State: true},
			}, meter.LabelSet(metricLabels(check.Labels)...))
	}
	return nil
}

func metricLabels(values map[string]string) []metrix.Label {
	labels := make([]metrix.Label, 0, len(values))
	for key, value := range values {
		labels = append(labels, metrix.Label{
			Key:   key,
			Value: value,
		})
	}
	return labels
}
