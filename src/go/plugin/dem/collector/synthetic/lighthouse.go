// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema_lighthouse.json
var lighthouseSchema string

type LighthouseConfig struct {
	CheckEvery  int              `yaml:"check_every"            json:"check_every"`
	Vnode       string           `yaml:"vnode,omitempty"        json:"vnode,omitempty"`
	Name        string           `yaml:"name"                   json:"name"`
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout"                json:"timeout"`
	URL         string           `yaml:"url"                    json:"url"`
	SaveReport  bool             `yaml:"save_report"            json:"save_report"`
}

// Lighthouse is the lighthouse module: one job audits one URL. It has no DynCfg Test, because a full audit per Test is
// slow and holds a browser slot; DynCfg detects Test by method set, so drafts get Init and the preparation Check.
type Lighthouse struct {
	collectorapi.Base
	LighthouseConfig `yaml:",inline" json:""`
	syntheticjob.Job `yaml:"-"       json:"-"`
}

func LighthouseCreator(deps Dependencies) collectorapi.Creator {
	return newCreator(
		deps,
		lighthouseSchema,
		func() any { return &LighthouseConfig{} },
		func() collectorapi.CollectorV2 { return NewLighthouse(deps) },
	)
}

func NewLighthouse(deps Dependencies) *Lighthouse {
	return &Lighthouse{
		LighthouseConfig: LighthouseConfig{
			CheckEvery:  1800,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(3 * time.Minute),
		},
		Job: syntheticjob.New(deps, model.Lighthouse),
	}
}

func (c *Lighthouse) Configuration() any        { return c.LighthouseConfig }
func (c *Lighthouse) ChartTemplateYAML() string { return charts }

func (c *Lighthouse) Init(ctx context.Context) error {
	return c.Job.Init(ctx, c.CheckEvery, c.UpdateEvery, c.newRequest)
}

// newRequest validates the audit options and builds the execution request.
func (c *Lighthouse) newRequest(vnodeGUID string) (model.Request, error) {
	request := model.Request{
		Kind:      model.Lighthouse,
		VnodeGUID: vnodeGUID,
		Name:      c.Name,
		URL:       c.URL,
		Timeout:   time.Duration(c.Timeout),
		Capture:   c.SaveReport,
	}
	if err := model.ValidateRequest(request); err != nil {
		return model.Request{}, err
	}
	return request, nil
}
