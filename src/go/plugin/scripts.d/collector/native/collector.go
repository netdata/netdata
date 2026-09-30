// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	_ "embed"
	"errors"
	"sync"

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
			UpdateEvery: defaultUpdateEvery,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

var (
	_ collectorapi.CollectorV2              = (*Collector)(nil)
	_ collectorapi.CollectorV2Runner        = (*Collector)(nil)
	_ collectorapi.ChartTemplateSetProvider = (*Collector)(nil)
)

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:",inline"`

	store              metrix.CollectorStore
	validateExecutable func(string) (string, error)

	// Generic jobs load definition from Manifest in Init; registered package
	// jobs receive it at creation.
	definition     packageDefinition
	registered     bool
	configEnvelope []byte
	initialized    bool

	runtimeMu sync.Mutex
	runtime   *persistentRuntime // set while a persistent session accepts requests
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
			Timeout:     confopt.Duration(defaultTimeout),
		},
		store:              metrix.NewCollectorStore(),
		validateExecutable: pathvalidate.ValidateBinaryPath,
	}
}

func (c *Collector) Configuration() any {
	cfg := c.Config
	if c.registered {
		cfg.Manifest = ""
		cfg.ScriptConfig = c.definition.effectiveSettings(cfg.ScriptConfig)
	}
	return cfg
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateSet() *chartengine.TemplateSet { return c.definition.templates }

func (c *Collector) Init(context.Context) error {
	c.initialized = false
	if c.UpdateEvery < 1 || c.Timeout.Duration() <= 0 {
		return errors.New("update_every and timeout must be positive")
	}
	if err := c.initDefinition(); err != nil {
		return err
	}
	var err error
	if c.configEnvelope, err = c.definition.configEnvelope(c.ScriptConfig); err != nil {
		return err
	}
	c.initialized = true
	return nil
}

// Check validates local initialization only. A target's first observation may be critical.
func (c *Collector) Check(context.Context) error {
	if !c.initialized {
		return errors.New("collector is not initialized")
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
	snap, err := c.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The complete snapshot has been validated before staging any writes.
	c.writeSnapshot(snap)
	return nil
}

func (c *Collector) Cleanup(context.Context) {}
