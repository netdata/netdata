// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	"context"
	_ "embed"
	"errors"
	"net/url"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	shared "github.com/netdata/netdata/go/plugins/plugin/dem/collector/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
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

type Dependencies = shared.Dependencies

type Collector struct {
	collectorapi.Base
	Config  `yaml:",inline" json:""`
	runtime *shared.Runtime
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
	return &Collector{
		Config: Config{
			UpdateEvery: 1800,
			Timeout:     confopt.Duration(3 * time.Minute),
		},
		runtime: shared.New(deps),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.runtime.MetricStore() }
func (c *Collector) ChartTemplateYAML() string          { return charts }
func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	u, err := url.Parse(c.URL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return errors.New("url must be an absolute HTTP(S) URL without credentials")
	}
	return c.runtime.Init(c.Name, c.UpdateEvery, time.Duration(c.Timeout))
}
func (c *Collector) Check(ctx context.Context) error { return c.runtime.Check(ctx, model.Lighthouse) }
func (c *Collector) Run(ctx context.Context, ready func()) error {
	return c.runtime.Run(
		ctx,
		ready,
		model.Job{
			JobID:          "lighthouse:" + c.Name,
			Kind:           model.Lighthouse,
			Name:           c.Name,
			Target:         secrets.NewRedactor().ApplyURL(c.URL),
			CadenceSeconds: c.UpdateEvery,
			TimeoutSeconds: time.Duration(c.Timeout).Seconds(),
		},
	)
}
func (c *Collector) Collect(ctx context.Context) error {
	return c.runtime.Collect(
		ctx,
		model.Request{
			Kind:    model.Lighthouse,
			Name:    c.Name,
			URL:     c.URL,
			Timeout: time.Duration(c.Timeout),
			Capture: c.SaveReport,
		},
	)
}
func (c *Collector) Cleanup(context.Context) {}
